package domain

import (
	"errors"
	"testing"
	"time"
)

func TestRouteMembershipStrategyExposesBranchSpecificTerminalPaths(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	policy, err := NewMembershipStrategyRoutingPolicy("route-v1", 200, 100)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		tier       MembershipTier
		wantTarget StrategyID
		wantBranch MembershipRoutingBranch
		wantReason MembershipRoutingReasonCode
	}{
		{
			name:       "standard follows explicit baseline default",
			tier:       MembershipTierStandard,
			wantTarget: 100,
			wantBranch: MembershipRoutingBranchBaselineDefault,
			wantReason: MembershipRoutingReasonBaselineStrategy,
		},
		{
			name:       "premium follows explicit override",
			tier:       MembershipTierPremium,
			wantTarget: 200,
			wantBranch: MembershipRoutingBranchPremiumOverride,
			wantReason: MembershipRoutingReasonPremiumStrategy,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact, factErr := NewMembershipTierFactSnapshot(
				7,
				test.tier,
				evaluatedAt.Add(-time.Minute),
				"membership-directory",
				"fact-7",
			)
			if factErr != nil {
				t.Fatal(factErr)
			}
			decision, routeErr := RouteMembershipStrategy(policy, fact, evaluatedAt)
			if routeErr != nil {
				t.Fatalf("route: %v", routeErr)
			}
			if !decision.Confirmed() {
				t.Fatal("decision should be confirmed")
			}
			if decision.Target() != test.wantTarget ||
				decision.Branch() != test.wantBranch ||
				decision.ReasonCode() != test.wantReason {
				t.Fatalf("unexpected route: target=%d branch=%q reason=%q", decision.Target(), decision.Branch(), decision.ReasonCode())
			}
			if decision.RuleCode() != MembershipStrategyRoutingRuleCode ||
				decision.PolicyRevision() != "route-v1" ||
				decision.FactSource() != "membership-directory" ||
				decision.FactRevision() != "fact-7" ||
				!decision.EvaluatedAt().Equal(evaluatedAt) {
				t.Fatalf("unexpected decision evidence: %#v", decision)
			}
			path := decision.Path()
			if len(path) != 1 || path[0].RuleCode() != MembershipStrategyRoutingRuleCode ||
				path[0].Branch() != test.wantBranch || path[0].Target() != test.wantTarget {
				t.Fatalf("unexpected path: %#v", path)
			}
		})
	}
}

func TestMembershipRoutingStableCodesAreLiteralContracts(t *testing.T) {
	if MembershipStrategyRoutingRuleCode != "lottery.membership_tier.route_strategy" {
		t.Fatalf("rule code changed: %q", MembershipStrategyRoutingRuleCode)
	}
	if MembershipRoutingBranchPremiumOverride != "premium_override" {
		t.Fatalf("premium branch changed: %q", MembershipRoutingBranchPremiumOverride)
	}
	if MembershipRoutingBranchBaselineDefault != "baseline_default" {
		t.Fatalf("baseline branch changed: %q", MembershipRoutingBranchBaselineDefault)
	}
	if MembershipRoutingReasonPremiumStrategy != "premium_strategy_selected" ||
		MembershipRoutingReasonBaselineStrategy != "baseline_strategy_selected" {
		t.Fatalf(
			"reason contract changed: %q/%q",
			MembershipRoutingReasonPremiumStrategy,
			MembershipRoutingReasonBaselineStrategy,
		)
	}
}

