package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewMembershipTierFactSnapshotCanonicalizesUTC(t *testing.T) {
	observedAt := time.Date(2026, time.August, 30, 18, 0, 0, 123, time.FixedZone("CST", 8*60*60))
	snapshot, err := NewMembershipTierFactSnapshot(
		19,
		MembershipTierPremium,
		observedAt,
		"membership-directory",
		"member-rev-19",
	)
	if err != nil {
		t.Fatalf("construct fact: %v", err)
	}
	if snapshot.SubjectRef() != 19 || snapshot.Tier() != MembershipTierPremium {
		t.Fatalf("unexpected subject/tier: %#v", snapshot)
	}
	if got, want := snapshot.ObservedAt(), observedAt.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("observed-at = %v (%v), want %v UTC", got, got.Location(), want)
	}
	if snapshot.Source() != "membership-directory" || snapshot.Revision() != "member-rev-19" {
		t.Fatalf("unexpected provenance: %q/%q", snapshot.Source(), snapshot.Revision())
	}
}

func TestMembershipTierFactSnapshotRejectsInvalidInputs(t *testing.T) {
	validTime := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		ref      MembershipSubjectRef
		tier     MembershipTier
		observed time.Time
		source   string
		revision string
		want     error
	}{
		{name: "zero ref", tier: MembershipTierStandard, observed: validTime, source: "directory", revision: "r1", want: ErrMembershipSubjectRefRequired},
		{name: "zero tier", ref: 1, observed: validTime, source: "directory", revision: "r1", want: ErrMembershipTierFactInvalid},
		{name: "unsupported tier", ref: 1, tier: "gold", observed: validTime, source: "directory", revision: "r1", want: ErrMembershipTierFactInvalid},
		{name: "zero observed", ref: 1, tier: MembershipTierStandard, source: "directory", revision: "r1", want: ErrMembershipTierFactInvalid},
		{name: "blank source", ref: 1, tier: MembershipTierStandard, observed: validTime, revision: "r1", want: ErrMembershipTierFactInvalid},
		{name: "trimmed source", ref: 1, tier: MembershipTierStandard, observed: validTime, source: " directory", revision: "r1", want: ErrMembershipTierFactInvalid},
		{name: "control revision", ref: 1, tier: MembershipTierStandard, observed: validTime, source: "directory", revision: "r\n1", want: ErrMembershipTierFactInvalid},
		{name: "oversized source", ref: 1, tier: MembershipTierStandard, observed: validTime, source: strings.Repeat("s", maxMembershipFactSourceBytes+1), revision: "r1", want: ErrMembershipTierFactInvalid},
		{name: "oversized revision", ref: 1, tier: MembershipTierStandard, observed: validTime, source: "directory", revision: strings.Repeat("r", maxMembershipFactRevisionBytes+1), want: ErrMembershipTierFactInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NewMembershipTierFactSnapshot(
				test.ref,
				test.tier,
				test.observed,
				test.source,
				test.revision,
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is %v", err, test.want)
			}
			if got != (MembershipTierFactSnapshot{}) {
				t.Fatalf("invalid construction returned non-zero fact: %#v", got)
			}
		})
	}
}

func TestMembershipTierFactSnapshotValidateRejectsManualZeroAndUnknown(t *testing.T) {
	if !errors.Is((MembershipTierFactSnapshot{}).Validate(), ErrMembershipSubjectRefRequired) {
		t.Fatal("zero snapshot must be invalid")
	}
	manual := MembershipTierFactSnapshot{
		subjectRef: 1,
		tier:       "vip",
		observedAt: time.Now().UTC(),
		source:     "directory",
		revision:   "r1",
	}
	if !errors.Is(manual.Validate(), ErrMembershipTierFactInvalid) {
		t.Fatal("unsupported manual tier must fail closed")
	}
}

func FuzzMembershipTierFactSnapshotNeverAcceptsUnsupportedTier(f *testing.F) {
	f.Add("gold")
	f.Add("premium")
	f.Fuzz(func(t *testing.T, tier string) {
		fact, err := NewMembershipTierFactSnapshot(
			1,
			MembershipTier(tier),
			time.Unix(1, 0).UTC(),
			"directory",
			"r1",
		)
		supported := tier == string(MembershipTierStandard) || tier == string(MembershipTierPremium)
		if supported && err != nil {
			t.Fatalf("supported tier %q rejected: %v", tier, err)
		}
		if !supported && err == nil {
			t.Fatalf("unsupported tier %q accepted as %#v", tier, fact)
		}
	})
}
