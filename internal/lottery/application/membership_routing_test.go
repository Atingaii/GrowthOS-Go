package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestMembershipStrategyRoutingServiceRoutesClosedTiersInDependencyOrder(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	tests := []struct {
		name       string
		tier       domain.MembershipTier
		wantTarget domain.StrategyID
		wantBranch domain.MembershipRoutingBranch
		wantReason domain.MembershipRoutingReasonCode
	}{
		{
			name:       "standard uses explicit baseline default",
			tier:       domain.MembershipTierStandard,
			wantTarget: 100,
			wantBranch: domain.MembershipRoutingBranchBaselineDefault,
			wantReason: domain.MembershipRoutingReasonBaselineStrategy,
		},
		{
			name:       "premium uses explicit override",
			tier:       domain.MembershipTierPremium,
			wantTarget: 200,
			wantBranch: domain.MembershipRoutingBranchPremiumOverride,
			wantReason: domain.MembershipRoutingReasonPremiumStrategy,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := make([]string, 0, 2)
			fact := routingTestFact(t, 42, test.tier, evaluatedAt.Add(-time.Minute))
			reader := &membershipFactReaderStub{
				fact: fact,
				afterRead: func() {
					calls = append(calls, "reader")
				},
			}
			clock := &membershipRoutingClockStub{
				now: evaluatedAt,
				afterNow: func() {
					calls = append(calls, "clock")
				},
			}
			service := routingTestService(t, reader, clock, time.Hour)

			ctx := context.WithValue(context.Background(), routingContextKey{}, "request-context")
			decision, err := service.Route(ctx, 42, policy)
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}
			if !decision.Confirmed() ||
				decision.Target() != test.wantTarget ||
				decision.Branch() != test.wantBranch ||
				decision.ReasonCode() != test.wantReason {
				t.Fatalf(
					"decision = confirmed %t target %d branch %q reason %q",
					decision.Confirmed(),
					decision.Target(),
					decision.Branch(),
					decision.ReasonCode(),
				)
			}
			if decision.RuleCode() != domain.MembershipStrategyRoutingRuleCode ||
				decision.PolicyRevision() != policy.Revision() ||
				decision.FactSource() != fact.Source() ||
				decision.FactRevision() != fact.Revision() ||
				!decision.EvaluatedAt().Equal(evaluatedAt) ||
				decision.EvaluatedAt().Location() != time.UTC {
				t.Fatalf("unexpected decision evidence: %#v", decision)
			}
			if reader.calls != 1 || reader.ctx != ctx || reader.ref != 42 || clock.calls != 1 {
				t.Fatalf(
					"dependency calls = reader %d ctx-match %t ref %d clock %d",
					reader.calls,
					reader.ctx == ctx,
					reader.ref,
					clock.calls,
				)
			}
			if !reflect.DeepEqual(calls, []string{"clock", "reader"}) {
				t.Fatalf("call order = %v, want [clock reader]", calls)
			}
		})
	}
}

func TestMembershipStrategyRoutingServiceCapturesClockExactlyOnce(t *testing.T) {
	initial := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	fact := routingTestFact(t, 42, domain.MembershipTierPremium, initial.Add(-time.Minute))
	clock := &membershipRoutingClockStub{now: initial}
	reader := &membershipFactReaderStub{
		fact: fact,
		afterRead: func() {
			// A second read would change both freshness and recorded evidence.
			clock.now = initial.Add(24 * time.Hour)
		},
	}
	service := routingTestService(t, reader, clock, 15*time.Minute)

	decision, err := service.Route(context.Background(), 42, policy)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if clock.calls != 1 {
		t.Fatalf("clock calls = %d, want exactly one", clock.calls)
	}
	if !decision.EvaluatedAt().Equal(initial) || decision.Target() != 200 {
		t.Fatalf("decision used a recaptured instant: %#v", decision)
	}
}

