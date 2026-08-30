package application

import (
	"context"
	"fmt"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

// EligibilityPrerequisiteChain is the fixed Participation v1 gate plan. It is
// deliberately not an open rule platform: registration is evaluated before
// risk screening, and only a confirmed eligible step continues the sequence.
type EligibilityPrerequisiteChain struct {
	registrationFacts      RegistrationFactReader
	riskScreeningFacts     RiskScreeningFactReader
	clock                  Clock
	maxRegistrationFactAge time.Duration
	maxRiskScreeningAge    time.Duration
}

// NewEligibilityPrerequisiteChain constructs the two-reader, one-clock plan.
// Readers remain lazy so a new-user rejection never touches the risk authority.
func NewEligibilityPrerequisiteChain(
	registrationFacts RegistrationFactReader,
	riskScreeningFacts RiskScreeningFactReader,
	clock Clock,
	maxRegistrationFactAge time.Duration,
	maxRiskScreeningAge time.Duration,
) (*EligibilityPrerequisiteChain, error) {
	chain := &EligibilityPrerequisiteChain{
		registrationFacts:      registrationFacts,
		riskScreeningFacts:     riskScreeningFacts,
		clock:                  clock,
		maxRegistrationFactAge: maxRegistrationFactAge,
		maxRiskScreeningAge:    maxRiskScreeningAge,
	}
	if err := chain.Validate(); err != nil {
		return nil, err
	}
	return chain, nil
}

// Validate rejects zero, partial, and typed-nil plans before any fact read.
func (chain *EligibilityPrerequisiteChain) Validate() error {
	if chain == nil ||
		dependencyIsNil(chain.registrationFacts) ||
		dependencyIsNil(chain.riskScreeningFacts) ||
		dependencyIsNil(chain.clock) ||
		chain.maxRegistrationFactAge <= 0 ||
		chain.maxRiskScreeningAge <= 0 {
		return ErrPrerequisiteChainNotConfigured
	}
	return nil
}

// Evaluate executes the fixed ordered plan at one logical server instant. A
// technical failure or cancellation always returns a zero aggregate; callers
// must never interpret a partially completed trace as a business decision.
func (chain *EligibilityPrerequisiteChain) Evaluate(
	ctx context.Context,
	participantRef domain.ParticipantRef,
	ruleSetRevision domain.RuleSetRevision,
	newUserPolicy domain.NewUserPolicy,
	riskAdmissionPolicy domain.RiskAdmissionPolicy,
) (PrerequisiteEvaluation, error) {
	if ctx == nil || participantRef == 0 {
		return PrerequisiteEvaluation{}, ErrPrerequisiteChainInvalidArgument
	}
	if err := ruleSetRevision.Validate(); err != nil {
		return PrerequisiteEvaluation{}, fmt.Errorf(
			"%w: %w",
			ErrPrerequisiteChainInvalidArgument,
			err,
		)
	}
	if err := newUserPolicy.Validate(); err != nil {
		return PrerequisiteEvaluation{}, fmt.Errorf(
			"%w: %w",
			ErrPrerequisiteChainInvalidArgument,
			err,
		)
	}
	if err := riskAdmissionPolicy.Validate(); err != nil {
		return PrerequisiteEvaluation{}, fmt.Errorf(
			"%w: %w",
			ErrPrerequisiteChainInvalidArgument,
			err,
		)
	}
	if err := chain.Validate(); err != nil {
		return PrerequisiteEvaluation{}, err
	}
	if err := ctx.Err(); err != nil {
		return PrerequisiteEvaluation{}, err
	}

	instant, err := captureEvaluationInstant(chain.clock)
	if contextError := ctx.Err(); contextError != nil {
		return PrerequisiteEvaluation{}, contextError
	}
	if err != nil {
		return PrerequisiteEvaluation{}, err
	}

	plan := []prerequisiteStep{
		{
			code: domain.NewUserRuleCode,
			evaluate: func(ctx context.Context) (EligibilityTraceStep, error) {
				fact, err := readRegistrationFact(ctx, chain.registrationFacts, participantRef)
				if err != nil {
					return EligibilityTraceStep{}, err
				}
				decision, err := evaluateRegistrationFactAt(
					ctx,
					participantRef,
					newUserPolicy,
					fact,
					instant,
					chain.maxRegistrationFactAge,
				)
				if err != nil {
					return EligibilityTraceStep{}, err
				}
				return traceStepFromNewUser(decision), nil
			},
		},
		{
			code: domain.RiskAdmissionRuleCode,
			evaluate: func(ctx context.Context) (EligibilityTraceStep, error) {
				fact, err := readRiskScreeningFact(ctx, chain.riskScreeningFacts, participantRef)
				if err != nil {
					return EligibilityTraceStep{}, err
				}
				decision, err := evaluateRiskScreeningFactAt(
					ctx,
					participantRef,
					riskAdmissionPolicy,
					fact,
					instant,
					chain.maxRiskScreeningAge,
				)
				if err != nil {
					return EligibilityTraceStep{}, err
				}
				return traceStepFromRiskAdmission(decision), nil
			},
		},
	}

	trace := make([]EligibilityTraceStep, 0, len(plan))
	for _, step := range plan {
		if err := ctx.Err(); err != nil {
			return PrerequisiteEvaluation{}, err
		}
		result, err := step.evaluate(ctx)
		if contextError := ctx.Err(); contextError != nil {
			return PrerequisiteEvaluation{}, contextError
		}
		if err != nil {
			return PrerequisiteEvaluation{}, err
		}
		if err := result.validate(step.code, instant); err != nil {
			return PrerequisiteEvaluation{}, err
		}
		trace = append(trace, result)
		switch result.Outcome() {
		case domain.EligibilityOutcomeEligible:
			continue
		case domain.EligibilityOutcomeIneligible:
			return newPrerequisiteEvaluation(
				domain.EligibilityOutcomeIneligible,
				result.ReasonCode(),
				ruleSetRevision,
				instant,
				trace,
			), nil
		default:
			return PrerequisiteEvaluation{}, ErrPrerequisiteStepInvalid
		}
	}

	return newPrerequisiteEvaluation(
		domain.EligibilityOutcomeEligible,
		domain.ReasonAllPrerequisitesSatisfied,
		ruleSetRevision,
		instant,
		trace,
	), nil
}

type prerequisiteStep struct {
	code     domain.RuleCode
	evaluate func(context.Context) (EligibilityTraceStep, error)
}

// EligibilityTraceStep is a bounded internal projection of one confirmed rule
// decision. Fact revision is diagnostic evidence, never a metric label or UI
// field; the type contains no participant reference or raw risk information.
type EligibilityTraceStep struct {
	ruleCode       domain.RuleCode
	outcome        domain.EligibilityOutcome
	reasonCode     domain.ReasonCode
	policyRevision domain.PolicyRevision
	factSource     domain.FactSource
	factRevision   domain.FactRevision
	evaluatedAt    time.Time
}

func traceStepFromNewUser(decision domain.NewUserEligibilityDecision) EligibilityTraceStep {
	return EligibilityTraceStep{
		ruleCode:       decision.RuleCode(),
		outcome:        decision.Outcome(),
		reasonCode:     decision.ReasonCode(),
		policyRevision: decision.PolicyRevision(),
		factSource:     decision.FactSource(),
		factRevision:   decision.FactRevision(),
		evaluatedAt:    decision.EvaluatedAt(),
	}
}

func traceStepFromRiskAdmission(decision domain.RiskAdmissionDecision) EligibilityTraceStep {
	return EligibilityTraceStep{
		ruleCode:       decision.RuleCode(),
		outcome:        decision.Outcome(),
		reasonCode:     decision.ReasonCode(),
		policyRevision: decision.PolicyRevision(),
		factSource:     decision.FactSource(),
		factRevision:   decision.FactRevision(),
		evaluatedAt:    decision.EvaluatedAt(),
	}
}

func (step EligibilityTraceStep) validate(
	expectedCode domain.RuleCode,
	instant evaluationInstant,
) error {
	if step.ruleCode != expectedCode ||
		(step.outcome != domain.EligibilityOutcomeEligible &&
			step.outcome != domain.EligibilityOutcomeIneligible) ||
		step.reasonCode == "" ||
		step.policyRevision == "" ||
		step.factSource == "" ||
		step.factRevision == "" ||
		!step.evaluatedAt.Equal(instant.time()) {
		return ErrPrerequisiteStepInvalid
	}
	return nil
}

// RuleCode returns the stable identity of the executed concrete rule.
func (step EligibilityTraceStep) RuleCode() domain.RuleCode { return step.ruleCode }

// Outcome returns the confirmed result of this step.
func (step EligibilityTraceStep) Outcome() domain.EligibilityOutcome { return step.outcome }

// ReasonCode returns the stable step-level explanation.
func (step EligibilityTraceStep) ReasonCode() domain.ReasonCode { return step.reasonCode }

// PolicyRevision returns the exact concrete policy revision.
func (step EligibilityTraceStep) PolicyRevision() domain.PolicyRevision {
	return step.policyRevision
}

// FactSource returns the controlled authority identifier.
func (step EligibilityTraceStep) FactSource() domain.FactSource { return step.factSource }

// FactRevision returns the controlled source snapshot revision.
func (step EligibilityTraceStep) FactRevision() domain.FactRevision { return step.factRevision }

// EvaluatedAt returns the chain-wide logical as-of instant.
func (step EligibilityTraceStep) EvaluatedAt() time.Time { return step.evaluatedAt }

// PrerequisiteEvaluation is a confirmed aggregate. Its zero value is not a
// business decision and is the only value returned alongside technical errors.
type PrerequisiteEvaluation struct {
	outcome         domain.EligibilityOutcome
	reasonCode      domain.ReasonCode
	ruleSetRevision domain.RuleSetRevision
	evaluatedAt     time.Time
	steps           []EligibilityTraceStep
}

// Confirmed distinguishes a complete business evaluation from the zero value
// returned with invalid input, dependency failure, or cancellation.
func (evaluation PrerequisiteEvaluation) Confirmed() bool {
	return (evaluation.outcome == domain.EligibilityOutcomeEligible ||
		evaluation.outcome == domain.EligibilityOutcomeIneligible) &&
		evaluation.reasonCode != "" &&
		evaluation.ruleSetRevision != "" &&
		!evaluation.evaluatedAt.IsZero() &&
		len(evaluation.steps) > 0
}

func newPrerequisiteEvaluation(
	outcome domain.EligibilityOutcome,
	reasonCode domain.ReasonCode,
	ruleSetRevision domain.RuleSetRevision,
	instant evaluationInstant,
	steps []EligibilityTraceStep,
) PrerequisiteEvaluation {
	return PrerequisiteEvaluation{
		outcome:         outcome,
		reasonCode:      reasonCode,
		ruleSetRevision: ruleSetRevision,
		evaluatedAt:     instant.time(),
		steps:           append([]EligibilityTraceStep(nil), steps...),
	}
}

// Outcome returns the confirmed aggregate eligibility outcome.
func (evaluation PrerequisiteEvaluation) Outcome() domain.EligibilityOutcome {
	return evaluation.outcome
}

// ReasonCode returns the terminal rejection reason or all-gates-passed reason.
func (evaluation PrerequisiteEvaluation) ReasonCode() domain.ReasonCode {
	return evaluation.reasonCode
}

// RuleSetRevision returns the version of the fixed ordered plan.
func (evaluation PrerequisiteEvaluation) RuleSetRevision() domain.RuleSetRevision {
	return evaluation.ruleSetRevision
}

// EvaluatedAt returns the single chain-wide logical as-of instant.
func (evaluation PrerequisiteEvaluation) EvaluatedAt() time.Time {
	return evaluation.evaluatedAt
}

// Steps returns a copy so callers cannot rewrite the stored execution evidence.
func (evaluation PrerequisiteEvaluation) Steps() []EligibilityTraceStep {
	return append([]EligibilityTraceStep(nil), evaluation.steps...)
}
