package application

import "errors"

var (
	// ErrActivityInvalidArgument reports a caller contract violation before
	// external work begins.
	ErrActivityInvalidArgument = errors.New("marketing activity: invalid argument")
	// ErrActivityNotConfigured reports a nil, typed-nil, partial dependency set,
	// or non-positive internal operation duration.
	ErrActivityNotConfigured = errors.New("marketing activity: not configured")
	// ErrActivityClockInvalid reports a zero business instant from the controlled
	// Clock. It is not an ended Activity decision.
	ErrActivityClockInvalid = errors.New("marketing activity: clock is invalid")
	// ErrActivityOperationTimedOut reports the private service budget expiring
	// while the caller remained live.
	ErrActivityOperationTimedOut = errors.New("marketing activity: operation timed out")
	// ErrActivityOperationFailure is the fail-closed class for an otherwise
	// unclassified internal or dependency failure.
	ErrActivityOperationFailure = errors.New("marketing activity: operation failed")
	// ErrActivityResolutionInvalid reports a corrupt or mismatched current
	// snapshot that cannot form a trusted gate decision.
	ErrActivityResolutionInvalid = errors.New("marketing activity: resolution is invalid")
	// ErrActivityPublicationCandidateInvalid reports a release request that
	// cannot form one complete immutable publication.
	ErrActivityPublicationCandidateInvalid = errors.New("marketing activity: publication candidate is invalid")
	// ErrActivityRollbackTargetInvalid reports a source that is not an older,
	// restorable, not-ended publication of the same Activity.
	ErrActivityRollbackTargetInvalid = errors.New("marketing activity: rollback target is invalid")

	// ErrActivityApprovalRejected means Governance explicitly rejected the exact
	// candidate. It is distinct from caller authorization.
	ErrActivityApprovalRejected = errors.New("marketing activity: approval rejected")
	// ErrActivityApprovalUnavailable means Governance could not establish an
	// approval result while the operation remained live.
	ErrActivityApprovalUnavailable = errors.New("marketing activity: approval unavailable")
	// ErrActivityApprovalEvidenceInvalid means a verifier returned nil error but
	// an empty or malformed evidence reference.
	ErrActivityApprovalEvidenceInvalid = errors.New("marketing activity: approval evidence is invalid")
	// ErrLotteryPublicationInvalid means exact Lottery content is absent,
	// corrupt, identity-mismatched, or not a closed terminal/manifest set.
	ErrLotteryPublicationInvalid = errors.New("marketing activity: Lottery publication is invalid")
	// ErrLotteryPublicationUnavailable means the Lottery authority could not
	// verify exact content while the operation remained live.
	ErrLotteryPublicationUnavailable = errors.New("marketing activity: Lottery publication is unavailable")

	// ErrRepositoryInvalidArgument reports a repository call contract violation.
	ErrRepositoryInvalidArgument = errors.New("marketing repository: invalid argument")
	// ErrRepositoryNotConfigured reports an unusable storage handle.
	ErrRepositoryNotConfigured = errors.New("marketing repository: not configured")
	// ErrActivityNotFound means no Activity root exists for the requested ID.
	ErrActivityNotFound = errors.New("marketing repository: activity not found")
	// ErrActivityAlreadyExists means draft creation lost the root identity race.
	ErrActivityAlreadyExists = errors.New("marketing repository: activity already exists")
	// ErrActivityPublicationNotFound means the exact Activity/version history row
	// does not exist. Readers never substitute another version.
	ErrActivityPublicationNotFound = errors.New("marketing repository: activity publication not found")
	// ErrStoredActivityInvalid means persisted root state failed strict restore.
	ErrStoredActivityInvalid = errors.New("marketing repository: stored activity is invalid")
	// ErrStoredActivityPublicationInvalid means current or historical publication
	// rows failed strict restore.
	ErrStoredActivityPublicationInvalid = errors.New("marketing repository: stored activity publication is invalid")
	// ErrActivityStateConflict means expected lifecycle/state/active CAS lost or a
	// command was planned from a no-longer-current state.
	ErrActivityStateConflict = errors.New("marketing repository: activity state conflict")
	// ErrRepositoryRetryable identifies a transient transaction failure. Services
	// do not automatically replay high-risk publication commands.
	ErrRepositoryRetryable = errors.New("marketing repository: transaction may be retried")
	// ErrCommitOutcomeUnknown means COMMIT may already be durable. Blind retry is
	// unsafe; callers must use an exact read-back procedure.
	ErrCommitOutcomeUnknown = errors.New("marketing repository: commit outcome is unknown")
	// ErrRepositoryFailure covers unclassified storage, permission, schema, scan,
	// and transaction failures.
	ErrRepositoryFailure = errors.New("marketing repository: storage operation failed")
)

// RepositoryError preserves a trusted storage cause without rendering it or
// exposing it through errors.Unwrap.
type RepositoryError struct {
	class error
	cause error
}