func TestMembershipStrategyRoutingServiceEnforcesFreshnessNanosecondBoundary(t *testing.T) {
	evaluatedAt := routingTestInstant()
	maxAge := 15 * time.Minute
	policy := routingTestPolicy(t, 200, 100)
	tests := []struct {
		name       string
		ref        domain.MembershipSubjectRef
		fact       domain.MembershipTierFactSnapshot
		wantError  error
		wantTarget domain.StrategyID
	}{
		{
			name:       "exact maximum age remains valid",
			ref:        42,
			fact:       routingTestFact(t, 42, domain.MembershipTierStandard, evaluatedAt.Add(-maxAge)),
			wantTarget: 100,
		},
		{
			name:      "one nanosecond beyond maximum age is stale",
			ref:       42,
			fact:      routingTestFact(t, 42, domain.MembershipTierStandard, evaluatedAt.Add(-maxAge-time.Nanosecond)),
			wantError: ErrMembershipTierFactStale,
		},
		{
			name:      "one nanosecond in the future is invalid",
			ref:       42,
			fact:      routingTestFact(t, 42, domain.MembershipTierStandard, evaluatedAt.Add(time.Nanosecond)),
			wantError: ErrMembershipTierFactInvalid,
		},
		{
			name:      "different subject is invalid",
			ref:       42,
			fact:      routingTestFact(t, 99, domain.MembershipTierPremium, evaluatedAt),
			wantError: ErrMembershipTierFactInvalid,
		},
		{
			name:      "zero fact is invalid",
			ref:       42,
			wantError: ErrMembershipTierFactInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &membershipFactReaderStub{fact: test.fact}
			clock := &membershipRoutingClockStub{now: evaluatedAt}
			service := routingTestService(t, reader, clock, maxAge)

			decision, err := service.Route(context.Background(), test.ref, policy)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("Route() error = %v, want errors.Is %v", err, test.wantError)
				}
				assertZeroMembershipRouteDecision(t, decision)
			} else {
				if err != nil || !decision.Confirmed() || decision.Target() != test.wantTarget {
					t.Fatalf("Route() = %#v, %v; want target %d", decision, err, test.wantTarget)
				}
			}
			if clock.calls != 1 || reader.calls != 1 {
				t.Fatalf("dependency calls = clock %d reader %d, want one each", clock.calls, reader.calls)
			}
		})
	}
}

func TestMembershipStrategyRoutingServiceMakesFactErrorWinReturnedFact(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	fact := routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt)
	providerFailure := errors.New("private provider failure")
	reader := &membershipFactReaderStub{fact: fact, err: providerFailure}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := routingTestService(t, reader, clock, time.Hour)

	decision, err := service.Route(context.Background(), 42, policy)
	if !errors.Is(err, ErrMembershipTierFactReadFailure) {
		t.Fatalf("Route() error = %v, want read failure", err)
	}
	assertZeroMembershipRouteDecision(t, decision)
	var readError *MembershipTierFactReadError
	if !errors.As(err, &readError) || readError.Cause() != providerFailure {
		t.Fatalf("diagnostic cause = %#v, want original provider failure", readError)
	}
	if errors.Is(err, providerFailure) {
		t.Fatal("provider cause leaked into the public errors.Is tree")
	}
	if clock.calls != 1 || reader.calls != 1 {
		t.Fatalf("dependency calls = clock %d reader %d, want one each", clock.calls, reader.calls)
	}
}

