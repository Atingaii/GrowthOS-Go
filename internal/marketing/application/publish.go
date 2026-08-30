package application

import (
	"context"
	"errors"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// PublishActivityCommand requests one exact release publication. The caller
// chooses refs and window, but not the next numeric version or published-at.
type PublishActivityCommand struct {
	ActivityID               domain.ActivityID
	ExpectedStateVersion     domain.ActivityStateVersion
	StartsAt                 time.Time
	EndsAt                   time.Time
	GraphReference           domain.LotteryGraphReference
	StrategyRevisionManifest []domain.LotteryStrategyRevisionReference
}

// PublishActivityService plans, verifies, approves, and atomically compares
// and swaps one immutable release. It remains intentionally unassembled.
type PublishActivityService struct {
	activities       ActivityReader
	publications     ActivityPublicationWriter
	lotteryVerifier  LotteryVerifier
	approvalVerifier ApprovalVerifier
	clock            Clock
	maxDuration      time.Duration
}

// NewPublishActivityService constructs the release use case.
func NewPublishActivityService(
	activities ActivityReader,
	publications ActivityPublicationWriter,
	lotteryVerifier LotteryVerifier,
	approvalVerifier ApprovalVerifier,
	clock Clock,
	maxDuration time.Duration,
) (*PublishActivityService, error) {
	service := &PublishActivityService{
		activities:       activities,
		publications:     publications,
		lotteryVerifier:  lotteryVerifier,
		approvalVerifier: approvalVerifier,
		clock:            clock,
		maxDuration:      maxDuration,
	}
	if err := service.Validate(); err != nil {
		return nil, err
	}
	return service, nil
}

// Validate rejects partial service composition, typed nil, and invalid budget.
func (service *PublishActivityService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.activities) ||
		dependencyIsNil(service.publications) ||
		dependencyIsNil(service.lotteryVerifier) ||
		dependencyIsNil(service.approvalVerifier) ||
		dependencyIsNil(service.clock) ||
		service.maxDuration <= 0 {
		return ErrActivityNotConfigured
	}
	return nil
}

// Publish follows read-current -> pure plan -> exact Lottery verification ->
// exact Governance approval -> one publication CAS. Every failure returns the
// zero publication and no partial candidate.
func (service *PublishActivityService) Publish(
	callerCtx context.Context,
	command PublishActivityCommand,
) (domain.ActivityPublication, error) {
	if callerCtx == nil {
		return domain.ActivityPublication{}, ErrActivityInvalidArgument
	}
	if err := validatePublicationCommandShape(
		command.ActivityID,
		command.StartsAt,
		command.EndsAt,
		command.GraphReference,
		command.StrategyRevisionManifest,
	); err != nil {
		return domain.ActivityPublication{}, err
	}
	if err := service.Validate(); err != nil {
		return domain.ActivityPublication{}, err
	}
	if err := callerCtx.Err(); err != nil {
		return domain.ActivityPublication{}, err
	}

	operationCtx, cancel := activityOperationContext(callerCtx, service.maxDuration)
	defer cancel()
	if err := activityOperationContextError(callerCtx, operationCtx); err != nil {
		return domain.ActivityPublication{}, err
	}

	current, dependencyErr := service.activities.FindActivityByID(operationCtx, command.ActivityID)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityPublication{}, classifyRepositoryOperationError(dependencyErr)
	}
	if err := validateCurrentActivity(current, command.ActivityID, command.ExpectedStateVersion); err != nil {
		return domain.ActivityPublication{}, err
	}

	publishedAt, err := readOperationInstant(callerCtx, operationCtx, service.clock)
	if err != nil {
		return domain.ActivityPublication{}, err
	}
	provisional, err := domain.PlanPublish(
		current,
		command.StartsAt,
		command.EndsAt,
		command.GraphReference,
		append([]domain.LotteryStrategyRevisionReference(nil), command.StrategyRevisionManifest...),
		planningEvidenceReference,
		publishedAt,
	)
	if err != nil {
		return domain.ActivityPublication{}, classifyPublishPlanningError(err)
	}
	provisionalRecord, ok := provisional.Record()
	if !ok {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrActivityOperationFailure,
			errors.New("publish plan did not append a publication"),
		)
	}
	candidate, err := NewActivityPublicationCandidate(provisionalRecord)
	if err != nil {
		return domain.ActivityPublication{}, wrapActivityOperationError(ErrActivityOperationFailure, err)
	}
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}

	dependencyErr = service.lotteryVerifier.VerifyPublication(operationCtx, candidate)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityPublication{}, classifyLotteryVerificationError(dependencyErr)
	}

	evidence, dependencyErr := service.approvalVerifier.VerifyPublication(operationCtx, candidate)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityPublication{}, classifyApprovalError(dependencyErr)
	}
	if err := validateApprovalEvidence(evidence); err != nil {
		return domain.ActivityPublication{}, err
	}

	transition, err := domain.PlanPublish(
		current,
		candidate.StartsAt(),
		candidate.EndsAt(),
		candidate.GraphReference(),
		candidate.StrategyRevisionManifest(),
		evidence,
		candidate.PublishedAt(),
	)
	if err != nil {
		return domain.ActivityPublication{}, wrapActivityOperationError(ErrActivityOperationFailure, err)
	}
	record, ok := transition.Record()
	if !ok {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrActivityOperationFailure,
			errors.New("approved publish plan did not append a publication"),
		)
	}
	approvedCandidate, err := NewActivityPublicationCandidate(record)
	if err != nil || !sameActivityPublicationCandidate(candidate, approvedCandidate) {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrActivityOperationFailure,
			errors.New("approved publish plan changed exact candidate"),
		)
	}
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}

	dependencyErr = service.publications.CompareAndSwapPublication(operationCtx, transition)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityPublication{}, classifyRepositoryOperationError(dependencyErr)
	}
	return record, nil
}

func validatePublicationCommandShape(
	activityID domain.ActivityID,
	startsAt time.Time,
	endsAt time.Time,
	graphReference domain.LotteryGraphReference,
	manifest []domain.LotteryStrategyRevisionReference,
) error {
	if activityID == 0 || startsAt.IsZero() || endsAt.IsZero() || !startsAt.Before(endsAt) {
		return ErrActivityInvalidArgument
	}
	if err := graphReference.Validate(); err != nil {
		return wrapActivityOperationError(ErrActivityInvalidArgument, err)
	}
	if len(manifest) == 0 || len(manifest) > domain.MaxStrategyRevisionManifestEntries {
		return ErrActivityInvalidArgument
	}
	seen := make(map[domain.LotteryStrategyID]struct{}, len(manifest))
	for _, reference := range manifest {
		if err := reference.Validate(); err != nil {
			return wrapActivityOperationError(ErrActivityInvalidArgument, err)
		}
		if _, duplicate := seen[reference.StrategyID()]; duplicate {
			return wrapActivityOperationError(
				ErrActivityInvalidArgument,
				errors.New("Strategy identity is duplicated"),
			)
		}
		seen[reference.StrategyID()] = struct{}{}
	}
	return nil
}

func classifyPublishPlanningError(err error) error {
	class := ErrActivityPublicationCandidateInvalid
	if errors.Is(err, domain.ErrActivityLifecycleTransitionInvalid) ||
		errors.Is(err, domain.ErrActivityVersionOverflow) {
		class = ErrActivityStateConflict
	}
	return wrapActivityOperationError(class, err)
}
