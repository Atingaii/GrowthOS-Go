package domain

import (
	"fmt"
	"time"
)

// ActivityGateStatus is the closed business outcome of evaluating Activity
// availability. The zero status is reserved for technical failure.
type ActivityGateStatus string

const (
	ActivityGateStatusNotPublished ActivityGateStatus = "not_published"
	ActivityGateStatusScheduled    ActivityGateStatus = "scheduled"
	ActivityGateStatusActive       ActivityGateStatus = "active"
	ActivityGateStatusEnded        ActivityGateStatus = "ended"
	ActivityGateStatusRetired      ActivityGateStatus = "retired"
)

// ActivityGateDecision is a formed business result. Invalid Activity,
// publication, identity, or clock inputs return its zero value plus an error.
type ActivityGateDecision struct {
	status             ActivityGateStatus
	activityID         ActivityID
	publicationVersion ActivityPublicationVersion
	evaluatedAt        time.Time
}

// DecideActivityGate evaluates the current exact publication at one controlled
// server instant. The window is [starts-at, ends-at): start is inclusive and end
// is exclusive. Retirement takes precedence over the window after all technical
// input has been validated.
func DecideActivityGate(
	activity Activity,
	publication ActivityPublication,
	evaluatedAt time.Time,
) (ActivityGateDecision, error) {
	if err := activity.Validate(); err != nil {
		return ActivityGateDecision{}, fmt.Errorf("%w: Activity: %v", ErrActivityGateInvalid, err)
	}
	canonicalEvaluatedAt := canonicalUTCInstant(evaluatedAt)
	if canonicalEvaluatedAt.IsZero() {
		return ActivityGateDecision{}, fmt.Errorf("%w: evaluated-at is required", ErrActivityGateInvalid)
	}
	if activity.lifecycle == ActivityLifecycleDraft {
		if !publication.isZero() {
			return ActivityGateDecision{}, fmt.Errorf(
				"%w: draft must not carry a publication",
				ErrActivityGateInvalid,
			)
		}
		decision := ActivityGateDecision{
			status:      ActivityGateStatusNotPublished,
			activityID:  activity.id,
			evaluatedAt: canonicalEvaluatedAt,
		}
		if err := decision.Validate(); err != nil {
			return ActivityGateDecision{}, err
		}
		return decision, nil
	}
	if publication.isZero() {
		if activity.lifecycle != ActivityLifecycleRetired {
			return ActivityGateDecision{}, fmt.Errorf(
				"%w: published Activity requires its active publication",
				ErrActivityGateInvalid,
			)
		}
	} else {
		if err := publication.Validate(); err != nil {
			return ActivityGateDecision{}, fmt.Errorf("%w: publication: %v", ErrActivityGateInvalid, err)
		}
		if publication.activityID != activity.id ||
			publication.version != activity.activePublicationVersion {
			return ActivityGateDecision{}, fmt.Errorf(
				"%w: publication identity does not match active Activity state",
				ErrActivityGateInvalid,
			)
		}
	}

	status := ActivityGateStatusRetired
	if activity.lifecycle == ActivityLifecyclePublished {
		switch {
		case canonicalEvaluatedAt.Before(publication.startsAt):
			status = ActivityGateStatusScheduled
		case !canonicalEvaluatedAt.Before(publication.endsAt):
			status = ActivityGateStatusEnded
		default:
			status = ActivityGateStatusActive
		}
	}
	decision := ActivityGateDecision{
		status:             status,
		activityID:         activity.id,
		publicationVersion: activity.activePublicationVersion,
		evaluatedAt:        canonicalEvaluatedAt,
	}
	if err := decision.Validate(); err != nil {
		return ActivityGateDecision{}, err
	}
	return decision, nil
}

// Validate rejects a zero or forged partial business decision.
func (decision ActivityGateDecision) Validate() error {
	switch decision.status {
	case ActivityGateStatusNotPublished:
		if decision.publicationVersion != 0 {
			return fmt.Errorf("%w: not-published decision cannot identify a publication", ErrActivityGateInvalid)
		}
	case ActivityGateStatusScheduled,
		ActivityGateStatusActive,
		ActivityGateStatusEnded,
		ActivityGateStatusRetired:
		if decision.publicationVersion == 0 {
			return fmt.Errorf("%w: publication identity is required", ErrActivityGateInvalid)
		}
	default:
		return fmt.Errorf("%w: status %q is unsupported", ErrActivityGateInvalid, decision.status)
	}
	if decision.activityID == 0 {
		return fmt.Errorf("%w: Activity identity is required", ErrActivityGateInvalid)
	}
	if err := validateCanonicalUTCInstant(decision.evaluatedAt); err != nil {
		return fmt.Errorf("%w: evaluated-at %v", ErrActivityGateInvalid, err)
	}
	return nil
}

// Status returns not_published, scheduled, active, ended, or retired.
func (decision ActivityGateDecision) Status() ActivityGateStatus { return decision.status }

// ActivityID returns the exact Activity evaluated.
func (decision ActivityGateDecision) ActivityID() ActivityID { return decision.activityID }

// PublicationVersion returns the exact active publication evaluated.
func (decision ActivityGateDecision) PublicationVersion() ActivityPublicationVersion {
	return decision.publicationVersion
}

// EvaluatedAt returns the canonical UTC-microsecond decision instant.
func (decision ActivityGateDecision) EvaluatedAt() time.Time { return decision.evaluatedAt }

// AllowsParticipation is true only for the active business window.
func (decision ActivityGateDecision) AllowsParticipation() bool {
	return decision.status == ActivityGateStatusActive
}