func TestMembershipStrategyRoutingServiceClassifiesProviderDeadlineWhileCallerLives(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	providerDeadline := fmt.Errorf("private membership authority: %w", context.DeadlineExceeded)
	reader := &membershipFactReaderStub{err: providerDeadline}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := routingTestService(t, reader, clock, time.Hour)

	decision, err := service.Route(context.Background(), 42, policy)
	if !errors.Is(err, ErrMembershipTierFactUnavailable) {
		t.Fatalf("Route() error = %v, want unavailable", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("provider deadline was mistaken for caller deadline")
	}
	assertZeroMembershipRouteDecision(t, decision)
	var readError *MembershipTierFactReadError
	if !errors.As(err, &readError) || !errors.Is(readError.Cause(), context.DeadlineExceeded) {
		t.Fatal("provider deadline was not retained through the explicit Cause channel")
	}
}

func TestMembershipStrategyRoutingServiceClassifiesProviderPayloadContractErrorsAsInvalid(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	// A returned standard fact would select the baseline default if the service
	// accidentally ignored the accompanying provider contract error.
	returnedFact := routingTestFact(t, 42, domain.MembershipTierStandard, evaluatedAt)
	tests := []struct {
		name        string
		providerErr error
		rawClass    error
	}{
		{
			name: "unsupported or corrupt tier payload",
			providerErr: fmt.Errorf(
				"private provider payload detail: %w",
				domain.ErrMembershipTierFactInvalid,
			),
			rawClass: domain.ErrMembershipTierFactInvalid,
		},
		{
			name: "missing provider subject reference",
			providerErr: fmt.Errorf(
				"private provider payload detail: %w",
				domain.ErrMembershipSubjectRefRequired,
			),
			rawClass: domain.ErrMembershipSubjectRefRequired,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &membershipFactReaderStub{fact: returnedFact, err: test.providerErr}
			clock := &membershipRoutingClockStub{now: evaluatedAt}
			service := routingTestService(t, reader, clock, time.Hour)

			decision, err := service.Route(context.Background(), 42, policy)
			if !errors.Is(err, ErrMembershipTierFactInvalid) {
				t.Fatalf("Route() error = %v, want application invalid fact", err)
			}
			if errors.Is(err, test.rawClass) {
				t.Fatalf("raw domain class %v leaked into public errors.Is", test.rawClass)
			}
			assertZeroMembershipRouteDecision(t, decision)
			var readError *MembershipTierFactReadError
			if !errors.As(err, &readError) || !errors.Is(readError.Cause(), test.rawClass) {
				t.Fatalf("explicit Cause() did not retain %v", test.rawClass)
			}
			if err.Error() != ErrMembershipTierFactInvalid.Error() {
				t.Fatalf("Error() = %q, want safe application class", err.Error())
			}
			if clock.calls != 1 || reader.calls != 1 {
				t.Fatalf("dependency calls = clock %d reader %d, want one each", clock.calls, reader.calls)
			}
		})
	}
}

func TestMembershipStrategyRoutingServiceRejectsInvalidInputsBeforeDependencies(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	reader := &membershipFactReaderStub{
		fact: routingTestFact(t, 42, domain.MembershipTierStandard, evaluatedAt),
	}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := routingTestService(t, reader, clock, time.Hour)
	tests := []struct {
		name   string
		ctx    context.Context
		ref    domain.MembershipSubjectRef
		policy domain.MembershipStrategyRoutingPolicy
	}{
		{name: "nil context", ref: 42, policy: policy},
		{name: "zero subject", ctx: context.Background(), policy: policy},
		{name: "zero policy", ctx: context.Background(), ref: 42},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := service.Route(test.ctx, test.ref, test.policy)
			if !errors.Is(err, ErrMembershipRoutingInvalidArgument) {
				t.Fatalf("Route() error = %v, want invalid argument", err)
			}
			assertZeroMembershipRouteDecision(t, decision)
		})
	}
	if clock.calls != 0 || reader.calls != 0 {
		t.Fatalf("invalid inputs reached dependencies: clock %d reader %d", clock.calls, reader.calls)
	}
}

