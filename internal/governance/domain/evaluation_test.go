package domain

import (
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestPolicyEvaluateReturnsExactAllowEvidence(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	binding := mustRoleBinding(
		t,
		"allow-marketing",
		principal,
		RoleMarketingOperator,
		NewSystemScope(),
		BindingEffectAllow,
	)
	policy := mustBaselinePolicy(t, "growthos-access", 7, []RoleBinding{binding})
	request := mustAuthorizationRequest(
		t,
		principal,
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			mustResourceID(t, "activity-1"),
			"",
			Principal{},
		),
		ActionPublish,
		"evaluation-1",
	)

	decision, err := policy.Evaluate(request)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !decision.Confirmed() || !decision.Allowed() {
		t.Fatalf("decision not confirmed allow: %#v", decision)
	}
	if decision.Outcome() != DecisionOutcomeAllow ||
		decision.Reason() != DecisionReasonExplicitAllow {
		t.Fatalf("outcome/reason = %q/%q", decision.Outcome(), decision.Reason())
	}
	if decision.PolicyIdentity() != policy.Identity() ||
		decision.Principal() != request.Principal() ||
		decision.Resource() != request.Resource() ||
		decision.Action() != request.Action() ||
		decision.AuditContext() != request.AuditContext() {
		t.Fatal("decision did not preserve exact policy/request evidence")
	}
	matches := decision.Matches()
	if len(matches) != 1 {
		t.Fatalf("match count = %d", len(matches))
	}
	match := matches[0]
	if match.BindingID() != binding.ID() ||
		match.RoleID() != RoleMarketingOperator ||
		match.Effect() != BindingEffectAllow ||
		match.ScopeKind() != ScopeKindSystem ||
		match.Permission() != mustPermission(
			t,
			ResourceKindObject,
			ResourceTypeMarketingActivity,
			ActionPublish,
		) {
		t.Fatalf("match evidence = %#v", match)
	}
}

func TestPolicyEvaluateDefaultDenyReasonsAreDistinct(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	otherPrincipal := mustPrincipal(t, PrincipalKindHuman, "operator-2")
	tenantA := mustTenantID(t, "tenant-a")
	tenantB := mustTenantID(t, "tenant-b")
	resource := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		mustResourceID(t, "activity-1"),
		tenantB,
		Principal{},
	)
	request := mustAuthorizationRequest(t, principal, resource, ActionPublish, "evaluation-default")
	tenantAScope, err := NewTenantScope(tenantA)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}

	tests := []struct {
		name     string
		bindings []RoleBinding
		want     DecisionReason
	}{
		{
			name: "no principal binding",
			bindings: []RoleBinding{mustRoleBinding(
				t,
				"other-principal",
				otherPrincipal,
				RoleMarketingOperator,
				NewSystemScope(),
				BindingEffectAllow,
			)},
			want: DecisionReasonNoBinding,
		},
		{
			name: "bound role has no capability",
			bindings: []RoleBinding{mustRoleBinding(
				t,
				"member-binding",
				principal,
				RoleGrowthMember,
				NewSystemScope(),
				BindingEffectAllow,
			)},
			want: DecisionReasonNoPermission,
		},
		{
			name: "capability exists but tenant differs",
			bindings: []RoleBinding{mustRoleBinding(
				t,
				"tenant-a-binding",
				principal,
				RoleMarketingOperator,
				tenantAScope,
				BindingEffectAllow,
			)},
			want: DecisionReasonScopeMismatch,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			policy := mustBaselinePolicy(t, "growthos-access", 1, test.bindings)
			decision, err := policy.Evaluate(request)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if !decision.Confirmed() || decision.Allowed() ||
				decision.Outcome() != DecisionOutcomeDeny ||
				decision.Reason() != test.want {
				t.Fatalf("decision = %#v, want confirmed deny %q", decision, test.want)
			}
			if len(decision.Matches()) != 0 {
				t.Fatalf("default deny fabricated matches: %#v", decision.Matches())
			}
		})
	}
}

