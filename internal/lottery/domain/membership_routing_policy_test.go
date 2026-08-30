package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestMembershipStrategyRoutingPolicyPreservesDistinctVersionAndTargets(t *testing.T) {
	policy, err := NewMembershipStrategyRoutingPolicy("membership-route-v1", 200, 100)
	if err != nil {
		t.Fatalf("construct policy: %v", err)
	}
	if policy.Revision() != "membership-route-v1" ||
		policy.PremiumTarget() != 200 ||
		policy.BaselineDefault() != 100 {
		t.Fatalf("unexpected policy: %#v", policy)
	}
}

func TestMembershipStrategyRoutingPolicyAllowsConvergingTargets(t *testing.T) {
	policy, err := NewMembershipStrategyRoutingPolicy("rollout-v1", 100, 100)
	if err != nil {
		t.Fatalf("same target should remain valid: %v", err)
	}
	if policy.PremiumTarget() != policy.BaselineDefault() {
		t.Fatal("test fixture should prove branch convergence")
	}
}

func TestMembershipStrategyRoutingPolicyRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		revision string
		premium  StrategyID
		baseline StrategyID
	}{
		{name: "empty revision", premium: 2, baseline: 1},
		{name: "trimmed revision", revision: " route-v1", premium: 2, baseline: 1},
		{name: "control revision", revision: "route\nv1", premium: 2, baseline: 1},
		{name: "oversized revision", revision: strings.Repeat("r", maxMembershipRoutingPolicyRevisionBytes+1), premium: 2, baseline: 1},
		{name: "zero premium", revision: "route-v1", baseline: 1},
		{name: "zero baseline", revision: "route-v1", premium: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewMembershipStrategyRoutingPolicy(
				test.revision,
				test.premium,
				test.baseline,
			)
			if !errors.Is(err, ErrMembershipRoutingPolicyInvalid) {
				t.Fatalf("error = %v, want invalid policy", err)
			}
			if policy != (MembershipStrategyRoutingPolicy{}) {
				t.Fatalf("invalid construction returned non-zero policy: %#v", policy)
			}
		})
	}
	if !errors.Is((MembershipStrategyRoutingPolicy{}).Validate(), ErrMembershipRoutingPolicyInvalid) {
		t.Fatal("manual zero policy must fail validation")
	}
}
