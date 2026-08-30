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
	maxMembershipFactSourceBytes   = 128
	maxMembershipFactRevisionBytes = 256
)

// MembershipSubjectRef is a Lottery-side lookup reference for an external
// membership subject. Possessing it proves neither caller identity nor access.
type MembershipSubjectRef uint64

// MembershipTier is the bounded vocabulary accepted from the membership
// authority by the first Lottery routing policy. It is a fact, not a route.
type MembershipTier string

const (
	MembershipTierStandard MembershipTier = "standard"
	MembershipTierPremium  MembershipTier = "premium"
)

// MembershipFactSource identifies the authority that formed a tier snapshot.
// It is controlled provenance and must not become a user-facing or metric label.
type MembershipFactSource string

// MembershipFactRevision identifies one source-owned membership snapshot.
type MembershipFactRevision string

// MembershipTierFactSnapshot is the minimum immutable fact consumed by
// Lottery. The external authority owns tier lifecycle; it never supplies a
// StrategyID or a Lottery routing verdict.
type MembershipTierFactSnapshot struct {
	subjectRef MembershipSubjectRef
	tier       MembershipTier
	observedAt time.Time
	source     MembershipFactSource
	revision   MembershipFactRevision
}

// NewMembershipTierFactSnapshot constructs a canonical UTC fact snapshot.
func NewMembershipTierFactSnapshot(
	subjectRef MembershipSubjectRef,
	tier MembershipTier,
	observedAt time.Time,
	source string,
	revision string,
) (MembershipTierFactSnapshot, error) {
	snapshot := MembershipTierFactSnapshot{
		subjectRef: subjectRef,
		tier:       tier,
		observedAt: canonicalMembershipInstant(observedAt),
		source:     MembershipFactSource(source),
		revision:   MembershipFactRevision(revision),
	}
	if err := snapshot.Validate(); err != nil {
		return MembershipTierFactSnapshot{}, err
	}
	return snapshot, nil
}

// Validate rejects zero, unsupported, non-canonical, or unsafe source values.
// Unsupported does not mean baseline: only a confirmed standard fact may use
// the v1 default branch.
func (snapshot MembershipTierFactSnapshot) Validate() error {
	if snapshot.subjectRef == 0 {
		return ErrMembershipSubjectRefRequired
	}
	switch snapshot.tier {
	case MembershipTierStandard, MembershipTierPremium:
	default:
		return fmt.Errorf("%w: tier is unsupported", ErrMembershipTierFactInvalid)
	}
	if snapshot.observedAt.IsZero() {
		return fmt.Errorf("%w: observed-at is required", ErrMembershipTierFactInvalid)
	}
	if err := validateMembershipMetadataToken(
		string(snapshot.source),
		maxMembershipFactSourceBytes,
	); err != nil {
		return fmt.Errorf("%w: source %v", ErrMembershipTierFactInvalid, err)
	}
	if err := validateMembershipMetadataToken(
		string(snapshot.revision),
		maxMembershipFactRevisionBytes,
	); err != nil {
		return fmt.Errorf("%w: revision %v", ErrMembershipTierFactInvalid, err)
	}
	return nil
}

// SubjectRef returns the opaque Lottery-side membership lookup reference.
func (snapshot MembershipTierFactSnapshot) SubjectRef() MembershipSubjectRef {
	return snapshot.subjectRef
}

// Tier returns the source-owned membership tier.
func (snapshot MembershipTierFactSnapshot) Tier() MembershipTier { return snapshot.tier }

// ObservedAt returns when the source formed the snapshot, in canonical UTC.
func (snapshot MembershipTierFactSnapshot) ObservedAt() time.Time {
	return snapshot.observedAt
}

// Source returns the controlled authority identifier.
func (snapshot MembershipTierFactSnapshot) Source() MembershipFactSource {
	return snapshot.source
}

// Revision returns the source-owned snapshot revision.
func (snapshot MembershipTierFactSnapshot) Revision() MembershipFactRevision {
	return snapshot.revision
}

func canonicalMembershipInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0)
}

func validateMembershipMetadataToken(value string, maximumBytes int) error {
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
