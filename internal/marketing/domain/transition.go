package domain

import (
	"fmt"
	"time"
)

// ActivityTransition is a pure write plan for one repository transaction. The
// repository must update the Activity root where state_version equals
// ExpectedStateVersion and, when present, insert Record append-only in the same
// transaction. Planning alone is not concurrency control.
type ActivityTransition struct {
	expectedLifecycle                ActivityLifecycle
	expectedStateVersion             ActivityStateVersion
	expectedActivePublicationVersion ActivityPublicationVersion
	next                             Activity
	record                           ActivityPublication
	appendsPublication               bool
}

// PlanPublish appends a release at active publication version + 1. It accepts
// both draft and already-published roots so a later exact release can replace an
// earlier one. Retirement is terminal.
func PlanPublish(
	current Activity,
	startsAt time.Time,
	endsAt time.Time,
	graphReference LotteryGraphReference,
	strategyRevisionManifest []LotteryStrategyRevisionReference,
	approvalEvidenceReference EvidenceReference,
	publishedAt time.Time,
) (ActivityTransition, error) {
	if err := current.Validate(); err != nil {
		return ActivityTransition{}, err
	}
	if current.lifecycle != ActivityLifecycleDraft && current.lifecycle != ActivityLifecyclePublished {
		return ActivityTransition{}, fmt.Errorf(
			"%w: cannot publish from %q",
			ErrActivityLifecycleTransitionInvalid,
			current.lifecycle,
		)
	}
	nextVersion, err := nextPublicationVersion(current.activePublicationVersion)
	if err != nil {
		return ActivityTransition{}, err
	}
	record, err := newActivityPublication(
		current.id,
		nextVersion,
		ActivityPublicationKindRelease,
		0,
		startsAt,
		endsAt,
		publishedAt,
		graphReference,
		strategyRevisionManifest,
		approvalEvidenceReference,
	)
	if err != nil {
		return ActivityTransition{}, err
	}
	next := current
	next.lifecycle = ActivityLifecyclePublished
	next.stateVersion = ActivityStateVersion(nextVersion)
	next.activePublicationVersion = nextVersion
	next.retiredAt = time.Time{}
	next.retirementReference = ""
	return newActivityTransition(current, next, record, true)
}

// PlanRollback appends a new rollback publication; it never edits or reactivates
// the target row. The application boundary supplies the independently checked
// historical-publication fact. Exact release content is copied from target,
// while version, kind, rollback-of, published-at, and approval evidence are new.
func PlanRollback(
	current Activity,
	target ActivityPublication,
	targetWasPreviouslyPublished bool,
	approvalEvidenceReference EvidenceReference,
	publishedAt time.Time,
) (ActivityTransition, error) {
	if err := current.Validate(); err != nil {
		return ActivityTransition{}, err
	}
	if current.lifecycle != ActivityLifecyclePublished {
		return ActivityTransition{}, fmt.Errorf(
			"%w: cannot rollback from %q",
			ErrActivityLifecycleTransitionInvalid,
			current.lifecycle,
		)
	}
	if err := target.Validate(); err != nil {
		return ActivityTransition{}, fmt.Errorf("%w: %v", ErrActivityRollbackTargetInvalid, err)
	}
	if target.activityID != current.id {
		return ActivityTransition{}, fmt.Errorf(
			"%w: target Activity %d does not match %d",
			ErrActivityRollbackTargetInvalid,
			target.activityID,
			current.id,
		)
	}
	if target.version >= current.activePublicationVersion {
		return ActivityTransition{}, fmt.Errorf(
			"%w: target version %d is not older than active version %d",
			ErrActivityRollbackTargetInvalid,
			target.version,
			current.activePublicationVersion,
		)
	}
	if !targetWasPreviouslyPublished {
		return ActivityTransition{}, ErrActivityRollbackTargetNotPublished
	}
	canonicalPublishedAt := canonicalUTCInstant(publishedAt)
	if canonicalPublishedAt.IsZero() {
		return ActivityTransition{}, fmt.Errorf(
			"%w: rollback published-at is required",
			ErrActivityRollbackTargetInvalid,
		)
	}
	if !canonicalPublishedAt.Before(target.endsAt) {
		return ActivityTransition{}, fmt.Errorf(
			"%w: %w",
			ErrActivityRollbackTargetInvalid,
			ErrActivityPublicationExpired,
		)
	}
	nextVersion, err := nextPublicationVersion(current.activePublicationVersion)
	if err != nil {
		return ActivityTransition{}, err
	}
	record, err := newActivityPublication(
		current.id,
		nextVersion,
		ActivityPublicationKindRollback,
		target.version,
		target.startsAt,
		target.endsAt,
		canonicalPublishedAt,
		target.graphReference,
		target.strategyRevisionManifest,
		approvalEvidenceReference,
	)
	if err != nil {
		return ActivityTransition{}, err
	}
	next := current
	next.stateVersion = ActivityStateVersion(nextVersion)
	next.activePublicationVersion = nextVersion
	return newActivityTransition(current, next, record, true)
}

