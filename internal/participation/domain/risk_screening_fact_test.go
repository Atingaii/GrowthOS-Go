package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRiskScreeningFactSnapshotCanonicalizesSourceAssessment(t *testing.T) {
	assessedAt := time.Date(
		2026,
		time.August,
		30,
		16,
		30,
		0,
		0,
		time.FixedZone("UTC+8", 8*60*60),
	)
	snapshot, err := NewRiskScreeningFactSnapshot(
		42,
		RiskScreeningDispositionPassed,
		assessedAt,
		"risk-authority",
		"screening:9001",
	)
	if err != nil {
		t.Fatalf("construct risk screening fact: %v", err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if snapshot.ParticipantRef() != 42 || snapshot.Disposition() != RiskScreeningDispositionPassed {
		t.Fatalf(
			"snapshot subject/disposition = %d/%q",
			snapshot.ParticipantRef(),
			snapshot.Disposition(),
		)
	}
	if snapshot.AssessedAt().Location() != time.UTC || !snapshot.AssessedAt().Equal(assessedAt) {
		t.Fatalf("assessed-at = %s; want canonical instant %s", snapshot.AssessedAt(), assessedAt)
	}
	if snapshot.Source() != "risk-authority" || snapshot.Revision() != "screening:9001" {
		t.Fatalf("source/revision = %q/%q", snapshot.Source(), snapshot.Revision())
	}
}

func TestRiskScreeningFactSnapshotAcceptsOnlyConfirmedPassedOrBlocked(t *testing.T) {
	assessedAt := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
	for _, disposition := range []RiskScreeningDisposition{
		RiskScreeningDispositionPassed,
		RiskScreeningDispositionBlocked,
	} {
		t.Run(string(disposition), func(t *testing.T) {
			snapshot, err := NewRiskScreeningFactSnapshot(
				42,
				disposition,
				assessedAt,
				"risk-authority",
				"screening:9001",
			)
			if err != nil || snapshot.Disposition() != disposition {
				t.Fatalf("constructor = %#v, %v", snapshot, err)
			}
		})
	}
}

func TestRiskScreeningFactSnapshotRejectsInvalidValues(t *testing.T) {
	assessedAt := time.Date(2026, time.August, 30, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		ref         ParticipantRef
		disposition RiskScreeningDisposition
		assessedAt  time.Time
		source      string
		revision    string
		want        error
	}{
		{
			name:        "zero participant",
			disposition: RiskScreeningDispositionPassed,
			assessedAt:  assessedAt,
			source:      "risk-authority",
			revision:    "v1",
			want:        ErrParticipantRefRequired,
		},
		{name: "zero disposition", ref: 42, assessedAt: assessedAt, source: "risk-authority", revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "unknown disposition", ref: 42, disposition: "review", assessedAt: assessedAt, source: "risk-authority", revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "zero assessed-at", ref: 42, disposition: RiskScreeningDispositionPassed, source: "risk-authority", revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "empty source", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "trimmed source", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: " risk-authority", revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "control source", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: "risk\nauthority", revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "format source", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: "risk\u202Eauthority", revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "long source", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: strings.Repeat("s", maxFactSourceBytes+1), revision: "v1", want: ErrRiskScreeningFactInvalid},
		{name: "empty revision", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: "risk-authority", want: ErrRiskScreeningFactInvalid},
		{name: "trimmed revision", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: "risk-authority", revision: "v1 ", want: ErrRiskScreeningFactInvalid},
		{name: "control revision", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: "risk-authority", revision: "v1\n", want: ErrRiskScreeningFactInvalid},
		{name: "long revision", ref: 42, disposition: RiskScreeningDispositionPassed, assessedAt: assessedAt, source: "risk-authority", revision: strings.Repeat("r", maxFactRevisionBytes+1), want: ErrRiskScreeningFactInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := NewRiskScreeningFactSnapshot(
				test.ref,
				test.disposition,
				test.assessedAt,
				test.source,
				test.revision,
			)
			if !errors.Is(err, test.want) || snapshot != (RiskScreeningFactSnapshot{}) {
				t.Fatalf("constructor = %#v, %v; want zero and %v", snapshot, err, test.want)
			}
		})
	}
	if err := (RiskScreeningFactSnapshot{}).Validate(); !errors.Is(err, ErrParticipantRefRequired) {
		t.Fatalf("zero snapshot Validate() error = %v", err)
	}
}
