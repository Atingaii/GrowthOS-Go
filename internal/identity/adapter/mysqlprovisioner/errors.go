package mysqlprovisioner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
)

var (
	// ErrInvalidArgument reports a nil context or invalid account aggregate.
	ErrInvalidArgument = errors.New("identity mysql provisioner: invalid argument")
	// ErrNotConfigured reports an adapter without a usable database handle.
	ErrNotConfigured = errors.New("identity mysql provisioner: not configured")
	// ErrAlreadyExists reports a conflict with one of the three reviewed account
	// identities. Create is never an upsert and never treats this as success.
	ErrAlreadyExists = errors.New("identity mysql provisioner: account already exists")
	// ErrDependencyUnavailable covers every unreviewed storage or driver failure.
	ErrDependencyUnavailable = errors.New("identity mysql provisioner: dependency unavailable")
	// ErrCommitOutcomeUnknown means COMMIT may already be durable. Retrying the
	// create blindly is unsafe.
	ErrCommitOutcomeUnknown = errors.New("identity mysql provisioner: commit outcome is unknown")
	// ErrOperationCanceled is returned only outside COMMIT, or when the COMMIT
	// error proves that cancellation prevented a successful commit.
	ErrOperationCanceled = errors.New("identity mysql provisioner: operation canceled")
)

// Error retains a private cause for explicit trusted diagnostics while every
// ordinary formatting, logging, JSON, and errors.Is boundary exposes only a
// reviewed stable class. It intentionally has no Unwrap method.
type Error struct {
	class        error
	cause        error
	contextClass error
}

func newError(class, cause error) *Error {
	if !knownClass(class) {
		class = ErrDependencyUnavailable
	}
	return &Error{class: class, cause: cause}
}

func canceledError(cause error) *Error {
	contextClass := error(nil)
	switch {
	case errors.Is(cause, context.Canceled):
		contextClass = context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		contextClass = context.DeadlineExceeded
	}
	return &Error{
		class:        ErrOperationCanceled,
		cause:        cause,
		contextClass: contextClass,
	}
}

func (provisionError *Error) Error() string {
	if provisionError == nil || !knownClass(provisionError.class) {
		return ErrDependencyUnavailable.Error()
	}
	return provisionError.class.Error()
}

func (provisionError *Error) GoString() string { return provisionError.Error() }

func (provisionError *Error) LogValue() slog.Value {
	return slog.StringValue(provisionError.Error())
}

func (provisionError *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(provisionError.Error())
}

func (provisionError *Error) Is(target error) bool {
	if provisionError == nil || !knownClass(provisionError.class) {
		return target == ErrDependencyUnavailable
	}
	return target == provisionError.class ||
		(provisionError.contextClass != nil && target == provisionError.contextClass)
}

// Cause returns the retained dependency detail only through explicit trusted
// inspection. Ordinary errors.Is traversal cannot reach it.
func (provisionError *Error) Cause() error {
	if provisionError == nil {
		return nil
	}
	return provisionError.cause
}

func knownClass(class error) bool {
	switch class {
	case ErrInvalidArgument,
		ErrNotConfigured,
		ErrAlreadyExists,
		ErrDependencyUnavailable,
		ErrCommitOutcomeUnknown,
		ErrOperationCanceled:
		return true
	default:
		return false
	}
}