// PlanRetire terminally retires a published root, retaining the last active
// publication pointer and adding no publication record. Its independent bounded
// reference records retirement evidence without claiming an identity system.
func PlanRetire(
	current Activity,
	retirementReference EvidenceReference,
	retiredAt time.Time,
) (ActivityTransition, error) {
	if err := current.Validate(); err != nil {
		return ActivityTransition{}, err
	}
	if current.lifecycle != ActivityLifecyclePublished {
		return ActivityTransition{}, fmt.Errorf(
			"%w: cannot retire from %q",
			ErrActivityLifecycleTransitionInvalid,
			current.lifecycle,
		)
	}
	if err := retirementReference.Validate(); err != nil {
		return ActivityTransition{}, err
	}
	canonicalRetiredAt := canonicalUTCInstant(retiredAt)
	if canonicalRetiredAt.IsZero() {
		return ActivityTransition{}, fmt.Errorf(
			"%w: retired-at is required",
			ErrActivityLifecycleTransitionInvalid,
		)
	}
	if current.stateVersion == ActivityStateVersion(^uint64(0)) {
		return ActivityTransition{}, ErrActivityVersionOverflow
	}
	next := current
	next.lifecycle = ActivityLifecycleRetired
	next.stateVersion++
	next.retiredAt = canonicalRetiredAt
	next.retirementReference = retirementReference
	return newActivityTransition(current, next, ActivityPublication{}, false)
}

// Validate rechecks a transition before an application or repository trusts it.
func (transition ActivityTransition) Validate() error {
	if transition.expectedStateVersion == ActivityStateVersion(^uint64(0)) {
		return fmt.Errorf("%w: expected state version cannot advance", ErrActivityTransitionInvalid)
	}
	if err := transition.next.Validate(); err != nil {
		return fmt.Errorf("%w: next Activity: %v", ErrActivityTransitionInvalid, err)
	}
	if transition.next.stateVersion != transition.expectedStateVersion+1 {
		return fmt.Errorf(
			"%w: next state version %d does not follow expected %d",
			ErrActivityTransitionInvalid,
			transition.next.stateVersion,
			transition.expectedStateVersion,
		)
	}
	if transition.appendsPublication {
		if transition.expectedLifecycle != ActivityLifecycleDraft &&
			transition.expectedLifecycle != ActivityLifecyclePublished {
			return fmt.Errorf("%w: publication has invalid expected lifecycle", ErrActivityTransitionInvalid)
		}
		if ActivityStateVersion(transition.expectedActivePublicationVersion) != transition.expectedStateVersion {
			return fmt.Errorf("%w: expected state and active versions diverge", ErrActivityTransitionInvalid)
		}
		if err := transition.record.Validate(); err != nil {
			return fmt.Errorf("%w: record: %v", ErrActivityTransitionInvalid, err)
		}
		if transition.next.lifecycle != ActivityLifecyclePublished {
			return fmt.Errorf("%w: publication transition must remain published", ErrActivityTransitionInvalid)
		}
		if transition.record.activityID != transition.next.id ||
			transition.record.version != transition.next.activePublicationVersion {
			return fmt.Errorf("%w: publication does not match next active identity", ErrActivityTransitionInvalid)
		}
		if ActivityStateVersion(transition.record.version) != transition.next.stateVersion {
			return fmt.Errorf("%w: publication and state versions diverge", ErrActivityTransitionInvalid)
		}
		if transition.record.version != transition.expectedActivePublicationVersion+1 {
			return fmt.Errorf("%w: publication version is not expected active + 1", ErrActivityTransitionInvalid)
		}
		return nil
	}
	if !transition.record.isZero() {
		return fmt.Errorf("%w: non-publication transition carries a record", ErrActivityTransitionInvalid)
	}
	if transition.next.lifecycle != ActivityLifecycleRetired {
		return fmt.Errorf("%w: transition without record must retire", ErrActivityTransitionInvalid)
	}
	if transition.expectedLifecycle != ActivityLifecyclePublished ||
		transition.expectedActivePublicationVersion == 0 ||
		ActivityStateVersion(transition.expectedActivePublicationVersion) != transition.expectedStateVersion ||
		transition.next.activePublicationVersion != transition.expectedActivePublicationVersion {
		return fmt.Errorf("%w: retirement CAS state is inconsistent", ErrActivityTransitionInvalid)
	}
	return nil
}

