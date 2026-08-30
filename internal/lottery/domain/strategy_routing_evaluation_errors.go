package domain

import "errors"

var (
	// ErrStrategyRoutingGraphEvaluationInvalid classifies every fail-closed
	// domain evaluation failure that cannot produce a complete decision.
	ErrStrategyRoutingGraphEvaluationInvalid = errors.New("lottery: strategy routing graph evaluation is invalid")
	// ErrStrategyRoutingGraphEvaluationStepBudgetExceeded reports a graph or
	// actual path that cannot be evaluated within the configured step budget.
	ErrStrategyRoutingGraphEvaluationStepBudgetExceeded = errors.New("lottery: strategy routing graph evaluation step budget exceeded")
	// ErrStrategyRoutingGraphOperatorUnsupported reports a decision node whose
	// concrete rule is not understood by the closed v1 evaluator.
	ErrStrategyRoutingGraphOperatorUnsupported = errors.New("lottery: strategy routing graph operator is unsupported")
	// ErrStrategyRoutingGraphBranchUnavailable reports that the exact branch
	// selected by the concrete rule does not identify exactly one outgoing edge.
	ErrStrategyRoutingGraphBranchUnavailable = errors.New("lottery: strategy routing graph branch is unavailable")
	// ErrStrategyRoutingGraphDecisionInvalid reports an incomplete or internally
	// inconsistent result. Such a result is never returned as partial success.
	ErrStrategyRoutingGraphDecisionInvalid = errors.New("lottery: strategy routing graph decision is invalid")
)
