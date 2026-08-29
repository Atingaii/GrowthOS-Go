// Package dbmigration provides a deliberately forward-only database migration
// boundary. It does not expose down, drop, force, or arbitrary-version APIs.
package dbmigration

import "errors"

// Stage is a stable, non-sensitive migration failure identifier.
type Stage string

const (
	StageConfigInvalid   Stage = "migration_config_invalid"
	StageSourceInvalid   Stage = "migration_source_invalid"
	StageOpen            Stage = "migration_open"
	StageStatus          Stage = "migration_status"
	StageVersionMismatch Stage = "migration_version_mismatch"
	StageDirty           Stage = "migration_dirty"
	StageApply           Stage = "migration_apply"
	StageCancelled       Stage = "migration_cancelled"
	StageClose           Stage = "migration_close"
)

var (
	// ErrDirty is safe for errors.Is checks at command boundaries.
	ErrDirty = errors.New(string(StageDirty))
	// ErrCancelled distinguishes a caller-requested graceful stop.
	ErrCancelled = errors.New(string(StageCancelled))
	// ErrVersionMismatch means the database reports a clean version that is
	// absent from, or newer than, the embedded forward-only history.
	ErrVersionMismatch = errors.New(string(StageVersionMismatch))
)

// Error renders only its stable stage. Its cause is available to trusted code
// through errors.Is/errors.As but must not be serialized or logged blindly.
type Error struct {
	stage Stage
	cause error
}

func newError(stage Stage, cause error) *Error {
	return &Error{stage: stage, cause: cause}
}

func (e *Error) Error() string {
	if e == nil || e.stage == "" {
		return string(StageConfigInvalid)
	}
	return string(e.stage)
}

func (e *Error) Stage() Stage {
	if e == nil || e.stage == "" {
		return StageConfigInvalid
	}
	return e.stage
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// IsStage safely inspects a wrapped migration Error.
func IsStage(err error, stage Stage) bool {
	var migrationErr *Error
	return errors.As(err, &migrationErr) && migrationErr.Stage() == stage
}