func TestMembershipStrategyRoutingServiceRejectsNilTypedNilAndPartialConfiguration(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	validReader := &membershipFactReaderStub{
		fact: routingTestFact(t, 42, domain.MembershipTierStandard, evaluatedAt),
	}
	validClock := &membershipRoutingClockStub{now: evaluatedAt}
	var typedNilReader *membershipFactReaderStub
	var typedNilClock *membershipRoutingClockStub
	var typedNilClockFunc MembershipRoutingClockFunc
	tests := []struct {
		name   string
		reader MembershipTierFactReader
		clock  MembershipRoutingClock
		age    time.Duration
	}{
		{name: "nil reader", clock: validClock, age: time.Minute},
		{name: "typed nil reader", reader: typedNilReader, clock: validClock, age: time.Minute},
		{name: "nil clock", reader: validReader, age: time.Minute},
		{name: "typed nil clock pointer", reader: validReader, clock: typedNilClock, age: time.Minute},
		{name: "typed nil clock function", reader: validReader, clock: typedNilClockFunc, age: time.Minute},
		{name: "zero maximum age", reader: validReader, clock: validClock},
		{name: "negative maximum age", reader: validReader, clock: validClock, age: -time.Nanosecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewMembershipStrategyRoutingService(test.reader, test.clock, test.age)
			if !errors.Is(err, ErrMembershipRoutingNotConfigured) || service != nil {
				t.Fatalf("constructor = %#v, %v; want nil and not configured", service, err)
			}
		})
	}

	var nilService *MembershipStrategyRoutingService
	if err := nilService.Validate(); !errors.Is(err, ErrMembershipRoutingNotConfigured) {
		t.Fatalf("nil service Validate() error = %v", err)
	}
	decision, err := nilService.Route(context.Background(), 42, policy)
	if !errors.Is(err, ErrMembershipRoutingNotConfigured) {
		t.Fatalf("nil service Route() error = %v", err)
	}
	assertZeroMembershipRouteDecision(t, decision)

	partialServices := []struct {
		name    string
		service *MembershipStrategyRoutingService
	}{
		{name: "zero value", service: &MembershipStrategyRoutingService{}},
		{
			name: "missing reader",
			service: &MembershipStrategyRoutingService{
				clock:      validClock,
				maxFactAge: time.Minute,
			},
		},
		{
			name: "typed nil reader field",
			service: &MembershipStrategyRoutingService{
				membershipFacts: typedNilReader,
				clock:           validClock,
				maxFactAge:      time.Minute,
			},
		},
		{
			name: "missing clock",
			service: &MembershipStrategyRoutingService{
				membershipFacts: validReader,
				maxFactAge:      time.Minute,
			},
		},
		{
			name: "missing freshness bound",
			service: &MembershipStrategyRoutingService{
				membershipFacts: validReader,
				clock:           validClock,
			},
		},
	}
	for _, test := range partialServices {
		t.Run(test.name, func(t *testing.T) {
			if err := test.service.Validate(); !errors.Is(err, ErrMembershipRoutingNotConfigured) {
				t.Fatalf("Validate() error = %v, want not configured", err)
			}
			decision, err := test.service.Route(context.Background(), 42, policy)
			if !errors.Is(err, ErrMembershipRoutingNotConfigured) {
				t.Fatalf("Route() error = %v, want not configured", err)
			}
			assertZeroMembershipRouteDecision(t, decision)
		})
	}
	if validClock.calls != 0 || validReader.calls != 0 {
		t.Fatalf("invalid configuration reached dependencies: clock %d reader %d", validClock.calls, validReader.calls)
	}
}

func TestMembershipStrategyRoutingServiceRejectsZeroClockBeforeFactRead(t *testing.T) {
	policy := routingTestPolicy(t, 200, 100)
	reader := &membershipFactReaderStub{}
	clock := &membershipRoutingClockStub{}
	service := routingTestService(t, reader, clock, time.Hour)

	decision, err := service.Route(context.Background(), 42, policy)
	if !errors.Is(err, ErrMembershipRoutingClockInvalid) {
		t.Fatalf("Route() error = %v, want invalid clock", err)
	}
	assertZeroMembershipRouteDecision(t, decision)
	if clock.calls != 1 || reader.calls != 0 {
		t.Fatalf("dependency calls = clock %d reader %d, want 1/0", clock.calls, reader.calls)
	}
}

