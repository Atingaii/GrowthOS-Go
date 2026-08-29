package domain

import (
	"fmt"
	"reflect"
)

// BoundedRandomSource returns a uniformly distributed value in the half-open
// interval [0, upper). Implementations must reject upper == 0 and must never
// repair an out-of-range value with modulo arithmetic. A source shared by
// concurrent selectors or calls must itself be safe for concurrent use.
type BoundedRandomSource interface {
	Uint64N(upper uint64) (uint64, error)
}

// WeightedSelector selects an Award using at most one unbiased bounded random
// value. A one-Award Strategy is deterministic and bypasses the source. The
// selector does not load a Strategy, persist a draw result, reserve inventory,
// or deliver a benefit. It has no mutable state of its own and is safe for
// concurrent use when its BoundedRandomSource is safe for concurrent use.
type WeightedSelector struct {
	source BoundedRandomSource
}

// NewWeightedSelector creates a selector with an explicitly supplied entropy
// boundary. Injection keeps interval mapping deterministic in tests.
func NewWeightedSelector(source BoundedRandomSource) (*WeightedSelector, error) {
	if boundedRandomSourceIsNil(source) {
		return nil, newSelectionError(ErrSelectorNotConfigured, nil)
	}
	return &WeightedSelector{source: source}, nil
}

// Select chooses exactly one Award according to its relative Weight.
//
// Strategy construction already sorts Awards by AwardID and checks their sum.
// Subtracting each weight from the ticket avoids total+1 and cumulative-add
// overflow, including when TotalWeight is math.MaxUint64.
func (s *WeightedSelector) Select(strategy Strategy) (Award, error) {
	if s == nil || s.source == nil {
		return Award{}, newSelectionError(ErrSelectorNotConfigured, nil)
	}
	if len(strategy.awards) == 0 || strategy.totalWeight == 0 {
		return Award{}, newSelectionError(ErrSelectionStrategyInvalid, ErrStrategyAwardsRequired)
	}
	if len(strategy.awards) == 1 {
		if uint64(strategy.awards[0].weight) != strategy.totalWeight {
			return Award{}, newSelectionError(
				ErrSelectionInvariantViolation,
				unmappedStrategyCause(strategy),
			)
		}
		return strategy.awards[0], nil
	}

	ticket, err := s.source.Uint64N(strategy.totalWeight)
	if err != nil {
		return Award{}, newSelectionError(ErrRandomSourceFailure, err)
	}
	if ticket >= strategy.totalWeight {
		cause := fmt.Errorf("bounded random value %d is outside [0,%d)", ticket, strategy.totalWeight)
		return Award{}, newSelectionError(ErrRandomSourceContractViolation, cause)
	}

	for _, award := range strategy.awards {
		weight := uint64(award.weight)
		if ticket < weight {
			return award, nil
		}
		ticket -= weight
	}

	return Award{}, newSelectionError(
		ErrSelectionInvariantViolation,
		unmappedStrategyCause(strategy),
	)
}

func unmappedStrategyCause(strategy Strategy) error {
	return fmt.Errorf(
		"no award matched strategy %d with declared total weight %d",
		strategy.id,
		strategy.totalWeight,
	)
}

func boundedRandomSourceIsNil(source BoundedRandomSource) bool {
	if source == nil {
		return true
	}
	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
