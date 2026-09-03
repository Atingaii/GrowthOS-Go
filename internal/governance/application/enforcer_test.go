package application

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/governance/domain"
)

var errDependency = errors.New("private dependency detail")

func TestEnforcerRequiresRecordedAllow(t *testing.T) {
	t.Parallel()

	fixture := newEnforcerFixture(t, allowPolicy(t))
	if err := fixture.enforcer.Require(
		context.Background(),
		testPrincipal(t),
		testStrategyResource(t, "21003"),
		domain.ActionSimulate,
		testAuditReference(t, "request-allow"),
	); err != nil {
		t.Fatalf("Require() error = %v", err)
	}
	if fixture.policyCalls.Load() != 1 || fixture.referenceCalls.Load() != 1 || fixture.auditCalls.Load() != 1 {
		t.Fatalf(
			"calls policy/reference/audit = %d/%d/%d, want 1/1/1",
			fixture.policyCalls.Load(),
			fixture.referenceCalls.Load(),
			fixture.auditCalls.Load(),
		)
	}
	decision := fixture.singleDecision(t)
	if !decision.Allowed() || decision.Action() != domain.ActionSimulate ||
		decision.Principal() != testPrincipal(t) || decision.Resource() != testStrategyResource(t, "21003") {
		t.Fatalf("recorded decision does not preserve the exact allow question: %#v", decision)
	}
	if decision.AuditContext().CorrelationReference().String() != "request-allow" {
		t.Fatalf("correlation = %q", decision.AuditContext().CorrelationReference())
	}
}

func TestEnforcerKeepsConfirmedDenySeparateFromTechnicalFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		policy     domain.Policy
		wantReason domain.DecisionReason
	}{
		{name: "no binding", policy: emptyPolicy(t), wantReason: domain.DecisionReasonNoBinding},
		{name: "no permission", policy: readOnlyPolicy(t), wantReason: domain.DecisionReasonNoPermission},
		{name: "scope mismatch", policy: mismatchedScopePolicy(t), wantReason: domain.DecisionReasonScopeMismatch},
		{name: "explicit deny", policy: explicitDenyPolicy(t, false), wantReason: domain.DecisionReasonExplicitDeny},
		{name: "deny overrides allow", policy: explicitDenyPolicy(t, true), wantReason: domain.DecisionReasonExplicitDenyOverrodeAllow},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newEnforcerFixture(t, testCase.policy)
			err := fixture.enforcer.Require(
				context.Background(),
				testPrincipal(t),
				testStrategyResource(t, "21003"),
				domain.ActionSimulate,
				testAuditReference(t, "request-deny"),
			)
			if !errors.Is(err, ErrAuthorizationDenied) ||
				errors.Is(err, ErrAuthorizationUnavailable) ||
				errors.Is(err, ErrDecisionAuditUnavailable) {
				t.Fatalf("Require() error = %v, want confirmed deny only", err)
			}
			decision := fixture.singleDecision(t)
			if decision.Allowed() || decision.Reason() != testCase.wantReason {
				t.Fatalf("decision = %s/%s, want deny/%s", decision.Outcome(), decision.Reason(), testCase.wantReason)
			}
		})
	}
}

func TestEnforcerAuditFailureCannotCreateAnUnauditedAllow(t *testing.T) {
	t.Parallel()

	fixture := newEnforcerFixture(t, allowPolicy(t))
	fixture.auditErr = errDependency
	err := fixture.enforcer.Require(
		context.Background(),
		testPrincipal(t),
		testStrategyResource(t, "21003"),
		domain.ActionSimulate,
		testAuditReference(t, "request-audit-error"),
	)
	if !errors.Is(err, ErrAuthorizationUnavailable) ||
		!errors.Is(err, ErrDecisionAuditUnavailable) || errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("Require() error = %v, want unavailable audit failure", err)
	}
	if fixture.auditCalls.Load() != 1 {
		t.Fatalf("audit calls = %d, want 1", fixture.auditCalls.Load())
	}
}

func TestEnforcerDenyRemainsBindingWhenAuditFails(t *testing.T) {
	t.Parallel()

	fixture := newEnforcerFixture(t, emptyPolicy(t))
	fixture.auditErr = errDependency
	err := fixture.enforcer.Require(
		context.Background(),
		testPrincipal(t),
		testStrategyResource(t, "21003"),
		domain.ActionSimulate,
		testAuditReference(t, "request-deny-audit-error"),
	)
	if !errors.Is(err, ErrAuthorizationDenied) ||
		!errors.Is(err, ErrDecisionAuditUnavailable) || errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatalf("Require() error = %v, want deny plus audit-unavailable", err)
	}
}

