package domain

import (
	"context"
	"fmt"
	"time"
)

// StrategyRoutingGraphStepBudget bounds the number of decision edges one
// evaluation may traverse. V1 deliberately shares the graph depth ceiling.
type StrategyRoutingGraphStepBudget struct {
	maxSteps uint8
}

// NewStrategyRoutingGraphStepBudget constructs a server-controlled budget in
// the closed range 1..16.
func NewStrategyRoutingGraphStepBudget(maxSteps int) (StrategyRoutingGraphStepBudget, error) {
	if maxSteps < 1 || maxSteps > MaxStrategyRoutingGraphDepth {
		return StrategyRoutingGraphStepBudget{}, invalidStrategyRoutingGraphEvaluation(
			ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
			"max steps %d must be within 1..%d",
			maxSteps,
			MaxStrategyRoutingGraphDepth,
		)
	}
	return StrategyRoutingGraphStepBudget{maxSteps: uint8(maxSteps)}, nil
}

// Validate rejects a zero or manually forged out-of-range budget.
func (budget StrategyRoutingGraphStepBudget) Validate() error {
	if budget.maxSteps < 1 || int(budget.maxSteps) > MaxStrategyRoutingGraphDepth {
		return invalidStrategyRoutingGraphEvaluation(
			ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
			"max steps %d must be within 1..%d",
			budget.maxSteps,
			MaxStrategyRoutingGraphDepth,
		)
	}
	return nil
}

// MaxSteps returns the maximum number of decision edges that may be traversed.
func (budget StrategyRoutingGraphStepBudget) MaxSteps() int {
	return int(budget.maxSteps)
}

// StrategyRoutingGraphPathStep is immutable evidence for one exact graph edge
// selected by the concrete membership rule.
type StrategyRoutingGraphPathStep struct {
	fromNodeID StrategyRoutingNodeID
	rule       MembershipRoutingRuleCode
	branch     MembershipRoutingBranch
	reason     MembershipRoutingReasonCode
	toNodeID   StrategyRoutingNodeID
}

// FromNodeID returns the decision node at which this step began.
func (step StrategyRoutingGraphPathStep) FromNodeID() StrategyRoutingNodeID {
	return step.fromNodeID
}

// RuleCode returns the exact concrete rule evaluated at the source node.
func (step StrategyRoutingGraphPathStep) RuleCode() MembershipRoutingRuleCode {
	return step.rule
}

// Branch returns the exact outgoing branch selected by the rule.
func (step StrategyRoutingGraphPathStep) Branch() MembershipRoutingBranch {
	return step.branch
}

// ReasonCode returns the stable explanation paired with the selected branch.
func (step StrategyRoutingGraphPathStep) ReasonCode() MembershipRoutingReasonCode {
	return step.reason
}

// ToNodeID returns the graph node reached by this step.
func (step StrategyRoutingGraphPathStep) ToNodeID() StrategyRoutingNodeID {
	return step.toNodeID
}

// StrategyRoutingGraphDecision is one complete, immutable graph traversal
// result. It contains routing evidence only, not the subject, raw provider
// payload, a loaded Strategy, an Award selection, or a Draw.
type StrategyRoutingGraphDecision struct {
	identity       StrategyRoutingGraphIdentity
	schemaVersion  StrategyRoutingGraphSchemaVersion
	rootNodeID     StrategyRoutingNodeID
	terminalNodeID StrategyRoutingNodeID
	target         StrategyID
	terminalTarget StrategyID
	factSource     MembershipFactSource
	factRevision   MembershipFactRevision
	evaluatedAt    time.Time
	stepBudget     StrategyRoutingGraphStepBudget
	path           []StrategyRoutingGraphPathStep
}

// Confirmed reports whether the decision carries a complete, internally
// consistent v1 path. The evaluator additionally verifies every step and the
// terminal target against the supplied immutable graph before constructing it.
func (decision StrategyRoutingGraphDecision) Confirmed() bool {
	if err := decision.identity.Validate(); err != nil {
		return false
	}
	if decision.schemaVersion != StrategyRoutingGraphSchemaVersionV1 ||
		decision.rootNodeID == 0 ||
		decision.terminalNodeID == 0 ||
		decision.rootNodeID == decision.terminalNodeID ||
		decision.target == 0 ||
		decision.target != decision.terminalTarget {
		return false
	}
	if err := decision.stepBudget.Validate(); err != nil {
		return false
	}
	if err := validateMembershipMetadataToken(
		string(decision.factSource),
		maxMembershipFactSourceBytes,
	); err != nil {
		return false
	}
	if err := validateMembershipMetadataToken(
		string(decision.factRevision),
		maxMembershipFactRevisionBytes,
	); err != nil {
		return false
	}
	if decision.evaluatedAt.IsZero() ||
		decision.evaluatedAt != canonicalMembershipInstant(decision.evaluatedAt) {
		return false
	}
	if len(decision.path) < 1 || len(decision.path) > decision.stepBudget.MaxSteps() {
		return false
	}

	current := decision.rootNodeID
	visited := map[StrategyRoutingNodeID]struct{}{current: {}}
	for _, step := range decision.path {
		if step.fromNodeID != current || step.toNodeID == 0 ||
			step.rule != MembershipStrategyRoutingRuleCode ||
			!strategyRoutingGraphBranchReasonConfirmed(step.branch, step.reason) {
			return false
		}
		if _, repeated := visited[step.toNodeID]; repeated {
			return false
		}
		visited[step.toNodeID] = struct{}{}
		current = step.toNodeID
	}
	return current == decision.terminalNodeID
}

