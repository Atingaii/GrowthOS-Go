package application

import (
	"context"
	"errors"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// RetireActivityCommand carries only identity and expected CAS state. The
// terminal instant and evidence remain server/provider owned.
type RetireActivityCommand struct {
	ActivityID           domain.ActivityID
	ExpectedStateVersion domain.ActivityStateVersion
}

// RetireActivityService obtains exact retirement approval and performs the
// terminal root CAS without modifying publication history.
type RetireActivityService struct {
	activities       ActivityReader
	retirer          ActivityRetirer
	approvalVerifier ApprovalVerifier
	clock            Clock
	maxDuration      time.Duration
}

// NewRetireActivityService constructs the terminal transition use case.
func NewRetireActivityService(
	activities ActivityReader,
	retirer ActivityRetirer,
	approvalVerifier ApprovalVerifier,
	clock Clock,
	maxDuration time.Duration,
) (*RetireActivityService, error) {
	service := &RetireActivityService{
		activities:       activities,
		retirer:          retirer,
		approvalVerifier: approvalVerifier,
		clock:            clock,
		maxDuration:      maxDuration,
	}
	if err := service.Validate(); err != nil {
		return nil, err
	}
	return service, nil
}

// Validate rejects partial and unbounded service composition.
func (service *RetireActivityService) Validate() error {
	if service == nil ||
		dependencyIsNil(service.activities) ||
		dependencyIsNil(service.retirer) ||
		dependencyIsNil(service.approvalVerifier) ||
		dependencyIsNil(service.clock) ||
		service.maxDuration <= 0 {
		return ErrActivityNotConfigured
	}
	return nil
}

// Retire follows read-current -> one Clock -> exact approval -> terminal CAS.
// Every failure returns the zero Activity.
func (service *RetireActivityService) Retire(
	callerCtx context.Context,
	command RetireActivityCommand,
) (domain.Activity, error) {
	if callerCtx == nil || command.ActivityID == 0 {
		return domain.Activity{}, ErrActivityInvalidArgument
	}
	if err := service.Validate(); err != nil {
		return domain.Activity{}, err
	}
	if err := callerCtx.Err(); err != nil {
		return domain.Activity{}, err
	}

	operationCtx, cancel := activityOperationContext(callerCtx, service.maxDuration)
	defer cancel()
	if err := activityOperationContextError(callerCtx, operationCtx); err != nil {
		return domain.Activity{}, err
	}

	current, dependencyErr := service.activities.FindActivityByID(operationCtx, command.ActivityID)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.Activity{}, contextErr
	}
	if dependencyErr != nil {
		return domain.Activity{}, classifyRepositoryOperationError(dependencyErr)
	}
	if err := validateCurrentActivity(current, command.ActivityID, command.ExpectedStateVersion); err != nil {
		return domain.Activity{}, err
	}

	retiredAt, err := readOperationInstant(callerCtx, operationCtx, service.clock)
	if err != nil {
		return domain.Activity{}, err
	}
	candidate, err := newActivityRetirementCandidate(current, retiredAt)
	if err != nil {
		class := ErrActivityPublicationCandidateInvalid
		if errors.Is(err, domain.ErrActivityLifecycleTransitionInvalid) ||
			errors.Is(err, domain.ErrActivityVersionOverflow) {
			class = ErrActivityStateConflict
		}
		return domain.Activity{}, wrapActivityOperationError(class, err)
	}
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.Activity{}, contextErr
	}

	evidence, dependencyErr := service.approvalVerifier.VerifyRetirement(operationCtx, candidate)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.Activity{}, contextErr
	}
	if dependencyErr != nil {
		return domain.Activity{}, classifyApprovalError(dependencyErr)
	}
	if err := validateApprovalEvidence(evidence); err != nil {
		return domain.Activity{}, err
	}

	transition, err := domain.PlanRetire(current, evidence, candidate.RetiredAt())
	if err != nil {
		return domain.Activity{}, wrapActivityOperationError(ErrActivityOperationFailure, err)
	}
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.Activity{}, contextErr
	}
	dependencyErr = service.retirer.CompareAndSwapRetirement(operationCtx, transition)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.Activity{}, contextErr
	}
	if dependencyErr != nil {
		return domain.Activity{}, classifyRepositoryOperationError(dependencyErr)
	}
	return transition.Next(), nil
}