func TestMembershipStrategyRoutingServiceMakesObservedCallerCancellationWin(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	fact := routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt)

	preCanceled, cancelPre := context.WithCancel(context.Background())
	cancelPre()
	reader := &membershipFactReaderStub{fact: fact}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := routingTestService(t, reader, clock, time.Hour)
	decision, err := service.Route(preCanceled, 42, policy)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Route() error = %v", err)
	}
	assertZeroMembershipRouteDecision(t, decision)
	if clock.calls != 0 || reader.calls != 0 {
		t.Fatal("pre-canceled call reached a dependency")
	}

	clockContext, cancelClock := context.WithCancel(context.Background())
	reader = &membershipFactReaderStub{fact: fact}
	clock = &membershipRoutingClockStub{now: evaluatedAt, afterNow: cancelClock}
	service = routingTestService(t, reader, clock, time.Hour)
	decision, err = service.Route(clockContext, 42, policy)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("clock cancellation Route() error = %v", err)
	}
	assertZeroMembershipRouteDecision(t, decision)
	if clock.calls != 1 || reader.calls != 0 {
		t.Fatalf("clock cancellation calls = clock %d reader %d, want 1/0", clock.calls, reader.calls)
	}

	readerContext, cancelReader := context.WithCancel(context.Background())
	dependencyFailure := errors.New("dependency completed after caller cancellation")
	reader = &membershipFactReaderStub{
		fact:      fact,
		err:       dependencyFailure,
		afterRead: cancelReader,
	}
	clock = &membershipRoutingClockStub{now: evaluatedAt}
	service = routingTestService(t, reader, clock, time.Hour)
	decision, err = service.Route(readerContext, 42, policy)
	if !errors.Is(err, context.Canceled) || errors.Is(err, dependencyFailure) {
		t.Fatalf("reader cancellation Route() error = %v, want caller cancellation only", err)
	}
	assertZeroMembershipRouteDecision(t, decision)
	if clock.calls != 1 || reader.calls != 1 {
		t.Fatalf("reader cancellation calls = clock %d reader %d, want 1/1", clock.calls, reader.calls)
	}
}

func TestMembershipStrategyRoutingServiceObservesCancellationAfterBlockingReaderReturns(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	started := make(chan struct{})
	release := make(chan struct{})
	reader := &blockingMembershipFactReader{
		started: started,
		release: release,
		err:     errors.New("provider failure after release"),
	}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := routingTestService(t, reader, clock, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan routingCallResult, 1)
	go func() {
		decision, err := service.Route(ctx, 42, policy)
		result <- routingCallResult{decision: decision, err: err}
	}()

	<-started
	cancel()
	close(release)
	got := <-result
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("blocking reader Route() error = %v, want canceled", got.err)
	}
	assertZeroMembershipRouteDecision(t, got.decision)
	if clock.calls != 1 {
		t.Fatalf("clock calls = %d, want one before blocking reader", clock.calls)
	}
}

func TestMembershipRoutingClockFuncAdaptsControlledClock(t *testing.T) {
	want := routingTestInstant()
	clock := MembershipRoutingClockFunc(func() time.Time { return want })
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("MembershipRoutingClockFunc.Now() = %v, want %v", got, want)
	}
}

func TestMembershipStrategyRoutingServiceSupportsConcurrentReadOnlyCalls(t *testing.T) {
	evaluatedAt := routingTestInstant()
	policy := routingTestPolicy(t, 200, 100)
	fact := routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt.Add(-time.Minute))
	reader := &concurrentMembershipFactReader{fact: fact}
	clock := &concurrentMembershipRoutingClock{now: evaluatedAt}
	service := routingTestService(t, reader, clock, time.Hour)

	const workers = 64
	var waitGroup sync.WaitGroup
	results := make(chan domain.MembershipStrategyRouteDecision, workers)
	errorsSeen := make(chan error, workers)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			decision, err := service.Route(context.Background(), 42, policy)
			results <- decision
			errorsSeen <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent Route() error = %v", err)
		}
	}
	var first domain.MembershipStrategyRouteDecision
	for decision := range results {
		if !decision.Confirmed() || decision.Target() != 200 {
			t.Fatalf("unexpected concurrent decision: %#v", decision)
		}
		if !first.Confirmed() {
			first = decision
			continue
		}
		if !reflect.DeepEqual(first, decision) {
			t.Fatalf("concurrent decisions differ: %#v vs %#v", first, decision)
		}
	}
	if reader.Calls() != workers || clock.Calls() != workers {
		t.Fatalf(
			"dependency calls = reader %d clock %d, want %d each",
			reader.Calls(),
			clock.Calls(),
			workers,
		)
	}
}

type routingContextKey struct{}