func TestPolicyEvaluateExplicitDenyOverridesAllowAndRetainsConflictEvidence(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	tenantID := mustTenantID(t, "tenant-a")
	resourceID := mustResourceID(t, "activity-9")
	tenantScope, err := NewTenantScope(tenantID)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}
	allow := mustRoleBinding(
		t,
		"allow-tenant",
		principal,
		RoleMarketingOperator,
		tenantScope,
		BindingEffectAllow,
	)
	deny := mustRoleBinding(
		t,
		"deny-resource",
		principal,
		RoleMarketingOperator,
		mustResourceScope(t, ResourceTypeMarketingActivity, resourceID, tenantID),
		BindingEffectDeny,
	)
	policy := mustBaselinePolicy(t, "growthos-access", 2, []RoleBinding{deny, allow})
	request := mustAuthorizationRequest(
		t,
		principal,
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			resourceID,
			tenantID,
			Principal{},
		),
		ActionPublish,
		"evaluation-conflict",
	)

	decision, err := policy.Evaluate(request)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !decision.Confirmed() || decision.Allowed() ||
		decision.Reason() != DecisionReasonExplicitDenyOverrodeAllow {
		t.Fatalf("decision = %#v", decision)
	}
	matches := decision.Matches()
	if len(matches) != 2 {
		t.Fatalf("match count = %d, want allow and deny", len(matches))
	}
	if matches[0].BindingID() != allow.ID() || matches[0].Effect() != BindingEffectAllow ||
		matches[1].BindingID() != deny.ID() || matches[1].Effect() != BindingEffectDeny {
		t.Fatalf("canonical conflict evidence = %#v", matches)
	}
}

func TestPolicyEvaluateExplicitDenyWithoutAllow(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindService, "scheduler-1")
	deny := mustRoleBinding(
		t,
		"deny-publish",
		principal,
		RoleMarketingOperator,
		NewSystemScope(),
		BindingEffectDeny,
	)
	policy := mustBaselinePolicy(t, "growthos-access", 3, []RoleBinding{deny})
	request := mustAuthorizationRequest(
		t,
		principal,
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			mustResourceID(t, "activity-2"),
			"",
			Principal{},
		),
		ActionPublish,
		"evaluation-deny",
	)
	decision, err := policy.Evaluate(request)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Reason() != DecisionReasonExplicitDeny || decision.Allowed() || !decision.Confirmed() {
		t.Fatalf("decision = %#v", decision)
	}
	if got := decision.Matches(); len(got) != 1 || got[0].Effect() != BindingEffectDeny {
		t.Fatalf("deny evidence = %#v", got)
	}
}

func TestPolicyEvaluateOwnedScopeRequiresTenantAndOwner(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "member-1")
	tenantA := mustTenantID(t, "tenant-a")
	tenantB := mustTenantID(t, "tenant-b")
	ownedScope, err := NewOwnedScope(tenantA)
	if err != nil {
		t.Fatalf("new owned scope: %v", err)
	}
	binding := mustRoleBinding(
		t,
		"owned-read",
		principal,
		RoleSecurityAuditor,
		ownedScope,
		BindingEffectAllow,
	)
	policy := mustBaselinePolicy(t, "growthos-access", 4, []RoleBinding{binding})
	resourceID := mustResourceID(t, "activity-5")

	tests := []struct {
		name     string
		resource Resource
		want     DecisionReason
	}{
		{
			name: "same tenant and owner",
			resource: mustObjectResource(
				t,
				ResourceTypeMarketingActivity,
				resourceID,
				tenantA,
				principal,
			),
			want: DecisionReasonExplicitAllow,
		},
		{
			name: "same owner different tenant",
			resource: mustObjectResource(
				t,
				ResourceTypeMarketingActivity,
				resourceID,
				tenantB,
				principal,
			),
			want: DecisionReasonScopeMismatch,
		},
		{
			name: "same tenant different owner",
			resource: mustObjectResource(
				t,
				ResourceTypeMarketingActivity,
				resourceID,
				tenantA,
				mustPrincipal(t, PrincipalKindHuman, "member-2"),
			),
			want: DecisionReasonScopeMismatch,
		},
		{
			name: "missing tenant and owner facts",
			resource: mustObjectResource(
				t,
				ResourceTypeMarketingActivity,
				resourceID,
				"",
				Principal{},
			),
			want: DecisionReasonScopeMismatch,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision, err := policy.Evaluate(mustAuthorizationRequest(
				t,
				principal,
				test.resource,
				ActionRead,
				"evaluation-owned",
			))
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if decision.Reason() != test.want {
				t.Fatalf("reason = %q, want %q", decision.Reason(), test.want)
			}
		})
	}
}

func TestPolicyEvaluateDistinguishesCollectionAndObjectCapabilities(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	policy := mustBaselinePolicy(t, "growthos-access", 5, []RoleBinding{
		mustRoleBinding(
			t,
			"marketing-system",
			principal,
			RoleMarketingOperator,
			NewSystemScope(),
			BindingEffectAllow,
		),
	})
	collection, err := NewCollectionResource(ResourceTypeMarketingActivity, "")
	if err != nil {
		t.Fatalf("new collection: %v", err)
	}
	createDecision, err := policy.Evaluate(mustAuthorizationRequest(
		t,
		principal,
		collection,
		ActionCreate,
		"evaluation-create",
	))
	if err != nil || !createDecision.Allowed() {
		t.Fatalf("collection create = %#v/%v", createDecision, err)
	}

	if _, err := NewAuthorizationRequest(
		principal,
		collection,
		ActionPublish,
		mustAuditContext(t, "evaluation-invalid-kind"),
	); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("collection publish request: %v", err)
	}
}

