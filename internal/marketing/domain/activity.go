package domain

import (
	"fmt"
	"time"
)

// ActivityID identifies one durable Marketing-owned Activity aggregate.
type ActivityID uint64

// ActivityStateVersion is the optimistic-CAS generation of the Activity root.
// It is intentionally independent from immutable publication versions because
// retirement changes root state without appending a new publication.
type ActivityStateVersion uint64

// ActivityLifecycle is the closed v1 lifecycle of an Activity root.
type ActivityLifecycle string

const (
	ActivityLifecycleDraft     ActivityLifecycle = "draft"
	ActivityLifecyclePublished ActivityLifecycle = "published"
	ActivityLifecycleRetired   ActivityLifecycle = "retired"
)

// Activity is the compact mutable root. Exact release content lives in
// append-only ActivityPublication records rather than being copied into the
// aggregate row.
type Activity struct {
	id                       ActivityID
	name                     ActivityName
	lifecycle                ActivityLifecycle
	stateVersion             ActivityStateVersion
	activePublicationVersion ActivityPublicationVersion
	retiredAt                time.Time
	retirementReference      EvidenceReference
}

// NewActivity creates the initial draft at CAS state version zero.
func NewActivity(id ActivityID, name string) (Activity, error) {
	canonicalName, err := NewActivityName(name)
	if err != nil {
		return Activity{}, fmt.Errorf("%w: %w", ErrActivityInvalid, err)
	}
	activity := Activity{
		id:        id,
		name:      canonicalName,
		lifecycle: ActivityLifecycleDraft,
	}
	if err := activity.Validate(); err != nil {
		return Activity{}, err
	}
	return activity, nil
}

// RestoreActivity reconstructs persisted root state without normalizing it.
// Non-canonical names, timestamps, and lifecycle combinations fail closed.
func RestoreActivity(
	id ActivityID,
	name ActivityName,
	lifecycle ActivityLifecycle,
	stateVersion ActivityStateVersion,
	activePublicationVersion ActivityPublicationVersion,
	retiredAt time.Time,
	retirementReference EvidenceReference,
) (Activity, error) {
	activity := Activity{
		id:                       id,
		name:                     name,
		lifecycle:                lifecycle,
		stateVersion:             stateVersion,
		activePublicationVersion: activePublicationVersion,
		retiredAt:                retiredAt,
		retirementReference:      retirementReference,
	}
	if err := activity.Validate(); err != nil {
		return Activity{}, err
	}
	return activity, nil
}

// Validate rejects zero, future, and mixed lifecycle representations. A
// retired root retains its last active publication identity for audit and gate
// resolution, but retirement itself is terminal.
func (activity Activity) Validate() error {
	if activity.id == 0 {
		return invalidActivity(ErrActivityIDInvalid, "id is required")
	}
	if err := activity.name.Validate(); err != nil {
		return invalidActivity(ErrActivityNameInvalid, "name %v", err)
	}
	switch activity.lifecycle {
	case ActivityLifecycleDraft:
		if activity.stateVersion != 0 || activity.activePublicationVersion != 0 {
			return invalidActivity(
				ErrActivityStateVersionInvalid,
				"draft requires zero state and active publication versions",
			)
		}
		if !activity.retiredAt.IsZero() || activity.retirementReference != "" {
			return invalidActivity(ErrActivityLifecycleUnsupported, "draft cannot carry retirement evidence")
		}
	case ActivityLifecyclePublished:
		if activity.activePublicationVersion == 0 {
			return invalidActivity(ErrActivityLifecycleUnsupported, "published requires an active publication")
		}
		if ActivityPublicationVersion(activity.stateVersion) != activity.activePublicationVersion {
			return invalidActivity(
				ErrActivityStateVersionInvalid,
				"published state version must equal active publication version",
			)
		}
		if !activity.retiredAt.IsZero() || activity.retirementReference != "" {
			return invalidActivity(ErrActivityLifecycleUnsupported, "published cannot carry retirement evidence")
		}
	case ActivityLifecycleRetired:
		if activity.activePublicationVersion == 0 {
			return invalidActivity(ErrActivityLifecycleUnsupported, "retired requires its last active publication")
		}
		if activity.activePublicationVersion == maxActivityVersion ||
			activity.stateVersion != ActivityStateVersion(activity.activePublicationVersion+1) {
			return invalidActivity(
				ErrActivityStateVersionInvalid,
				"retired state version must be one greater than its active publication version",
			)
		}
		if err := validateCanonicalUTCInstant(activity.retiredAt); err != nil {
			return invalidActivity(ErrActivityLifecycleUnsupported, "retired-at %v", err)
		}
		if err := activity.retirementReference.Validate(); err != nil {
			return invalidActivity(ErrActivityLifecycleUnsupported, "retirement reference %v", err)
		}
	default:
		return invalidActivity(ErrActivityLifecycleUnsupported, "lifecycle %q", activity.lifecycle)
	}
	return nil
}

// ID returns the durable Marketing identity.
func (activity Activity) ID() ActivityID { return activity.id }

// Name returns the canonical Activity label.
func (activity Activity) Name() ActivityName { return activity.name }

// Lifecycle returns draft, published, or retired.
func (activity Activity) Lifecycle() ActivityLifecycle { return activity.lifecycle }

// StateVersion returns the current optimistic-CAS generation.
func (activity Activity) StateVersion() ActivityStateVersion { return activity.stateVersion }

// ActivePublicationVersion returns zero only for a draft. Retired roots retain
// the last active publication identity, although the gate decision is retired.
func (activity Activity) ActivePublicationVersion() ActivityPublicationVersion {
	return activity.activePublicationVersion
}

// RetiredAt returns the canonical UTC-microsecond retirement instant and true
// only after the terminal retirement transition.
func (activity Activity) RetiredAt() (time.Time, bool) {
	return activity.retiredAt, !activity.retiredAt.IsZero()
}

// RetirementReference returns the opaque retirement evidence reference and
// true only for a retired Activity.
func (activity Activity) RetirementReference() (EvidenceReference, bool) {
	return activity.retirementReference, activity.retirementReference != ""
}

func invalidActivity(classification error, format string, arguments ...any) error {
	reason := fmt.Sprintf(format, arguments...)
	return fmt.Errorf("%w: %w: %s", ErrActivityInvalid, classification, reason)
}

const maxActivityVersion = ActivityPublicationVersion(^uint64(0))
