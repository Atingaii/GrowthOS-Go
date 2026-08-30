package application

import (
	"context"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
)

// CreateDraftCommand contains the stable Marketing identity and canonicalized
// operator-facing label for a new draft root.
type CreateDraftCommand struct {
	ActivityID domain.ActivityID
	Name       string
}

// CreateDraftService persists one draft without implying edit or publication.
type CreateDraftService struct {
	drafts      ActivityDraftCreator
	maxDuration time.Duration
}

// NewCreateDraftService constructs the bounded create-only use case.
func NewCreateDraftService(
	drafts ActivityDraftCreator,
	maxDuration time.Duration,
) (*CreateDraftService, error) {
	service := &CreateDraftService{drafts: drafts, maxDuration: maxDuration}
	if err := service.Validate(); err != nil {
		return nil, err
	}
	return service, nil
}

// Validate rejects nil, typed-nil, and non-positive duration configuration.
func (service *CreateDraftService) Validate() error {
	if service == nil || dependencyIsNil(service.drafts) || service.maxDuration <= 0 {
		return ErrActivityNotConfigured
	}
	return nil
}

// Create constructs and persists a state-version-zero draft. Every failure
// returns the zero Activity.
func (service *CreateDraftService) Create(
	callerCtx context.Context,
	command CreateDraftCommand,
) (domain.Activity, error) {
	if callerCtx == nil || command.ActivityID == 0 {
		return domain.Activity{}, ErrActivityInvalidArgument
	}
	activity, err := domain.NewActivity(command.ActivityID, command.Name)
	if err != nil {
		return domain.Activity{}, wrapActivityOperationError(ErrActivityInvalidArgument, err)
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
	dependencyErr := service.drafts.CreateDraft(operationCtx, activity)
	if contextErr := activityOperationContextError(callerCtx, operationCtx); contextErr != nil {
		return domain.Activity{}, contextErr
	}
	if dependencyErr != nil {
		return domain.Activity{}, classifyRepositoryOperationError(dependencyErr)
	}
	return activity, nil
}
