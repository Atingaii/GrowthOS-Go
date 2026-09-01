package application

import (
	"context"
	"errors"
	"time"
)

const (
	// SessionHistoryRetention preserves expired and revoked session evidence for
	// incident response before it becomes eligible for bounded removal.
	SessionHistoryRetention = 7 * 24 * time.Hour
	// MaintenanceMaximumRows is the hard per-operation deletion ceiling across
	// every Identity authority table.
	MaintenanceMaximumRows = 500
	// MaintenanceSessionBudget and MaintenanceThrottleBudget deliberately split
	// the operation in half. Unused capacity is not lent between tables: a busy
	// session table therefore cannot starve throttle reclamation (or vice versa),
	// and each independently committed transaction remains small and predictable.
	MaintenanceSessionBudget  = MaintenanceMaximumRows / 2
	MaintenanceThrottleBudget = MaintenanceMaximumRows - MaintenanceSessionBudget
)

// MaintenanceRepository removes only rows that remain eligible when the
// authority write is executed. Implementations must not retry an unknown
// commit outcome inside this call.
type MaintenanceRepository interface {
	RunMaintenance(context.Context, MaintenanceOperation) (MaintenanceResult, error)
}

// MaintenanceOperation is one immutable, server-clock-owned cleanup snapshot.
// Throttle rows already encode their 24-hour retention in row_expires_at, so
// their closed eligibility boundary is ObservedAt.
type MaintenanceOperation struct {
	observedAt     time.Time
	sessionCutoff  time.Time
	sessionBudget  int
	throttleBudget int
}

// NewMaintenanceOperation freezes every time boundary from one canonical
// clock observation. It is exported for adapter contract tests and alternative
// trusted composition roots; HTTP or other caller-controlled input must never
// construct it.
func NewMaintenanceOperation(observedAt time.Time) (MaintenanceOperation, error) {
	observedAt = canonicalInstant(observedAt)
	operation := MaintenanceOperation{
		observedAt:     observedAt,
		sessionCutoff:  observedAt.Add(-SessionHistoryRetention),
		sessionBudget:  MaintenanceSessionBudget,
		throttleBudget: MaintenanceThrottleBudget,
	}
	if operation.Validate() != nil {
		return MaintenanceOperation{}, ErrInvalidArgument
	}
	return operation, nil
}

// Validate rejects zero, non-canonical, or forged cleanup boundaries and
// budgets. Exact budgets keep dynamic SQL cardinality internally bounded.
func (operation MaintenanceOperation) Validate() error {
	if operation.observedAt.IsZero() || canonicalInstant(operation.observedAt) != operation.observedAt ||
		operation.sessionCutoff.IsZero() || canonicalInstant(operation.sessionCutoff) != operation.sessionCutoff ||
		operation.sessionCutoff != operation.observedAt.Add(-SessionHistoryRetention) ||
		operation.sessionBudget != MaintenanceSessionBudget ||
		operation.throttleBudget != MaintenanceThrottleBudget ||
		operation.sessionBudget <= 0 || operation.throttleBudget <= 0 ||
		operation.sessionBudget+operation.throttleBudget != MaintenanceMaximumRows {
		return ErrInvalidArgument
	}
	return nil
}

func (operation MaintenanceOperation) ObservedAt() time.Time { return operation.observedAt }

func (operation MaintenanceOperation) SessionCutoff() time.Time {
	return operation.sessionCutoff
}

func (operation MaintenanceOperation) SessionBudget() int { return operation.sessionBudget }

func (operation MaintenanceOperation) ThrottleBudget() int { return operation.throttleBudget }

// MaintenanceResult is trusted only when Run returns a nil error.
type MaintenanceResult struct {
	sessionsDeleted  int
	throttlesDeleted int
}

func NewMaintenanceResult(sessionsDeleted, throttlesDeleted int) (MaintenanceResult, error) {
	result := MaintenanceResult{
		sessionsDeleted:  sessionsDeleted,
		throttlesDeleted: throttlesDeleted,
	}
	if result.Validate() != nil {
		return MaintenanceResult{}, ErrInvalidArgument
	}
	return result, nil
}

func (result MaintenanceResult) Validate() error {
	if result.sessionsDeleted < 0 || result.sessionsDeleted > MaintenanceSessionBudget ||
		result.throttlesDeleted < 0 || result.throttlesDeleted > MaintenanceThrottleBudget ||
		result.sessionsDeleted+result.throttlesDeleted > MaintenanceMaximumRows {
		return ErrInvalidArgument
	}
	return nil
}

func (result MaintenanceResult) SessionsDeleted() int { return result.sessionsDeleted }

func (result MaintenanceResult) ThrottlesDeleted() int { return result.throttlesDeleted }

func (result MaintenanceResult) TotalDeleted() int {
	return result.sessionsDeleted + result.throttlesDeleted
}

// MaintenanceService owns the application boundary for one operator-triggered
// maintenance operation. It performs no hidden loop and no automatic retry.
type MaintenanceService struct {
	clock      Clock
	repository MaintenanceRepository
}

func NewMaintenanceService(clock Clock, repository MaintenanceRepository) (*MaintenanceService, error) {
	service := &MaintenanceService{clock: clock, repository: repository}
	if service.Validate() != nil {
		return nil, ErrNotConfigured
	}
	return service, nil
}

func (service *MaintenanceService) Validate() error {
	if service == nil || dependencyIsNil(service.clock) || dependencyIsNil(service.repository) {
		return ErrNotConfigured
	}
	return nil
}

func (service *MaintenanceService) Run(ctx context.Context) (MaintenanceResult, error) {
	if service.Validate() != nil {
		return MaintenanceResult{}, wrapOperationError(ErrNotConfigured, ErrNotConfigured)
	}
	if ctx == nil {
		return MaintenanceResult{}, wrapOperationError(ErrInvalidArgument, ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return MaintenanceResult{}, wrapOperationError(ErrOperationCanceled, err)
	}
	operation, err := NewMaintenanceOperation(service.clock.Now())
	if err != nil {
		return MaintenanceResult{}, wrapOperationError(
			ErrAuthenticationUnavailable,
			errors.New("maintenance clock returned an invalid instant"),
		)
	}
	result, err := service.repository.RunMaintenance(ctx, operation)
	if err != nil {
		return MaintenanceResult{}, classifyMaintenanceError(ctx, err)
	}
	if result.Validate() != nil {
		return MaintenanceResult{}, wrapOperationError(
			ErrAuthenticationUnavailable,
			errors.New("maintenance repository returned an invalid result"),
		)
	}
	return result, nil
}

func classifyMaintenanceError(ctx context.Context, err error) error {
	// A repository commit-unknown classification is stronger evidence than a
	// caller cancellation observed concurrently. Downgrading it would invite an
	// operator to retry a write that may already have committed.
	if errors.Is(err, ErrCommitOutcomeUnknown) {
		return wrapOperationError(ErrCommitOutcomeUnknown, err)
	}
	if canceled := canceledOperationError(ctx, err); canceled != nil {
		return canceled
	}
	return wrapOperationError(ErrAuthenticationUnavailable, err)
}
