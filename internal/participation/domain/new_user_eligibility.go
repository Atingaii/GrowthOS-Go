package domain

import (
	"fmt"
	"time"
)

// EligibilityOutcome is a confirmed Participation business result. Technical
// inability to decide is represented by an error and a zero decision instead.
type EligibilityOutcome string

const (
	EligibilityOutcomeEligible   EligibilityOutcome = "eligible"
	EligibilityOutcomeIneligible EligibilityOutcome = "ineligible"
)

// ReasonCode is a stable, low-cardinality explanation of a confirmed decision.
type ReasonCode string

const (
	ReasonRegistrationOnOrAfterCutoff ReasonCode = "registration_on_or_after_cutoff"
	ReasonRegistrationBeforeCutoff    ReasonCode = "registration_before_cutoff"
	// ReasonAllPrerequisitesSatisfied is the aggregate success reason emitted
	// only after every prerequisite in one ordered Participation plan passes.
	ReasonAllPrerequisitesSatisfied ReasonCode = "all_prerequisites_satisfied"
)

// NewUserEligibilityDecision is the immutable result of the one concrete rule.
// It omits registered-at, cutoff, full source payloads, and user-facing text.
type NewUserEligibilityDecision struct {
	outcome        EligibilityOutcome
	ruleCode       RuleCode
	reasonCode     ReasonCode
	policyRevision PolicyRevision
	factSource     FactSource
	factRevision   FactRevision
	evaluatedAt    time.Time
}

// EvaluateNewUserEligibility deterministically evaluates an already assembled
// policy and fact snapshot at one controlled server instant. Freshness belongs
// to the application/source contract because it is not intrinsic to the rule.
func EvaluateNewUserEligibility(
	policy NewUserPolicy,
	fact RegistrationFactSnapshot,
	evaluatedAt time.Time,
) (NewUserEligibilityDecision, error) {
	if err := policy.Validate(); err != nil {
		return NewUserEligibilityDecision{}, fmt.Errorf("%w: %w", ErrEligibilityEvaluationInvalid, err)
	}
	if err := fact.Validate(); err != nil {
		return NewUserEligibilityDecision{}, fmt.Errorf("%w: %w", ErrEligibilityEvaluationInvalid, err)
	}
	evaluatedAt = canonicalInstant(evaluatedAt)
	if evaluatedAt.IsZero() {
		return NewUserEligibilityDecision{}, fmt.Errorf(
			"%w: evaluated-at is required",
			ErrEligibilityEvaluationInvalid,
		)
	}
	if fact.RegisteredAt().After(evaluatedAt) || fact.ObservedAt().After(evaluatedAt) {
		return NewUserEligibilityDecision{}, fmt.Errorf(
			"%w: %w",
			ErrEligibilityEvaluationInvalid,
			ErrEligibilityFactFromFuture,
		)
	}

	outcome := EligibilityOutcomeEligible
	reason := ReasonRegistrationOnOrAfterCutoff
	if fact.RegisteredAt().Before(policy.RegisteredAtOrAfter()) {
		outcome = EligibilityOutcomeIneligible
		reason = ReasonRegistrationBeforeCutoff
	}
	return NewUserEligibilityDecision{
		outcome:        outcome,
		ruleCode:       NewUserRuleCode,
		reasonCode:     reason,
		policyRevision: policy.Revision(),
		factSource:     fact.Source(),
		factRevision:   fact.Revision(),
		evaluatedAt:    evaluatedAt,
	}, nil
}

// Outcome returns the confirmed business outcome.
func (decision NewUserEligibilityDecision) Outcome() EligibilityOutcome {
	return decision.outcome
}

// RuleCode returns the stable concrete rule identity.
func (decision NewUserEligibilityDecision) RuleCode() RuleCode { return decision.ruleCode }

// ReasonCode returns the stable business reason.
func (decision NewUserEligibilityDecision) ReasonCode() ReasonCode {
	return decision.reasonCode
}

// PolicyRevision returns the exact policy snapshot revision.
func (decision NewUserEligibilityDecision) PolicyRevision() PolicyRevision {
	return decision.policyRevision
}

// FactSource returns the authority identifier without exposing its payload.
func (decision NewUserEligibilityDecision) FactSource() FactSource {
	return decision.factSource
}

// FactRevision returns the fact provider's snapshot revision.
func (decision NewUserEligibilityDecision) FactRevision() FactRevision {
	return decision.factRevision
}

// EvaluatedAt returns the single canonical UTC evaluation instant.
func (decision NewUserEligibilityDecision) EvaluatedAt() time.Time {
	return decision.evaluatedAt
}
