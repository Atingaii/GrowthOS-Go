package application

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
)

var (
	// Public application classes. Their strings are deliberately stable and
	// contain no account, login, token, digest, session, SQL, or provider data.
	ErrInvalidArgument           = errors.New("identity application: invalid argument")
	ErrNotConfigured             = errors.New("identity application: not configured")
	ErrAuthenticationFailed      = errors.New("identity application: authentication failed")
	ErrUnauthenticated           = errors.New("identity application: unauthenticated")
	ErrAuthenticationThrottled   = errors.New("identity application: authentication throttled")
	ErrAuthenticationUnavailable = errors.New("identity application: authentication unavailable")
	ErrCommitOutcomeUnknown      = errors.New("identity application: commit outcome is unknown")
	ErrRevocationIndeterminate   = errors.New("identity application: session revocation is indeterminate")
	ErrOperationCanceled         = errors.New("identity application: operation canceled")

	// Adapter/repository contract classes. Adapters wrap private causes with
	// WrapDependencyError; ordinary rendering and unwrapping expose only these
	// reviewed classes.
	ErrAccountNotFound           = errors.New("identity dependency: account not found")
	ErrSessionNotFound           = errors.New("identity dependency: session not found")
	ErrSessionInactive           = errors.New("identity dependency: session inactive")
	ErrAdmissionRejected         = errors.New("identity dependency: admission rejected")
	ErrAdmissionStale            = errors.New("identity dependency: admission reservation stale")
	ErrTokenDigestCollision      = errors.New("identity dependency: token digest collision")
	ErrAccountStateConflict      = errors.New("identity dependency: account state conflict")
	ErrStoredIdentityInvalid     = errors.New("identity dependency: stored identity is invalid")
	ErrDependencyUnavailable     = errors.New("identity dependency: unavailable")
	ErrDependencyInvalidArgument = errors.New("identity dependency: invalid argument")
)

// DependencyError retains an adapter cause for explicit trusted diagnostics.
// It intentionally has no Unwrap method.
type DependencyError struct {
	class error
	cause error
}

// WrapDependencyError constructs a low-disclosure adapter error. Unknown
// classes fail closed as ErrDependencyUnavailable.
func WrapDependencyError(class, cause error) *DependencyError {
	if !knownDependencyClass(class) {
		class = ErrDependencyUnavailable
	}
	return &DependencyError{class: class, cause: cause}
}

func (dependencyError *DependencyError) Error() string {
	if dependencyError == nil || !knownDependencyClass(dependencyError.class) {
		return ErrDependencyUnavailable.Error()
	}
	return dependencyError.class.Error()
}

func (dependencyError *DependencyError) GoString() string { return dependencyError.Error() }

func (dependencyError *DependencyError) LogValue() slog.Value {
	return slog.StringValue(dependencyError.Error())
}

func (dependencyError *DependencyError) MarshalJSON() ([]byte, error) {
	return json.Marshal(dependencyError.Error())
}

func (dependencyError *DependencyError) Is(target error) bool {
	if dependencyError == nil || !knownDependencyClass(dependencyError.class) {
		return target == ErrDependencyUnavailable
	}
	return target == dependencyError.class
}

// Cause is available only through explicit trusted inspection and is never in
// Error or the standard errors.Unwrap chain.
func (dependencyError *DependencyError) Cause() error {
	if dependencyError == nil {
		return nil
	}
	return dependencyError.cause
}

func knownDependencyClass(class error) bool {
	switch class {
	case ErrAccountNotFound,
		ErrSessionNotFound,
		ErrSessionInactive,
		ErrAdmissionRejected,
		ErrAdmissionStale,
		ErrTokenDigestCollision,
		ErrAccountStateConflict,
		ErrStoredIdentityInvalid,
		ErrDependencyUnavailable,
		ErrDependencyInvalidArgument,
		ErrCommitOutcomeUnknown:
		return true
	default:
		return false
	}
}

// OperationError prevents private dependency details and recovery receipts
// from entering ordinary error chains.
type OperationError struct {
	class      error
	cause      error
	receipt    SessionCommitReceipt
	hasReceipt bool
}

func wrapOperationError(class, cause error) *OperationError {
	if !knownOperationClass(class) {
		class = ErrAuthenticationUnavailable
	}
	return &OperationError{class: class, cause: cause}
}

func wrapOperationErrorWithReceipt(
	class error,
	cause error,
	receipt SessionCommitReceipt,
) *OperationError {
	if (class != ErrCommitOutcomeUnknown && class != ErrRevocationIndeterminate) ||
		receipt.Validate() != nil {
		return wrapOperationError(ErrAuthenticationUnavailable, cause)
	}
	return &OperationError{
		class:      class,
		cause:      cause,
		receipt:    receipt,
		hasReceipt: true,
	}
}

func (operationError *OperationError) Error() string {
	if operationError == nil || !knownOperationClass(operationError.class) {
		return ErrAuthenticationUnavailable.Error()
	}
	return operationError.class.Error()
}

func (operationError *OperationError) GoString() string { return operationError.Error() }

func (operationError *OperationError) LogValue() slog.Value {
	return slog.StringValue(operationError.Error())
}

func (operationError *OperationError) MarshalJSON() ([]byte, error) {
	return json.Marshal(operationError.Error())
}

func (operationError *OperationError) Is(target error) bool {
	if operationError == nil || !knownOperationClass(operationError.class) {
		return target == ErrAuthenticationUnavailable
	}
	return target == operationError.class
}

// Cause returns the retained cause only through explicit trusted inspection.
func (operationError *OperationError) Cause() error {
	if operationError == nil {
		return nil
	}
	return operationError.cause
}

// CommitReceipt returns an immutable receipt only for commit-outcome-unknown.
func (operationError *OperationError) CommitReceipt() (SessionCommitReceipt, bool) {
	if operationError == nil ||
		(operationError.class != ErrCommitOutcomeUnknown &&
			operationError.class != ErrRevocationIndeterminate) ||
		!operationError.hasReceipt || operationError.receipt.Validate() != nil {
		return SessionCommitReceipt{}, false
	}
	return operationError.receipt, true
}

// SessionCommitReceiptFromError explicitly inspects trusted operation errors.
func SessionCommitReceiptFromError(err error) (SessionCommitReceipt, bool) {
	var operationError *OperationError
	if !errors.As(err, &operationError) {
		return SessionCommitReceipt{}, false
	}
	return operationError.CommitReceipt()
}

func knownOperationClass(class error) bool {
	switch class {
	case ErrInvalidArgument,
		ErrNotConfigured,
		ErrAuthenticationFailed,
		ErrUnauthenticated,
		ErrAuthenticationThrottled,
		ErrAuthenticationUnavailable,
		ErrCommitOutcomeUnknown,
		ErrRevocationIndeterminate,
		ErrOperationCanceled:
		return true
	default:
		return false
	}
}

func canceledOperationError(ctx context.Context, cause error) error {
	if ctx != nil && ctx.Err() != nil {
		return wrapOperationError(ErrOperationCanceled, ctx.Err())
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return wrapOperationError(ErrOperationCanceled, cause)
	}
	return nil
}
