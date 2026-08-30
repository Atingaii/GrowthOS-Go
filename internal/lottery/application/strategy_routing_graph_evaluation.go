package application

import (
	"context"
	"errors"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

var errStrategyRoutingGraphEvaluationInternalDeadline = errors.New(
	"lottery strategy routing graph evaluation: internal deadline",
)

// StrategyRoutingGraphEvaluationService reads one exact immutable graph and
// one authoritative membership fact, then delegates bounded traversal to the
// closed Lottery domain evaluator. It is intentionally not runtime-wired.
type StrategyRoutingGraphEvaluationService struct {
	graphs          StrategyRoutingGraphReader
	membershipFacts MembershipTierFactReader
	clock           MembershipRoutingClock
	maxFactAge      time.Duration
	stepBudget      domain.StrategyRoutingGraphStepBudget
	maxDuration     time.Duration
}

// NewStrategyRoutingGraphEvaluationService constructs the server-configured,
// read-only graph evaluation use case.
func NewStrategyRoutingGraphEvaluationService(
	graphs StrategyRoutingGraphReader,
	membershipFacts MembershipTierFactReader,
	clock MembershipRoutingClock,
	maxFactAge time.Duration,
	stepBudget domain.StrategyRoutingGraphStepBudget,
	maxDuration time.Duration,
) (*StrategyRoutingGraphEvaluationService, error) {
	service := &StrategyRoutingGraphEvaluationService{
		graphs:          graphs,
		membershipFacts: membershipFacts,
		clock:           clock,
		maxFactAge:      maxFactAge,
		stepBudget:      stepBudget,
		maxDuration:     maxDuration,
	}
	if err := service.Validate(); err != nil {
		return nil, err
	}
	return service, nil
}

// Validate rejects nil, typed-nil, zero, and partial configurations.
func (service *StrategyRoutingGraphEvaluationService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.graphs) ||
		dependencyIsNil(service.membershipFacts) ||
		dependencyIsNil(service.clock) ||
		service.maxFactAge <= 0 ||
		service.maxDuration <= 0 {
		return ErrStrategyRoutingGraphEvaluationNotConfigured
	}
	if err := service.stepBudget.Validate(); err != nil {
		return ErrStrategyRoutingGraphEvaluationNotConfigured
	}
	return nil
}

// Evaluate executes one exact graph revision. Every failure returns the zero
// decision; no prefix path or target is exposed before final confirmation.
func (service *StrategyRoutingGraphEvaluationService) Evaluate(
	callerCtx context.Context,
	subjectRef domain.MembershipSubjectRef,
	identity domain.StrategyRoutingGraphIdentity,
) (domain.StrategyRoutingGraphDecision, error) {
	if callerCtx == nil || subjectRef == 0 {
		return domain.StrategyRoutingGraphDecision{}, ErrStrategyRoutingGraphEvaluationInvalidArgument
	}
	if err := identity.Validate(); err != nil {
		return domain.StrategyRoutingGraphDecision{}, wrapStrategyRoutingGraphEvaluationError(
			ErrStrategyRoutingGraphEvaluationInvalidArgument,
			err,
		)
	}
	if err := service.Validate(); err != nil {
		return domain.StrategyRoutingGraphDecision{}, err
	}
	if err := callerCtx.Err(); err != nil {
		return domain.StrategyRoutingGraphDecision{}, err
	}

	evaluationCtx, cleanup := strategyRoutingGraphEvaluationContext(
		callerCtx,
		time.Now().Add(service.maxDuration),
	)
	defer cleanup()
	if err := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); err != nil {
		return domain.StrategyRoutingGraphDecision{}, err
	}

	graph, err := service.graphs.FindByIdentity(evaluationCtx, identity)
	if contextError := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); contextError != nil {
		return domain.StrategyRoutingGraphDecision{}, contextError
	}
	if err != nil {
		return domain.StrategyRoutingGraphDecision{}, classifyStrategyRoutingGraphEvaluationReadError(err)
	}
	if graph.Identity() != identity {
		return domain.StrategyRoutingGraphDecision{}, wrapStrategyRoutingGraphEvaluationError(
			ErrStrategyRoutingGraphEvaluationGraphInvalid,
			errors.New("graph reader returned a different exact identity"),
		)
	}
	if err := graph.Validate(); err != nil {
		return domain.StrategyRoutingGraphDecision{}, wrapStrategyRoutingGraphEvaluationError(
			ErrStrategyRoutingGraphEvaluationGraphInvalid,
			err,
		)
	}
	if graph.Depth() > service.stepBudget.MaxSteps() {
		return domain.StrategyRoutingGraphDecision{}, wrapStrategyRoutingGraphEvaluationError(
			domain.ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
			domain.ErrStrategyRoutingGraphEvaluationStepBudgetExceeded,
		)
	}
	if contextError := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); contextError != nil {
		return domain.StrategyRoutingGraphDecision{}, contextError
	}

	fact, evaluatedAt, err := readFreshMembershipTierFact(
		evaluationCtx,
		subjectRef,
		service.membershipFacts,
		service.clock,
		service.maxFactAge,
	)
	if contextError := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); contextError != nil {
		return domain.StrategyRoutingGraphDecision{}, contextError
	}
	if err != nil {
		return domain.StrategyRoutingGraphDecision{}, err
	}

	decision, err := domain.EvaluateStrategyRoutingGraph(
		evaluationCtx,
		graph,
		fact,
		evaluatedAt,
		service.stepBudget,
	)
	if contextError := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); contextError != nil {
		return domain.StrategyRoutingGraphDecision{}, contextError
	}
	if err != nil {
		return domain.StrategyRoutingGraphDecision{}, classifyStrategyRoutingGraphDomainEvaluationError(err)
	}
	if !decision.Confirmed() {
		return domain.StrategyRoutingGraphDecision{}, wrapStrategyRoutingGraphEvaluationError(
			ErrStrategyRoutingGraphEvaluationDecisionInvalid,
			domain.ErrStrategyRoutingGraphDecisionInvalid,
		)
	}
	if contextError := strategyRoutingGraphEvaluationContextError(callerCtx, evaluationCtx); contextError != nil {
		return domain.StrategyRoutingGraphDecision{}, contextError
	}
	return decision, nil
}

