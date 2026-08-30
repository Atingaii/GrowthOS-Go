package application

import (
	"context"
	"errors"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// ResolveActivityService restores one RR current snapshot, re-verifies exact
// Lottery refs for non-drafts, captures one Clock instant, and forms a gate
// decision. It does not evaluate the graph or load a Strategy for selection.
type ResolveActivityService struct {
	current         ActivityCurrentReader
	lotteryVerifier LotteryVerifier
	clock           Clock
	maxDuration     time.Duration
}

// NewResolveActivityService constructs the fail-closed current resolver.
func NewResolveActivityService(
	current ActivityCurrentReader,
	lotteryVerifier LotteryVerifier,
	clock Clock,
	maxDuration time.Duration,
) (*ResolveActivityService, error) {
	service := &ResolveActivityService{
		current:         current,
		lotteryVerifier: lotteryVerifier,
		clock:           clock,
		maxDuration:     maxDuration,
	}
	if err := service.Validate(); err != nil {
		return nil, err
	}
	return service, nil
}

// Validate rejects nil, typed-nil, partial, or unbounded composition.
func (service *ResolveActivityService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.current) ||
		dependencyIsNil(service.lotteryVerifier) ||
		dependencyIsNil(service.clock) ||
		service.maxDuration <= 0 {
		return ErrActivityNotConfigured
	}
	return nil
}

// Resolve returns a confirmed not_published/scheduled/active/ended/retired
// decision only after all required technical inputs are trustworthy. Every
// error returns the zero decision.
func (service *ResolveActivityService) Resolve(
	callerCtx context.Context,
	activityID domain.ActivityID,
) (domain.ActivityGateDecision, error) {
	if callerCtx == nil || activityID == 0 {
		return domain.ActivityGateDecision{}, ErrActivityInvalidArgument
	}
	if err := service.Validate(); err != nil {
		return domain.ActivityGateDecision{}, err
	}
	if err := callerCtx.Err(); err != nil {
		return domain.ActivityGateDecision{}, err
	}

	operationCtx, cancel := activityOperationContext(callerCtx, service.maxDuration)
	defer cancel()
	if err := activityOperationContextError(callerCtx, operationCtx); err != nil {
		return domain.ActivityGateDecision{}, err
	}

	activity, publication, dependencyErr := service.current.FindCurrentActivity(operationCtx, activityID)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityGateDecision{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityGateDecision{}, classifyRepositoryOperationError(dependencyErr)
	}
	if err := activity.Validate(); err != nil || activity.ID() != activityID {
		return domain.ActivityGateDecision{}, wrapActivityOperationError(
			ErrActivityResolutionInvalid,
			errors.New("current reader returned an invalid Activity root"),
		)
	}

	if activity.Lifecycle() == domain.ActivityLifecycleDraft {
		if !publicationIsZero(publication) {
			return domain.ActivityGateDecision{}, wrapActivityOperationError(
				ErrActivityResolutionInvalid,
				errors.New("draft current snapshot carries a publication"),
			)
		}
	} else {
		if err := publication.Validate(); err != nil ||
			publication.ActivityID() != activityID ||
			publication.Version() != activity.ActivePublicationVersion() {
			return domain.ActivityGateDecision{}, wrapActivityOperationError(
				ErrActivityResolutionInvalid,
				errors.New("current reader returned an invalid active publication"),
			)
		}
		candidate, err := NewActivityPublicationCandidate(publication)
		if err != nil {
			return domain.ActivityGateDecision{}, wrapActivityOperationError(
				ErrActivityResolutionInvalid,
				err,
			)
		}
		if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
			return domain.ActivityGateDecision{}, contextErr
		}
		dependencyErr = service.lotteryVerifier.VerifyPublication(operationCtx, candidate)
		if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
			return domain.ActivityGateDecision{}, contextErr
		}
		if dependencyErr != nil {
			return domain.ActivityGateDecision{}, classifyLotteryVerificationError(dependencyErr)
		}
	}

	evaluatedAt, err := readOperationInstant(callerCtx, operationCtx, service.clock)
	if err != nil {
		return domain.ActivityGateDecision{}, err
	}
	decision, err := domain.DecideActivityGate(activity, publication, evaluatedAt)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityGateDecision{}, contextErr
	}
	if err != nil {
		return domain.ActivityGateDecision{}, wrapActivityOperationError(
			ErrActivityResolutionInvalid,
			err,
		)
	}
	if err := decision.Validate(); err != nil || decision.ActivityID() != activityID {
		return domain.ActivityGateDecision{}, wrapActivityOperationError(
			ErrActivityResolutionInvalid,
			errors.New("domain returned an invalid gate decision"),
		)
	}
	return decision, nil
}
