package domain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStrategyRoutingGraphStepBudgetIsClosedToOneThroughSixteen(t *testing.T) {
	for _, maxSteps := range []int{1, MaxStrategyRoutingGraphDepth} {
		budget, err := NewStrategyRoutingGraphStepBudget(maxSteps)
		if err != nil {
			t.Fatalf("construct budget %d: %v", maxSteps, err)
		}
		if err := budget.Validate(); err != nil {
			t.Fatalf("validate budget %d: %v", maxSteps, err)
		}
		if budget.MaxSteps() != maxSteps {
			t.Fatalf("max steps = %d, want %d", budget.MaxSteps(), maxSteps)
		}
	}

	for _, maxSteps := range []int{-1, 0, MaxStrategyRoutingGraphDepth + 1, math.MaxInt} {
		budget, err := NewStrategyRoutingGraphStepBudget(maxSteps)
		if !errors.Is(err, ErrStrategyRoutingGraphEvaluationInvalid) ||
			!errors.Is(err, ErrStrategyRoutingGraphEvaluationStepBudgetExceeded) {
			t.Fatalf("budget %d error = %v, want invalid step budget", maxSteps, err)
		}
		if budget != (StrategyRoutingGraphStepBudget{}) {
			t.Fatalf("budget %d failure returned value: %#v", maxSteps, budget)
		}
	}

	for name, budget := range map[string]StrategyRoutingGraphStepBudget{
		"zero":           {},
		"forged maximum": {maxSteps: math.MaxUint8},
	} {
		if err := budget.Validate(); !errors.Is(err, ErrStrategyRoutingGraphEvaluationStepBudgetExceeded) {
			t.Fatalf("%s error = %v, want step budget exceeded", name, err)
		}
	}
}

func TestEvaluateStrategyRoutingGraphMatchesConcreteRouterAndIgnoresCanonicalEdgeOrder(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, 1)
	budget := mustEvaluationStepBudget(t, 1)
	evaluatedAt := evaluationInstant()
	policy, err := NewMembershipStrategyRoutingPolicy("oracle-v1", 202, 101)
	if err != nil {
		t.Fatalf("construct oracle policy: %v", err)
	}

	outgoing := graph.OutgoingEdges(graph.RootNodeID())
	if len(outgoing) != 2 || outgoing[0].Branch() != MembershipRoutingBranchBaselineDefault {
		t.Fatalf("fixture does not exercise canonical-order trap: %#v", outgoing)
	}

	for _, tier := range []MembershipTier{MembershipTierStandard, MembershipTierPremium} {
		t.Run(string(tier), func(t *testing.T) {
			fact := mustEvaluationMembershipFact(t, tier, evaluatedAt.Add(-time.Minute))
			oracle, err := RouteMembershipStrategy(policy, fact, evaluatedAt)
			if err != nil {
				t.Fatalf("concrete oracle: %v", err)
			}
			decision, err := EvaluateStrategyRoutingGraph(
				context.Background(),
				graph,
				fact,
				evaluatedAt,
				budget,
			)
			if err != nil {
				t.Fatalf("evaluate graph: %v", err)
			}
			if !decision.Confirmed() {
				t.Fatalf("decision is not confirmed: %#v", decision)
			}
			if decision.Target() != oracle.Target() ||
				decision.FactSource() != oracle.FactSource() ||
				decision.FactRevision() != oracle.FactRevision() ||
				decision.EvaluatedAt() != oracle.EvaluatedAt() {
				t.Fatalf("graph decision diverged from oracle: graph=%#v oracle=%#v", decision, oracle)
			}
			identity := decision.Identity()
			if identity.ID() != graph.ID() || identity.Revision() != graph.Revision() ||
				decision.SchemaVersion() != graph.SchemaVersion() ||
				decision.RootNodeID() != graph.RootNodeID() {
				t.Fatalf("decision lost graph snapshot evidence: %#v", decision)
			}
			path := decision.Path()
			if len(path) != 1 || path[0].FromNodeID() != graph.RootNodeID() ||
				path[0].RuleCode() != oracle.RuleCode() ||
				path[0].Branch() != oracle.Branch() ||
				path[0].ReasonCode() != oracle.ReasonCode() ||
				path[0].ToNodeID() != decision.TerminalNodeID() {
				t.Fatalf("unexpected graph path: %#v", path)
			}
		})
	}
}

