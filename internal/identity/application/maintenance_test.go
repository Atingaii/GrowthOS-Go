package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type maintenanceRepositoryStub struct {
	run func(context.Context, MaintenanceOperation) (MaintenanceResult, error)
}

func (repository *maintenanceRepositoryStub) RunMaintenance(
	ctx context.Context,
	operation MaintenanceOperation,
) (MaintenanceResult, error) {
	return repository.run(ctx, operation)
}

func TestMaintenanceServiceFreezesOneClockSnapshotAndBudgets(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 123456000, time.UTC)
	clockCalls := 0
	repositoryCalls := 0
	repository := &maintenanceRepositoryStub{run: func(
		ctx context.Context,
		operation MaintenanceOperation,
	) (MaintenanceResult, error) {
		repositoryCalls++
		if ctx == nil || operation.Validate() != nil {
			t.Fatal("repository received an invalid operation")
		}
		if operation.ObservedAt() != now ||
			operation.SessionCutoff() != now.Add(-SessionHistoryRetention) ||
			operation.SessionBudget() != 250 || operation.ThrottleBudget() != 250 {
			t.Fatalf("unexpected operation: %#v", operation)
		}
		return NewMaintenanceResult(17, 23)
	}}
	service, err := NewMaintenanceService(ClockFunc(func() time.Time {
		clockCalls++
		return now
	}), repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if clockCalls != 1 || repositoryCalls != 1 || result.SessionsDeleted() != 17 ||
		result.ThrottlesDeleted() != 23 || result.TotalDeleted() != 40 {
		t.Fatalf("clock=%d repository=%d result=%#v", clockCalls, repositoryCalls, result)
	}
}

func TestMaintenanceOperationUsesClosedSevenDayCutoff(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 999999999, time.FixedZone("test", 8*60*60))
	operation, err := NewMaintenanceOperation(now)
	if err != nil {
		t.Fatal(err)
	}
	wantObserved := now.UTC().Truncate(time.Microsecond)
	if operation.ObservedAt() != wantObserved ||
		operation.SessionCutoff() != wantObserved.Add(-7*24*time.Hour) {
		t.Fatalf("observed=%v cutoff=%v", operation.ObservedAt(), operation.SessionCutoff())
	}
	if operation.SessionBudget()+operation.ThrottleBudget() != MaintenanceMaximumRows {
		t.Fatal("maintenance budget is not globally bounded")
	}
}

func TestMaintenanceServiceFailsClosedBeforeRepository(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ctx       context.Context
		clock     Clock
		wantClass error
	}{
		{name: "nil context", clock: ClockFunc(time.Now), wantClass: ErrInvalidArgument},
		{name: "zero clock", ctx: context.Background(), clock: ClockFunc(func() time.Time { return time.Time{} }), wantClass: ErrAuthenticationUnavailable},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			called := false
			repository := &maintenanceRepositoryStub{run: func(context.Context, MaintenanceOperation) (MaintenanceResult, error) {
				called = true
				return MaintenanceResult{}, nil
			}}
			service, err := NewMaintenanceService(test.clock, repository)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Run(test.ctx)
			if !errors.Is(err, test.wantClass) || called {
				t.Fatalf("err=%v called=%v", err, called)
			}
		})
	}
}

func TestMaintenanceServiceCancellationAndCommitUnknownRemainLowDisclosure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	private := errors.New("mysql password=sentinel private commit detail")
	tests := []struct {
		name      string
		setup     func() (context.Context, error)
		wantClass error
	}{
		{
			name: "dependency cancellation",
			setup: func() (context.Context, error) {
				return context.Background(), context.Canceled
			},
			wantClass: ErrOperationCanceled,
		},
		{
			name: "commit unknown",
			setup: func() (context.Context, error) {
				return context.Background(), WrapDependencyError(ErrCommitOutcomeUnknown, private)
			},
			wantClass: ErrCommitOutcomeUnknown,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, dependencyErr := test.setup()
			repository := &maintenanceRepositoryStub{run: func(context.Context, MaintenanceOperation) (MaintenanceResult, error) {
				return MaintenanceResult{}, dependencyErr
			}}
			service, err := NewMaintenanceService(ClockFunc(func() time.Time { return now }), repository)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Run(ctx)
			if !errors.Is(err, test.wantClass) {
				t.Fatalf("err=%v, want %v", err, test.wantClass)
			}
			if strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("public error leaked private detail: %v", err)
			}
		})
	}
}

func TestMaintenanceCommitUnknownWinsOverConcurrentCancellation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	repository := &maintenanceRepositoryStub{run: func(context.Context, MaintenanceOperation) (MaintenanceResult, error) {
		cancel()
		return MaintenanceResult{}, WrapDependencyError(
			ErrCommitOutcomeUnknown,
			errors.New("private commit acknowledgement"),
		)
	}}
	service, err := NewMaintenanceService(ClockFunc(func() time.Time { return now }), repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Run(ctx)
	if !errors.Is(err, ErrCommitOutcomeUnknown) || errors.Is(err, ErrOperationCanceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestMaintenanceServiceRejectsInvalidRepositoryResult(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	repository := &maintenanceRepositoryStub{run: func(context.Context, MaintenanceOperation) (MaintenanceResult, error) {
		return MaintenanceResult{sessionsDeleted: MaintenanceSessionBudget + 1}, nil
	}}
	service, err := NewMaintenanceService(ClockFunc(func() time.Time { return now }), repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Run(context.Background())
	if !errors.Is(err, ErrAuthenticationUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestMaintenanceConstructorsRejectTypedNilDependenciesAndInvalidCounts(t *testing.T) {
	t.Parallel()
	var repository *maintenanceRepositoryStub
	if _, err := NewMaintenanceService(ClockFunc(time.Now), repository); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("typed-nil repository err=%v", err)
	}
	if _, err := NewMaintenanceResult(-1, 0); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("negative result err=%v", err)
	}
	if _, err := NewMaintenanceResult(MaintenanceSessionBudget, MaintenanceThrottleBudget+1); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("overflow result err=%v", err)
	}
}
