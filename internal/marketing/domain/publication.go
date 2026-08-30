package domain

import (
	"fmt"
	"slices"
	"time"
)

// ActivityPublicationVersion is the monotonic numeric identity of one
// append-only Activity publication record.
type ActivityPublicationVersion uint64

// ActivityPublicationSchemaVersion identifies the closed persisted publication
// shape. It is independent from Activity state, publication, graph, and Strategy
// business revisions.
type ActivityPublicationSchemaVersion uint16

const (
	// ActivityPublicationSchemaVersionV1 is the only shape this domain slice can
	// construct or restore. Unknown versions fail closed.
	ActivityPublicationSchemaVersionV1 ActivityPublicationSchemaVersion = 1
)

// ActivityPublicationKind is the closed publication discriminant.
type ActivityPublicationKind string

const (
	ActivityPublicationKindRelease  ActivityPublicationKind = "release"
	ActivityPublicationKindRollback ActivityPublicationKind = "rollback"
)

// ActivityPublication is one immutable, append-only release snapshot. The
// Strategy manifest is canonicalized by ascending Strategy identity and owns
// exactly one revision per Strategy.
type ActivityPublication struct {
	activityID                ActivityID
	version                   ActivityPublicationVersion
	schemaVersion             ActivityPublicationSchemaVersion
	kind                      ActivityPublicationKind
	rollbackOf                ActivityPublicationVersion
	startsAt                  time.Time
	endsAt                    time.Time
	publishedAt               time.Time
	graphReference            LotteryGraphReference
	strategyRevisionManifest  []LotteryStrategyRevisionReference
	approvalEvidenceReference EvidenceReference
}

// RestoreActivityPublication reconstructs an immutable record. Collection
// order is canonicalized, while scalar values and persisted instants are never
// silently rewritten.
func RestoreActivityPublication(
	activityID ActivityID,
	version ActivityPublicationVersion,
	schemaVersion ActivityPublicationSchemaVersion,
	kind ActivityPublicationKind,
	rollbackOf ActivityPublicationVersion,
	startsAt time.Time,
	endsAt time.Time,
	publishedAt time.Time,
	graphReference LotteryGraphReference,
	strategyRevisionManifest []LotteryStrategyRevisionReference,
	approvalEvidenceReference EvidenceReference,
) (ActivityPublication, error) {
	canonicalManifest, err := canonicalStrategyRevisionManifest(strategyRevisionManifest)
	if err != nil {
		return ActivityPublication{}, fmt.Errorf("%w: %w", ErrActivityPublicationInvalid, err)
	}
	publication := ActivityPublication{
		activityID:                activityID,
		version:                   version,
		schemaVersion:             schemaVersion,
		kind:                      kind,
		rollbackOf:                rollbackOf,
		startsAt:                  startsAt,
		endsAt:                    endsAt,
		publishedAt:               publishedAt,
		graphReference:            graphReference,
		strategyRevisionManifest:  canonicalManifest,
		approvalEvidenceReference: approvalEvidenceReference,
	}
	if err := publication.Validate(); err != nil {
		return ActivityPublication{}, err
	}
	return publication, nil
}

func newActivityPublication(
	activityID ActivityID,
	version ActivityPublicationVersion,
	kind ActivityPublicationKind,
	rollbackOf ActivityPublicationVersion,
	startsAt time.Time,
	endsAt time.Time,
	publishedAt time.Time,
	graphReference LotteryGraphReference,
	strategyRevisionManifest []LotteryStrategyRevisionReference,
	approvalEvidenceReference EvidenceReference,
) (ActivityPublication, error) {
	return RestoreActivityPublication(
		activityID,
		version,
		ActivityPublicationSchemaVersionV1,
		kind,
		rollbackOf,
		canonicalUTCInstant(startsAt),
		canonicalUTCInstant(endsAt),
		canonicalUTCInstant(publishedAt),
		graphReference,
		strategyRevisionManifest,
		approvalEvidenceReference,
	)
}