func TestPolicyEvaluateIsOrderIndependent(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	tenantID := mustTenantID(t, "tenant-a")
	resourceID := mustResourceID(t, "activity-1")
	tenantScope, err := NewTenantScope(tenantID)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}
	bindings := []RoleBinding{
		mustRoleBinding(t, "z-allow", principal, RoleMarketingOperator, tenantScope, BindingEffectAllow),
		mustRoleBinding(
			t,
			"a-deny",
			principal,
			RoleMarketingOperator,
			mustResourceScope(t, ResourceTypeMarketingActivity, resourceID, tenantID),
			BindingEffectDeny,
		),
		mustRoleBinding(t, "m-audit", principal, RoleSecurityAuditor, tenantScope, BindingEffectAllow),
	}
	reversedBindings := append([]RoleBinding(nil), bindings...)
	slices.Reverse(reversedBindings)
	roles := BaselineRoles()
	reversedRoles := BaselineRoles()
	slices.Reverse(reversedRoles)
	identity := mustPolicyIdentity(t, "growthos-access", 8)
	left, err := NewPolicy(identity, roles, bindings)
	if err != nil {
		t.Fatalf("new left policy: %v", err)
	}
	right, err := NewPolicy(identity, reversedRoles, reversedBindings)
	if err != nil {
		t.Fatalf("new right policy: %v", err)
	}
	request := mustAuthorizationRequest(
		t,
		principal,
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			resourceID,
			tenantID,
			Principal{},
		),
		ActionRead,
		"evaluation-order",
	)
	leftDecision, leftErr := left.Evaluate(request)
	rightDecision, rightErr := right.Evaluate(request)
	if leftErr != nil || rightErr != nil {
		t.Fatalf("evaluate errors = %v/%v", leftErr, rightErr)
	}
	if !reflect.DeepEqual(leftDecision, rightDecision) {
		t.Fatalf("order changed decision:\nleft=%#v\nright=%#v", leftDecision, rightDecision)
	}
}

func TestPolicyEvaluateInvalidStateReturnsStrictZeroDecision(t *testing.T) {
	t.Parallel()

	validPolicy := mustBaselinePolicy(t, "growthos-access", 1, nil)
	validRequest := mustAuthorizationRequest(
		t,
		mustPrincipal(t, PrincipalKindHuman, "operator-1"),
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			mustResourceID(t, "activity-1"),
			"",
			Principal{},
		),
		ActionRead,
		"evaluation-valid",
	)

	tests := []struct {
		name    string
		policy  Policy
		request AuthorizationRequest
	}{
		{name: "zero policy", policy: Policy{}, request: validRequest},
		{name: "zero request", policy: validPolicy, request: AuthorizationRequest{}},
		{
			name:    "corrupt policy role",
			policy:  Policy{identity: validPolicy.identity, roles: []Role{{id: "admin"}}},
			request: validRequest,
		},
		{
			name:   "corrupt request action",
			policy: validPolicy,
			request: AuthorizationRequest{
				principal:    validRequest.principal,
				resource:     validRequest.resource,
				action:       "*",
				auditContext: validRequest.auditContext,
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision, err := test.policy.Evaluate(test.request)
			if !errors.Is(err, ErrAuthorizationEvaluationInvalid) {
				t.Fatalf("evaluate error = %v", err)
			}
			if !reflect.DeepEqual(decision, Decision{}) || decision.Confirmed() || decision.Allowed() {
				t.Fatalf("failure returned non-zero decision %#v", decision)
			}
		})
	}
}

func TestDecisionMatchesAreDefensiveCopies(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	policy := mustBaselinePolicy(t, "growthos-access", 1, []RoleBinding{
		mustRoleBinding(
			t,
			"binding-1",
			principal,
			RoleMarketingOperator,
			NewSystemScope(),
			BindingEffectAllow,
		),
	})
	request := mustAuthorizationRequest(
		t,
		principal,
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			mustResourceID(t, "activity-1"),
			"",
			Principal{},
		),
		ActionRead,
		"evaluation-copy",
	)
	decision, err := policy.Evaluate(request)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	matches := decision.Matches()
	matches[0] = DecisionMatch{}
	if err := decision.Validate(); err != nil {
		t.Fatalf("getter mutation changed decision: %v", err)
	}
	if decision.Matches()[0].BindingID() != "binding-1" {
		t.Fatalf("decision match changed: %#v", decision.Matches())
	}
}