func TestEvaluateStrategyRoutingGraphPreservesBranchEvidenceAtSharedSuccessor(t *testing.T) {
	decisionNode := mustEvaluationDecisionNode(t, 1)
	terminal := mustEvaluationTargetNode(t, 2, 808)
	graph, err := NewStrategyRoutingGraph(
		81,
		"converged-evaluation-v1",
		1,
		[]StrategyRoutingNode{terminal, decisionNode},
		[]StrategyRoutingEdge{
			mustEvaluationEdge(t, 1, 2, MembershipRoutingBranchPremiumOverride),
			mustEvaluationEdge(t, 1, 2, MembershipRoutingBranchBaselineDefault),
		},
	)
	if err != nil {
		t.Fatalf("construct converged graph: %v", err)
	}
	budget := mustEvaluationStepBudget(t, 1)
	evaluatedAt := evaluationInstant()

	decisions := make(map[MembershipTier]StrategyRoutingGraphDecision, 2)
	for _, tier := range []MembershipTier{MembershipTierStandard, MembershipTierPremium} {
		decision, err := EvaluateStrategyRoutingGraph(
			context.Background(),
			graph,
			mustEvaluationMembershipFact(t, tier, evaluatedAt.Add(-time.Minute)),
			evaluatedAt,
			budget,
		)
		if err != nil {
			t.Fatalf("evaluate %s: %v", tier, err)
		}
		decisions[tier] = decision
	}
	standard := decisions[MembershipTierStandard]
	premium := decisions[MembershipTierPremium]
	if standard.Target() != 808 || premium.Target() != 808 ||
		standard.TerminalNodeID() != premium.TerminalNodeID() {
		t.Fatalf("converged branches did not share target: standard=%#v premium=%#v", standard, premium)
	}
	if standard.Path()[0].Branch() != MembershipRoutingBranchBaselineDefault ||
		premium.Path()[0].Branch() != MembershipRoutingBranchPremiumOverride ||
		standard.Path()[0].ReasonCode() == premium.Path()[0].ReasonCode() {
		t.Fatalf("convergence erased branch evidence: standard=%#v premium=%#v", standard.Path(), premium.Path())
	}
}

func TestEvaluateStrategyRoutingGraphContinuesAfterBranchesConvergeOnDecision(t *testing.T) {
	graph, err := NewStrategyRoutingGraph(
		82,
		"shared-decision-evaluation-v1",
		1,
		[]StrategyRoutingNode{
			mustEvaluationDecisionNode(t, 1),
			mustEvaluationDecisionNode(t, 2),
			mustEvaluationTargetNode(t, 3, 909),
		},
		[]StrategyRoutingEdge{
			mustEvaluationEdge(t, 1, 2, MembershipRoutingBranchPremiumOverride),
			mustEvaluationEdge(t, 1, 2, MembershipRoutingBranchBaselineDefault),
			mustEvaluationEdge(t, 2, 3, MembershipRoutingBranchPremiumOverride),
			mustEvaluationEdge(t, 2, 3, MembershipRoutingBranchBaselineDefault),
		},
	)
	if err != nil {
		t.Fatalf("construct shared-decision graph: %v", err)
	}
	if graph.Depth() != 2 {
		t.Fatalf("shared-decision graph depth = %d, want 2", graph.Depth())
	}
	evaluatedAt := evaluationInstant()

	for _, tier := range []MembershipTier{MembershipTierStandard, MembershipTierPremium} {
		t.Run(string(tier), func(t *testing.T) {
			decision, err := EvaluateStrategyRoutingGraph(
				context.Background(),
				graph,
				mustEvaluationMembershipFact(t, tier, evaluatedAt.Add(-time.Minute)),
				evaluatedAt,
				mustEvaluationStepBudget(t, 2),
			)
			if err != nil {
				t.Fatalf("evaluate shared-decision graph: %v", err)
			}
			path := decision.Path()
			if !decision.Confirmed() || decision.Target() != 909 ||
				decision.TerminalNodeID() != 3 || len(path) != 2 {
				t.Fatalf("unexpected converged decision: %#v", decision)
			}
			if path[0].FromNodeID() != 1 || path[0].ToNodeID() != 2 ||
				path[1].FromNodeID() != 2 || path[1].ToNodeID() != 3 {
				t.Fatalf("shared successor broke path continuity: %#v", path)
			}
			wantBranch := MembershipRoutingBranchBaselineDefault
			wantReason := MembershipRoutingReasonBaselineStrategy
			if tier == MembershipTierPremium {
				wantBranch = MembershipRoutingBranchPremiumOverride
				wantReason = MembershipRoutingReasonPremiumStrategy
			}
			for index, step := range path {
				if step.RuleCode() != MembershipStrategyRoutingRuleCode ||
					step.Branch() != wantBranch || step.ReasonCode() != wantReason {
					t.Fatalf("step %d lost concrete branch evidence: %#v", index, step)
				}
			}
		})
	}
}

