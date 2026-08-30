package domain

import (
	"fmt"
	"time"
)

// RiskScreeningDisposition is a source-owned screening fact, not the final
// Participation eligibility decision. Only an explicit passed disposition can
// satisfy the v1 admission policy; missing or unknown source results are errors.
type RiskScreeningDisposition string

const (
	RiskScreeningDispositionPassed  RiskScreeningDisposition = "passed"
	RiskScreeningDispositionBlocked RiskScreeningDisposition = "blocked"
)

// RiskScreeningFactSnapshot is the immutable minimum fact supplied by a
// controlled risk authority. AssessedAt is source-owned: adapter retrieval time
// must never refresh an older disposition or hide that it has become stale.
type RiskScreeningFactSnapshot struct {
	participantRef ParticipantRef
	disposition    RiskScreeningDisposition
	assessedAt     time.Time
	source         FactSource
	revision       FactRevision
}

// NewRiskScreeningFactSnapshot constructs a canonical risk screening fact.
// It deliberately accepts no score, model feature, threshold, or Participation
// eligibility verdict.
func NewRiskScreeningFactSnapshot(
	participantRef ParticipantRef,
	disposition RiskScreeningDisposition,
	assessedAt time.Time,
	source string,
	revision string,
) (RiskScreeningFactSnapshot, error) {
	snapshot := RiskScreeningFactSnapshot{
		participantRef: participantRef,
		disposition:    disposition,
		assessedAt:     canonicalInstant(assessedAt),
		source:         FactSource(source),
		revision:       FactRevision(revision),
	}
	if err := snapshot.Validate(); err != nil {
		return RiskScreeningFactSnapshot{}, err
	}
	return snapshot, nil
}

// Validate rejects zero values, unsupported dispositions, missing source time,
// and unsafe provenance tokens at every adapter boundary.
func (snapshot RiskScreeningFactSnapshot) Validate() error {
	if snapshot.participantRef == 0 {
		return ErrParticipantRefRequired
	}
	switch snapshot.disposition {
	case RiskScreeningDispositionPassed, RiskScreeningDispositionBlocked:
	default:
		return fmt.Errorf(
			"%w: disposition is unsupported",
			ErrRiskScreeningFactInvalid,
		)
	}
	if snapshot.assessedAt.IsZero() {
		return fmt.Errorf("%w: assessed-at is required", ErrRiskScreeningFactInvalid)
	}
	if err := validateMetadataToken(string(snapshot.source), maxFactSourceBytes); err != nil {
		return fmt.Errorf("%w: source %v", ErrRiskScreeningFactInvalid, err)
	}
	if err := validateMetadataToken(string(snapshot.revision), maxFactRevisionBytes); err != nil {
		return fmt.Errorf("%w: revision %v", ErrRiskScreeningFactInvalid, err)
	}
	return nil
}

// ParticipantRef returns the opaque subject lookup reference in the snapshot.
func (snapshot RiskScreeningFactSnapshot) ParticipantRef() ParticipantRef {
	return snapshot.participantRef
}

// Disposition returns the source-owned passed or blocked screening fact.
func (snapshot RiskScreeningFactSnapshot) Disposition() RiskScreeningDisposition {
	return snapshot.disposition
}

// AssessedAt returns the canonical UTC instant at which the source formed the
// disposition. Application freshness must be measured from this instant.
func (snapshot RiskScreeningFactSnapshot) AssessedAt() time.Time {
	return snapshot.assessedAt
}

// Source returns the controlled risk authority identifier.
func (snapshot RiskScreeningFactSnapshot) Source() FactSource { return snapshot.source }

// Revision returns the risk source's snapshot revision.
func (snapshot RiskScreeningFactSnapshot) Revision() FactRevision {
	return snapshot.revision
}