// WrapRepositoryError builds a low-disclosure adapter error. Unknown classes
// fail closed as ErrRepositoryFailure.
func WrapRepositoryError(class, cause error) *RepositoryError {
	if !knownRepositoryErrorClass(class) {
		class = ErrRepositoryFailure
	}
	return &RepositoryError{class: class, cause: cause}
}

func (e *RepositoryError) Error() string {
	if e == nil || !knownRepositoryErrorClass(e.class) {
		return ErrRepositoryFailure.Error()
	}
	return e.class.Error()
}

// Is exposes only the reviewed semantic class.
func (e *RepositoryError) Is(target error) bool {
	if e == nil || !knownRepositoryErrorClass(e.class) {
		return target == ErrRepositoryFailure
	}
	return target == e.class
}

// Cause exposes the retained driver cause only to trusted diagnostics.
func (e *RepositoryError) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownRepositoryErrorClass(class error) bool {
	return class == ErrRepositoryInvalidArgument ||
		class == ErrRepositoryNotConfigured ||
		class == ErrActivityNotFound ||
		class == ErrActivityAlreadyExists ||
		class == ErrActivityPublicationNotFound ||
		class == ErrStoredActivityInvalid ||
		class == ErrStoredActivityPublicationInvalid ||
		class == ErrActivityStateConflict ||
		class == ErrRepositoryRetryable ||
		class == ErrCommitOutcomeUnknown ||
		class == ErrRepositoryFailure
}

// DependencyError is shared by approval and Lottery adapters. Ordinary
// rendering and errors.Is expose one reviewed class; the private cause is only
// available through Cause.
type DependencyError struct {
	class error
	cause error
}

// WrapApprovalError creates a safe Governance-adapter error.
func WrapApprovalError(class, cause error) *DependencyError {
	if class != ErrActivityApprovalRejected && class != ErrActivityApprovalUnavailable {
		class = ErrActivityApprovalUnavailable
	}
	return &DependencyError{class: class, cause: cause}
}

// WrapLotteryVerificationError creates a safe Lottery ACL-adapter error.
func WrapLotteryVerificationError(class, cause error) *DependencyError {
	if class != ErrLotteryPublicationInvalid && class != ErrLotteryPublicationUnavailable {
		class = ErrLotteryPublicationUnavailable
	}
	return &DependencyError{class: class, cause: cause}
}

func (e *DependencyError) Error() string {
	if e == nil || !knownDependencyErrorClass(e.class) {
		return ErrActivityOperationFailure.Error()
	}
	return e.class.Error()
}

// Is exposes only the reviewed dependency class.
func (e *DependencyError) Is(target error) bool {
	if e == nil || !knownDependencyErrorClass(e.class) {
		return target == ErrActivityOperationFailure
	}
	return target == e.class
}

// Cause returns the retained provider cause to trusted diagnostics.
func (e *DependencyError) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownDependencyErrorClass(class error) bool {
	return class == ErrActivityApprovalRejected ||
		class == ErrActivityApprovalUnavailable ||
		class == ErrLotteryPublicationInvalid ||
		class == ErrLotteryPublicationUnavailable
}

// ActivityOperationError prevents domain, storage, provider, and private
// deadline details from leaking through ordinary error chains.
type ActivityOperationError struct {
	class error
	cause error
}

func wrapActivityOperationError(class, cause error) *ActivityOperationError {
	if !knownActivityOperationErrorClass(class) {
		class = ErrActivityOperationFailure
	}
	return &ActivityOperationError{class: class, cause: cause}
}

func (e *ActivityOperationError) Error() string {
	if e == nil || !knownActivityOperationErrorClass(e.class) {
		return ErrActivityOperationFailure.Error()
	}
	return e.class.Error()
}

// Is exposes exactly one stable operation class.
func (e *ActivityOperationError) Is(target error) bool {
	if e == nil || !knownActivityOperationErrorClass(e.class) {
		return target == ErrActivityOperationFailure
	}
	return target == e.class
}

// Cause exposes a diagnostic cause only through explicit trusted inspection.
func (e *ActivityOperationError) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownActivityOperationErrorClass(class error) bool {
	return class == ErrActivityInvalidArgument ||
		class == ErrActivityNotConfigured ||
		class == ErrActivityClockInvalid ||
		class == ErrActivityOperationTimedOut ||
		class == ErrActivityOperationFailure ||
		class == ErrActivityResolutionInvalid ||
		class == ErrActivityPublicationCandidateInvalid ||
		class == ErrActivityRollbackTargetInvalid ||
		class == ErrActivityApprovalRejected ||
		class == ErrActivityApprovalUnavailable ||
		class == ErrActivityApprovalEvidenceInvalid ||
		class == ErrLotteryPublicationInvalid ||
		class == ErrLotteryPublicationUnavailable ||
		knownRepositoryErrorClass(class)
}
