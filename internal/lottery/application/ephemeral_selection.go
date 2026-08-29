package application

import (
	"context"
	"errors"
	"reflect"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

var (
	// ErrSelectionInvalidArgument reports a caller contract violation such as a
	// nil context or a zero StrategyID. HTTP adapters should validate those
	// values before invoking the use case.
	ErrSelectionInvalidArgument = errors.New("lottery ephemeral selection: invalid argument")
	// ErrSelectionNotConfigured means a required reader or selector was absent
	// at composition time. It is never a no_reward business outcome.
	ErrSelectionNotConfigured = errors.New("lottery ephemeral selection: not configured")
	// ErrSelectionResultInvalid means a dependency returned a Strategy or Award
	// that does not match the requested aggregate snapshot. The use case fails
	// closed rather than exposing a cross-strategy or invented result.
	ErrSelectionResultInvalid = errors.New("lottery ephemeral selection: result is invalid")
)

// AwardSelector is owned by the application consumer. The domain
// WeightedSelector satisfies this narrow port without learning about storage,
// HTTP, or the use case that invokes it.
type AwardSelector interface {
	Select(strategy domain.Strategy) (domain.Award, error)
}

// EphemeralSelection is the synchronous, non-durable output of one use-case
// invocation. It is deliberately not named DrawResult: no identity or final
// result has been persisted, and a later invocation may select again.
type EphemeralSelection struct {
	Strategy domain.Strategy
	Award    domain.Award
}

// EphemeralSelectionService loads one canonical Strategy snapshot and selects
// one configured Award. It has no mutable state of its own; concurrent safety
// is conditional on the injected reader and selector.
type EphemeralSelectionService struct {
	strategies StrategyReader
	selector   AwardSelector
}

// NewEphemeralSelectionService wires the smallest ports required by the first
// Lottery API. Nil and typed-nil dependencies are rejected during composition.
func NewEphemeralSelectionService(
	strategies StrategyReader,
	selector AwardSelector,
) (*EphemeralSelectionService, error) {
	if dependencyIsNil(strategies) || dependencyIsNil(selector) {
		return nil, ErrSelectionNotConfigured
	}
	return &EphemeralSelectionService{
		strategies: strategies,
		selector:   selector,
	}, nil
}

// Validate reports whether all required dependencies were supplied through the
// constructor. Inbound adapters call it during startup so a manually-created
// zero value cannot leave readiness green while every business request fails.
func (service *EphemeralSelectionService) Validate() error {
	if service == nil || dependencyIsNil(service.strategies) || dependencyIsNil(service.selector) {
		return ErrSelectionNotConfigured
	}
	return nil
}

// Select executes one explicitly ephemeral selection. Context cancellation can
// stop the repository read and prevents selection when observed before the
// selector call. The synchronous selector port itself is not interruptible.
func (service *EphemeralSelectionService) Select(
	ctx context.Context,
	strategyID domain.StrategyID,
) (EphemeralSelection, error) {
	if ctx == nil || strategyID == 0 {
		return EphemeralSelection{}, ErrSelectionInvalidArgument
	}
	if err := service.Validate(); err != nil {
		return EphemeralSelection{}, err
	}
	if err := ctx.Err(); err != nil {
		return EphemeralSelection{}, err
	}

	strategy, err := service.strategies.FindByID(ctx, strategyID)
	if contextError := ctx.Err(); contextError != nil {
		return EphemeralSelection{}, contextError
	}
	if err != nil {
		return EphemeralSelection{}, err
	}
	if strategy.ID() != strategyID {
		return EphemeralSelection{}, ErrSelectionResultInvalid
	}
	if err := ctx.Err(); err != nil {
		return EphemeralSelection{}, err
	}

	award, err := service.selector.Select(strategy)
	if contextError := ctx.Err(); contextError != nil {
		return EphemeralSelection{}, contextError
	}
	if err != nil {
		return EphemeralSelection{}, err
	}
	configuredAward, found := strategy.Award(award.ID())
	if !found || !sameAward(configuredAward, award) {
		return EphemeralSelection{}, ErrSelectionResultInvalid
	}
	if err := ctx.Err(); err != nil {
		return EphemeralSelection{}, err
	}

	return EphemeralSelection{Strategy: strategy, Award: award}, nil
}

func sameAward(left, right domain.Award) bool {
	return left.ID() == right.ID() &&
		left.Name() == right.Name() &&
		left.Weight() == right.Weight() &&
		left.Outcome() == right.Outcome()
}

func dependencyIsNil(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
