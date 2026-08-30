package domain

import (
	"fmt"
	"slices"
)

const (
	// MaxStrategyRevisionBytes bounds a persisted Strategy snapshot revision.
	// The v1 grammar is [A-Za-z0-9][A-Za-z0-9._:-]{0,127}.
	MaxStrategyRevisionBytes = 128
)

// StrategyRevision identifies one immutable Strategy configuration snapshot.
// The token becomes content-bound through create-only persistence; it is not
// by itself a content hash, timestamp, or application release version.
type StrategyRevision string

// StrategySnapshotIdentity is the exact lookup key for one immutable Strategy
// configuration. StrategyID alone remains the reusable logical identity.
type StrategySnapshotIdentity struct {
	id       StrategyID
	revision StrategyRevision
}

// NewStrategySnapshotIdentity constructs a canonical exact Strategy snapshot
// lookup identity without trimming or otherwise rewriting its revision.
func NewStrategySnapshotIdentity(
	id StrategyID,
	revision string,
) (StrategySnapshotIdentity, error) {
	identity := StrategySnapshotIdentity{
		id:       id,
		revision: StrategyRevision(revision),
	}
	if err := identity.Validate(); err != nil {
		return StrategySnapshotIdentity{}, err
	}
	return identity, nil
}

// Validate rejects zero Strategy identity and revisions outside the closed v1
// ASCII grammar.
func (identity StrategySnapshotIdentity) Validate() error {
	if identity.id == 0 {
		return invalidStrategySnapshotIdentity(
			ErrStrategyIDRequired,
			"strategy id is required",
		)
	}
	if err := validateStrategyRevision(identity.revision); err != nil {
		return invalidStrategySnapshotIdentity(err, "revision is invalid")
	}
	return nil
}

// ID returns the stable logical Strategy identity.
func (identity StrategySnapshotIdentity) ID() StrategyID { return identity.id }

// Revision returns the exact immutable configuration revision token.
func (identity StrategySnapshotIdentity) Revision() StrategyRevision {
	return identity.revision
}

// StrategySnapshotSchemaVersion identifies the persisted snapshot shape. It is
// independent from StrategyRevision and from the cache projection schema.
type StrategySnapshotSchemaVersion uint16

const (
	// StrategySnapshotSchemaVersionV1 is the only Strategy snapshot shape this
	// code can construct or restore.
	StrategySnapshotSchemaVersionV1 StrategySnapshotSchemaVersion = 1
)

// StrategySnapshot binds one exact revision to a complete immutable Strategy
// aggregate. It carries configuration only: no Activity, publication status,
// time window, selection result, inventory, or Benefit state.
type StrategySnapshot struct {
	identity      StrategySnapshotIdentity
	schemaVersion StrategySnapshotSchemaVersion
	strategy      Strategy
}

// NewStrategySnapshot freezes an already constructed Strategy under an exact
// create-only revision. The Strategy is reconstructed to reject forged state
// and to own a canonical Award slice.
func NewStrategySnapshot(
	revision string,
	strategy Strategy,
) (StrategySnapshot, error) {
	canonical, err := canonicalStrategyForSnapshot(strategy)
	if err != nil {
		return StrategySnapshot{}, invalidStrategySnapshot(
			err,
			"strategy aggregate is invalid",
		)
	}
	identity, err := NewStrategySnapshotIdentity(canonical.ID(), revision)
	if err != nil {
		return StrategySnapshot{}, invalidStrategySnapshot(
			err,
			"identity is invalid",
		)
	}
	return StrategySnapshot{
		identity:      identity,
		schemaVersion: StrategySnapshotSchemaVersionV1,
		strategy:      canonical,
	}, nil
}