type membershipFactReaderStub struct {
	fact      domain.MembershipTierFactSnapshot
	err       error
	afterRead func()
	ctx       context.Context
	ref       domain.MembershipSubjectRef
	calls     int
}

func (reader *membershipFactReaderStub) FindMembershipTierFact(
	ctx context.Context,
	subjectRef domain.MembershipSubjectRef,
) (domain.MembershipTierFactSnapshot, error) {
	reader.calls++
	reader.ctx = ctx
	reader.ref = subjectRef
	if reader.afterRead != nil {
		reader.afterRead()
	}
	return reader.fact, reader.err
}

type blockingMembershipFactReader struct {
	started chan<- struct{}
	release <-chan struct{}
	err     error
}

func (reader *blockingMembershipFactReader) FindMembershipTierFact(
	context.Context,
	domain.MembershipSubjectRef,
) (domain.MembershipTierFactSnapshot, error) {
	close(reader.started)
	<-reader.release
	return domain.MembershipTierFactSnapshot{}, reader.err
}

type membershipRoutingClockStub struct {
	now      time.Time
	afterNow func()
	calls    int
}

func (clock *membershipRoutingClockStub) Now() time.Time {
	clock.calls++
	if clock.afterNow != nil {
		clock.afterNow()
	}
	return clock.now
}

type concurrentMembershipFactReader struct {
	mu    sync.Mutex
	fact  domain.MembershipTierFactSnapshot
	calls int
}

func (reader *concurrentMembershipFactReader) FindMembershipTierFact(
	context.Context,
	domain.MembershipSubjectRef,
) (domain.MembershipTierFactSnapshot, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.calls++
	return reader.fact, nil
}

func (reader *concurrentMembershipFactReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

type concurrentMembershipRoutingClock struct {
	mu    sync.Mutex
	now   time.Time
	calls int
}

func (clock *concurrentMembershipRoutingClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.calls++
	return clock.now
}

func (clock *concurrentMembershipRoutingClock) Calls() int {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.calls
}

type routingCallResult struct {
	decision domain.MembershipStrategyRouteDecision
	err      error
}

func routingTestService(
	t *testing.T,
	reader MembershipTierFactReader,
	clock MembershipRoutingClock,
	maxFactAge time.Duration,
) *MembershipStrategyRoutingService {
	t.Helper()
	service, err := NewMembershipStrategyRoutingService(reader, clock, maxFactAge)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	return service
}

func routingTestPolicy(
	t *testing.T,
	premiumTarget domain.StrategyID,
	baselineDefault domain.StrategyID,
) domain.MembershipStrategyRoutingPolicy {
	t.Helper()
	policy, err := domain.NewMembershipStrategyRoutingPolicy(
		"membership-route-v1",
		premiumTarget,
		baselineDefault,
	)
	if err != nil {
		t.Fatalf("construct routing policy: %v", err)
	}
	return policy
}

func routingTestFact(
	t *testing.T,
	subjectRef domain.MembershipSubjectRef,
	tier domain.MembershipTier,
	observedAt time.Time,
) domain.MembershipTierFactSnapshot {
	t.Helper()
	fact, err := domain.NewMembershipTierFactSnapshot(
		subjectRef,
		tier,
		observedAt,
		"membership-directory",
		"membership-snapshot-42",
	)
	if err != nil {
		t.Fatalf("construct membership fact: %v", err)
	}
	return fact
}

func routingTestInstant() time.Time {
	return time.Date(2026, time.August, 30, 12, 0, 0, 123, time.FixedZone("UTC+8", 8*60*60))
}

func assertZeroMembershipRouteDecision(
	t *testing.T,
	decision domain.MembershipStrategyRouteDecision,
) {
	t.Helper()
	if decision.Confirmed() ||
		decision.Target() != 0 ||
		decision.RuleCode() != "" ||
		decision.Branch() != "" ||
		decision.ReasonCode() != "" ||
		decision.PolicyRevision() != "" ||
		decision.FactSource() != "" ||
		decision.FactRevision() != "" ||
		!decision.EvaluatedAt().IsZero() ||
		len(decision.Path()) != 0 {
		t.Fatalf("expected zero membership route decision, got %#v", decision)
	}
}
