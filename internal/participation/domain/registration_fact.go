package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxFactSourceBytes   = 128
	maxFactRevisionBytes = 256
)

// ParticipantRef is an opaque, Participation-side lookup reference for an
// external subject. It is deliberately not named Principal or UserID: knowing
// this value does not prove caller identity, tenancy, role, or authorization.
type ParticipantRef uint64

// FactSource identifies the authority that supplied a registration snapshot.
// It is metadata for internal traceability, not a user-facing label.
type FactSource string

// FactRevision identifies the source snapshot used by one evaluation. Its
// lifecycle is owned by the fact provider, independently of policy revisions.
type FactRevision string

// RegistrationFactSnapshot is the immutable minimum account fact needed by
// the concrete new-user policy. It is a value observed for one evaluation, not
// a local User aggregate or a promise that GrowthOS persists the source fact.
type RegistrationFactSnapshot struct {
	participantRef ParticipantRef
	registeredAt   time.Time
	observedAt     time.Time
	source         FactSource
	revision       FactRevision
}

// NewRegistrationFactSnapshot constructs a canonical UTC snapshot. The source
// and revision are bounded diagnostic tokens; the constructor never accepts an
// is-new verdict because Participation owns that decision.
func NewRegistrationFactSnapshot(
	participantRef ParticipantRef,
	registeredAt time.Time,
	observedAt time.Time,
	source string,
	revision string,
) (RegistrationFactSnapshot, error) {
	snapshot := RegistrationFactSnapshot{
		participantRef: participantRef,
		registeredAt:   canonicalInstant(registeredAt),
		observedAt:     canonicalInstant(observedAt),
		source:         FactSource(source),
		revision:       FactRevision(revision),
	}
	if err := snapshot.Validate(); err != nil {
		return RegistrationFactSnapshot{}, err
	}
	return snapshot, nil
}

// Validate lets an application boundary fail closed when an adapter returns a
// zero or otherwise invalid domain value without using the constructor.
func (snapshot RegistrationFactSnapshot) Validate() error {
	if snapshot.participantRef == 0 {
		return ErrParticipantRefRequired
	}
	if snapshot.registeredAt.IsZero() {
		return fmt.Errorf("%w: registered-at is required", ErrRegistrationFactInvalid)
	}
	if snapshot.observedAt.IsZero() {
		return fmt.Errorf("%w: observed-at is required", ErrRegistrationFactInvalid)
	}
	if snapshot.registeredAt.After(snapshot.observedAt) {
		return fmt.Errorf("%w: registered-at is after observed-at", ErrRegistrationFactInvalid)
	}
	if err := validateMetadataToken(string(snapshot.source), maxFactSourceBytes); err != nil {
		return fmt.Errorf("%w: source %v", ErrRegistrationFactInvalid, err)
	}
	if err := validateMetadataToken(string(snapshot.revision), maxFactRevisionBytes); err != nil {
		return fmt.Errorf("%w: revision %v", ErrRegistrationFactInvalid, err)
	}
	return nil
}

// ParticipantRef returns the subject lookup reference carried by the source.
func (snapshot RegistrationFactSnapshot) ParticipantRef() ParticipantRef {
	return snapshot.participantRef
}

// RegisteredAt returns the canonical UTC account-registration instant.
func (snapshot RegistrationFactSnapshot) RegisteredAt() time.Time {
	return snapshot.registeredAt
}

// ObservedAt returns the canonical UTC instant at which the source captured or
// observed this snapshot. Application freshness is measured from this instant.
func (snapshot RegistrationFactSnapshot) ObservedAt() time.Time {
	return snapshot.observedAt
}

// Source returns the fact authority identifier.
func (snapshot RegistrationFactSnapshot) Source() FactSource { return snapshot.source }

// Revision returns the fact provider's snapshot revision.
func (snapshot RegistrationFactSnapshot) Revision() FactRevision { return snapshot.revision }

func canonicalInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0)
}

func validateMetadataToken(value string, maximumBytes int) error {
	if value == "" {
		return errors.New("value is required")
	}
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return errors.New("value is not canonical")
	}
	if len(value) > maximumBytes {
		return fmt.Errorf("exceeds %d bytes", maximumBytes)
	}
	for _, character := range value {
		if !unicode.IsPrint(character) {
			return errors.New("value contains a non-printing character")
		}
	}
	return nil
}
