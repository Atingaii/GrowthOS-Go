package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestEvaluateRiskAdmissionMapsOnlyConfirmedScreeningDispositions(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
	policy := riskAdmissionTestPolicy(t)
	tests := []struct {
		name        string
		disposition RiskScreeningDisposition
		outcome     EligibilityOutcome
		reason      ReasonCode
	}{
		{
			name:        "passed",
			disposition: RiskScreeningDispositionPassed,
			outcome:     EligibilityOutcomeEligible,
			reason:      ReasonRiskScreeningPassed,
		},
		{
			name:        "blocked",
			disposition: RiskScreeningDispositionBlocked,
			outcome:     EligibilityOutcomeIneligible,
			reason:      ReasonRiskScreeningBlocked,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact := riskAdmissionTestFact(t, test.disposition, evaluatedAt.Add(-time.Minute))
			decision, err := EvaluateRiskAdmission(policy, fact, evaluatedAt)
			if err != nil {
				t.Fatalf("EvaluateRiskAdmission() error = %v", err)
			}
			if decision.Outcome() != test.outcome || decision.ReasonCode() != test.reason {
				t.Fatalf(
					"decision = outcome %q reason %q",
					decision.Outcome(),
					decision.ReasonCode(),
				)
			}
			if decision.RuleCode() != RiskAdmissionRuleCode || decision.PolicyRevision() != policy.Revision() {
				t.Fatalf("rule/policy = %q/%q", decision.RuleCode(), decision.PolicyRevision())
			}
			if decision.FactSource() != fact.Source() || decision.FactRevision() != fact.Revision() {
				t.Fatalf("fact provenance = %q/%q", decision.FactSource(), decision.FactRevision())
			}
			if !decision.EvaluatedAt().Equal(evaluatedAt) || decision.EvaluatedAt().Location() != time.UTC {
				t.Fatalf("evaluated-at = %s", decision.EvaluatedAt())
			}
		})
	}
}

func TestEvaluateRiskAdmissionAcceptsAssessmentAtExactEvaluationBoundary(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
	decision, err := EvaluateRiskAdmission(
		riskAdmissionTestPolicy(t),
		riskAdmissionTestFact(t, RiskScreeningDispositionPassed, evaluatedAt),
		evaluatedAt,
	)
	if err != nil || decision.Outcome() != EligibilityOutcomeEligible {
		t.Fatalf("EvaluateRiskAdmission() = %#v, %v", decision, err)
	}
}

func TestEvaluateRiskAdmissionIsDeterministicAcrossTimeZones(t *testing.T) {
	evaluatedUTC := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
	evaluatedShanghai := evaluatedUTC.In(time.FixedZone("UTC+8", 8*60*60))
	assessedUTC := evaluatedUTC.Add(-time.Minute)
	assessedNewYork := assessedUTC.In(time.FixedZone("UTC-4", -4*60*60))
	policy := riskAdmissionTestPolicy(t)

	first, err := EvaluateRiskAdmission(
		policy,
		riskAdmissionTestFact(t, RiskScreeningDispositionPassed, assessedUTC),
		evaluatedUTC,
	)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	second, err := EvaluateRiskAdmission(
		policy,
		riskAdmissionTestFact(t, RiskScreeningDispositionPassed, assessedNewYork),
		evaluatedShanghai,
	)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same instants produced different decisions: %#v vs %#v", first, second)
	}
	repeated, err := EvaluateRiskAdmission(
		policy,
		riskAdmissionTestFact(t, RiskScreeningDispositionPassed, assessedUTC),
		evaluatedUTC,
	)
	if err != nil || !reflect.DeepEqual(first, repeated) {
		t.Fatalf("repeated evaluation = %#v, %v", repeated, err)
	}
}

func TestEvaluateRiskAdmissionRejectsInvalidOrFutureInputWithoutDecision(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
	policy := riskAdmissionTestPolicy(t)
	validFact := riskAdmissionTestFact(t, RiskScreeningDispositionPassed, evaluatedAt)
	futureFact := riskAdmissionTestFact(
		t,
		RiskScreeningDispositionPassed,
		evaluatedAt.Add(time.Nanosecond),
	)
	tests := []struct {
		name        string
		policy      RiskAdmissionPolicy
		fact        RiskScreeningFactSnapshot
		evaluatedAt time.Time
		want        error
	}{
		{name: "zero policy", fact: validFact, evaluatedAt: evaluatedAt, want: ErrRiskAdmissionEvaluationInvalid},
		{name: "zero fact", policy: policy, evaluatedAt: evaluatedAt, want: ErrRiskAdmissionEvaluationInvalid},
		{name: "zero evaluated-at", policy: policy, fact: validFact, want: ErrRiskAdmissionEvaluationInvalid},
		{name: "future assessed-at", policy: policy, fact: futureFact, evaluatedAt: evaluatedAt, want: ErrRiskScreeningFactFromFuture},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := EvaluateRiskAdmission(test.policy, test.fact, test.evaluatedAt)
			if !errors.Is(err, test.want) || decision != (RiskAdmissionDecision{}) {
				t.Fatalf("evaluation = %#v, %v; want zero and %v", decision, err, test.want)
			}
		})
	}
}

func FuzzEvaluateRiskAdmissionAssessedAtBoundary(f *testing.F) {
	f.Add(int64(-1), false)
	f.Add(int64(0), false)
	f.Add(int64(1), false)
	f.Add(int64(time.Hour), true)
	f.Fuzz(func(t *testing.T, rawOffset int64, blocked bool) {
		evaluatedAt := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
		offset := time.Duration(rawOffset % int64(12*time.Hour))
		disposition := RiskScreeningDispositionPassed
		wantOutcome := EligibilityOutcomeEligible
		if blocked {
			disposition = RiskScreeningDispositionBlocked
			wantOutcome = EligibilityOutcomeIneligible
		}
		fact := riskAdmissionTestFact(t, disposition, evaluatedAt.Add(offset))
		decision, err := EvaluateRiskAdmission(riskAdmissionTestPolicy(t), fact, evaluatedAt)
		if offset > 0 {
			if !errors.Is(err, ErrRiskScreeningFactFromFuture) || decision != (RiskAdmissionDecision{}) {
				t.Fatalf("future offset %s = %#v, %v", offset, decision, err)
			}
			return
		}
		if err != nil || decision.Outcome() != wantOutcome {
			t.Fatalf("offset %s = %#v, %v; want %q", offset, decision, err, wantOutcome)
		}
	})
}

func riskAdmissionTestPolicy(t *testing.T) RiskAdmissionPolicy {
	t.Helper()
	policy, err := NewRiskAdmissionPolicy("risk-admission-v1")
	if err != nil {
		t.Fatalf("construct policy: %v", err)
	}
	return policy
}

func riskAdmissionTestFact(
	t *testing.T,
	disposition RiskScreeningDisposition,
	assessedAt time.Time,
) RiskScreeningFactSnapshot {
	t.Helper()
	fact, err := NewRiskScreeningFactSnapshot(
		42,
		disposition,
		assessedAt,
		"risk-authority",
		"screening:9001",
	)
	if err != nil {
		t.Fatalf("construct fact: %v", err)
	}
	return fact
}