// Validate rechecks all immutable state so adapters cannot inject a forged zero
// or mixed record. It does not prove that the foreign Lottery snapshots exist.
func (publication ActivityPublication) Validate() error {
	if publication.activityID == 0 {
		return invalidActivityPublication(ErrActivityIDInvalid, "activity id is required")
	}
	if publication.version == 0 {
		return invalidActivityPublication(ErrActivityPublicationInvalid, "version is required")
	}
	if publication.schemaVersion != ActivityPublicationSchemaVersionV1 {
		return invalidActivityPublication(
			ErrActivityPublicationSchemaUnsupported,
			"schema version %d",
			publication.schemaVersion,
		)
	}
	switch publication.kind {
	case ActivityPublicationKindRelease:
		if publication.rollbackOf != 0 {
			return invalidActivityPublication(
				ErrActivityPublicationInvalid,
				"release cannot carry rollback-of version",
			)
		}
	case ActivityPublicationKindRollback:
		if publication.rollbackOf == 0 || publication.rollbackOf >= publication.version {
			return invalidActivityPublication(
				ErrActivityPublicationInvalid,
				"rollback-of version must identify an older publication",
			)
		}
	default:
		return invalidActivityPublication(
			ErrActivityPublicationKindUnsupported,
			"kind %q",
			publication.kind,
		)
	}
	if err := validateCanonicalUTCInstant(publication.startsAt); err != nil {
		return invalidActivityPublication(ErrActivityPublicationWindowInvalid, "starts-at %v", err)
	}
	if err := validateCanonicalUTCInstant(publication.endsAt); err != nil {
		return invalidActivityPublication(ErrActivityPublicationWindowInvalid, "ends-at %v", err)
	}
	if !publication.startsAt.Before(publication.endsAt) {
		return invalidActivityPublication(
			ErrActivityPublicationWindowInvalid,
			"starts-at must be before exclusive ends-at",
		)
	}
	if err := validateCanonicalUTCInstant(publication.publishedAt); err != nil {
		return invalidActivityPublication(ErrActivityPublicationWindowInvalid, "published-at %v", err)
	}
	if !publication.publishedAt.Before(publication.endsAt) {
		return invalidActivityPublication(
			ErrActivityPublicationExpired,
			"published-at must be before exclusive ends-at",
		)
	}
	if err := publication.graphReference.Validate(); err != nil {
		return invalidActivityPublication(ErrLotteryGraphReferenceInvalid, "graph reference %v", err)
	}
	if err := validateCanonicalStrategyRevisionManifest(publication.strategyRevisionManifest); err != nil {
		return invalidActivityPublication(ErrStrategyRevisionManifestInvalid, "manifest %v", err)
	}
	if err := publication.approvalEvidenceReference.Validate(); err != nil {
		return invalidActivityPublication(ErrEvidenceReferenceInvalid, "approval evidence %v", err)
	}
	return nil
}

// ActivityID returns the owning Activity identity.
func (publication ActivityPublication) ActivityID() ActivityID { return publication.activityID }

// Version returns the immutable numeric publication identity.
func (publication ActivityPublication) Version() ActivityPublicationVersion {
	return publication.version
}

// SchemaVersion returns the closed persisted shape marker.
func (publication ActivityPublication) SchemaVersion() ActivityPublicationSchemaVersion {
	return publication.schemaVersion
}

// Kind returns release or rollback.
func (publication ActivityPublication) Kind() ActivityPublicationKind { return publication.kind }

// RollbackOf returns the exact historical target and true for rollback records.
func (publication ActivityPublication) RollbackOf() (ActivityPublicationVersion, bool) {
	return publication.rollbackOf, publication.kind == ActivityPublicationKindRollback
}

// StartsAt returns the inclusive UTC-microsecond window boundary.
func (publication ActivityPublication) StartsAt() time.Time { return publication.startsAt }

// EndsAt returns the exclusive UTC-microsecond window boundary.
func (publication ActivityPublication) EndsAt() time.Time { return publication.endsAt }

// PublishedAt returns the one controlled Clock instant captured for this
// release or rollback. Rollback records receive a new instant.
func (publication ActivityPublication) PublishedAt() time.Time { return publication.publishedAt }

// GraphReference returns the exact Lottery graph snapshot identity.
func (publication ActivityPublication) GraphReference() LotteryGraphReference {
	return publication.graphReference
}

// StrategyRevisionManifest returns a defensive copy in ascending Strategy-ID
// order. Mutation of the returned slice cannot alter the publication.
func (publication ActivityPublication) StrategyRevisionManifest() []LotteryStrategyRevisionReference {
	return append([]LotteryStrategyRevisionReference(nil), publication.strategyRevisionManifest...)
}

// StrategyRevision finds one exact Strategy snapshot without reading a mutable
// latest revision. Unknown Strategy identities return the zero reference.
func (publication ActivityPublication) StrategyRevision(
	strategyID LotteryStrategyID,
) (LotteryStrategyRevisionReference, bool) {
	index, found := slices.BinarySearchFunc(
		publication.strategyRevisionManifest,
		strategyID,
		func(reference LotteryStrategyRevisionReference, target LotteryStrategyID) int {
			if reference.strategyID < target {
				return -1
			}
			if reference.strategyID > target {
				return 1
			}
			return 0
		},
	)
	if !found {
		return LotteryStrategyRevisionReference{}, false
	}
	return publication.strategyRevisionManifest[index], true
}

// ApprovalEvidenceReference returns the bounded evidence pointer captured for
// this release or rollback. It is not itself proof of caller authorization.
func (publication ActivityPublication) ApprovalEvidenceReference() EvidenceReference {
	return publication.approvalEvidenceReference
}

func (publication ActivityPublication) clone() ActivityPublication {
	publication.strategyRevisionManifest = publication.StrategyRevisionManifest()
	return publication
}

func invalidActivityPublication(classification error, format string, arguments ...any) error {
	reason := fmt.Sprintf(format, arguments...)
	return fmt.Errorf("%w: %w: %s", ErrActivityPublicationInvalid, classification, reason)
}
