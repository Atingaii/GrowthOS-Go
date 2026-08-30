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

func TestStrategyRoutingGraphEvaluationServiceEvaluatesExactGraphInDependencyOrder(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 71, "graph-r1", 200, 100)
	tests := []struct {
		name       string
		tier       domain.MembershipTier
		wantTarget domain.StrategyID
		wantBranch domain.MembershipRoutingBranch
		wantReason domain.MembershipRoutingReasonCode
		wantEnd    domain.StrategyRoutingNodeID
	}{
		{
			name:       "premium override",
			tier:       domain.MembershipTierPremium,
			wantTarget: 200,
			wantBranch: domain.MembershipRoutingBranchPremiumOverride,
			wantReason: domain.MembershipRoutingReasonPremiumStrategy,
			wantEnd:    2,
		},
		{
			name:       "standard explicit baseline",
			tier:       domain.MembershipTierStandard,
			wantTarget: 100,
			wantBranch: domain.MembershipRoutingBranchBaselineDefault,
			wantReason: domain.MembershipRoutingReasonBaselineStrategy,
			wantEnd:    3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := make([]string, 0, 3)
			graphReader := &strategyRoutingGraphReaderStub{
				graph: graph,
				afterRead: func() {
					calls = append(calls, "graph")
				},
			}
			fact := routingTestFact(t, 42, test.tier, evaluatedAt.Add(-time.Minute))
			factReader := &membershipFactReaderStub{
				fact: fact,
				afterRead: func() {
					calls = append(calls, "fact")
				},
			}
			clock := &membershipRoutingClockStub{
				now: evaluatedAt,
				afterNow: func() {
					calls = append(calls, "clock")
				},
			}
			service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

			callerCtx := context.WithValue(context.Background(), graphEvaluationContextKey{}, "caller")
			decision, err := service.Evaluate(callerCtx, 42, graph.Identity())
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if !decision.Confirmed() || decision.Identity() != graph.Identity() ||
				decision.SchemaVersion() != domain.StrategyRoutingGraphSchemaVersionV1 ||
				decision.RootNodeID() != 1 || decision.TerminalNodeID() != test.wantEnd ||
				decision.Target() != test.wantTarget ||
				decision.FactSource() != fact.Source() || decision.FactRevision() != fact.Revision() ||
				!decision.EvaluatedAt().Equal(evaluatedAt) || decision.EvaluatedAt().Location() != time.UTC {
				t.Fatalf("unexpected decision: %#v", decision)
			}
			path := decision.Path()
			if len(path) != 1 || path[0].FromNodeID() != 1 || path[0].ToNodeID() != test.wantEnd ||
				path[0].RuleCode() != domain.MembershipStrategyRoutingRuleCode ||
				path[0].Branch() != test.wantBranch || path[0].ReasonCode() != test.wantReason {
				t.Fatalf("unexpected path: %#v", path)
			}
			if !reflect.DeepEqual(calls, []string{"graph", "clock", "fact"}) {
				t.Fatalf("dependency order = %v, want graph/clock/fact", calls)
			}
			if graphReader.calls != 1 || graphReader.identity != graph.Identity() ||
				graphReader.ctx == callerCtx || graphReader.ctx.Value(graphEvaluationContextKey{}) != "caller" ||
				factReader.calls != 1 || factReader.ref != 42 || factReader.ctx != graphReader.ctx ||
				clock.calls != 1 {
				t.Fatalf(
					"dependency evidence = graph %d identity %#v fact %d/%d clock %d",
					graphReader.calls,
					graphReader.identity,
					factReader.calls,
					factReader.ref,
					clock.calls,
				)
			}
		})
	}
}

func TestStrategyRoutingGraphEvaluationServiceAppliesWorstCaseDepthBeforeFact(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTwoStepGraph(t, 81, "graph-depth-r1", 300, 200, 100)
	tests := []struct {
		name       string
		maxSteps   int
		wantError  error
		wantTarget domain.StrategyID
		wantReads  int
	}{
		{
			name:      "depth above service budget stops before clock and fact",
			maxSteps:  1,
			wantError: domain.ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
		},
		{
			name:       "depth equal to service budget succeeds",
			maxSteps:   2,
			wantTarget: 300,
			wantReads:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graphReader := &strategyRoutingGraphReaderStub{graph: graph}
			factReader := &membershipFactReaderStub{
				fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
			}
			clock := &membershipRoutingClockStub{now: evaluatedAt}
			service := graphEvaluationTestService(
				t,
				graphReader,
				factReader,
				clock,
				time.Hour,
				test.maxSteps,
				time.Second,
			)

			decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("Evaluate() error = %v, want %v", err, test.wantError)
				}
				assertZeroStrategyRoutingGraphDecision(t, decision)
			} else if err != nil || !decision.Confirmed() || decision.Target() != test.wantTarget || len(decision.Path()) != 2 {
				t.Fatalf("Evaluate() = %#v, %v; want two-step target %d", decision, err, test.wantTarget)
			}
			if graphReader.calls != 1 || clock.calls != test.wantReads || factReader.calls != test.wantReads {
				t.Fatalf(
					"dependency calls = graph %d clock %d fact %d, want 1/%d/%d",
					graphReader.calls,
					clock.calls,
					factReader.calls,
					test.wantReads,
					test.wantReads,
				)
			}
		})
	}
}

