package domain

import (
	"errors"
	"testing"
	"time"
)

func TestEvaluateMembershipRoutingBranchUsesClosedTierSemantics(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		tier       MembershipTier
		wantBranch MembershipRoutingBranch
		wantReason MembershipRoutingReasonCode
	}{
		{
			name:       "standard selects baseline default",
			tier:       MembershipTierStandard,
			wantBranch: MembershipRoutingBranchBaselineDefault,
			wantReason: MembershipRoutingReasonBaselineStrategy,
		},
		{
			name:       "premium selects explicit override",
			tier:       MembershipTierPremium,
			wantBranch: MembershipRoutingBranchPremiumOverride,
			wantReason: MembershipRoutingReasonPremiumStrategy,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact, err := NewMembershipTierFactSnapshot(
				7,
				test.tier,
				evaluatedAt.Add(-time.Minute),
				"membership-directory",
				"fact-r1",
			)
			if err != nil {
				t.Fatal(err)
			}

			branch, reason, err := evaluateMembershipRoutingBranch(fact, evaluatedAt)
			if err != nil {
				t.Fatalf("evaluate branch: %v", err)
			}
			if branch != test.wantBranch || reason != test.wantReason {
				t.Fatalf(
					"branch/reason = %q/%q, want %q/%q",
					branch,
					reason,
					test.wantBranch,
					test.wantReason,
				)
			}
		})
	}
}

func TestEvaluateMembershipRoutingBranchRejectsZeroAndFutureInputs(t *testing.T) {
	evaluatedAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	futureFact, err := NewMembershipTierFactSnapshot(
		7,
		MembershipTierPremium,
		evaluatedAt.Add(time.Nanosecond),
		"membership-directory",
		"fact-r1",
	)
	if err != nil {
		t.Fatal(err)
	}
	validFact, err := NewMembershipTierFactSnapshot(
		7,
		MembershipTierPremium,
		evaluatedAt,
		"membership-directory",
		"fact-r1",
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		fact MembershipTierFactSnapshot
		at   time.Time
		want error
	}{
		{
			name: "zero fact",
			at:   evaluatedAt,
			want: ErrMembershipRoutingEvaluationInvalid,
		},
		{
			name: "zero evaluated at",
			fact: validFact,
			want: ErrMembershipRoutingEvaluationInvalid,
		},
		{
			name: "fact from future",
			fact: futureFact,
			at:   evaluatedAt,
			want: ErrMembershipTierFactFromFuture,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			branch, reason, err := evaluateMembershipRoutingBranch(test.fact, test.at)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is %v", err, test.want)
			}
			if branch != "" || reason != "" {
				t.Fatalf("failure returned branch/reason %q/%q", branch, reason)
			}
		})
	}
}

func TestEvaluateMembershipRoutingBranchTreatsEquivalentUTCInstantsEqually(t *testing.T) {
	local := time.Date(
		2026,
		time.August,
		30,
		20,
		0,
		0,
		9,
		time.FixedZone("UTC+8", 8*60*60),
	)
	fact, err := NewMembershipTierFactSnapshot(
		7,
		MembershipTierPremium,
		local.Add(-time.Second),
		"membership-directory",
		"fact-r1",
	)
	if err != nil {
		t.Fatal(err)
	}

	localBranch, localReason, err := evaluateMembershipRoutingBranch(fact, local)
	if err != nil {
		t.Fatal(err)
	}
	utcBranch, utcReason, err := evaluateMembershipRoutingBranch(fact, local.UTC())
	if err != nil {
		t.Fatal(err)
	}
	if localBranch != utcBranch || localReason != utcReason {
		t.Fatalf(
			"equivalent instants changed branch/reason: %q/%q and %q/%q",
			localBranch,
			localReason,
			utcBranch,
			utcReason,
		)
	}
}
