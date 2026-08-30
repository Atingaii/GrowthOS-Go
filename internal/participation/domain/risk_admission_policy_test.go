package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestRiskAdmissionPolicyCarriesIndependentRevision(t *testing.T) {
	policy, err := NewRiskAdmissionPolicy("risk-admission-v1")
	if err != nil {
		t.Fatalf("construct risk admission policy: %v", err)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if policy.Revision() != "risk-admission-v1" {
		t.Fatalf("revision = %q", policy.Revision())
	}
	if RiskAdmissionRuleCode != "participation.risk.screening_admission" {
		t.Fatalf("risk admission rule code changed to %q", RiskAdmissionRuleCode)
	}
}

func TestRiskAdmissionPolicyRejectsInvalidRevision(t *testing.T) {
	for _, revision := range []string{
		"",
		" v1",
		"v1 ",
		"v1\n",
		"v\u202E1",
		strings.Repeat("r", maxPolicyRevisionBytes+1),
	} {
		policy, err := NewRiskAdmissionPolicy(revision)
		if !errors.Is(err, ErrRiskAdmissionPolicyInvalid) || policy != (RiskAdmissionPolicy{}) {
			t.Fatalf("constructor(%q) = %#v, %v", revision, policy, err)
		}
	}
	if err := (RiskAdmissionPolicy{}).Validate(); !errors.Is(err, ErrRiskAdmissionPolicyInvalid) {
		t.Fatalf("zero policy Validate() error = %v", err)
	}
}

func TestRuleSetRevisionIsIndependentAndBounded(t *testing.T) {
	revision, err := NewRuleSetRevision("new-user-then-risk-v1")
	if err != nil {
		t.Fatalf("construct rule-set revision: %v", err)
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if revision.String() != "new-user-then-risk-v1" {
		t.Fatalf("String() = %q", revision.String())
	}
	if ReasonAllPrerequisitesSatisfied != "all_prerequisites_satisfied" {
		t.Fatalf("aggregate success reason changed to %q", ReasonAllPrerequisitesSatisfied)
	}

	for _, invalid := range []string{
		"",
		" v1",
		"v1 ",
		"v1\n",
		"v\u202E1",
		strings.Repeat("r", maxPolicyRevisionBytes+1),
	} {
		got, err := NewRuleSetRevision(invalid)
		if !errors.Is(err, ErrRuleSetRevisionInvalid) || got != "" {
			t.Fatalf("NewRuleSetRevision(%q) = %q, %v", invalid, got, err)
		}
	}
	if err := (RuleSetRevision("")).Validate(); !errors.Is(err, ErrRuleSetRevisionInvalid) {
		t.Fatalf("zero rule-set revision Validate() error = %v", err)
	}
}