func TestEnforcerFailsClosedBeforeAuditForPolicyAndCorrelationFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*enforcerFixture)
	}{
		{name: "policy dependency", configure: func(fixture *enforcerFixture) { fixture.policyErr = errDependency }},
		{name: "zero policy", configure: func(fixture *enforcerFixture) { fixture.policy = domain.Policy{} }},
		{name: "reference dependency", configure: func(fixture *enforcerFixture) { fixture.referenceErr = errDependency }},
		{name: "invalid reference", configure: func(fixture *enforcerFixture) { fixture.reference = "INVALID" }},
		{name: "zero clock", configure: func(fixture *enforcerFixture) { fixture.now = time.Time{} }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fixture := newEnforcerFixture(t, allowPolicy(t))
			testCase.configure(fixture)
			err := fixture.enforcer.Require(
				context.Background(),
				testPrincipal(t),
				testStrategyResource(t, "21003"),
				domain.ActionSimulate,
				testAuditReference(t, "request-unavailable"),
			)
			if !errors.Is(err, ErrAuthorizationUnavailable) || errors.Is(err, ErrAuthorizationDenied) {
				t.Fatalf("Require() error = %v, want unavailable", err)
			}
			if fixture.auditCalls.Load() != 0 {
				t.Fatalf("audit calls = %d, want 0", fixture.auditCalls.Load())
			}
		})
	}
}

func TestEnforcerRejectsInvalidOrCanceledQuestionsBeforeDependencies(t *testing.T) {
	t.Parallel()

	fixture := newEnforcerFixture(t, allowPolicy(t))
	tests := []struct {
		name        string
		ctx         context.Context
		principal   domain.Principal
		resource    domain.Resource
		action      domain.Action
		correlation domain.AuditReference
		want        error
	}{
		{
			name: "nil context", principal: testPrincipal(t), resource: testStrategyResource(t, "21003"),
			action: domain.ActionSimulate, correlation: testAuditReference(t, "request-invalid"),
			want: ErrAuthorizationInvalidArgument,
		},
		{
			name: "zero principal", ctx: context.Background(), resource: testStrategyResource(t, "21003"),
			action: domain.ActionSimulate, correlation: testAuditReference(t, "request-invalid"),
			want: ErrAuthorizationInvalidArgument,
		},
		{
			name: "unsupported tuple", ctx: context.Background(), principal: testPrincipal(t), resource: testStrategyResource(t, "21003"),
			action: domain.ActionPublish, correlation: testAuditReference(t, "request-invalid"),
			want: ErrAuthorizationInvalidArgument,
		},
		{
			name: "zero correlation", ctx: context.Background(), principal: testPrincipal(t), resource: testStrategyResource(t, "21003"),
			action: domain.ActionSimulate, want: ErrAuthorizationInvalidArgument,
		},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests = append(tests, struct {
		name        string
		ctx         context.Context
		principal   domain.Principal
		resource    domain.Resource
		action      domain.Action
		correlation domain.AuditReference
		want        error
	}{
		name: "canceled", ctx: canceled, principal: testPrincipal(t), resource: testStrategyResource(t, "21003"),
		action: domain.ActionSimulate, correlation: testAuditReference(t, "request-canceled"), want: context.Canceled,
	})

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := fixture.enforcer.Require(
				testCase.ctx,
				testCase.principal,
				testCase.resource,
				testCase.action,
				testCase.correlation,
			)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Require() error = %v, want %v", err, testCase.want)
			}
		})
	}
	if fixture.policyCalls.Load() != 0 || fixture.referenceCalls.Load() != 0 || fixture.auditCalls.Load() != 0 {
		t.Fatalf(
			"dependencies called for rejected questions: %d/%d/%d",
			fixture.policyCalls.Load(),
			fixture.referenceCalls.Load(),
			fixture.auditCalls.Load(),
		)
	}
}

func TestNewEnforcerRejectsNilAndTypedNilDependencies(t *testing.T) {
	t.Parallel()

	valid := validDependencies(t)
	tests := []Dependencies{
		{},
		{Policies: (*stubPolicyReader)(nil), Audit: valid.Audit, Clock: valid.Clock, References: valid.References},
		{Policies: valid.Policies, Audit: (*stubAuditSink)(nil), Clock: valid.Clock, References: valid.References},
		{Policies: valid.Policies, Audit: valid.Audit, Clock: ClockFunc(nil), References: valid.References},
		{Policies: valid.Policies, Audit: valid.Audit, Clock: valid.Clock, References: EvaluationReferenceSourceFunc(nil)},
	}
	for index, dependencies := range tests {
		if enforcer, err := NewEnforcer(dependencies); enforcer != nil || !errors.Is(err, ErrAuthorizationNotConfigured) {
			t.Fatalf("case %d NewEnforcer() = (%v, %v), want nil/not configured", index, enforcer, err)
		}
	}
}

