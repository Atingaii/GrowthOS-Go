package domain

import "errors"

var (
	// ErrStrategySnapshotInvalid is the stable fail-closed class for a Strategy
	// configuration snapshot that cannot be trusted as immutable Lottery input.
	ErrStrategySnapshotInvalid = errors.New("lottery: strategy snapshot is invalid")
	// ErrStrategySnapshotIdentityInvalid reports a missing Strategy identity or
	// a revision outside the canonical v1 ASCII grammar.
	ErrStrategySnapshotIdentityInvalid = errors.New("lottery: strategy snapshot identity is invalid")
	// ErrStrategySnapshotRevisionInvalid reports a revision token outside the
	// bounded grammar [A-Za-z0-9][A-Za-z0-9._:-]{0,127}.
	ErrStrategySnapshotRevisionInvalid = errors.New("lottery: strategy snapshot revision is invalid")
	// ErrStrategySnapshotSchemaUnsupported reports a zero, future, or otherwise
	// unknown persisted snapshot shape.
	ErrStrategySnapshotSchemaUnsupported = errors.New("lottery: strategy snapshot schema is unsupported")
)
