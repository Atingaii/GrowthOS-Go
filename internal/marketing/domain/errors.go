package domain

import "errors"

var (
	// ErrActivityInvalid classifies an Activity aggregate that cannot safely
	// participate in a publication transition or gate decision.
	ErrActivityInvalid = errors.New("marketing: activity is invalid")
	// ErrActivityIDInvalid reports a missing durable Activity identity.
	ErrActivityIDInvalid = errors.New("marketing: activity id is invalid")
	// ErrActivityNameInvalid reports a missing, non-canonical, or oversized name.
	ErrActivityNameInvalid = errors.New("marketing: activity name is invalid")
	// ErrActivityLifecycleUnsupported reports an unknown persisted lifecycle.
	ErrActivityLifecycleUnsupported = errors.New("marketing: activity lifecycle is unsupported")
	// ErrActivityLifecycleTransitionInvalid reports an operation that the current
	// lifecycle does not permit. Retirement is terminal in this lesson.
	ErrActivityLifecycleTransitionInvalid = errors.New("marketing: activity lifecycle transition is invalid")
	// ErrActivityStateVersionInvalid reports a lifecycle-inconsistent state CAS
	// generation. Draft deliberately starts at generation zero.
	ErrActivityStateVersionInvalid = errors.New("marketing: activity state version is invalid")
	// ErrActivityVersionOverflow reports that the next state or publication
	// generation cannot be represented without wrapping to zero.
	ErrActivityVersionOverflow = errors.New("marketing: activity version overflow")
	// ErrActivityPublicationInvalid classifies an incomplete, mixed, or otherwise
	// unsafe immutable publication record.
	ErrActivityPublicationInvalid = errors.New("marketing: activity publication is invalid")
	// ErrActivityPublicationSchemaUnsupported reports a zero or future persisted
	// publication shape. Schema version is independent from business versions.
	ErrActivityPublicationSchemaUnsupported = errors.New("marketing: activity publication schema is unsupported")
	// ErrActivityPublicationKindUnsupported reports a zero or future publication
	// kind. The v1 model accepts only release and rollback.
	ErrActivityPublicationKindUnsupported = errors.New("marketing: activity publication kind is unsupported")
	// ErrActivityPublicationWindowInvalid reports a missing, non-canonical, or
	// non-increasing [starts-at, ends-at) UTC-microsecond window.
	ErrActivityPublicationWindowInvalid = errors.New("marketing: activity publication window is invalid")
	// ErrActivityPublicationExpired reports an attempt to publish content whose
	// exclusive end has already been reached at the controlled planning instant.
	ErrActivityPublicationExpired = errors.New("marketing: activity publication has ended")
	// ErrLotteryGraphReferenceInvalid reports an incomplete exact foreign graph
	// identity. It does not imply that Marketing owns or loaded the graph.
	ErrLotteryGraphReferenceInvalid = errors.New("marketing: Lottery graph reference is invalid")
	// ErrLotteryStrategyRevisionReferenceInvalid reports an incomplete exact
	// foreign Strategy snapshot identity.
	ErrLotteryStrategyRevisionReferenceInvalid = errors.New("marketing: Lottery Strategy revision reference is invalid")
	// ErrStrategyRevisionManifestInvalid reports a missing, ambiguous, duplicate,
	// or non-canonical Strategy revision manifest.
	ErrStrategyRevisionManifestInvalid = errors.New("marketing: Strategy revision manifest is invalid")
	// ErrStrategyRevisionManifestLimitExceeded bounds validation, persistence, and
	// later exact Strategy lookup work for one publication.
	ErrStrategyRevisionManifestLimitExceeded = errors.New("marketing: Strategy revision manifest limit exceeded")
	// ErrEvidenceReferenceInvalid reports a missing or non-canonical bounded
	// approval or retirement evidence reference.
	ErrEvidenceReferenceInvalid = errors.New("marketing: evidence reference is invalid")
	// ErrActivityRollbackTargetInvalid reports a target that is not an older
	// publication of the same Activity or whose exact record is malformed.
	ErrActivityRollbackTargetInvalid = errors.New("marketing: rollback target is invalid")
	// ErrActivityRollbackTargetNotPublished reports that the application boundary
	// could not establish the target as an append-only historical publication.
	ErrActivityRollbackTargetNotPublished = errors.New("marketing: rollback target was not previously published")
	// ErrActivityTransitionInvalid reports a zero or internally inconsistent plan.
	ErrActivityTransitionInvalid = errors.New("marketing: activity transition is invalid")
	// ErrActivityGateInvalid reports missing or mismatched technical input. It is
	// distinct from scheduled, ended, and retired business decisions.
	ErrActivityGateInvalid = errors.New("marketing: activity gate input is invalid")
)
