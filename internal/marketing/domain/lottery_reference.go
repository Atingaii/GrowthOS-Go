package domain

import (
	"cmp"
	"fmt"
	"slices"
)

const (
	// MaxStrategyRevisionManifestEntries bounds exact Strategy resolution work.
	MaxStrategyRevisionManifestEntries = 128
)

// LotteryGraphID is Marketing's local foreign-key representation of a
// Lottery-owned routing graph. It deliberately is not a Lottery aggregate type.
type LotteryGraphID uint64

// LotteryStrategyID is Marketing's local foreign-key representation of a
// Lottery-owned Strategy.
type LotteryStrategyID uint64

// LotteryRevision is an opaque exact Lottery snapshot revision token.
type LotteryRevision string

// LotteryGraphReference identifies one exact immutable Lottery graph snapshot.
type LotteryGraphReference struct {
	id       LotteryGraphID
	revision LotteryRevision
}

// NewLotteryGraphReference constructs a validated exact foreign graph identity.
func NewLotteryGraphReference(id LotteryGraphID, revision string) (LotteryGraphReference, error) {
	reference := LotteryGraphReference{id: id, revision: LotteryRevision(revision)}
	if err := reference.Validate(); err != nil {
		return LotteryGraphReference{}, err
	}
	return reference, nil
}

// Validate checks identity shape only. Existence and ownership remain Lottery
// or repository responsibilities.
func (reference LotteryGraphReference) Validate() error {
	if reference.id == 0 {
		return fmt.Errorf("%w: graph id is required", ErrLotteryGraphReferenceInvalid)
	}
	if err := validateRevisionToken(string(reference.revision)); err != nil {
		return fmt.Errorf("%w: revision %v", ErrLotteryGraphReferenceInvalid, err)
	}
	return nil
}

// ID returns the foreign graph identity.
func (reference LotteryGraphReference) ID() LotteryGraphID { return reference.id }

// Revision returns the exact foreign graph revision token.
func (reference LotteryGraphReference) Revision() LotteryRevision { return reference.revision }

// LotteryStrategyRevisionReference identifies one exact immutable Lottery
// Strategy snapshot used by a graph terminal.
type LotteryStrategyRevisionReference struct {
	strategyID LotteryStrategyID
	revision   LotteryRevision
}

// NewLotteryStrategyRevisionReference constructs one exact foreign Strategy
// snapshot identity.
func NewLotteryStrategyRevisionReference(
	strategyID LotteryStrategyID,
	revision string,
) (LotteryStrategyRevisionReference, error) {
	reference := LotteryStrategyRevisionReference{
		strategyID: strategyID,
		revision:   LotteryRevision(revision),
	}
	if err := reference.Validate(); err != nil {
		return LotteryStrategyRevisionReference{}, err
	}
	return reference, nil
}

// Validate checks identity shape only; it never loads a Lottery Strategy.
func (reference LotteryStrategyRevisionReference) Validate() error {
	if reference.strategyID == 0 {
		return fmt.Errorf("%w: Strategy id is required", ErrLotteryStrategyRevisionReferenceInvalid)
	}
	if err := validateRevisionToken(string(reference.revision)); err != nil {
		return fmt.Errorf("%w: revision %v", ErrLotteryStrategyRevisionReferenceInvalid, err)
	}
	return nil
}

// StrategyID returns the foreign Strategy identity.
func (reference LotteryStrategyRevisionReference) StrategyID() LotteryStrategyID {
	return reference.strategyID
}

// Revision returns the exact foreign Strategy revision token.
func (reference LotteryStrategyRevisionReference) Revision() LotteryRevision {
	return reference.revision
}

func canonicalStrategyRevisionManifest(
	manifest []LotteryStrategyRevisionReference,
) ([]LotteryStrategyRevisionReference, error) {
	if len(manifest) == 0 {
		return nil, fmt.Errorf("%w: at least one Strategy revision is required", ErrStrategyRevisionManifestInvalid)
	}
	if len(manifest) > MaxStrategyRevisionManifestEntries {
		return nil, fmt.Errorf(
			"%w: entry count %d exceeds %d",
			ErrStrategyRevisionManifestLimitExceeded,
			len(manifest),
			MaxStrategyRevisionManifestEntries,
		)
	}
	canonical := append([]LotteryStrategyRevisionReference(nil), manifest...)
	for index, reference := range canonical {
		if err := reference.Validate(); err != nil {
			return nil, fmt.Errorf("%w: entry %d: %v", ErrStrategyRevisionManifestInvalid, index, err)
		}
	}
	slices.SortFunc(canonical, compareLotteryStrategyRevisionReferences)
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].strategyID == canonical[index].strategyID {
			return nil, fmt.Errorf(
				"%w: Strategy id %d is duplicated",
				ErrStrategyRevisionManifestInvalid,
				canonical[index].strategyID,
			)
		}
	}
	return canonical, nil
}

func validateCanonicalStrategyRevisionManifest(
	manifest []LotteryStrategyRevisionReference,
) error {
	canonical, err := canonicalStrategyRevisionManifest(manifest)
	if err != nil {
		return err
	}
	if !slices.Equal(canonical, manifest) {
		return fmt.Errorf("%w: entries are not in canonical Strategy-id order", ErrStrategyRevisionManifestInvalid)
	}
	return nil
}

func compareLotteryStrategyRevisionReferences(
	left LotteryStrategyRevisionReference,
	right LotteryStrategyRevisionReference,
) int {
	if result := cmp.Compare(left.strategyID, right.strategyID); result != 0 {
		return result
	}
	return cmp.Compare(left.revision, right.revision)
}
