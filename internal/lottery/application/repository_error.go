package application

import "errors"

var (
	// ErrRepositoryInvalidArgument reports a programmer-facing call contract
	// violation that is not a valid domain value, such as a nil context.
	ErrRepositoryInvalidArgument = errors.New("lottery repository: invalid argument")
	// ErrRepositoryNotConfigured means the adapter has no usable storage handle.
	ErrRepositoryNotConfigured = errors.New("lottery repository: not configured")
	// ErrStrategyNotFound means no aggregate root exists for the requested ID.
	ErrStrategyNotFound = errors.New("lottery repository: strategy not found")
	// ErrStrategyAlreadyExists means Create lost an identity uniqueness race or
	// was called for an identity that is already durable.
	ErrStrategyAlreadyExists = errors.New("lottery repository: strategy already exists")
	// ErrStoredStrategyInvalid means rows were readable but could not reconstruct
	// one valid Strategy aggregate. Callers must fail closed.
	ErrStoredStrategyInvalid = errors.New("lottery repository: stored strategy is invalid")
	// ErrRepositoryRetryable identifies a transient transaction failure such as
	// a deadlock or lock wait timeout. The adapter itself never retries.
	ErrRepositoryRetryable = errors.New("lottery repository: transaction may be retried")
	// ErrCommitOutcomeUnknown means a write commit returned an error after the
	// server may already have made the transaction durable. Blind retry is unsafe.
	ErrCommitOutcomeUnknown = errors.New("lottery repository: commit outcome is unknown")
	// ErrRepositoryFailure covers SQL, permission, schema, scan, or dependency
	// failures without claiming that retrying will make them succeed.
	ErrRepositoryFailure = errors.New("lottery repository: storage operation failed")
)

// RepositoryError retains a diagnostic cause for errors.Is/errors.As while its
// rendered message exposes only a reviewed semantic class.
type RepositoryError struct {
	class error
	cause error
}

// WrapRepositoryError builds a safe repository error for an adapter. Unknown
// classes fail closed as ErrRepositoryFailure.
func WrapRepositoryError(class, cause error) *RepositoryError {
	if !knownRepositoryError(class) {
		class = ErrRepositoryFailure
	}
	return &RepositoryError{class: class, cause: cause}
}

func (e *RepositoryError) Error() string {
	if e == nil || !knownRepositoryError(e.class) {
		return ErrRepositoryFailure.Error()
	}
	return e.class.Error()
}

// Is makes the semantic class usable with errors.Is without rendering the
// driver error or SQL text.
func (e *RepositoryError) Is(target error) bool {
	if e == nil || !knownRepositoryError(e.class) {
		return target == ErrRepositoryFailure
	}
	return target == e.class
}

// Unwrap preserves cancellation and driver error inspection for trusted code.
func (e *RepositoryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownRepositoryError(class error) bool {
	return class == ErrRepositoryInvalidArgument ||
		class == ErrRepositoryNotConfigured ||
		class == ErrStrategyNotFound ||
		class == ErrStrategyAlreadyExists ||
		class == ErrStoredStrategyInvalid ||
		class == ErrRepositoryRetryable ||
		class == ErrCommitOutcomeUnknown ||
		class == ErrRepositoryFailure
}