// Identity returns the exact graph ID/revision snapshot that was evaluated.
func (decision StrategyRoutingGraphDecision) Identity() StrategyRoutingGraphIdentity {
	return decision.identity
}

// SchemaVersion returns the closed graph schema version that was evaluated.
func (decision StrategyRoutingGraphDecision) SchemaVersion() StrategyRoutingGraphSchemaVersion {
	return decision.schemaVersion
}

// RootNodeID returns the entry decision node of the evaluated graph.
func (decision StrategyRoutingGraphDecision) RootNodeID() StrategyRoutingNodeID {
	return decision.rootNodeID
}

// TerminalNodeID returns the terminal graph node reached by the selected path.
func (decision StrategyRoutingGraphDecision) TerminalNodeID() StrategyRoutingNodeID {
	return decision.terminalNodeID
}

// Target returns the terminal Strategy identity without loading its aggregate.
func (decision StrategyRoutingGraphDecision) Target() StrategyID {
	return decision.target
}

// FactSource returns the authority that formed the evaluated fact snapshot.
func (decision StrategyRoutingGraphDecision) FactSource() MembershipFactSource {
	return decision.factSource
}

// FactRevision returns the exact evaluated provider snapshot revision.
func (decision StrategyRoutingGraphDecision) FactRevision() MembershipFactRevision {
	return decision.factRevision
}

// EvaluatedAt returns the canonical UTC logical evaluation instant.
func (decision StrategyRoutingGraphDecision) EvaluatedAt() time.Time {
	return decision.evaluatedAt
}

// Path returns a defensive copy of the ordered edge evidence.
func (decision StrategyRoutingGraphDecision) Path() []StrategyRoutingGraphPathStep {
	return append([]StrategyRoutingGraphPathStep(nil), decision.path...)
}