func TestDecisionValidateRejectsContradictoryEvidence(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	resource := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		mustResourceID(t, "activity-1"),
		"",
		Principal{},
	)
	auditContext := mustAuditContext(t, "evaluation-decision")
	permission := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	allowMatch := DecisionMatch{
		bindingID:  mustRoleBindingID(t, "allow-1"),
		roleID:     RoleMarketingOperator,
		effect:     BindingEffectAllow,
		scopeKind:  ScopeKindSystem,
		permission: permission,
	}
	base := Decision{
		outcome:        DecisionOutcomeAllow,
		reason:         DecisionReasonExplicitAllow,
		policyIdentity: mustPolicyIdentity(t, "growthos-access", 1),
		principal:      principal,
		resource:       resource,
		action:         ActionRead,
		auditContext:   auditContext,
		matches:        []DecisionMatch{allowMatch},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("base decision invalid: %v", err)
	}

	invalid := []Decision{
		{},
		func() Decision { value := base; value.outcome = DecisionOutcomeDeny; return value }(),
		func() Decision { value := base; value.reason = DecisionReasonExplicitDeny; return value }(),
		func() Decision { value := base; value.matches = nil; return value }(),
		func() Decision {
			value := base
			value.matches = []DecisionMatch{allowMatch, allowMatch}
			return value
		}(),
		func() Decision {
			value := base
			value.matches[0].permission = mustPermission(
				t,
				ResourceKindObject,
				ResourceTypeMarketingActivity,
				ActionPublish,
			)
			return value
		}(),
	}
	for index, decision := range invalid {
		if err := decision.Validate(); !errors.Is(err, ErrDecisionInvalid) {
			t.Fatalf("validate invalid %d: got %v", index, err)
		}
	}
}

func TestPolicyEvaluationIsConcurrentReadSafe(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	policy := mustBaselinePolicy(t, "growthos-access", 9, []RoleBinding{
		mustRoleBinding(
			t,
			"binding-1",
			principal,
			RoleMarketingOperator,
			NewSystemScope(),
			BindingEffectAllow,
		),
	})
	request := mustAuthorizationRequest(
		t,
		principal,
		mustObjectResource(
			t,
			ResourceTypeMarketingActivity,
			mustResourceID(t, "activity-1"),
			"",
			Principal{},
		),
		ActionPublish,
		"evaluation-concurrent",
	)

	const workers = 64
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for range workers {
		go func() {
			defer waitGroup.Done()
			decision, err := policy.Evaluate(request)
			if err != nil {
				errorsChannel <- err
				return
			}
			if !decision.Allowed() || decision.Reason() != DecisionReasonExplicitAllow {
				errorsChannel <- errors.New("unexpected concurrent decision")
			}
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}
}

func TestClosedDecisionEnums(t *testing.T) {
	t.Parallel()

	if !DecisionOutcomeAllow.Valid() || !DecisionOutcomeDeny.Valid() {
		t.Fatal("known outcome invalid")
	}
	if DecisionOutcome("permit").Valid() || DecisionOutcome("").Valid() {
		t.Fatal("unknown outcome became valid")
	}
	for _, reason := range []DecisionReason{
		DecisionReasonExplicitAllow,
		DecisionReasonExplicitDeny,
		DecisionReasonExplicitDenyOverrodeAllow,
		DecisionReasonNoBinding,
		DecisionReasonNoPermission,
		DecisionReasonScopeMismatch,
	} {
		if !reason.Valid() {
			t.Fatalf("known reason %q invalid", reason)
		}
	}
	if DecisionReason("admin").Valid() || DecisionReason("").Valid() {
		t.Fatal("unknown reason became valid")
	}
}

func mustAuditContext(t *testing.T, evaluationReference string) AuditContext {
	t.Helper()
	context, err := NewAuditContext(
		mustAuditReference(t, evaluationReference),
		mustAuditReference(t, "operation-31"),
		time.Date(2026, 8, 31, 12, 0, 0, 123456000, time.UTC),
	)
	if err != nil {
		t.Fatalf("new audit context: %v", err)
	}
	return context
}

func mustAuthorizationRequest(
	t *testing.T,
	principal Principal,
	resource Resource,
	action Action,
	evaluationReference string,
) AuthorizationRequest {
	t.Helper()
	request, err := NewAuthorizationRequest(
		principal,
		resource,
		action,
		mustAuditContext(t, evaluationReference),
	)
	if err != nil {
		t.Fatalf("new authorization request: %v", err)
	}
	return request
}

func mustBaselinePolicy(
	t *testing.T,
	policyID string,
	revision PolicyRevision,
	bindings []RoleBinding,
) Policy {
	t.Helper()
	policy, err := NewBaselinePolicy(mustPolicyIdentity(t, policyID, revision), bindings)
	if err != nil {
		t.Fatalf("new baseline policy: %v", err)
	}
	return policy
}