// ExpectedLifecycle is an additional fail-closed CAS predicate for the root.
func (transition ActivityTransition) ExpectedLifecycle() ActivityLifecycle {
	return transition.expectedLifecycle
}

// ExpectedStateVersion is the CAS predicate that must protect persistence.
func (transition ActivityTransition) ExpectedStateVersion() ActivityStateVersion {
	return transition.expectedStateVersion
}

// ExpectedActivePublicationVersion returns the old active publication CAS
// predicate. It is zero for the initial draft publish.
func (transition ActivityTransition) ExpectedActivePublicationVersion() ActivityPublicationVersion {
	return transition.expectedActivePublicationVersion
}

// Next returns the complete next aggregate root.
func (transition ActivityTransition) Next() Activity { return transition.next }

// Record returns a defensive publication copy and true for publish/rollback.
// Retirement returns the zero publication and false.
func (transition ActivityTransition) Record() (ActivityPublication, bool) {
	if !transition.appendsPublication {
		return ActivityPublication{}, false
	}
	return transition.record.clone(), true
}

// AppendsPublication reports whether the repository must insert a history row.
func (transition ActivityTransition) AppendsPublication() bool {
	return transition.appendsPublication
}

func newActivityTransition(
	current Activity,
	next Activity,
	record ActivityPublication,
	appendsPublication bool,
) (ActivityTransition, error) {
	transition := ActivityTransition{
		expectedLifecycle:                current.lifecycle,
		expectedStateVersion:             current.stateVersion,
		expectedActivePublicationVersion: current.activePublicationVersion,
		next:                             next,
		record:                           record.clone(),
		appendsPublication:               appendsPublication,
	}
	if err := transition.Validate(); err != nil {
		return ActivityTransition{}, err
	}
	return transition, nil
}

func nextPublicationVersion(current ActivityPublicationVersion) (ActivityPublicationVersion, error) {
	if current == maxActivityVersion {
		return 0, ErrActivityVersionOverflow
	}
	return current + 1, nil
}

func (publication ActivityPublication) isZero() bool {
	return publication.activityID == 0 &&
		publication.version == 0 &&
		publication.schemaVersion == 0 &&
		publication.kind == "" &&
		publication.rollbackOf == 0 &&
		publication.startsAt.IsZero() &&
		publication.endsAt.IsZero() &&
		publication.publishedAt.IsZero() &&
		publication.graphReference == (LotteryGraphReference{}) &&
		len(publication.strategyRevisionManifest) == 0 &&
		publication.approvalEvidenceReference == ""
}