// EvaluateStrategyRoutingGraph deterministically follows one exact branch at
// each decision node. Every failure returns the zero decision; no prefix path,
// fallback target, or last successful node escapes.
func EvaluateStrategyRoutingGraph(
	ctx context.Context,
	graph StrategyRoutingGraph,
	fact MembershipTierFactSnapshot,
	evaluatedAt time.Time,
	budget StrategyRoutingGraphStepBudget,
) (StrategyRoutingGraphDecision, error) {
	if ctx == nil {
		return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
			nil,
			"context is required",
		)
	}
	if err := ctx.Err(); err != nil {
		return StrategyRoutingGraphDecision{}, err
	}
	if err := budget.Validate(); err != nil {
		return StrategyRoutingGraphDecision{}, err
	}
	if err := graph.Validate(); err != nil {
		return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
			err,
			"graph is not executable",
		)
	}
	if err := ctx.Err(); err != nil {
		return StrategyRoutingGraphDecision{}, err
	}
	if graph.Depth() > budget.MaxSteps() {
		return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
			ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
			"graph depth %d exceeds max steps %d",
			graph.Depth(),
			budget.MaxSteps(),
		)
	}
	if err := fact.Validate(); err != nil {
		return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
			err,
			"membership fact is not executable",
		)
	}
	if err := ctx.Err(); err != nil {
		return StrategyRoutingGraphDecision{}, err
	}
	evaluatedAt = canonicalMembershipInstant(evaluatedAt)
	if evaluatedAt.IsZero() {
		return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
			ErrMembershipRoutingEvaluationInvalid,
			"evaluated-at is required",
		)
	}
	if fact.ObservedAt().After(evaluatedAt) {
		return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
			ErrMembershipTierFactFromFuture,
			"membership fact was observed after evaluated-at",
		)
	}
	if err := ctx.Err(); err != nil {
		return StrategyRoutingGraphDecision{}, err
	}

	currentNodeID := graph.RootNodeID()
	visited := make(map[StrategyRoutingNodeID]struct{}, graph.Depth()+1)
	visited[currentNodeID] = struct{}{}
	path := make([]StrategyRoutingGraphPathStep, 0, graph.Depth())

	for {
		if err := ctx.Err(); err != nil {
			return StrategyRoutingGraphDecision{}, err
		}
		node, found := graph.Node(currentNodeID)
		if !found {
			return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
				ErrStrategyRoutingGraphDecisionInvalid,
				"current node %d is unavailable",
				currentNodeID,
			)
		}

		switch node.Kind() {
		case StrategyRoutingNodeKindStrategyTarget:
			if node.StrategyID() == 0 || len(path) == 0 {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphDecisionInvalid,
					"terminal node %d is incomplete",
					currentNodeID,
				)
			}
			decision := StrategyRoutingGraphDecision{
				identity:       graph.Identity(),
				schemaVersion:  graph.SchemaVersion(),
				rootNodeID:     graph.RootNodeID(),
				terminalNodeID: currentNodeID,
				target:         node.StrategyID(),
				terminalTarget: node.StrategyID(),
				factSource:     fact.Source(),
				factRevision:   fact.Revision(),
				evaluatedAt:    evaluatedAt,
				stepBudget:     budget,
				path:           append([]StrategyRoutingGraphPathStep(nil), path...),
			}
			if !decision.Confirmed() {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphDecisionInvalid,
					"completed traversal did not form a confirmed decision",
				)
			}
			if err := ctx.Err(); err != nil {
				return StrategyRoutingGraphDecision{}, err
			}
			return decision, nil

		case StrategyRoutingNodeKindDecision:
			if node.RuleCode() != MembershipStrategyRoutingRuleCode {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphOperatorUnsupported,
					"node %d rule %q is unsupported",
					node.ID(),
					node.RuleCode(),
				)
			}
			if len(path) >= budget.MaxSteps() {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
					"actual path reached max steps %d before a terminal",
					budget.MaxSteps(),
				)
			}

			branch, reason, err := evaluateMembershipRoutingBranch(fact, evaluatedAt)
			if err != nil {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					err,
					"node %d rule evaluation failed",
					node.ID(),
				)
			}
			if err := ctx.Err(); err != nil {
				return StrategyRoutingGraphDecision{}, err
			}

			var selected StrategyRoutingEdge
			matches := 0
			for _, edge := range graph.OutgoingEdges(currentNodeID) {
				if edge.Branch() == branch {
					selected = edge
					matches++
				}
			}
			if matches != 1 {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphBranchUnavailable,
					"node %d branch %q matched %d edges",
					currentNodeID,
					branch,
					matches,
				)
			}
			if err := selected.Validate(); err != nil ||
				selected.From() != currentNodeID || selected.Branch() != branch {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphBranchUnavailable,
					"node %d selected branch %q is inconsistent",
					currentNodeID,
					branch,
				)
			}
			if _, repeated := visited[selected.To()]; repeated {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphDecisionInvalid,
					"path revisits node %d",
					selected.To(),
				)
			}
			if _, exists := graph.Node(selected.To()); !exists {
				return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
					ErrStrategyRoutingGraphBranchUnavailable,
					"branch %q successor %d is unavailable",
					branch,
					selected.To(),
				)
			}

			path = append(path, StrategyRoutingGraphPathStep{
				fromNodeID: currentNodeID,
				rule:       node.RuleCode(),
				branch:     branch,
				reason:     reason,
				toNodeID:   selected.To(),
			})
			visited[selected.To()] = struct{}{}
			currentNodeID = selected.To()
			if err := ctx.Err(); err != nil {
				return StrategyRoutingGraphDecision{}, err
			}

		default:
			return StrategyRoutingGraphDecision{}, invalidStrategyRoutingGraphEvaluation(
				ErrStrategyRoutingGraphOperatorUnsupported,
				"node %d kind %q is unsupported",
				node.ID(),
				node.Kind(),
			)
		}
	}
}

func strategyRoutingGraphBranchReasonConfirmed(
	branch MembershipRoutingBranch,
	reason MembershipRoutingReasonCode,
) bool {
	switch branch {
	case MembershipRoutingBranchPremiumOverride:
		return reason == MembershipRoutingReasonPremiumStrategy
	case MembershipRoutingBranchBaselineDefault:
		return reason == MembershipRoutingReasonBaselineStrategy
	default:
		return false
	}
}

func invalidStrategyRoutingGraphEvaluation(
	cause error,
	format string,
	arguments ...any,
) error {
	detail := fmt.Sprintf(format, arguments...)
	if cause == nil {
		return fmt.Errorf("%w: %s", ErrStrategyRoutingGraphEvaluationInvalid, detail)
	}
	return fmt.Errorf(
		"%w: %w: %s",
		ErrStrategyRoutingGraphEvaluationInvalid,
		cause,
		detail,
	)
}
