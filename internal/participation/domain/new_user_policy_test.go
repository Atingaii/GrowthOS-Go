package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewUserPolicyCanonicalizesInclusiveCutoff(t *testing.T) {
	cutoff := time.Date(2026, 8, 30, 16, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	policy, err := NewNewUserPolicy("new-user-2026-08-v1", cutoff)
	if err != nil {
		t.Fatalf("construct policy: %v", err)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if policy.Revision() != "new-user-2026-08-v1" {
		t.Fatalf("revision = %q", policy.Revision())
	}
	if policy.RegisteredAtOrAfter().Location() != time.UTC || !policy.RegisteredAtOrAfter().Equal(cutoff) {
		t.Fatal("policy cutoff was not canonically preserved")
	}
}

func TestNewUserPolicyRejectsInvalidValues(t *testing.T) {
	cutoff := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		revision string
		cutoff   time.Time
	}{
		{name: "empty revision", cutoff: cutoff},
		{name: "trimmed revision", revision: " v1", cutoff: cutoff},
		{name: "control revision", revision: "v1\n", cutoff: cutoff},
		{name: "format revision", revision: "v\u202E1", cutoff: cutoff},
		{name: "long revision", revision: strings.Repeat("r", maxPolicyRevisionBytes+1), cutoff: cutoff},
		{name: "zero cutoff", revision: "v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewNewUserPolicy(test.revision, test.cutoff)
			if !errors.Is(err, ErrNewUserPolicyInvalid) || policy != (NewUserPolicy{}) {
				t.Fatalf("constructor = %#v, %v; want zero invalid policy", policy, err)
			}
		})
	}
	if err := (NewUserPolicy{}).Validate(); !errors.Is(err, ErrNewUserPolicyInvalid) {
		t.Fatalf("zero policy Validate() error = %v", err)
	}
}
