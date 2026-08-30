package application

import (
	"fmt"
	"slices"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// planningEvidenceReference is valid only for a pure provisional domain plan.
// It is never passed to a dependency or persistence writer.
const planningEvidenceReference domain.EvidenceReference = "application-plan"

// ActivityPublicationCandidate is the complete immutable content reviewed by
// Lottery and Governance before an evidence-bearing transition is rebuilt.
// Fields are private so a verifier cannot mutate or partially forge it.
type ActivityPublicationCandidate struct {
	activityID               domain.ActivityID
	version                  domain.ActivityPublicationVersion
	schemaVersion            domain.ActivityPublicationSchemaVersion
	kind                     domain.ActivityPublicationKind
	rollbackOf               domain.ActivityPublicationVersion
	startsAt                 time.Time
	endsAt                   time.Time
	publishedAt              time.Time
	graphReference           domain.LotteryGraphReference
	strategyRevisionManifest []domain.LotteryStrategyRevisionReference
}

// NewActivityPublicationCandidate converts one complete publication shape into
// the evidence-free immutable verifier view. It is also used by strict current
// resolution; the publication's existing approval reference is never exposed
// as proof for a different candidate.
func NewActivityPublicationCandidate(
	publication domain.ActivityPublication,
) (ActivityPublicationCandidate, error) {
	if err := publication.Validate(); err != nil {
		return ActivityPublicationCandidate{}, fmt.Errorf("candidate publication: %w", err)
	}
	candidate := ActivityPublicationCandidate{
		activityID:               publication.ActivityID(),
		version:                  publication.Version(),
		schemaVersion:            publication.SchemaVersion(),
		kind:                     publication.Kind(),
		startsAt:                 publication.StartsAt(),
		endsAt:                   publication.EndsAt(),
		publishedAt:              publication.PublishedAt(),
		graphReference:           publication.GraphReference(),
		strategyRevisionManifest: publication.StrategyRevisionManifest(),
	}
	if rollbackOf, ok := publication.RollbackOf(); ok {
		candidate.rollbackOf = rollbackOf
	}
	if err := candidate.Validate(); err != nil {
		return ActivityPublicationCandidate{}, err
	}
	return candidate, nil
}

// Validate rechecks the candidate shape through the authoritative publication
// constructor, without accepting or exposing the provisional evidence token.
func (candidate ActivityPublicationCandidate) Validate() error {
	publication, err := domain.RestoreActivityPublication(
		candidate.activityID,
		candidate.version,
		candidate.schemaVersion,
		candidate.kind,
		candidate.rollbackOf,
		candidate.startsAt,
		candidate.endsAt,
		candidate.publishedAt,
		candidate.graphReference,
		candidate.strategyRevisionManifest,
		planningEvidenceReference,
	)
	if err != nil {
		return fmt.Errorf("publication candidate is invalid: %w", err)
	}
	if publication.ActivityID() != candidate.activityID ||
		publication.Version() != candidate.version ||
		publication.SchemaVersion() != candidate.schemaVersion ||
		publication.Kind() != candidate.kind ||
		publication.StartsAt() != candidate.startsAt ||
		publication.EndsAt() != candidate.endsAt ||
		publication.PublishedAt() != candidate.publishedAt ||
		publication.GraphReference() != candidate.graphReference ||
		!slices.Equal(publication.StrategyRevisionManifest(), candidate.strategyRevisionManifest) {
		return fmt.Errorf("publication candidate is not canonical")
	}
	rollbackOf, rollback := publication.RollbackOf()
	if rollback != (candidate.kind == domain.ActivityPublicationKindRollback) ||
		rollbackOf != candidate.rollbackOf {
		return fmt.Errorf("publication candidate rollback shape is invalid")
	}
	return nil
}

// ActivityID returns the Marketing root identity.
func (candidate ActivityPublicationCandidate) ActivityID() domain.ActivityID {
	return candidate.activityID
}

// Version returns the proposed immutable Activity publication version.
func (candidate ActivityPublicationCandidate) Version() domain.ActivityPublicationVersion {
	return candidate.version
}

// SchemaVersion returns the exact candidate decoding contract.
func (candidate ActivityPublicationCandidate) SchemaVersion() domain.ActivityPublicationSchemaVersion {
	return candidate.schemaVersion
}

// Kind returns release or rollback.
func (candidate ActivityPublicationCandidate) Kind() domain.ActivityPublicationKind {
	return candidate.kind
}

// RollbackOf returns the exact source and true only for rollback.
func (candidate ActivityPublicationCandidate) RollbackOf() (domain.ActivityPublicationVersion, bool) {
	return candidate.rollbackOf, candidate.kind == domain.ActivityPublicationKindRollback
}

// StartsAt returns the inclusive UTC-microsecond boundary.
func (candidate ActivityPublicationCandidate) StartsAt() time.Time { return candidate.startsAt }

// EndsAt returns the exclusive UTC-microsecond boundary.
func (candidate ActivityPublicationCandidate) EndsAt() time.Time { return candidate.endsAt }

// PublishedAt returns the one controlled planning instant.
func (candidate ActivityPublicationCandidate) PublishedAt() time.Time { return candidate.publishedAt }

// GraphReference returns the exact Lottery graph identity.
func (candidate ActivityPublicationCandidate) GraphReference() domain.LotteryGraphReference {
	return candidate.graphReference
}

// StrategyRevisionManifest returns a defensive canonical copy.
func (candidate ActivityPublicationCandidate) StrategyRevisionManifest() []domain.LotteryStrategyRevisionReference {
	return append([]domain.LotteryStrategyRevisionReference(nil), candidate.strategyRevisionManifest...)
}

func sameActivityPublicationCandidate(
	left ActivityPublicationCandidate,
	right ActivityPublicationCandidate,
) bool {
	return left.activityID == right.activityID &&
		left.version == right.version &&
		left.schemaVersion == right.schemaVersion &&
		left.kind == right.kind &&
		left.rollbackOf == right.rollbackOf &&
		left.startsAt == right.startsAt &&
		left.endsAt == right.endsAt &&
		left.publishedAt == right.publishedAt &&
		left.graphReference == right.graphReference &&
		slices.Equal(left.strategyRevisionManifest, right.strategyRevisionManifest)
}

// ActivityRetirementCandidate is the exact root state and server instant that
// Governance approves before the terminal CAS.
type ActivityRetirementCandidate struct {
	activity  domain.Activity
	retiredAt time.Time
}

func newActivityRetirementCandidate(
	activity domain.Activity,
	retiredAt time.Time,
) (ActivityRetirementCandidate, error) {
	candidate := ActivityRetirementCandidate{activity: activity, retiredAt: canonicalOperationInstant(retiredAt)}
	if err := candidate.Validate(); err != nil {
		return ActivityRetirementCandidate{}, err
	}
	return candidate, nil
}

// Validate proves that this exact root can form a terminal transition.
func (candidate ActivityRetirementCandidate) Validate() error {
	if _, err := domain.PlanRetire(candidate.activity, planningEvidenceReference, candidate.retiredAt); err != nil {
		return fmt.Errorf("retirement candidate is invalid: %w", err)
	}
	return nil
}

// ActivityID returns the root being retired.
func (candidate ActivityRetirementCandidate) ActivityID() domain.ActivityID {
	return candidate.activity.ID()
}

// ExpectedStateVersion returns the exact CAS generation approved.
func (candidate ActivityRetirementCandidate) ExpectedStateVersion() domain.ActivityStateVersion {
	return candidate.activity.StateVersion()
}

// ActivePublicationVersion returns the retained active publication identity.
func (candidate ActivityRetirementCandidate) ActivePublicationVersion() domain.ActivityPublicationVersion {
	return candidate.activity.ActivePublicationVersion()
}

// RetiredAt returns the canonical UTC-microsecond terminal instant.
func (candidate ActivityRetirementCandidate) RetiredAt() time.Time { return candidate.retiredAt }
