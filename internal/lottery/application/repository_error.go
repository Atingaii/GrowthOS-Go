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
	// ErrStrategySnapshotNotFound means no immutable Strategy configuration
	// exists for the requested StrategyID/revision identity.
	ErrStrategySnapshotNotFound = errors.New("lottery repository: strategy snapshot not found")
	// ErrStrategySnapshotAlreadyExists means a create-only StrategyID/revision
	// identity is already durable and must never be overwritten or upserted.
	ErrStrategySnapshotAlreadyExists = errors.New("lottery repository: strategy snapshot already exists")
	// ErrStoredStrategySnapshotInvalid means readable rows could not reconstruct
	// one complete immutable Strategy snapshot. Callers must fail closed.
	ErrStoredStrategySnapshotInvalid = errors.New("lottery repository: stored strategy snapshot is invalid")
	// ErrStrategyRoutingGraphNotFound means no immutable graph snapshot exists
	// for the requested GraphID/revision identity.
	ErrStrategyRoutingGraphNotFound = errors.New("lottery repository: strategy routing graph not found")
	// ErrStrategyRoutingGraphAlreadyExists means Create was attempted for a
	// GraphID/revision identity that is already durable. Existing content must
	// never be overwritten or treated as an upsert.
	ErrStrategyRoutingGraphAlreadyExists = errors.New("lottery repository: strategy routing graph already exists")
	// ErrStoredStrategyRoutingGraphInvalid means persisted rows were readable but
	// could not restore one valid immutable rooted graph. Callers fail closed and
	// must not execute or automatically repair the stored topology.
	ErrStoredStrategyRoutingGraphInvalid = errors.New("lottery repository: stored strategy routing graph is invalid")
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
		class == ErrStrategySnapshotNotFound ||
		class == ErrStrategySnapshotAlreadyExists ||
		class == ErrStoredStrategySnapshotInvalid ||
		class == ErrStrategyRoutingGraphNotFound ||
		class == ErrStrategyRoutingGraphAlreadyExists ||
		class == ErrStoredStrategyRoutingGraphInvalid ||
		class == ErrRepositoryRetryable ||
		class == ErrCommitOutcomeUnknown ||
		class == ErrRepositoryFailure
}