func TestEvaluateStrategyRoutingGraphAdmitsWorstDepthAndCompletesDepthSixteen(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, MaxStrategyRoutingGraphDepth)
	if graph.Depth() != MaxStrategyRoutingGraphDepth {
		t.Fatalf("graph depth = %d, want %d", graph.Depth(), MaxStrategyRoutingGraphDepth)
	}
	evaluatedAt := evaluationInstant()
	premiumFact := mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute))
	decision, err := EvaluateStrategyRoutingGraph(
		context.Background(),
		graph,
		premiumFact,
		evaluatedAt,
		mustEvaluationStepBudget(t, MaxStrategyRoutingGraphDepth),
	)
	if err != nil {
		t.Fatalf("evaluate depth-16 graph: %v", err)
	}
	if !decision.Confirmed() || len(decision.Path()) != MaxStrategyRoutingGraphDepth ||
		decision.Target() != 202 || decision.TerminalNodeID() != 1002 {
		t.Fatalf("unexpected depth-16 decision: %#v", decision)
	}
	for index, step := range decision.Path() {
		wantFrom := StrategyRoutingNodeID(index + 1)
		wantTo := StrategyRoutingNodeID(index + 2)
		if index == MaxStrategyRoutingGraphDepth-1 {
			wantTo = 1002
		}
		if step.FromNodeID() != wantFrom || step.ToNodeID() != wantTo ||
			step.RuleCode() != MembershipStrategyRoutingRuleCode ||
			step.Branch() != MembershipRoutingBranchPremiumOverride ||
			step.ReasonCode() != MembershipRoutingReasonPremiumStrategy {
			t.Fatalf("path step %d = %#v, want from=%d to=%d", index, step, wantFrom, wantTo)
		}
	}

	decision, err = EvaluateStrategyRoutingGraph(
		context.Background(),
		graph,
		MembershipTierFactSnapshot{},
		evaluatedAt,
		mustEvaluationStepBudget(t, MaxStrategyRoutingGraphDepth-1),
	)
	if !errors.Is(err, ErrStrategyRoutingGraphEvaluationStepBudgetExceeded) {
		t.Fatalf("worst-depth admission error = %v, want step budget exceeded", err)
	}
	if errors.Is(err, ErrMembershipSubjectRefRequired) {
		t.Fatalf("fact was evaluated before worst-depth admission: %v", err)
	}
	assertEvaluationZeroDecision(t, decision)
}