// strategyRoutingGraphEvaluationContext gives an earlier-or-equal caller
// deadline ownership. Only a strictly earlier internal deadline receives the
// private cause, avoiding equal-deadline timer races.
func strategyRoutingGraphEvaluationContext(
	callerCtx context.Context,
	internalDeadline time.Time,
) (context.Context, context.CancelFunc) {
	if callerDeadline, hasCallerDeadline := callerCtx.Deadline(); hasCallerDeadline &&
		!internalDeadline.Before(callerDeadline) {
		return context.WithCancel(callerCtx)
	}
	return context.WithDeadlineCause(
		callerCtx,
		internalDeadline,
		errStrategyRoutingGraphEvaluationInternalDeadline,
	)
}

func strategyRoutingGraphEvaluationContextError(
	callerCtx context.Context,
	evaluationCtx context.Context,
) error {
	if err := callerCtx.Err(); err != nil {
		return err
	}
	if evaluationCtx.Err() == nil {
		return nil
	}
	cause := context.Cause(evaluationCtx)
	if cause == errStrategyRoutingGraphEvaluationInternalDeadline {
		return wrapStrategyRoutingGraphEvaluationError(
			ErrStrategyRoutingGraphEvaluationTimedOut,
			cause,
		)
	}
	return wrapStrategyRoutingGraphEvaluationError(
		ErrStrategyRoutingGraphEvaluationFailure,
		cause,
	)
}

func classifyStrategyRoutingGraphEvaluationReadError(err error) error {
	class := ErrStrategyRoutingGraphEvaluationFailure
	switch {
	case errors.Is(err, ErrStrategyRoutingGraphNotFound):
		class = ErrStrategyRoutingGraphNotFound
	case errors.Is(err, ErrStoredStrategyRoutingGraphInvalid),
		errors.Is(err, domain.ErrStrategyRoutingGraphInvalid),
		errors.Is(err, domain.ErrStrategyRoutingGraphIdentityInvalid),
		errors.Is(err, domain.ErrStrategyRoutingGraphSchemaUnsupported):
		class = ErrStrategyRoutingGraphEvaluationGraphInvalid
	}
	return wrapStrategyRoutingGraphEvaluationError(class, err)
}

func classifyStrategyRoutingGraphDomainEvaluationError(err error) error {
	class := ErrStrategyRoutingGraphEvaluationFailure
	switch {
	case errors.Is(err, domain.ErrStrategyRoutingGraphEvaluationStepBudgetExceeded):
		class = domain.ErrStrategyRoutingGraphEvaluationStepBudgetExceeded
	case errors.Is(err, domain.ErrStrategyRoutingGraphDecisionInvalid):
		class = ErrStrategyRoutingGraphEvaluationDecisionInvalid
	case errors.Is(err, domain.ErrStrategyRoutingGraphEvaluationInvalid),
		errors.Is(err, domain.ErrStrategyRoutingGraphOperatorUnsupported),
		errors.Is(err, domain.ErrStrategyRoutingGraphBranchUnavailable):
		class = ErrStrategyRoutingGraphEvaluationGraphInvalid
	}
	return wrapStrategyRoutingGraphEvaluationError(class, err)
}
