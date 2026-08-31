package application

import (
	"context"
	"errors"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// RollbackActivityCommand identifies one exact older source. Version, window,
// graph, and Strategy manifest are copied from storage, never caller supplied.
type RollbackActivityCommand struct {
	ActivityID           domain.ActivityID
	ExpectedStateVersion domain.ActivityStateVersion
	TargetVersion        domain.ActivityPublicationVersion
}

// RollbackActivityService appends a new exact rollback publication.
type RollbackActivityService struct {
	activities       ActivityReader
	history          ActivityPublicationReader
	publications     ActivityPublicationWriter
	lotteryVerifier  LotteryVerifier
	approvalVerifier ApprovalVerifier
	clock            Clock
	maxDuration      time.Duration
}

// NewRollbackActivityService constructs the append-only rollback use case.
func NewRollbackActivityService(
	activities ActivityReader,
	history ActivityPublicationReader,
	publications ActivityPublicationWriter,
	lotteryVerifier LotteryVerifier,
	approvalVerifier ApprovalVerifier,
	clock Clock,
	maxDuration time.Duration,
) (*RollbackActivityService, error) {
	service := &RollbackActivityService{
		activities:       activities,
		history:          history,
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

// Validate rejects partial or unbounded composition.
func (service *RollbackActivityService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.activities) ||
		dependencyIsNil(service.history) ||
		dependencyIsNil(service.publications) ||
		dependencyIsNil(service.lotteryVerifier) ||
		dependencyIsNil(service.approvalVerifier) ||
		dependencyIsNil(service.clock) ||
		service.maxDuration <= 0 {
		return ErrActivityNotConfigured
	}
	return nil
}

// Rollback reads exact history, copies it through the domain plan, re-verifies
// Lottery, obtains new exact approval evidence, and performs one CAS. Every
// failure returns the zero publication.
func (service *RollbackActivityService) Rollback(
	callerCtx context.Context,
	command RollbackActivityCommand,
) (domain.ActivityPublication, error) {
	if callerCtx == nil || command.ActivityID == 0 || command.TargetVersion == 0 {
		return domain.ActivityPublication{}, ErrActivityInvalidArgument
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

	target, dependencyErr := service.history.FindPublicationByIdentity(
		operationCtx,
		command.ActivityID,
		command.TargetVersion,
	)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityPublication{}, classifyRepositoryOperationError(dependencyErr)
	}
	if err := target.Validate(); err != nil ||
		target.ActivityID() != command.ActivityID ||
		target.Version() != command.TargetVersion {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrStoredActivityPublicationInvalid,
			errors.New("historical reader returned an invalid exact publication"),
		)
	}

	publishedAt, err := readOperationInstant(callerCtx, operationCtx, service.clock)
	if err != nil {
		return domain.ActivityPublication{}, err
	}
	provisional, err := domain.PlanRollback(
		current,
		target,
		true,
		planningEvidenceReference,
		publishedAt,
	)
	if err != nil {
		return domain.ActivityPublication{}, classifyRollbackPlanningError(err)
	}
	provisionalRecord, ok := provisional.Record()
	if !ok {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrActivityOperationFailure,
			errors.New("rollback plan did not append a publication"),
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

	transition, err := domain.PlanRollback(current, target, true, evidence, candidate.PublishedAt())
	if err != nil {
		return domain.ActivityPublication{}, wrapActivityOperationError(ErrActivityOperationFailure, err)
	}
	record, ok := transition.Record()
	if !ok {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrActivityOperationFailure,
			errors.New("approved rollback plan did not append a publication"),
		)
	}
	approvedCandidate, err := NewActivityPublicationCandidate(record)
	if err != nil || !sameActivityPublicationCandidate(candidate, approvedCandidate) {
		return domain.ActivityPublication{}, wrapActivityOperationError(
			ErrActivityOperationFailure,
			errors.New("approved rollback plan changed exact candidate"),
		)
	}
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	receipt, err := newActivityCommitReceipt(current, transition)
	if err != nil {
		return domain.ActivityPublication{}, wrapActivityOperationError(ErrActivityOperationFailure, err)
	}

	dependencyErr = service.publications.CompareAndSwapPublication(operationCtx, transition)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.ActivityPublication{}, contextErr
	}
	if dependencyErr != nil {
		return domain.ActivityPublication{}, classifyRepositoryOperationErrorWithCommitReceipt(
			dependencyErr,
			receipt,
		)
	}
	return record, nil
}

func classifyRollbackPlanningError(err error) error {
	class := ErrActivityRollbackTargetInvalid
	if errors.Is(err, domain.ErrActivityLifecycleTransitionInvalid) ||
		errors.Is(err, domain.ErrActivityVersionOverflow) {
		class = ErrActivityStateConflict
	}
	return wrapActivityOperationError(class, err)
}