func TestEvaluateStrategyRoutingGraphSupportsMaximumUnsignedIdentities(t *testing.T) {
	maximum := uint64(math.MaxUint64)
	rootNodeID := StrategyRoutingNodeID(maximum)
	terminalNodeID := StrategyRoutingNodeID(maximum - 1)
	target := StrategyID(maximum)
	graph, err := NewStrategyRoutingGraph(
		StrategyRoutingGraphID(maximum),
		"max-uint-v1",
		rootNodeID,
		[]StrategyRoutingNode{
			mustEvaluationDecisionNode(t, rootNodeID),
			mustEvaluationTargetNode(t, terminalNodeID, target),
		},
		[]StrategyRoutingEdge{
			mustEvaluationEdge(t, rootNodeID, terminalNodeID, MembershipRoutingBranchPremiumOverride),
			mustEvaluationEdge(t, rootNodeID, terminalNodeID, MembershipRoutingBranchBaselineDefault),
		},
	)
	if err != nil {
		t.Fatalf("construct max-uint graph: %v", err)
	}
	evaluatedAt := evaluationInstant()
	decision, err := EvaluateStrategyRoutingGraph(
		context.Background(),
		graph,
		mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute)),
		evaluatedAt,
		mustEvaluationStepBudget(t, 1),
	)
	if err != nil {
		t.Fatalf("evaluate max-uint graph: %v", err)
	}
	if !decision.Confirmed() || decision.Identity().ID() != StrategyRoutingGraphID(maximum) ||
		decision.RootNodeID() != rootNodeID || decision.TerminalNodeID() != terminalNodeID ||
		decision.Target() != target {
		t.Fatalf("maximum identities were narrowed: %#v", decision)
	}
}

func TestEvaluateStrategyRoutingGraphRejectsInvalidInputsWithZeroDecision(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, 1)
	budget := mustEvaluationStepBudget(t, 1)
	evaluatedAt := evaluationInstant()
	fact := mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute))
	corruptGraph := graph
	corruptGraph.depth = 0
	futureFact := mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(time.Second))

	tests := []struct {
		name string
		call func() (StrategyRoutingGraphDecision, error)
		want error
	}{
		{
			name: "nil context",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(nil, graph, fact, evaluatedAt, budget)
			},
			want: ErrStrategyRoutingGraphEvaluationInvalid,
		},
		{
			name: "zero budget",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(context.Background(), graph, fact, evaluatedAt, StrategyRoutingGraphStepBudget{})
			},
			want: ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
		},
		{
			name: "zero graph",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(context.Background(), StrategyRoutingGraph{}, fact, evaluatedAt, budget)
			},
			want: ErrStrategyRoutingGraphInvalid,
		},
		{
			name: "corrupt derived graph depth",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(context.Background(), corruptGraph, fact, evaluatedAt, budget)
			},
			want: ErrStrategyRoutingGraphInvalid,
		},
		{
			name: "zero fact",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(context.Background(), graph, MembershipTierFactSnapshot{}, evaluatedAt, budget)
			},
			want: ErrMembershipSubjectRefRequired,
		},
		{
			name: "zero evaluated at",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(context.Background(), graph, fact, time.Time{}, budget)
			},
			want: ErrMembershipRoutingEvaluationInvalid,
		},
		{
			name: "future fact",
			call: func() (StrategyRoutingGraphDecision, error) {
				return EvaluateStrategyRoutingGraph(context.Background(), graph, futureFact, evaluatedAt, budget)
			},
			want: ErrMembershipTierFactFromFuture,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := test.call()
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if !errors.Is(err, ErrStrategyRoutingGraphEvaluationInvalid) {
				t.Fatalf("error = %v, want general evaluation classification", err)
			}
			assertEvaluationZeroDecision(t, decision)
		})
	}
}

