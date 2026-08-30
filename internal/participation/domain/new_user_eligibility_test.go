package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestEvaluateNewUserEligibilityUsesInclusiveRegistrationCutoff(t *testing.T) {
	cutoff := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	evaluatedAt := cutoff.Add(24 * time.Hour)
	policy := eligibilityTestPolicy(t, cutoff)
	tests := []struct {
		name       string
		registered time.Time
		outcome    EligibilityOutcome
		reason     ReasonCode
	}{
		{name: "one nanosecond before", registered: cutoff.Add(-time.Nanosecond), outcome: EligibilityOutcomeIneligible, reason: ReasonRegistrationBeforeCutoff},
		{name: "exact cutoff", registered: cutoff, outcome: EligibilityOutcomeEligible, reason: ReasonRegistrationOnOrAfterCutoff},
		{name: "one nanosecond after", registered: cutoff.Add(time.Nanosecond), outcome: EligibilityOutcomeEligible, reason: ReasonRegistrationOnOrAfterCutoff},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact := eligibilityTestFact(t, test.registered, cutoff.Add(time.Hour))
			decision, err := EvaluateNewUserEligibility(policy, fact, evaluatedAt)
			if err != nil {
				t.Fatalf("EvaluateNewUserEligibility() error = %v", err)
			}
			if decision.Outcome() != test.outcome || decision.ReasonCode() != test.reason {
				t.Fatalf("decision = outcome %q reason %q", decision.Outcome(), decision.ReasonCode())
			}
			if decision.RuleCode() != NewUserRuleCode || decision.PolicyRevision() != policy.Revision() {
				t.Fatalf("decision rule/policy = %q/%q", decision.RuleCode(), decision.PolicyRevision())
			}
			if decision.FactSource() != fact.Source() || decision.FactRevision() != fact.Revision() {
				t.Fatalf("decision fact metadata = %q/%q", decision.FactSource(), decision.FactRevision())
			}
			if !decision.EvaluatedAt().Equal(evaluatedAt) || decision.EvaluatedAt().Location() != time.UTC {
				t.Fatal("decision did not retain the canonical evaluation instant")
			}
		})
	}
}

func TestEvaluateNewUserEligibilityIsDeterministicAcrossTimeZones(t *testing.T) {
	cutoffUTC := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	cutoffShanghai := cutoffUTC.In(time.FixedZone("UTC+8", 8*60*60))
	evaluatedUTC := cutoffUTC.Add(time.Hour)
	evaluatedNewYork := evaluatedUTC.In(time.FixedZone("UTC-4", -4*60*60))

	policyUTC := eligibilityTestPolicy(t, cutoffUTC)
	policyShanghai := eligibilityTestPolicy(t, cutoffShanghai)
	factUTC := eligibilityTestFact(t, cutoffUTC, cutoffUTC.Add(time.Minute))
	factShanghai := eligibilityTestFact(t, cutoffShanghai, cutoffUTC.Add(time.Minute).In(cutoffShanghai.Location()))

	first, err := EvaluateNewUserEligibility(policyUTC, factUTC, evaluatedUTC)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	second, err := EvaluateNewUserEligibility(policyShanghai, factShanghai, evaluatedNewYork)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same instants produced different decisions: %#v vs %#v", first, second)
	}
	repeated, err := EvaluateNewUserEligibility(policyUTC, factUTC, evaluatedUTC)
	if err != nil || !reflect.DeepEqual(first, repeated) {
		t.Fatalf("repeated evaluation = %#v, %v; want identical", repeated, err)
	}
}

func TestEvaluateNewUserEligibilityRejectsInvalidOrFutureInputsWithoutDecision(t *testing.T) {
	base := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	policy := eligibilityTestPolicy(t, base)
	validFact := eligibilityTestFact(t, base, base.Add(time.Minute))
	futureRegistered := eligibilityTestFact(t, base.Add(2*time.Hour), base.Add(2*time.Hour))
	futureObserved := eligibilityTestFact(t, base, base.Add(2*time.Hour))

	tests := []struct {
		name      string
		policy    NewUserPolicy
		fact      RegistrationFactSnapshot
		evaluated time.Time
		want      error
	}{
		{name: "zero policy", fact: validFact, evaluated: base.Add(time.Hour), want: ErrEligibilityEvaluationInvalid},
		{name: "zero fact", policy: policy, evaluated: base.Add(time.Hour), want: ErrEligibilityEvaluationInvalid},
		{name: "zero evaluated-at", policy: policy, fact: validFact, want: ErrEligibilityEvaluationInvalid},
		{name: "future registered-at", policy: policy, fact: futureRegistered, evaluated: base.Add(time.Hour), want: ErrEligibilityFactFromFuture},
		{name: "future observed-at", policy: policy, fact: futureObserved, evaluated: base.Add(time.Hour), want: ErrEligibilityFactFromFuture},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := EvaluateNewUserEligibility(test.policy, test.fact, test.evaluated)
			if !errors.Is(err, test.want) || decision != (NewUserEligibilityDecision{}) {
				t.Fatalf("evaluation = %#v, %v; want zero and %v", decision, err, test.want)
			}
		})
	}
}

func FuzzEvaluateNewUserEligibilityCutoffBoundary(f *testing.F) {
	f.Add(int64(-1))
	f.Add(int64(0))
	f.Add(int64(1))
	f.Add(int64(time.Hour))
	f.Fuzz(func(t *testing.T, rawOffset int64) {
		cutoff := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
		offset := time.Duration(rawOffset % int64(12*time.Hour))
		registeredAt := cutoff.Add(offset)
		observedAt := cutoff.Add(12 * time.Hour)
		policy := eligibilityTestPolicy(t, cutoff)
		fact := eligibilityTestFact(t, registeredAt, observedAt)
		decision, err := EvaluateNewUserEligibility(policy, fact, observedAt)
		if err != nil {
			t.Fatalf("evaluation error = %v", err)
		}
		want := EligibilityOutcomeEligible
		if offset < 0 {
			want = EligibilityOutcomeIneligible
		}
		if decision.Outcome() != want {
			t.Fatalf("offset %s outcome = %q, want %q", offset, decision.Outcome(), want)
		}
	})
}

func eligibilityTestPolicy(t *testing.T, cutoff time.Time) NewUserPolicy {
	t.Helper()
	policy, err := NewNewUserPolicy("new-user-policy-v1", cutoff)
	if err != nil {
		t.Fatalf("construct policy: %v", err)
	}
	return policy
}

func eligibilityTestFact(t *testing.T, registeredAt, observedAt time.Time) RegistrationFactSnapshot {
	t.Helper()
	fact, err := NewRegistrationFactSnapshot(
		42,
		registeredAt,
		observedAt,
		"account-directory",
		"registration-event:9001",
	)
	if err != nil {
		t.Fatalf("construct fact: %v", err)
	}
	return fact
}