func TestEnforcerIsSafeForConcurrentIndependentDecisions(t *testing.T) {
	t.Parallel()

	fixture := newEnforcerFixture(t, allowPolicy(t))
	const workers = 64
	principal := testPrincipal(t)
	resource := testStrategyResource(t, "21003")
	correlation := testAuditReference(t, "request-concurrent")
	var waitGroup sync.WaitGroup
	errorsFound := make(chan error, workers)
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			err := fixture.enforcer.Require(
				context.Background(),
				principal,
				resource,
				domain.ActionSimulate,
				correlation,
			)
			if err != nil {
				errorsFound <- err
			}
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("Require() error = %v", err)
	}
	if fixture.auditCalls.Load() != workers {
		t.Fatalf("audit calls = %d, want %d", fixture.auditCalls.Load(), workers)
	}
}

type enforcerFixture struct {
	enforcer       *Enforcer
	policy         domain.Policy
	policyErr      error
	auditErr       error
	reference      domain.AuditReference
	referenceErr   error
	now            time.Time
	policyCalls    atomic.Int64
	referenceCalls atomic.Int64
	auditCalls     atomic.Int64
	mu             sync.Mutex
	decisions      []domain.Decision
}

func newEnforcerFixture(t *testing.T, policy domain.Policy) *enforcerFixture {
	t.Helper()
	fixture := &enforcerFixture{
		policy:    policy,
		reference: testAuditReference(t, "evaluation-1"),
		now:       time.Date(2026, time.September, 3, 12, 30, 0, 123456000, time.UTC),
	}
	reader := &stubPolicyReader{load: func(context.Context) (domain.Policy, error) {
		fixture.policyCalls.Add(1)
		return fixture.policy, fixture.policyErr
	}}
	sink := &stubAuditSink{append: func(_ context.Context, decision domain.Decision) error {
		fixture.auditCalls.Add(1)
		fixture.mu.Lock()
		fixture.decisions = append(fixture.decisions, decision)
		fixture.mu.Unlock()
		return fixture.auditErr
	}}
	enforcer, err := NewEnforcer(Dependencies{
		Policies: reader,
		Audit:    sink,
		Clock: ClockFunc(func() time.Time {
			return fixture.now
		}),
		References: EvaluationReferenceSourceFunc(func() (domain.AuditReference, error) {
			call := fixture.referenceCalls.Add(1)
			if fixture.reference == "evaluation-1" && fixture.referenceErr == nil {
				return domain.NewAuditReference("evaluation-" + strconv.FormatInt(call, 10))
			}
			return fixture.reference, fixture.referenceErr
		}),
	})
	if err != nil {
		t.Fatalf("NewEnforcer() error = %v", err)
	}
	fixture.enforcer = enforcer
	return fixture
}

func (fixture *enforcerFixture) singleDecision(t *testing.T) domain.Decision {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.decisions) != 1 {
		t.Fatalf("recorded decisions = %d, want 1", len(fixture.decisions))
	}
	return fixture.decisions[0]
}

type stubPolicyReader struct {
	load func(context.Context) (domain.Policy, error)
}

func (reader *stubPolicyReader) LoadActivePolicy(ctx context.Context) (domain.Policy, error) {
	return reader.load(ctx)
}

type stubAuditSink struct {
	append func(context.Context, domain.Decision) error
}

func (sink *stubAuditSink) AppendDecision(ctx context.Context, decision domain.Decision) error {
	return sink.append(ctx, decision)
}

func validDependencies(t *testing.T) Dependencies {
	t.Helper()
	return Dependencies{
		Policies: &stubPolicyReader{load: func(context.Context) (domain.Policy, error) {
			return allowPolicy(t), nil
		}},
		Audit: &stubAuditSink{append: func(context.Context, domain.Decision) error { return nil }},
		Clock: ClockFunc(func() time.Time {
			return time.Date(2026, time.September, 3, 12, 30, 0, 0, time.UTC)
		}),
		References: EvaluationReferenceSourceFunc(func() (domain.AuditReference, error) {
			return testAuditReference(t, "evaluation-valid"), nil
		}),
	}
}

func allowPolicy(t *testing.T) domain.Policy {
	t.Helper()
	return policyWithBindings(t, []domain.RoleBinding{
		testBinding(t, "binding-allow", domain.RoleLotteryDesigner, domain.NewSystemScope(), domain.BindingEffectAllow),
	})
}