func TestStrategyRoutingGraphDecisionRejectsForgeryAndDefensivelyCopiesPath(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, 2)
	evaluatedAt := evaluationInstant()
	decision, err := EvaluateStrategyRoutingGraph(
		context.Background(),
		graph,
		mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute)),
		evaluatedAt,
		mustEvaluationStepBudget(t, 2),
	)
	if err != nil {
		t.Fatalf("evaluate graph: %v", err)
	}
	if !decision.Confirmed() || len(decision.Path()) != 2 {
		t.Fatalf("fixture is not a confirmed two-step decision: %#v", decision)
	}

	path := decision.Path()
	path[0] = StrategyRoutingGraphPathStep{}
	if !decision.Confirmed() || decision.Path()[0].FromNodeID() != graph.RootNodeID() {
		t.Fatal("Path exposed mutable decision storage")
	}

	forgeries := []struct {
		name   string
		mutate func(*StrategyRoutingGraphDecision)
	}{
		{name: "zero identity", mutate: func(value *StrategyRoutingGraphDecision) { value.identity = StrategyRoutingGraphIdentity{} }},
		{name: "unknown schema", mutate: func(value *StrategyRoutingGraphDecision) { value.schemaVersion = 2 }},
		{name: "zero root", mutate: func(value *StrategyRoutingGraphDecision) { value.rootNodeID = 0 }},
		{name: "terminal equals root", mutate: func(value *StrategyRoutingGraphDecision) { value.terminalNodeID = value.rootNodeID }},
		{name: "zero target", mutate: func(value *StrategyRoutingGraphDecision) { value.target = 0 }},
		{name: "terminal target mismatch", mutate: func(value *StrategyRoutingGraphDecision) { value.target++ }},
		{name: "missing source", mutate: func(value *StrategyRoutingGraphDecision) { value.factSource = "" }},
		{name: "missing revision", mutate: func(value *StrategyRoutingGraphDecision) { value.factRevision = "" }},
		{name: "non canonical evaluated at", mutate: func(value *StrategyRoutingGraphDecision) {
			value.evaluatedAt = time.Date(2026, time.August, 30, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
		}},
		{name: "zero budget", mutate: func(value *StrategyRoutingGraphDecision) { value.stepBudget = StrategyRoutingGraphStepBudget{} }},
		{name: "path exceeds decision budget", mutate: func(value *StrategyRoutingGraphDecision) { value.stepBudget.maxSteps = 1 }},
		{name: "empty path", mutate: func(value *StrategyRoutingGraphDecision) { value.path = nil }},
		{name: "wrong first source", mutate: func(value *StrategyRoutingGraphDecision) { value.path[0].fromNodeID = 999 }},
		{name: "discontinuous path", mutate: func(value *StrategyRoutingGraphDecision) { value.path[1].fromNodeID = 999 }},
		{name: "unknown rule", mutate: func(value *StrategyRoutingGraphDecision) { value.path[0].rule = "lottery.unknown" }},
		{name: "branch reason mismatch", mutate: func(value *StrategyRoutingGraphDecision) {
			value.path[0].reason = MembershipRoutingReasonBaselineStrategy
		}},
		{name: "repeated node", mutate: func(value *StrategyRoutingGraphDecision) { value.path[1].toNodeID = value.rootNodeID }},
		{name: "wrong terminal", mutate: func(value *StrategyRoutingGraphDecision) { value.terminalNodeID = 999 }},
	}
	for _, forgery := range forgeries {
		t.Run(forgery.name, func(t *testing.T) {
			forged := decision
			forged.path = append([]StrategyRoutingGraphPathStep(nil), decision.path...)
			forgery.mutate(&forged)
			if forged.Confirmed() {
				t.Fatalf("forged decision was confirmed: %#v", forged)
			}
		})
	}
	if (StrategyRoutingGraphDecision{}).Confirmed() {
		t.Fatal("zero decision must not be confirmed")
	}
}

func TestEvaluateStrategyRoutingGraphChecksCancellationAtTraversalBoundaries(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, MaxStrategyRoutingGraphDepth)
	evaluatedAt := evaluationInstant()
	fact := mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute))
	budget := mustEvaluationStepBudget(t, MaxStrategyRoutingGraphDepth)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := EvaluateStrategyRoutingGraph(canceled, graph, fact, evaluatedAt, budget)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled error = %v", err)
	}
	assertEvaluationZeroDecision(t, decision)

	checkpointContext := &evaluationCancelAfterContext{
		Context:  context.Background(),
		cancelAt: 8,
	}
	decision, err = EvaluateStrategyRoutingGraph(checkpointContext, graph, fact, evaluatedAt, budget)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-traversal cancellation error = %v", err)
	}
	if checkpointContext.calls.Load() < checkpointContext.cancelAt {
		t.Fatalf("context was not checked enough times: %d", checkpointContext.calls.Load())
	}
	assertEvaluationZeroDecision(t, decision)
}

