package application

import (
	"errors"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

var (
	// ErrStrategyRoutingGraphEvaluationInvalidArgument reports an invalid
	// context, subject reference, or exact graph identity.
	ErrStrategyRoutingGraphEvaluationInvalidArgument = errors.New("lottery strategy routing graph evaluation: invalid argument")
	// ErrStrategyRoutingGraphEvaluationNotConfigured reports a missing port or
	// an invalid server-owned freshness, step, or duration budget.
	ErrStrategyRoutingGraphEvaluationNotConfigured = errors.New("lottery strategy routing graph evaluation: not configured")
	// ErrStrategyRoutingGraphEvaluationGraphInvalid reports that the exact graph
	// snapshot cannot be trusted or executed by the closed evaluator.
	ErrStrategyRoutingGraphEvaluationGraphInvalid = errors.New("lottery strategy routing graph evaluation: graph is invalid")
	// ErrStrategyRoutingGraphEvaluationTimedOut reports exhaustion of this
	// service's internal wall-clock budget while the caller remained live.
	ErrStrategyRoutingGraphEvaluationTimedOut = errors.New("lottery strategy routing graph evaluation: timed out")
	// ErrStrategyRoutingGraphEvaluationDecisionInvalid reports an incomplete or
	// internally inconsistent decision returned by the domain evaluator.
	ErrStrategyRoutingGraphEvaluationDecisionInvalid = errors.New("lottery strategy routing graph evaluation: decision is invalid")
	// ErrStrategyRoutingGraphEvaluationFailure is the fail-closed class for an
	// otherwise unclassified graph dependency or evaluator failure.
	ErrStrategyRoutingGraphEvaluationFailure = errors.New("lottery strategy routing graph evaluation: failed")
)

// StrategyRoutingGraphEvaluationError retains a trusted diagnostic cause while
// ordinary rendering and errors.Is expose exactly one reviewed, low-disclosure
// semantic class. It intentionally has no Unwrap method.
type StrategyRoutingGraphEvaluationError struct {
	class error
	cause error
}

func wrapStrategyRoutingGraphEvaluationError(
	class error,
	cause error,
) *StrategyRoutingGraphEvaluationError {
	if !knownStrategyRoutingGraphEvaluationErrorClass(class) {
		class = ErrStrategyRoutingGraphEvaluationFailure
	}
	return &StrategyRoutingGraphEvaluationError{class: class, cause: cause}
}

func (e *StrategyRoutingGraphEvaluationError) Error() string {
	if e == nil || !knownStrategyRoutingGraphEvaluationErrorClass(e.class) {
		return ErrStrategyRoutingGraphEvaluationFailure.Error()
	}
	return e.class.Error()
}

// Is exposes only the stable application/repository/budget class, never the
// private timeout, storage, topology, or evaluator cause.
func (e *StrategyRoutingGraphEvaluationError) Is(target error) bool {
	if e == nil || !knownStrategyRoutingGraphEvaluationErrorClass(e.class) {
		return target == ErrStrategyRoutingGraphEvaluationFailure
	}
	return target == e.class
}

// Cause exposes the retained diagnostic only to trusted code that explicitly
// opts into it. In particular, an internal timeout does not become
// errors.Is(err, context.DeadlineExceeded).
func (e *StrategyRoutingGraphEvaluationError) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownStrategyRoutingGraphEvaluationErrorClass(class error) bool {
	return class == ErrStrategyRoutingGraphEvaluationInvalidArgument ||
		class == ErrStrategyRoutingGraphEvaluationNotConfigured ||
		class == ErrStrategyRoutingGraphEvaluationGraphInvalid ||
		class == ErrStrategyRoutingGraphEvaluationTimedOut ||
		class == ErrStrategyRoutingGraphEvaluationDecisionInvalid ||
		class == ErrStrategyRoutingGraphEvaluationFailure ||
		class == ErrStrategyRoutingGraphNotFound ||
		class == domain.ErrStrategyRoutingGraphEvaluationStepBudgetExceeded
}
