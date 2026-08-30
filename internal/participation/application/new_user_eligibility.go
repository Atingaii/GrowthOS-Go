package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

// NewUserEligibilityService loads one authoritative registration fact, applies
// a bounded freshness contract, and invokes the one concrete domain evaluator.
// It is immutable; concurrent safety depends on the injected reader and clock.
type NewUserEligibilityService struct {
	facts      RegistrationFactReader
	clock      Clock
	maxFactAge time.Duration
}

// NewNewUserEligibilityService wires the minimum dependencies proved by the
// first rule. Policy remains an explicit per-call value so this service does not
// invent a global Activity or ruleset before those contexts exist.
func NewNewUserEligibilityService(
	facts RegistrationFactReader,
	clock Clock,
	maxFactAge time.Duration,
) (*NewUserEligibilityService, error) {
	if dependencyIsNil(facts) || dependencyIsNil(clock) || maxFactAge <= 0 {
		return nil, ErrEligibilityNotConfigured
	}
	return &NewUserEligibilityService{
		facts:      facts,
		clock:      clock,
		maxFactAge: maxFactAge,
	}, nil
}

// Validate prevents a manually constructed zero or typed-nil service from
// leaving a future runtime apparently ready while every decision fails.
func (service *NewUserEligibilityService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.facts) ||
		dependencyIsNil(service.clock) ||
		service.maxFactAge <= 0 {
		return ErrEligibilityNotConfigured
	}
	return nil
}

// Evaluate forms either a confirmed eligible/ineligible decision or a zero
// decision plus error. Missing, stale, unavailable, corrupt, mismatched, or
// future facts all fail closed without pretending the participant was rejected.
func (service *NewUserEligibilityService) Evaluate(
	ctx context.Context,
	participantRef domain.ParticipantRef,
	policy domain.NewUserPolicy,
) (domain.NewUserEligibilityDecision, error) {
	if ctx == nil || participantRef == 0 {
		return domain.NewUserEligibilityDecision{}, ErrEligibilityInvalidArgument
	}
	if err := policy.Validate(); err != nil {
		return domain.NewUserEligibilityDecision{}, fmt.Errorf(
			"%w: %w",
			ErrEligibilityInvalidArgument,
			err,
		)
	}
	if err := service.Validate(); err != nil {
		return domain.NewUserEligibilityDecision{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.NewUserEligibilityDecision{}, err
	}

	fact, err := service.facts.FindRegistrationFact(ctx, participantRef)
	if contextError := ctx.Err(); contextError != nil {
		return domain.NewUserEligibilityDecision{}, contextError
	}
	if err != nil {
		return domain.NewUserEligibilityDecision{}, classifyRegistrationFactReadError(err)
	}

	evaluatedAt := canonicalApplicationInstant(service.clock.Now())
	if contextError := ctx.Err(); contextError != nil {
		return domain.NewUserEligibilityDecision{}, contextError
	}
	if evaluatedAt.IsZero() {
		return domain.NewUserEligibilityDecision{}, ErrEligibilityClockInvalid
	}
	if err := fact.Validate(); err != nil {
		return domain.NewUserEligibilityDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRegistrationFactInvalid,
			err,
		)
	}
	if fact.ParticipantRef() != participantRef {
		return domain.NewUserEligibilityDecision{}, ErrRegistrationFactInvalid
	}
	if fact.RegisteredAt().After(evaluatedAt) || fact.ObservedAt().After(evaluatedAt) {
		return domain.NewUserEligibilityDecision{}, ErrRegistrationFactInvalid
	}
	if evaluatedAt.Sub(fact.ObservedAt()) > service.maxFactAge {
		return domain.NewUserEligibilityDecision{}, ErrRegistrationFactStale
	}
	if err := ctx.Err(); err != nil {
		return domain.NewUserEligibilityDecision{}, err
	}

	decision, err := domain.EvaluateNewUserEligibility(policy, fact, evaluatedAt)
	if contextError := ctx.Err(); contextError != nil {
		return domain.NewUserEligibilityDecision{}, contextError
	}
	if err != nil {
		return domain.NewUserEligibilityDecision{}, fmt.Errorf(
			"%w: %w",
			ErrRegistrationFactInvalid,
			err,
		)
	}
	return decision, nil
}

func classifyRegistrationFactReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// The caller context was checked immediately after the read. A raw
		// context error while that context is still live therefore describes an
		// internal provider timeout/cancellation, not caller cancellation.
		return WrapRegistrationFactReadError(ErrRegistrationFactUnavailable, err)
	}
	class := ErrRegistrationFactReadFailure
	switch {
	case errors.Is(err, ErrRegistrationFactNotFound):
		class = ErrRegistrationFactNotFound
	case errors.Is(err, ErrRegistrationFactUnavailable):
		class = ErrRegistrationFactUnavailable
	case errors.Is(err, ErrRegistrationFactReadFailure):
		class = ErrRegistrationFactReadFailure
	}
	return WrapRegistrationFactReadError(class, err)
}

func canonicalApplicationInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0)
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