func TestRouteMembershipStrategyKeepsBranchEvidenceWhenTargetsConverge(t *testing.T) {
	evaluatedAt := time.Unix(100, 0).UTC()
	policy, _ := NewMembershipStrategyRoutingPolicy("converged-v1", 100, 100)
	standard, _ := NewMembershipTierFactSnapshot(1, MembershipTierStandard, evaluatedAt, "directory", "standard-r1")
	premium, _ := NewMembershipTierFactSnapshot(2, MembershipTierPremium, evaluatedAt, "directory", "premium-r1")

	standardDecision, err := RouteMembershipStrategy(policy, standard, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	premiumDecision, err := RouteMembershipStrategy(policy, premium, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if standardDecision.Target() != premiumDecision.Target() {
		t.Fatal("fixture should converge on one Strategy")
	}
	if standardDecision.Branch() == premiumDecision.Branch() {
		t.Fatal("converged target must not erase the selected branch")
	}
}

func TestMembershipRoutePathIsDefensivelyCopied(t *testing.T) {
	evaluatedAt := time.Unix(200, 0).UTC()
	policy, _ := NewMembershipStrategyRoutingPolicy("route-v1", 200, 100)
	fact, _ := NewMembershipTierFactSnapshot(1, MembershipTierPremium, evaluatedAt, "directory", "r1")
	decision, err := RouteMembershipStrategy(policy, fact, evaluatedAt)
	if err != nil {
		t.Fatal(err)
	}
	path := decision.Path()
	path[0] = MembershipRoutingPathStep{}
	again := decision.Path()
	if len(again) != 1 || again[0].Target() != 200 || again[0].Branch() != MembershipRoutingBranchPremiumOverride {
		t.Fatalf("caller mutation changed decision path: %#v", again)
	}
}

func TestRouteMembershipStrategyRejectsInvalidOrFutureInputsWithZeroDecision(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	policy, _ := NewMembershipStrategyRoutingPolicy("route-v1", 200, 100)
	futureFact, _ := NewMembershipTierFactSnapshot(1, MembershipTierStandard, evaluatedAt.Add(time.Nanosecond), "directory", "r1")
	validFact, _ := NewMembershipTierFactSnapshot(1, MembershipTierStandard, evaluatedAt, "directory", "r1")
	tests := []struct {
		name   string
		policy MembershipStrategyRoutingPolicy
		fact   MembershipTierFactSnapshot
		at     time.Time
		want   error
	}{
		{name: "zero policy", fact: validFact, at: evaluatedAt, want: ErrMembershipRoutingEvaluationInvalid},
		{name: "zero fact", policy: policy, at: evaluatedAt, want: ErrMembershipRoutingEvaluationInvalid},
		{name: "zero instant", policy: policy, fact: validFact, want: ErrMembershipRoutingEvaluationInvalid},
		{name: "future fact", policy: policy, fact: futureFact, at: evaluatedAt, want: ErrMembershipTierFactFromFuture},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := RouteMembershipStrategy(test.policy, test.fact, test.at)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is %v", err, test.want)
			}
			if decision.Target() != 0 || decision.RuleCode() != "" ||
				decision.Branch() != "" || decision.ReasonCode() != "" ||
				decision.PolicyRevision() != "" || decision.FactSource() != "" ||
				decision.FactRevision() != "" || !decision.EvaluatedAt().IsZero() ||
				decision.Confirmed() || len(decision.Path()) != 0 {
				t.Fatalf("failure returned a decision: %#v", decision)
			}
		})
	}
}

func TestRouteMembershipStrategyIsUTCAndRepeatDeterministic(t *testing.T) {
	local := time.Date(2026, time.August, 30, 20, 0, 0, 9, time.FixedZone("UTC+8", 8*60*60))
	policy, _ := NewMembershipStrategyRoutingPolicy("route-v1", 200, 100)
	fact, _ := NewMembershipTierFactSnapshot(1, MembershipTierPremium, local.Add(-time.Second), "directory", "r1")
	first, err := RouteMembershipStrategy(policy, fact, local)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RouteMembershipStrategy(policy, fact, local.UTC())
	if err != nil {
		t.Fatal(err)
	}
	if first.Target() != second.Target() || first.Branch() != second.Branch() ||
		!first.EvaluatedAt().Equal(second.EvaluatedAt()) || first.EvaluatedAt().Location() != time.UTC {
		t.Fatalf("route changed across equivalent instants: %#v %#v", first, second)
	}
}