func emptyPolicy(t *testing.T) domain.Policy {
	t.Helper()
	return policyWithBindings(t, nil)
}

func readOnlyPolicy(t *testing.T) domain.Policy {
	t.Helper()
	read, err := domain.NewPermission(
		domain.ResourceKindObject,
		domain.ResourceTypeLotteryStrategy,
		domain.ActionRead,
	)
	if err != nil {
		t.Fatalf("NewPermission(read) error = %v", err)
	}
	role, err := domain.NewRole(domain.RoleLotteryDesigner, []domain.Permission{read})
	if err != nil {
		t.Fatalf("NewRole(read-only) error = %v", err)
	}
	identity := testPolicyIdentity(t)
	policy, err := domain.NewPolicy(identity, []domain.Role{role}, []domain.RoleBinding{
		testBinding(t, "binding-read", domain.RoleLotteryDesigner, domain.NewSystemScope(), domain.BindingEffectAllow),
	})
	if err != nil {
		t.Fatalf("NewPolicy(read-only) error = %v", err)
	}
	return policy
}

func mismatchedScopePolicy(t *testing.T) domain.Policy {
	t.Helper()
	scope, err := domain.NewResourceScope(
		domain.ResourceTypeLotteryStrategy,
		testResourceID(t, "21004"),
		"",
	)
	if err != nil {
		t.Fatalf("NewResourceScope() error = %v", err)
	}
	return policyWithBindings(t, []domain.RoleBinding{
		testBinding(t, "binding-other-strategy", domain.RoleLotteryDesigner, scope, domain.BindingEffectAllow),
	})
}

func explicitDenyPolicy(t *testing.T, includeAllow bool) domain.Policy {
	t.Helper()
	bindings := []domain.RoleBinding{
		testBinding(t, "binding-deny", domain.RoleLotteryDesigner, domain.NewSystemScope(), domain.BindingEffectDeny),
	}
	if includeAllow {
		bindings = append(bindings,
			testBinding(t, "binding-allow", domain.RoleLotteryDesigner, domain.NewSystemScope(), domain.BindingEffectAllow),
		)
	}
	return policyWithBindings(t, bindings)
}

func policyWithBindings(t *testing.T, bindings []domain.RoleBinding) domain.Policy {
	t.Helper()
	policy, err := domain.NewBaselinePolicy(testPolicyIdentity(t), bindings)
	if err != nil {
		t.Fatalf("NewBaselinePolicy() error = %v", err)
	}
	return policy
}

func testPolicyIdentity(t *testing.T) domain.PolicyIdentity {
	t.Helper()
	id, err := domain.NewPolicyID("workforce-http")
	if err != nil {
		t.Fatalf("NewPolicyID() error = %v", err)
	}
	identity, err := domain.NewPolicyIdentity(id, 3)
	if err != nil {
		t.Fatalf("NewPolicyIdentity() error = %v", err)
	}
	return identity
}

func testBinding(
	t *testing.T,
	id string,
	roleID domain.RoleID,
	scope domain.Scope,
	effect domain.BindingEffect,
) domain.RoleBinding {
	t.Helper()
	bindingID, err := domain.NewRoleBindingID(id)
	if err != nil {
		t.Fatalf("NewRoleBindingID() error = %v", err)
	}
	binding, err := domain.NewRoleBinding(bindingID, testPrincipal(t), roleID, scope, effect)
	if err != nil {
		t.Fatalf("NewRoleBinding() error = %v", err)
	}
	return binding
}

func testPrincipal(t *testing.T) domain.Principal {
	t.Helper()
	id, err := domain.NewPrincipalID("operator-1")
	if err != nil {
		t.Fatalf("NewPrincipalID() error = %v", err)
	}
	principal, err := domain.NewPrincipal(domain.PrincipalKindHuman, id)
	if err != nil {
		t.Fatalf("NewPrincipal() error = %v", err)
	}
	return principal
}

func testStrategyResource(t *testing.T, id string) domain.Resource {
	t.Helper()
	resource, err := domain.NewObjectResource(
		domain.ResourceTypeLotteryStrategy,
		testResourceID(t, id),
		"",
		domain.Principal{},
	)
	if err != nil {
		t.Fatalf("NewObjectResource() error = %v", err)
	}
	return resource
}

func testResourceID(t *testing.T, value string) domain.ResourceID {
	t.Helper()
	id, err := domain.NewResourceID(value)
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	return id
}

func testAuditReference(t *testing.T, value string) domain.AuditReference {
	t.Helper()
	reference, err := domain.NewAuditReference(value)
	if err != nil {
		t.Fatalf("NewAuditReference() error = %v", err)
	}
	return reference
}
