package domain

import (
	"fmt"
	"time"
)

const (
	ReasonRiskScreeningPassed  ReasonCode = "risk_screening_passed"
	ReasonRiskScreeningBlocked ReasonCode = "risk_screening_blocked"
)

// RiskAdmissionDecision is a confirmed Participation result from the concrete
// risk prerequisite. It contains bounded provenance but no subject reference,
// risk feature, score, threshold, provider payload, or user-facing message.
type RiskAdmissionDecision struct {
	outcome        EligibilityOutcome
	ruleCode       RuleCode
	reasonCode     ReasonCode
	policyRevision PolicyRevision
	factSource     FactSource
	factRevision   FactRevision
	evaluatedAt    time.Time
}

// EvaluateRiskAdmission deterministically maps an authoritative screening fact
// at one controlled instant. Freshness remains an application/source contract;
// the domain rejects a source fact formed after the evaluation instant.
func EvaluateRiskAdmission(
	policy RiskAdmissionPolicy,
	fact RiskScreeningFactSnapshot,
	evaluatedAt time.Time,
) (RiskAdmissionDecision, error) {
	if err := policy.Validate(); err != nil {
		return RiskAdmissionDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRiskAdmissionEvaluationInvalid,
			err,
		)
	}
	if err := fact.Validate(); err != nil {
		return RiskAdmissionDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRiskAdmissionEvaluationInvalid,
			err,
		)
	}
	evaluatedAt = canonicalInstant(evaluatedAt)
	if evaluatedAt.IsZero() {
		return RiskAdmissionDecision{}, fmt.Errorf(
			"%w: evaluated-at is required",
			ErrRiskAdmissionEvaluationInvalid,
		)
	}
	if fact.AssessedAt().After(evaluatedAt) {
		return RiskAdmissionDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRiskAdmissionEvaluationInvalid,
			ErrRiskScreeningFactFromFuture,
		)
	}

	outcome := EligibilityOutcomeEligible
	reason := ReasonRiskScreeningPassed
	if fact.Disposition() == RiskScreeningDispositionBlocked {
		outcome = EligibilityOutcomeIneligible
		reason = ReasonRiskScreeningBlocked
	}
	return RiskAdmissionDecision{
		outcome:        outcome,
		ruleCode:       RiskAdmissionRuleCode,
		reasonCode:     reason,
		policyRevision: policy.Revision(),
		factSource:     fact.Source(),
		factRevision:   fact.Revision(),
		evaluatedAt:    evaluatedAt,
	}, nil
}

// Outcome returns the confirmed risk-admission business outcome.
func (decision RiskAdmissionDecision) Outcome() EligibilityOutcome {
	return decision.outcome
}

// RuleCode returns the stable concrete risk-admission rule identity.
func (decision RiskAdmissionDecision) RuleCode() RuleCode { return decision.ruleCode }

// ReasonCode returns the stable confirmed screening reason.
func (decision RiskAdmissionDecision) ReasonCode() ReasonCode {
	return decision.reasonCode
}

// PolicyRevision returns the exact policy snapshot revision.
func (decision RiskAdmissionDecision) PolicyRevision() PolicyRevision {
	return decision.policyRevision
}

// FactSource returns the controlled risk authority identifier.
func (decision RiskAdmissionDecision) FactSource() FactSource {
	return decision.factSource
}

// FactRevision returns the exact source snapshot revision.
func (decision RiskAdmissionDecision) FactRevision() FactRevision {
	return decision.factRevision
}

// EvaluatedAt returns the single canonical UTC evaluation instant.
func (decision RiskAdmissionDecision) EvaluatedAt() time.Time {
	return decision.evaluatedAt
}