func TestStrategyRoutingGraphEvaluationServiceRejectsInvalidInputAndTypedNilConfiguration(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 91, "graph-config-r1", 200, 100)
	validGraphReader := &strategyRoutingGraphReaderStub{graph: graph}
	validFactReader := &membershipFactReaderStub{
		fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
	}
	validClock := &membershipRoutingClockStub{now: evaluatedAt}
	validBudget := graphEvaluationTestBudget(t, 1)
	var typedNilGraphReader *strategyRoutingGraphReaderStub
	var typedNilFactReader *membershipFactReaderStub
	var typedNilClock *membershipRoutingClockStub
	var typedNilClockFunc MembershipRoutingClockFunc

	configurations := []struct {
		name        string
		graphs      StrategyRoutingGraphReader
		facts       MembershipTierFactReader
		clock       MembershipRoutingClock
		maxFactAge  time.Duration
		budget      domain.StrategyRoutingGraphStepBudget
		maxDuration time.Duration
	}{
		{name: "nil graph reader", facts: validFactReader, clock: validClock, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "typed nil graph reader", graphs: typedNilGraphReader, facts: validFactReader, clock: validClock, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "nil fact reader", graphs: validGraphReader, clock: validClock, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "typed nil fact reader", graphs: validGraphReader, facts: typedNilFactReader, clock: validClock, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "nil clock", graphs: validGraphReader, facts: validFactReader, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "typed nil clock pointer", graphs: validGraphReader, facts: validFactReader, clock: typedNilClock, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "typed nil clock function", graphs: validGraphReader, facts: validFactReader, clock: typedNilClockFunc, maxFactAge: time.Minute, budget: validBudget, maxDuration: time.Second},
		{name: "zero fact age", graphs: validGraphReader, facts: validFactReader, clock: validClock, budget: validBudget, maxDuration: time.Second},
		{name: "negative fact age", graphs: validGraphReader, facts: validFactReader, clock: validClock, maxFactAge: -time.Nanosecond, budget: validBudget, maxDuration: time.Second},
		{name: "zero step budget", graphs: validGraphReader, facts: validFactReader, clock: validClock, maxFactAge: time.Minute, maxDuration: time.Second},
		{name: "zero duration", graphs: validGraphReader, facts: validFactReader, clock: validClock, maxFactAge: time.Minute, budget: validBudget},
		{name: "negative duration", graphs: validGraphReader, facts: validFactReader, clock: validClock, maxFactAge: time.Minute, budget: validBudget, maxDuration: -time.Nanosecond},
	}
	for _, test := range configurations {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewStrategyRoutingGraphEvaluationService(
				test.graphs,
				test.facts,
				test.clock,
				test.maxFactAge,
				test.budget,
				test.maxDuration,
			)
			if service != nil || !errors.Is(err, ErrStrategyRoutingGraphEvaluationNotConfigured) {
				t.Fatalf("constructor = %#v, %v; want nil/not configured", service, err)
			}
		})
	}

	var nilService *StrategyRoutingGraphEvaluationService
	if err := nilService.Validate(); !errors.Is(err, ErrStrategyRoutingGraphEvaluationNotConfigured) {
		t.Fatalf("nil service Validate() error = %v", err)
	}
	decision, err := nilService.Evaluate(context.Background(), 42, graph.Identity())
	if !errors.Is(err, ErrStrategyRoutingGraphEvaluationNotConfigured) {
		t.Fatalf("nil service Evaluate() error = %v", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)

	service := graphEvaluationTestService(t, validGraphReader, validFactReader, validClock, time.Hour, 1, time.Second)
	inputs := []struct {
		name     string
		ctx      context.Context
		ref      domain.MembershipSubjectRef
		identity domain.StrategyRoutingGraphIdentity
	}{
		{name: "nil context", ref: 42, identity: graph.Identity()},
		{name: "zero subject", ctx: context.Background(), identity: graph.Identity()},
		{name: "zero identity", ctx: context.Background(), ref: 42},
	}
	for _, test := range inputs {
		t.Run(test.name, func(t *testing.T) {
			decision, err := service.Evaluate(test.ctx, test.ref, test.identity)
			if !errors.Is(err, ErrStrategyRoutingGraphEvaluationInvalidArgument) {
				t.Fatalf("Evaluate() error = %v, want invalid argument", err)
			}
			assertZeroStrategyRoutingGraphDecision(t, decision)
		})
	}
	if validGraphReader.calls != 0 || validClock.calls != 0 || validFactReader.calls != 0 {
		t.Fatalf("invalid calls reached dependencies: graph %d clock %d fact %d", validGraphReader.calls, validClock.calls, validFactReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceMakesGraphErrorWinReturnedValue(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 101, "graph-error-r1", 200, 100)
	privateFailure := fmt.Errorf("private sql and topology detail: %w", ErrRepositoryFailure)
	graphReader := &strategyRoutingGraphReaderStub{graph: graph, err: privateFailure}
	factReader := &membershipFactReaderStub{
		fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
	}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

	decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
	if !errors.Is(err, ErrStrategyRoutingGraphEvaluationFailure) || errors.Is(err, ErrRepositoryFailure) {
		t.Fatalf("Evaluate() error = %v, want only evaluation failure", err)
	}
	if err.Error() != ErrStrategyRoutingGraphEvaluationFailure.Error() {
		t.Fatalf("Error() = %q, want stable low-disclosure text", err.Error())
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	var evaluationError *StrategyRoutingGraphEvaluationError
	if !errors.As(err, &evaluationError) || evaluationError.Cause() != privateFailure {
		t.Fatalf("diagnostic cause = %#v, want private graph failure", evaluationError)
	}
	if clock.calls != 0 || factReader.calls != 0 {
		t.Fatalf("graph failure reached clock/fact: %d/%d", clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceClassifiesLiveGraphProviderDeadlineAsFailure(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 106, "graph-provider-timeout-r1", 200, 100)
	providerDeadline := fmt.Errorf("private graph storage deadline: %w", context.DeadlineExceeded)
	graphReader := &strategyRoutingGraphReaderStub{err: providerDeadline}
	factReader := &membershipFactReaderStub{
		fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
	}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

	decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
	if !errors.Is(err, ErrStrategyRoutingGraphEvaluationFailure) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrStrategyRoutingGraphEvaluationTimedOut) {
		t.Fatalf("Evaluate() error = %v, want graph provider failure only", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	var evaluationError *StrategyRoutingGraphEvaluationError
	if !errors.As(err, &evaluationError) || evaluationError.Cause() != providerDeadline {
		t.Fatalf("diagnostic cause = %#v, want graph provider deadline", evaluationError)
	}
	if graphReader.calls != 1 || clock.calls != 0 || factReader.calls != 0 {
		t.Fatalf("provider deadline calls = graph %d clock %d fact %d", graphReader.calls, clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceRejectsWrongExactIdentityAndPreservesNotFoundClass(t *testing.T) {
	evaluatedAt := routingTestInstant()
	requestedGraph := graphEvaluationTestGraph(t, 111, "graph-requested-r1", 200, 100)
	otherGraph := graphEvaluationTestGraph(t, 112, "graph-other-r1", 200, 100)
	tests := []struct {
		name      string
		graph     domain.StrategyRoutingGraph
		readError error
		wantError error
	}{
		{name: "different exact identity", graph: otherGraph, wantError: ErrStrategyRoutingGraphEvaluationGraphInvalid},
		{name: "exact identity absent", readError: WrapRepositoryError(ErrStrategyRoutingGraphNotFound, errors.New("private missing row")), wantError: ErrStrategyRoutingGraphNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graphReader := &strategyRoutingGraphReaderStub{graph: test.graph, err: test.readError}
			factReader := &membershipFactReaderStub{
				fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
			}
			clock := &membershipRoutingClockStub{now: evaluatedAt}
			service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

			decision, err := service.Evaluate(context.Background(), 42, requestedGraph.Identity())
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Evaluate() error = %v, want %v", err, test.wantError)
			}
			assertZeroStrategyRoutingGraphDecision(t, decision)
			if graphReader.calls != 1 || graphReader.identity != requestedGraph.Identity() ||
				clock.calls != 0 || factReader.calls != 0 {
				t.Fatalf("dependency evidence = graph %d identity %#v clock %d fact %d", graphReader.calls, graphReader.identity, clock.calls, factReader.calls)
			}
		})
	}
}

func TestStrategyRoutingGraphEvaluationServiceMapsStoredGraphInvalidWithoutDisclosure(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 116, "graph-stored-invalid-r1", 200, 100)
	privateStorageCause := errors.New("private SQL, graph identity and corrupt row detail")
	repositoryError := WrapRepositoryError(
		ErrStoredStrategyRoutingGraphInvalid,
		privateStorageCause,
	)
	graphReader := &strategyRoutingGraphReaderStub{graph: graph, err: repositoryError}
	factReader := &membershipFactReaderStub{
		fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
	}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

	decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
	if !errors.Is(err, ErrStrategyRoutingGraphEvaluationGraphInvalid) ||
		errors.Is(err, ErrStoredStrategyRoutingGraphInvalid) ||
		errors.Is(err, privateStorageCause) {
		t.Fatalf("Evaluate() error = %v, want only evaluation graph-invalid class", err)
	}
	if err.Error() != ErrStrategyRoutingGraphEvaluationGraphInvalid.Error() {
		t.Fatalf("Error() = %q, want stable graph-invalid text", err.Error())
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	var evaluationError *StrategyRoutingGraphEvaluationError
	if !errors.As(err, &evaluationError) || evaluationError.Cause() != repositoryError {
		t.Fatalf("trusted diagnostic Cause() = %#v, want repository wrapper", evaluationError)
	}
	if graphReader.calls != 1 || clock.calls != 0 || factReader.calls != 0 {
		t.Fatalf("stored invalid calls = graph %d clock %d fact %d", graphReader.calls, clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceMakesFactErrorWinReturnedValue(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 121, "graph-fact-error-r1", 200, 100)
	returnedFact := routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt)
	providerDeadline := fmt.Errorf("private membership deadline: %w", context.DeadlineExceeded)
	graphReader := &strategyRoutingGraphReaderStub{graph: graph}
	factReader := &membershipFactReaderStub{fact: returnedFact, err: providerDeadline}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

	decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
	if !errors.Is(err, ErrMembershipTierFactUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Evaluate() error = %v, want provider unavailable only", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	var readError *MembershipTierFactReadError
	if !errors.As(err, &readError) || readError.Cause() != providerDeadline {
		t.Fatalf("provider cause = %#v, want private deadline", readError)
	}
	if graphReader.calls != 1 || clock.calls != 1 || factReader.calls != 1 {
		t.Fatalf("dependency calls = graph %d clock %d fact %d", graphReader.calls, clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceRejectsZeroClockBeforeFact(t *testing.T) {
	graph := graphEvaluationTestGraph(t, 131, "graph-clock-r1", 200, 100)
	graphReader := &strategyRoutingGraphReaderStub{graph: graph}
	factReader := &membershipFactReaderStub{}
	clock := &membershipRoutingClockStub{}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)

	decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
	if !errors.Is(err, ErrMembershipRoutingClockInvalid) {
		t.Fatalf("Evaluate() error = %v, want invalid clock", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	if graphReader.calls != 1 || clock.calls != 1 || factReader.calls != 0 {
		t.Fatalf("dependency calls = graph %d clock %d fact %d, want 1/1/0", graphReader.calls, clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceCancellationWinsAtDependencyBoundaries(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 141, "graph-cancel-r1", 200, 100)
	fact := routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt)
	providerFailure := errors.New("private provider failure")

	preCanceled, cancelPre := context.WithCancel(context.Background())
	cancelPre()
	graphReader := &strategyRoutingGraphReaderStub{graph: graph}
	factReader := &membershipFactReaderStub{fact: fact}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)
	decision, err := service.Evaluate(preCanceled, 42, graph.Identity())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled Evaluate() error = %v", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	if graphReader.calls != 0 || clock.calls != 0 || factReader.calls != 0 {
		t.Fatal("pre-canceled evaluation reached a dependency")
	}

	graphCtx, cancelGraph := context.WithCancel(context.Background())
	graphReader = &strategyRoutingGraphReaderStub{graph: graph, err: providerFailure, afterRead: cancelGraph}
	factReader = &membershipFactReaderStub{fact: fact}
	clock = &membershipRoutingClockStub{now: evaluatedAt}
	service = graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)
	decision, err = service.Evaluate(graphCtx, 42, graph.Identity())
	if !errors.Is(err, context.Canceled) || errors.Is(err, providerFailure) {
		t.Fatalf("graph-boundary Evaluate() error = %v, want caller cancellation", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	if graphReader.calls != 1 || clock.calls != 0 || factReader.calls != 0 {
		t.Fatalf("graph cancellation calls = %d/%d/%d", graphReader.calls, clock.calls, factReader.calls)
	}

	clockCtx, cancelClock := context.WithCancel(context.Background())
	graphReader = &strategyRoutingGraphReaderStub{graph: graph}
	factReader = &membershipFactReaderStub{fact: fact}
	clock = &membershipRoutingClockStub{now: evaluatedAt, afterNow: cancelClock}
	service = graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)
	decision, err = service.Evaluate(clockCtx, 42, graph.Identity())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("clock-boundary Evaluate() error = %v", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	if graphReader.calls != 1 || clock.calls != 1 || factReader.calls != 0 {
		t.Fatalf("clock cancellation calls = %d/%d/%d", graphReader.calls, clock.calls, factReader.calls)
	}

	factCtx, cancelFact := context.WithCancel(context.Background())
	graphReader = &strategyRoutingGraphReaderStub{graph: graph}
	factReader = &membershipFactReaderStub{fact: fact, err: providerFailure, afterRead: cancelFact}
	clock = &membershipRoutingClockStub{now: evaluatedAt}
	service = graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)
	decision, err = service.Evaluate(factCtx, 42, graph.Identity())
	if !errors.Is(err, context.Canceled) || errors.Is(err, providerFailure) {
		t.Fatalf("fact-boundary Evaluate() error = %v, want caller cancellation", err)
	}
	assertZeroStrategyRoutingGraphDecision(t, decision)
	if graphReader.calls != 1 || clock.calls != 1 || factReader.calls != 1 {
		t.Fatalf("fact cancellation calls = %d/%d/%d", graphReader.calls, clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceInternalTimeoutUsesPrivateCauseWithoutSleep(t *testing.T) {
	graph := graphEvaluationTestGraph(t, 151, "graph-timeout-r1", 200, 100)
	started := make(chan struct{})
	graphReader := &contextBlockingStrategyRoutingGraphReader{
		graph:   graph,
		started: started,
		err:     errors.New("provider returned after operation deadline"),
	}
	factReader := &membershipFactReaderStub{}
	clock := &membershipRoutingClockStub{}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, 100*time.Millisecond)

	result := make(chan graphEvaluationCallResult, 1)
	go func() {
		decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
		result <- graphEvaluationCallResult{decision: decision, err: err}
	}()
	got := awaitBlockedGraphEvaluationResult(t, started, result)
	if !errors.Is(got.err, ErrStrategyRoutingGraphEvaluationTimedOut) {
		t.Fatalf("Evaluate() error = %v, want internal timeout", got.err)
	}
	if errors.Is(got.err, context.DeadlineExceeded) || errors.Is(got.err, context.Canceled) {
		t.Fatal("internal timeout leaked a context error through errors.Is")
	}
	if got.err.Error() != ErrStrategyRoutingGraphEvaluationTimedOut.Error() {
		t.Fatalf("Error() = %q, want stable timeout class", got.err.Error())
	}
	assertZeroStrategyRoutingGraphDecision(t, got.decision)
	var evaluationError *StrategyRoutingGraphEvaluationError
	if !errors.As(got.err, &evaluationError) || evaluationError.Cause() != errStrategyRoutingGraphEvaluationInternalDeadline {
		t.Fatalf("timeout Cause() = %#v, want exact private cause", evaluationError)
	}
	if graphReader.Calls() != 1 || clock.calls != 0 || factReader.calls != 0 {
		t.Fatalf("timeout calls = graph %d clock %d fact %d", graphReader.Calls(), clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationServiceFactBlockInternalTimeoutWinsProviderError(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTestGraph(t, 156, "graph-fact-timeout-r1", 200, 100)
	started := make(chan struct{})
	providerFailure := errors.New("membership provider returned after operation deadline")
	graphReader := &strategyRoutingGraphReaderStub{graph: graph}
	factReader := &contextBlockingMembershipFactReader{
		fact:    routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
		started: started,
		err:     providerFailure,
	}
	clock := &membershipRoutingClockStub{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, 100*time.Millisecond)

	result := make(chan graphEvaluationCallResult, 1)
	go func() {
		decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
		result <- graphEvaluationCallResult{decision: decision, err: err}
	}()
	got := awaitBlockedGraphEvaluationResult(t, started, result)
	if !errors.Is(got.err, ErrStrategyRoutingGraphEvaluationTimedOut) ||
		errors.Is(got.err, providerFailure) ||
		errors.Is(got.err, ErrMembershipTierFactReadFailure) ||
		errors.Is(got.err, context.DeadlineExceeded) {
		t.Fatalf("Evaluate() error = %v, want internal timeout to win provider error", got.err)
	}
	assertZeroStrategyRoutingGraphDecision(t, got.decision)
	var evaluationError *StrategyRoutingGraphEvaluationError
	if !errors.As(got.err, &evaluationError) || evaluationError.Cause() != errStrategyRoutingGraphEvaluationInternalDeadline {
		t.Fatalf("timeout Cause() = %#v, want exact private cause", evaluationError)
	}
	if graphReader.calls != 1 || clock.calls != 1 || factReader.Calls() != 1 {
		t.Fatalf("fact timeout calls = graph %d clock %d fact %d", graphReader.calls, clock.calls, factReader.Calls())
	}
}

func TestStrategyRoutingGraphEvaluationServiceCallerEarlierDeadlineWinsEndToEnd(t *testing.T) {
	graph := graphEvaluationTestGraph(t, 157, "graph-caller-deadline-r1", 200, 100)
	started := make(chan struct{})
	providerFailure := errors.New("graph provider returned after caller deadline")
	graphReader := &contextBlockingStrategyRoutingGraphReader{
		graph:   graph,
		started: started,
		err:     providerFailure,
	}
	factReader := &membershipFactReaderStub{}
	clock := &membershipRoutingClockStub{}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 1, time.Second)
	callerCtx, cancelCaller := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelCaller()
	callerDeadline, ok := callerCtx.Deadline()
	if !ok {
		t.Fatal("caller context did not expose its deadline")
	}

	result := make(chan graphEvaluationCallResult, 1)
	go func() {
		decision, err := service.Evaluate(callerCtx, 42, graph.Identity())
		result <- graphEvaluationCallResult{decision: decision, err: err}
	}()
	got := awaitBlockedGraphEvaluationResult(t, started, result)
	if got.err != context.DeadlineExceeded ||
		errors.Is(got.err, ErrStrategyRoutingGraphEvaluationTimedOut) ||
		errors.Is(got.err, providerFailure) {
		t.Fatalf("Evaluate() error = %v, want exact caller deadline", got.err)
	}
	assertZeroStrategyRoutingGraphDecision(t, got.decision)
	readerDeadline, hasReaderDeadline := graphReader.Deadline()
	if !hasReaderDeadline || !readerDeadline.Equal(callerDeadline) {
		t.Fatalf("graph reader deadline = %v/%t, want caller deadline %v", readerDeadline, hasReaderDeadline, callerDeadline)
	}
	if graphReader.Calls() != 1 || clock.calls != 0 || factReader.calls != 0 {
		t.Fatalf("caller deadline calls = graph %d clock %d fact %d", graphReader.Calls(), clock.calls, factReader.calls)
	}
}

func TestStrategyRoutingGraphEvaluationContextGivesCallerEarlierOrEqualDeadlinePriority(t *testing.T) {
	callerCause := errors.New("caller-owned deadline")
	past := time.Now().Add(-time.Second)
	callerCtx, cancelCaller := context.WithDeadlineCause(context.Background(), past, callerCause)
	defer cancelCaller()
	<-callerCtx.Done()
	callerDeadline, ok := callerCtx.Deadline()
	if !ok {
		t.Fatal("caller context did not expose its deadline")
	}

	for _, internalDeadline := range []time.Time{
		callerDeadline,
		callerDeadline.Add(time.Second),
	} {
		evaluationCtx, cleanup := strategyRoutingGraphEvaluationContext(callerCtx, internalDeadline)
		<-evaluationCtx.Done()
		if cause := context.Cause(evaluationCtx); cause != callerCause {
			cleanup()
			t.Fatalf("earlier/equal child cause = %v, want caller cause", cause)
		}
		if err := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); !errors.Is(err, context.DeadlineExceeded) {
			cleanup()
			t.Fatalf("context classification = %v, want caller deadline", err)
		}
		cleanup()
	}

	liveCaller := context.Background()
	internalCtx, cleanupInternal := strategyRoutingGraphEvaluationContext(liveCaller, past)
	defer cleanupInternal()
	<-internalCtx.Done()
	if cause := context.Cause(internalCtx); cause != errStrategyRoutingGraphEvaluationInternalDeadline {
		t.Fatalf("internal child cause = %v, want private internal cause", cause)
	}
	classified := strategyRoutingGraphEvaluationContextError(liveCaller, internalCtx)
	if !errors.Is(classified, ErrStrategyRoutingGraphEvaluationTimedOut) || errors.Is(classified, context.DeadlineExceeded) {
		t.Fatalf("internal classification = %v", classified)
	}

	future := time.Now().Add(time.Hour)
	cleanupCtx, cleanup := strategyRoutingGraphEvaluationContext(context.Background(), future)
	cleanup()
	<-cleanupCtx.Done()
	classified = strategyRoutingGraphEvaluationContextError(context.Background(), cleanupCtx)
	if errors.Is(classified, ErrStrategyRoutingGraphEvaluationTimedOut) {
		t.Fatal("explicit cleanup cancellation was misclassified as internal timeout")
	}
}

func TestStrategyRoutingGraphEvaluationErrorExposesOnlyReviewedClass(t *testing.T) {
	privateCause := fmt.Errorf("private graph id, node and SQL: %w", context.DeadlineExceeded)
	err := wrapStrategyRoutingGraphEvaluationError(ErrStrategyRoutingGraphEvaluationTimedOut, privateCause)
	if !errors.Is(err, ErrStrategyRoutingGraphEvaluationTimedOut) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, privateCause) {
		t.Fatalf("unexpected errors.Is exposure for %v", err)
	}
	if err.Error() != ErrStrategyRoutingGraphEvaluationTimedOut.Error() || err.Cause() != privateCause {
		t.Fatalf("wrapper = %q cause %#v", err.Error(), err.Cause())
	}

	unknown := errors.New("unknown public class")
	fallback := wrapStrategyRoutingGraphEvaluationError(unknown, privateCause)
	if !errors.Is(fallback, ErrStrategyRoutingGraphEvaluationFailure) || errors.Is(fallback, unknown) {
		t.Fatalf("unknown class did not fail closed: %v", fallback)
	}
	var nilError *StrategyRoutingGraphEvaluationError
	if nilError.Error() != ErrStrategyRoutingGraphEvaluationFailure.Error() ||
		!nilError.Is(ErrStrategyRoutingGraphEvaluationFailure) || nilError.Cause() != nil {
		t.Fatal("nil evaluation error did not fail closed")
	}
}

func TestStrategyRoutingGraphEvaluationServiceSupports64ConcurrentReadOnlyCalls(t *testing.T) {
	evaluatedAt := routingTestInstant()
	graph := graphEvaluationTwoStepGraph(t, 161, "graph-concurrent-r1", 300, 200, 100)
	graphReader := &concurrentStrategyRoutingGraphReader{graph: graph}
	factReader := &concurrentMembershipFactReader{
		fact: routingTestFact(t, 42, domain.MembershipTierPremium, evaluatedAt),
	}
	clock := &concurrentMembershipRoutingClock{now: evaluatedAt}
	service := graphEvaluationTestService(t, graphReader, factReader, clock, time.Hour, 2, time.Second)

	const workers = 64
	results := make(chan domain.StrategyRoutingGraphDecision, workers)
	errorsSeen := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			decision, err := service.Evaluate(context.Background(), 42, graph.Identity())
			results <- decision
			errorsSeen <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent Evaluate() error = %v", err)
		}
	}
	var first domain.StrategyRoutingGraphDecision
	for decision := range results {
		if !decision.Confirmed() || decision.Target() != 300 || len(decision.Path()) != 2 {
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
	if graphReader.Calls() != workers || factReader.Calls() != workers || clock.Calls() != workers {
		t.Fatalf("dependency calls = graph %d fact %d clock %d, want %d each", graphReader.Calls(), factReader.Calls(), clock.Calls(), workers)
	}
}

type graphEvaluationContextKey struct{}

type strategyRoutingGraphReaderStub struct {
	graph     domain.StrategyRoutingGraph
	err       error
	afterRead func()
	ctx       context.Context
	identity  domain.StrategyRoutingGraphIdentity
	calls     int
}

func (reader *strategyRoutingGraphReaderStub) FindByIdentity(
	ctx context.Context,
	identity domain.StrategyRoutingGraphIdentity,
) (domain.StrategyRoutingGraph, error) {
	reader.calls++
	reader.ctx = ctx
	reader.identity = identity
	if reader.afterRead != nil {
		reader.afterRead()
	}
	return reader.graph, reader.err
}

type contextBlockingStrategyRoutingGraphReader struct {
	mu          sync.Mutex
	graph       domain.StrategyRoutingGraph
	started     chan<- struct{}
	err         error
	calls       int
	deadline    time.Time
	hasDeadline bool
}

func (reader *contextBlockingStrategyRoutingGraphReader) FindByIdentity(
	ctx context.Context,
	_ domain.StrategyRoutingGraphIdentity,
) (domain.StrategyRoutingGraph, error) {
	reader.mu.Lock()
	reader.calls++
	reader.deadline, reader.hasDeadline = ctx.Deadline()
	reader.mu.Unlock()
	close(reader.started)
	<-ctx.Done()
	return reader.graph, reader.err
}

func (reader *contextBlockingStrategyRoutingGraphReader) Deadline() (time.Time, bool) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.deadline, reader.hasDeadline
}

type contextBlockingMembershipFactReader struct {
	mu      sync.Mutex
	fact    domain.MembershipTierFactSnapshot
	started chan<- struct{}
	err     error
	calls   int
}

func (reader *contextBlockingMembershipFactReader) FindMembershipTierFact(
	ctx context.Context,
	_ domain.MembershipSubjectRef,
) (domain.MembershipTierFactSnapshot, error) {
	reader.mu.Lock()
	reader.calls++
	reader.mu.Unlock()
	close(reader.started)
	<-ctx.Done()
	return reader.fact, reader.err
}

func (reader *contextBlockingMembershipFactReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

func (reader *contextBlockingStrategyRoutingGraphReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

type concurrentStrategyRoutingGraphReader struct {
	mu    sync.Mutex
	graph domain.StrategyRoutingGraph
	calls int
}

func (reader *concurrentStrategyRoutingGraphReader) FindByIdentity(
	context.Context,
	domain.StrategyRoutingGraphIdentity,
) (domain.StrategyRoutingGraph, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.calls++
	return reader.graph, nil
}

func (reader *concurrentStrategyRoutingGraphReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

type graphEvaluationCallResult struct {
	decision domain.StrategyRoutingGraphDecision
	err      error
}

func awaitBlockedGraphEvaluationResult(
	t *testing.T,
	started <-chan struct{},
	result <-chan graphEvaluationCallResult,
) graphEvaluationCallResult {
	t.Helper()
	watchdogCtx, cancelWatchdog := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelWatchdog()

	select {
	case <-started:
	case got := <-result:
		t.Fatalf("evaluation returned before blocking dependency started: %v", got.err)
	case <-watchdogCtx.Done():
		t.Fatalf("blocking dependency did not start before watchdog: %v", watchdogCtx.Err())
	}
	select {
	case got := <-result:
		return got
	case <-watchdogCtx.Done():
		t.Fatalf("evaluation did not return before watchdog: %v", watchdogCtx.Err())
	}
	return graphEvaluationCallResult{}
}

func graphEvaluationTestService(
	t *testing.T,
	graphs StrategyRoutingGraphReader,
	facts MembershipTierFactReader,
	clock MembershipRoutingClock,
	maxFactAge time.Duration,
	maxSteps int,
	maxDuration time.Duration,
) *StrategyRoutingGraphEvaluationService {
	t.Helper()
	service, err := NewStrategyRoutingGraphEvaluationService(
		graphs,
		facts,
		clock,
		maxFactAge,
		graphEvaluationTestBudget(t, maxSteps),
		maxDuration,
	)
	if err != nil {
		t.Fatalf("construct graph evaluation service: %v", err)
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	return service
}

func graphEvaluationTestBudget(t *testing.T, maxSteps int) domain.StrategyRoutingGraphStepBudget {
	t.Helper()
	budget, err := domain.NewStrategyRoutingGraphStepBudget(maxSteps)
	if err != nil {
		t.Fatalf("construct step budget: %v", err)
	}
	return budget
}

func graphEvaluationTestGraph(
	t *testing.T,
	id domain.StrategyRoutingGraphID,
	revision string,
	premiumTarget domain.StrategyID,
	baselineTarget domain.StrategyID,
) domain.StrategyRoutingGraph {
	t.Helper()
	root := graphEvaluationDecisionNode(t, 1)
	premium := graphEvaluationTargetNode(t, 2, premiumTarget)
	baseline := graphEvaluationTargetNode(t, 3, baselineTarget)
	return graphEvaluationGraph(
		t,
		id,
		revision,
		1,
		[]domain.StrategyRoutingNode{baseline, root, premium},
		[]domain.StrategyRoutingEdge{
			graphEvaluationEdge(t, 1, 3, domain.MembershipRoutingBranchBaselineDefault),
			graphEvaluationEdge(t, 1, 2, domain.MembershipRoutingBranchPremiumOverride),
		},
	)
}

func graphEvaluationTwoStepGraph(
	t *testing.T,
	id domain.StrategyRoutingGraphID,
	revision string,
	premiumTarget domain.StrategyID,
	intermediateBaselineTarget domain.StrategyID,
	rootBaselineTarget domain.StrategyID,
) domain.StrategyRoutingGraph {
	t.Helper()
	return graphEvaluationGraph(
		t,
		id,
		revision,
		1,
		[]domain.StrategyRoutingNode{
			graphEvaluationDecisionNode(t, 1),
			graphEvaluationDecisionNode(t, 2),
			graphEvaluationTargetNode(t, 3, premiumTarget),
			graphEvaluationTargetNode(t, 4, intermediateBaselineTarget),
			graphEvaluationTargetNode(t, 5, rootBaselineTarget),
		},
		[]domain.StrategyRoutingEdge{
			graphEvaluationEdge(t, 1, 2, domain.MembershipRoutingBranchPremiumOverride),
			graphEvaluationEdge(t, 1, 5, domain.MembershipRoutingBranchBaselineDefault),
			graphEvaluationEdge(t, 2, 3, domain.MembershipRoutingBranchPremiumOverride),
			graphEvaluationEdge(t, 2, 4, domain.MembershipRoutingBranchBaselineDefault),
		},
	)
}

func graphEvaluationDecisionNode(
	t *testing.T,
	id domain.StrategyRoutingNodeID,
) domain.StrategyRoutingNode {
	t.Helper()
	node, err := domain.NewStrategyRoutingDecisionNode(id, domain.MembershipStrategyRoutingRuleCode)
	if err != nil {
		t.Fatalf("construct decision node: %v", err)
	}
	return node
}

func graphEvaluationTargetNode(
	t *testing.T,
	id domain.StrategyRoutingNodeID,
	target domain.StrategyID,
) domain.StrategyRoutingNode {
	t.Helper()
	node, err := domain.NewStrategyRoutingTargetNode(id, target)
	if err != nil {
		t.Fatalf("construct target node: %v", err)
	}
	return node
}

func graphEvaluationEdge(
	t *testing.T,
	from domain.StrategyRoutingNodeID,
	to domain.StrategyRoutingNodeID,
	branch domain.MembershipRoutingBranch,
) domain.StrategyRoutingEdge {
	t.Helper()
	edge, err := domain.NewStrategyRoutingEdge(from, to, branch)
	if err != nil {
		t.Fatalf("construct graph edge: %v", err)
	}
	return edge
}

func graphEvaluationGraph(
	t *testing.T,
	id domain.StrategyRoutingGraphID,
	revision string,
	root domain.StrategyRoutingNodeID,
	nodes []domain.StrategyRoutingNode,
	edges []domain.StrategyRoutingEdge,
) domain.StrategyRoutingGraph {
	t.Helper()
	graph, err := domain.NewStrategyRoutingGraph(id, revision, root, nodes, edges)
	if err != nil {
		t.Fatalf("construct graph: %v", err)
	}
	return graph
}

func assertZeroStrategyRoutingGraphDecision(
	t *testing.T,
	decision domain.StrategyRoutingGraphDecision,
) {
	t.Helper()
	if decision.Confirmed() || decision.Identity() != (domain.StrategyRoutingGraphIdentity{}) ||
		decision.SchemaVersion() != 0 || decision.RootNodeID() != 0 ||
		decision.TerminalNodeID() != 0 || decision.Target() != 0 ||
		decision.FactSource() != "" || decision.FactRevision() != "" ||
		!decision.EvaluatedAt().IsZero() || len(decision.Path()) != 0 {
		t.Fatalf("expected zero graph decision, got %#v", decision)
	}
}