func TestEvaluateStrategyRoutingGraphChecksCancellationImmediatelyBeforeSuccess(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, 1)
	evaluatedAt := evaluationInstant()
	fact := mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute))
	budget := mustEvaluationStepBudget(t, 1)

	calibrationContext := &evaluationCountingContext{Context: context.Background()}
	calibrationDecision, err := EvaluateStrategyRoutingGraph(
		calibrationContext,
		graph,
		fact,
		evaluatedAt,
		budget,
	)
	if err != nil || !calibrationDecision.Confirmed() {
		t.Fatalf("calibrate successful checkpoints: decision=%#v error=%v", calibrationDecision, err)
	}
	// Depth one currently has eight fail-fast checks before the terminal
	// decision is complete and one final check immediately before success. Lock
	// that final checkpoint explicitly: deriving the cancellation point only
	// from the current implementation would still pass if the final check were
	// accidentally removed.
	const finalSuccessCheckpoint int32 = 9
	finalCheckpoint := calibrationContext.calls.Load()
	if finalCheckpoint != finalSuccessCheckpoint {
		t.Fatalf(
			"successful checkpoint count = %d, want final return checkpoint %d",
			finalCheckpoint,
			finalSuccessCheckpoint,
		)
	}

	finalCancelContext := &evaluationCancelAfterContext{
		Context:  context.Background(),
		cancelAt: finalCheckpoint,
	}
	decision, err := EvaluateStrategyRoutingGraph(
		finalCancelContext,
		graph,
		fact,
		evaluatedAt,
		budget,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("final checkpoint error = %v, want caller cancellation", err)
	}
	if finalCancelContext.calls.Load() != finalCheckpoint {
		t.Fatalf(
			"canceled at checkpoint %d, want calibrated final checkpoint %d",
			finalCancelContext.calls.Load(),
			finalCheckpoint,
		)
	}
	assertEvaluationZeroDecision(t, decision)
}

func TestEvaluateStrategyRoutingGraphIsConcurrentAndStateless(t *testing.T) {
	graph := mustEvaluationRoutingGraph(t, MaxStrategyRoutingGraphDepth)
	evaluatedAt := evaluationInstant()
	fact := mustEvaluationMembershipFact(t, MembershipTierPremium, evaluatedAt.Add(-time.Minute))
	budget := mustEvaluationStepBudget(t, MaxStrategyRoutingGraphDepth)

	const workers = 64
	errorsFromWorkers := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer group.Done()
			decision, err := EvaluateStrategyRoutingGraph(
				context.Background(),
				graph,
				fact,
				evaluatedAt,
				budget,
			)
			if err != nil {
				errorsFromWorkers <- err
				return
			}
			if !decision.Confirmed() || decision.Target() != 202 ||
				len(decision.Path()) != MaxStrategyRoutingGraphDepth {
				errorsFromWorkers <- fmt.Errorf("unexpected concurrent decision: %#v", decision)
			}
		}()
	}
	group.Wait()
	close(errorsFromWorkers)
	for err := range errorsFromWorkers {
		t.Error(err)
	}
}

type evaluationCancelAfterContext struct {
	context.Context
	calls    atomic.Int32
	cancelAt int32
}

type evaluationCountingContext struct {
	context.Context
	calls atomic.Int32
}

func (ctx *evaluationCountingContext) Err() error {
	ctx.calls.Add(1)
	return nil
}

