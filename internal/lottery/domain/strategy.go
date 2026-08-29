package domain

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

// StrategyID identifies one reusable Lottery decision configuration. It is not
// a Marketing activity identifier.
type StrategyID uint64

// Strategy is the aggregate root for the weighted outcomes that may participate
// in one draw. Construction establishes all invariants needed by a later draw
// algorithm without choosing that algorithm here.
type Strategy struct {
	id          StrategyID
	name        string
	awards      []Award
	totalWeight uint64
}

// NewStrategy constructs an immutable Strategy and defensively copies awards.
func NewStrategy(id StrategyID, name string, awards []Award) (Strategy, error) {
	if id == 0 {
		return Strategy{}, ErrStrategyIDRequired
	}

	name, err := normalizeName(
		name,
		MaxStrategyNameRunes,
		ErrStrategyNameRequired,
		ErrStrategyNameInvalid,
		ErrStrategyNameTooLong,
	)
	if err != nil {
		return Strategy{}, err
	}
	return RestoreStrategy(id, name, awards)
}

// RestoreStrategy reconstructs a Strategy from an already canonical
// persistence snapshot. It deliberately rejects names that would require
// trimming, so loading cannot silently change stored facts.
func RestoreStrategy(id StrategyID, name string, awards []Award) (Strategy, error) {
	if id == 0 {
		return Strategy{}, ErrStrategyIDRequired
	}
	if err := validateCanonicalName(
		name,
		MaxStrategyNameRunes,
		ErrStrategyNameRequired,
		ErrStrategyNameInvalid,
		ErrStrategyNameTooLong,
	); err != nil {
		return Strategy{}, err
	}
	if len(awards) == 0 {
		return Strategy{}, ErrStrategyAwardsRequired
	}

	awardIDs := make(map[AwardID]struct{}, len(awards))
	var totalWeight uint64
	for _, award := range awards {
		if err := award.validate(); err != nil {
			return Strategy{}, err
		}
		if _, exists := awardIDs[award.ID()]; exists {
			return Strategy{}, fmt.Errorf("%w: %d", ErrDuplicateAwardID, award.ID())
		}
		awardIDs[award.ID()] = struct{}{}

		weight := uint64(award.Weight())
		if weight > math.MaxUint64-totalWeight {
			return Strategy{}, ErrTotalWeightOverflow
		}
		totalWeight += weight
	}

	ownedAwards := append([]Award(nil), awards...)
	slices.SortFunc(ownedAwards, func(left, right Award) int {
		return cmp.Compare(left.ID(), right.ID())
	})
	return Strategy{
		id:          id,
		name:        name,
		awards:      ownedAwards,
		totalWeight: totalWeight,
	}, nil
}

// ID returns the strategy's durable identity.
func (s Strategy) ID() StrategyID { return s.id }

// Name returns the strategy's operator-facing name.
func (s Strategy) Name() string { return s.name }

// Awards returns a defensive copy in canonical AwardID order. Caller and
// persistence iteration order are not part of the domain state.
func (s Strategy) Awards() []Award { return append([]Award(nil), s.awards...) }

// TotalWeight returns the checked sum used as the range of a later weighted draw.
func (s Strategy) TotalWeight() uint64 { return s.totalWeight }

// Award finds one configured outcome by identity.
func (s Strategy) Award(id AwardID) (Award, bool) {
	for _, award := range s.awards {
		if award.ID() == id {
			return award, true
		}
	}
	return Award{}, false
}
