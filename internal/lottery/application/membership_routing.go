package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

// MembershipTierFactReader is the Lottery-owned consumption port for the
// external membership authority. It returns facts, never Strategy targets.
type MembershipTierFactReader interface {
	FindMembershipTierFact(
		ctx context.Context,
		subjectRef domain.MembershipSubjectRef,
	) (domain.MembershipTierFactSnapshot, error)
}

// MembershipRoutingClock provides the one controlled logical as-of instant.
type MembershipRoutingClock interface {
	Now() time.Time
}

// MembershipRoutingClockFunc adapts a function to MembershipRoutingClock.
type MembershipRoutingClockFunc func() time.Time

// Now returns the function result.
func (function MembershipRoutingClockFunc) Now() time.Time { return function() }

// MembershipStrategyRoutingService obtains one authoritative fact and delegates
// the pure branch decision to the Lottery domain. It loads no Strategy and has
// no random, persistence, HTTP, cache, or Participation dependency.
type MembershipStrategyRoutingService struct {
	membershipFacts MembershipTierFactReader
	clock           MembershipRoutingClock
	maxFactAge      time.Duration
}

// NewMembershipStrategyRoutingService wires the bounded v1 routing use case.
func NewMembershipStrategyRoutingService(
	membershipFacts MembershipTierFactReader,
	clock MembershipRoutingClock,
	maxFactAge time.Duration,
) (*MembershipStrategyRoutingService, error) {
	service := &MembershipStrategyRoutingService{
		membershipFacts: membershipFacts,
		clock:           clock,
		maxFactAge:      maxFactAge,
	}
	if err := service.Validate(); err != nil {
		return nil, err
	}
	return service, nil
}

// Validate rejects nil, typed-nil, zero, and partial service configurations.
func (service *MembershipStrategyRoutingService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.membershipFacts) ||
		dependencyIsNil(service.clock) ||
		service.maxFactAge <= 0 {
		return ErrMembershipRoutingNotConfigured
	}
	return nil
}

// Route evaluates one membership routing policy at one server-owned instant.
// Invalid input, dependency failure, and cancellation return a zero decision;
// only a complete domain decision can expose a branch, target, or path.
func (service *MembershipStrategyRoutingService) Route(
	ctx context.Context,
	subjectRef domain.MembershipSubjectRef,
	policy domain.MembershipStrategyRoutingPolicy,
) (domain.MembershipStrategyRouteDecision, error) {
	if ctx == nil || subjectRef == 0 {
		return domain.MembershipStrategyRouteDecision{}, ErrMembershipRoutingInvalidArgument
	}
	if err := policy.Validate(); err != nil {
		return domain.MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: %w",
			ErrMembershipRoutingInvalidArgument,
			err,
		)
	}
	if err := service.Validate(); err != nil {
		return domain.MembershipStrategyRouteDecision{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.MembershipStrategyRouteDecision{}, err
	}

	evaluatedAt := canonicalMembershipRoutingInstant(service.clock.Now())
	if contextError := ctx.Err(); contextError != nil {
		return domain.MembershipStrategyRouteDecision{}, contextError
	}
	if evaluatedAt.IsZero() {
		return domain.MembershipStrategyRouteDecision{}, ErrMembershipRoutingClockInvalid
	}

	fact, err := service.membershipFacts.FindMembershipTierFact(ctx, subjectRef)
	if contextError := ctx.Err(); contextError != nil {
		return domain.MembershipStrategyRouteDecision{}, contextError
	}
	if err != nil {
		return domain.MembershipStrategyRouteDecision{}, classifyMembershipTierFactReadError(err)
	}
	if err := fact.Validate(); err != nil {
		return domain.MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: %w",
			ErrMembershipTierFactInvalid,
			err,
		)
	}
	if fact.SubjectRef() != subjectRef || fact.ObservedAt().After(evaluatedAt) {
		return domain.MembershipStrategyRouteDecision{}, ErrMembershipTierFactInvalid
	}
	if evaluatedAt.Sub(fact.ObservedAt()) > service.maxFactAge {
		return domain.MembershipStrategyRouteDecision{}, ErrMembershipTierFactStale
	}
	if err := ctx.Err(); err != nil {
		return domain.MembershipStrategyRouteDecision{}, err
	}

	decision, err := domain.RouteMembershipStrategy(policy, fact, evaluatedAt)
	if contextError := ctx.Err(); contextError != nil {
		return domain.MembershipStrategyRouteDecision{}, contextError
	}
	if err != nil {
		return domain.MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: %w",
			ErrMembershipTierFactInvalid,
			err,
		)
	}
	if !decision.Confirmed() {
		return domain.MembershipStrategyRouteDecision{}, ErrMembershipRoutingDecisionInvalid
	}
	return decision, nil
}

func canonicalMembershipRoutingInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0)
}

func classifyMembershipTierFactReadError(err error) error {
	class := ErrMembershipTierFactReadFailure
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		class = ErrMembershipTierFactUnavailable
	case errors.Is(err, ErrMembershipTierFactNotFound):
		class = ErrMembershipTierFactNotFound
	case errors.Is(err, ErrMembershipTierFactUnavailable):
		class = ErrMembershipTierFactUnavailable
	case errors.Is(err, ErrMembershipTierFactReadFailure):
		class = ErrMembershipTierFactReadFailure
	case errors.Is(err, ErrMembershipTierFactInvalid),
		errors.Is(err, domain.ErrMembershipTierFactInvalid),
		errors.Is(err, domain.ErrMembershipSubjectRefRequired):
		class = ErrMembershipTierFactInvalid
	}
	return WrapMembershipTierFactReadError(class, err)
}