// RestoreStrategySnapshot reconstructs a persisted exact Strategy snapshot.
// Stored names and revisions are never normalized or repaired.
func RestoreStrategySnapshot(
	identity StrategySnapshotIdentity,
	schemaVersion StrategySnapshotSchemaVersion,
	name string,
	awards []Award,
) (StrategySnapshot, error) {
	if err := identity.Validate(); err != nil {
		return StrategySnapshot{}, invalidStrategySnapshot(
			err,
			"identity is invalid",
		)
	}
	if schemaVersion != StrategySnapshotSchemaVersionV1 {
		return StrategySnapshot{}, invalidStrategySnapshot(
			ErrStrategySnapshotSchemaUnsupported,
			"schema version %d is unsupported",
			schemaVersion,
		)
	}
	strategy, err := RestoreStrategy(identity.ID(), name, awards)
	if err != nil {
		return StrategySnapshot{}, invalidStrategySnapshot(
			err,
			"strategy aggregate is invalid",
		)
	}
	return StrategySnapshot{
		identity:      identity,
		schemaVersion: schemaVersion,
		strategy:      strategy,
	}, nil
}

// Validate rechecks the complete snapshot and rejects manually forged derived
// Strategy state as well as identity/schema mismatches.
func (snapshot StrategySnapshot) Validate() error {
	if err := snapshot.identity.Validate(); err != nil {
		return invalidStrategySnapshot(err, "identity is invalid")
	}
	if snapshot.schemaVersion != StrategySnapshotSchemaVersionV1 {
		return invalidStrategySnapshot(
			ErrStrategySnapshotSchemaUnsupported,
			"schema version %d is unsupported",
			snapshot.schemaVersion,
		)
	}
	if snapshot.strategy.ID() != snapshot.identity.ID() {
		return invalidStrategySnapshot(
			ErrStrategySnapshotIdentityInvalid,
			"snapshot and aggregate strategy ids differ",
		)
	}
	if _, err := canonicalStrategyForSnapshot(snapshot.strategy); err != nil {
		return invalidStrategySnapshot(err, "strategy aggregate is invalid")
	}
	return nil
}

// Identity returns the exact StrategyID/revision lookup key.
func (snapshot StrategySnapshot) Identity() StrategySnapshotIdentity {
	return snapshot.identity
}

// SchemaVersion returns the closed persisted snapshot shape marker.
func (snapshot StrategySnapshot) SchemaVersion() StrategySnapshotSchemaVersion {
	return snapshot.schemaVersion
}

// Strategy returns the immutable configuration aggregate. Strategy itself
// exposes its Awards only through defensive copies.
func (snapshot StrategySnapshot) Strategy() Strategy { return snapshot.strategy }

func canonicalStrategyForSnapshot(strategy Strategy) (Strategy, error) {
	canonical, err := RestoreStrategy(strategy.ID(), strategy.Name(), strategy.Awards())
	if err != nil {
		return Strategy{}, err
	}
	if canonical.TotalWeight() != strategy.TotalWeight() ||
		!strategySnapshotAwardsEqual(canonical.Awards(), strategy.Awards()) {
		return Strategy{}, ErrStrategySnapshotInvalid
	}
	return canonical, nil
}

func strategySnapshotAwardsEqual(left, right []Award) bool {
	return slices.Equal(left, right)
}

func validateStrategyRevision(revision StrategyRevision) error {
	value := string(revision)
	if len(value) == 0 || len(value) > MaxStrategyRevisionBytes {
		return ErrStrategySnapshotRevisionInvalid
	}
	for index, character := range []byte(value) {
		if isStrategyRevisionASCIILetterOrDigit(character) {
			continue
		}
		if index > 0 {
			switch character {
			case '.', '_', ':', '-':
				continue
			}
		}
		return ErrStrategySnapshotRevisionInvalid
	}
	return nil
}

func isStrategyRevisionASCIILetterOrDigit(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

func invalidStrategySnapshotIdentity(cause error, detail string) error {
	return fmt.Errorf(
		"%w: %w: %s",
		ErrStrategySnapshotIdentityInvalid,
		cause,
		detail,
	)
}

func invalidStrategySnapshot(cause error, format string, arguments ...any) error {
	return fmt.Errorf(
		"%w: %w: %s",
		ErrStrategySnapshotInvalid,
		cause,
		fmt.Sprintf(format, arguments...),
	)
}