func (ctx *evaluationCancelAfterContext) Err() error {
	if ctx.calls.Add(1) >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func evaluationInstant() time.Time {
	return time.Date(2026, time.August, 30, 12, 0, 0, 123456789, time.UTC)
}

func mustEvaluationStepBudget(t *testing.T, maxSteps int) StrategyRoutingGraphStepBudget {
	t.Helper()
	budget, err := NewStrategyRoutingGraphStepBudget(maxSteps)
	if err != nil {
		t.Fatalf("construct step budget: %v", err)
	}
	return budget
}

func mustEvaluationMembershipFact(
	t *testing.T,
	tier MembershipTier,
	observedAt time.Time,
) MembershipTierFactSnapshot {
	t.Helper()
	fact, err := NewMembershipTierFactSnapshot(
		9001,
		tier,
		observedAt,
		"membership-authority",
		"fact-revision-v1",
	)
	if err != nil {
		t.Fatalf("construct membership fact: %v", err)
	}
	return fact
}

func mustEvaluationRoutingGraph(t *testing.T, depth int) StrategyRoutingGraph {
	t.Helper()
	if depth < 1 || depth > MaxStrategyRoutingGraphDepth {
		t.Fatalf("unsupported test graph depth: %d", depth)
	}
	nodes := make([]StrategyRoutingNode, 0, depth+2)
	edges := make([]StrategyRoutingEdge, 0, depth*2)
	for step := 1; step <= depth; step++ {
		nodeID := StrategyRoutingNodeID(step)
		nodes = append(nodes, mustEvaluationDecisionNode(t, nodeID))
		edges = append(edges,
			mustEvaluationEdge(t, nodeID, 1001, MembershipRoutingBranchBaselineDefault),
		)
		premiumSuccessor := StrategyRoutingNodeID(step + 1)
		if step == depth {
			premiumSuccessor = 1002
		}
		edges = append(edges,
			mustEvaluationEdge(t, nodeID, premiumSuccessor, MembershipRoutingBranchPremiumOverride),
		)
	}
	nodes = append(nodes,
		mustEvaluationTargetNode(t, 1001, 101),
		mustEvaluationTargetNode(t, 1002, 202),
	)
	graph, err := NewStrategyRoutingGraph(
		70+StrategyRoutingGraphID(depth),
		fmt.Sprintf("evaluation-depth-%d-v1", depth),
		1,
		nodes,
		edges,
	)
	if err != nil {
		t.Fatalf("construct depth-%d graph: %v", depth, err)
	}
	return graph
}

func mustEvaluationDecisionNode(
	t *testing.T,
	id StrategyRoutingNodeID,
) StrategyRoutingNode {
	t.Helper()
	node, err := NewStrategyRoutingDecisionNode(id, MembershipStrategyRoutingRuleCode)
	if err != nil {
		t.Fatalf("construct decision node %d: %v", id, err)
	}
	return node
}

func mustEvaluationTargetNode(
	t *testing.T,
	id StrategyRoutingNodeID,
	target StrategyID,
) StrategyRoutingNode {
	t.Helper()
	node, err := NewStrategyRoutingTargetNode(id, target)
	if err != nil {
		t.Fatalf("construct target node %d: %v", id, err)
	}
	return node
}

func mustEvaluationEdge(
	t *testing.T,
	from StrategyRoutingNodeID,
	to StrategyRoutingNodeID,
	branch MembershipRoutingBranch,
) StrategyRoutingEdge {
	t.Helper()
	edge, err := NewStrategyRoutingEdge(from, to, branch)
	if err != nil {
		t.Fatalf("construct edge %d -> %d (%s): %v", from, to, branch, err)
	}
	return edge
}

func assertEvaluationZeroDecision(t *testing.T, decision StrategyRoutingGraphDecision) {
	t.Helper()
	if decision.identity != (StrategyRoutingGraphIdentity{}) ||
		decision.schemaVersion != 0 ||
		decision.rootNodeID != 0 ||
		decision.terminalNodeID != 0 ||
		decision.target != 0 ||
		decision.terminalTarget != 0 ||
		decision.factSource != "" ||
		decision.factRevision != "" ||
		!decision.evaluatedAt.IsZero() ||
		decision.stepBudget != (StrategyRoutingGraphStepBudget{}) ||
		decision.path != nil ||
		decision.Confirmed() {
		t.Fatalf("failure returned non-zero decision: %#v", decision)
	}
}
