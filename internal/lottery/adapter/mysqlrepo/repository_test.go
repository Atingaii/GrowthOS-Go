package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/go-sql-driver/mysql"
)

func TestNewRejectsNilDatabase(t *testing.T) {
	t.Parallel()

	_, err := New(nil)
	if !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("New(nil) error = %v, want not configured", err)
	}
}

func TestRepositoryMethodsRejectInvalidReceiverAndContext(t *testing.T) {
	t.Parallel()

	var repository Repository
	if _, err := repository.FindByID(context.Background(), 1); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero Repository.FindByID() error = %v, want not configured", err)
	}
	if err := repository.Create(context.Background(), domain.Strategy{}); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero Repository.Create() error = %v, want not configured", err)
	}
	if _, err := repository.FindByID(nil, 1); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("FindByID(nil) error = %v, want invalid argument", err)
	}
	if err := repository.Create(nil, domain.Strategy{}); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("Create(nil) error = %v, want invalid argument", err)
	}
}

func TestClassifyOperationError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cause   error
		wantErr error
	}{
		{name: "lock wait timeout", cause: &mysql.MySQLError{Number: 1205}, wantErr: application.ErrRepositoryRetryable},
		{name: "deadlock", cause: &mysql.MySQLError{Number: 1213}, wantErr: application.ErrRepositoryRetryable},
		{name: "canceled", cause: context.Canceled, wantErr: context.Canceled},
		{name: "deadline", cause: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "duplicate outside root insert", cause: &mysql.MySQLError{Number: 1062}, wantErr: application.ErrRepositoryFailure},
		{name: "other", cause: errors.New("driver detail"), wantErr: application.ErrRepositoryFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := classifyOperationError(tt.cause)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("classifyOperationError() = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
		})
	}
}

func TestClassifyRootInsertDuplicate(t *testing.T) {
	t.Parallel()

	err := classifyRootInsertError(&mysql.MySQLError{Number: 1062})
	if !errors.Is(err, application.ErrStrategyAlreadyExists) {
		t.Fatalf("classifyRootInsertError() = %v, want already exists", err)
	}
}

func TestCommitErrorClassification(t *testing.T) {
	t.Parallel()

	writeCause := errors.New("connection lost after commit request")
	writeErr := classifyWriteCommitError(context.Background(), writeCause)
	if !errors.Is(writeErr, application.ErrCommitOutcomeUnknown) || !errors.Is(writeErr, writeCause) {
		t.Fatalf("classifyWriteCommitError() = %v, want outcome unknown with retained cause", writeErr)
	}
	canceledWriteContext, cancelWrite := context.WithCancel(context.Background())
	cancelWrite()
	writeErr = classifyWriteCommitError(canceledWriteContext, sql.ErrTxDone)
	if !errors.Is(writeErr, context.Canceled) {
		t.Fatalf("classifyWriteCommitError(canceled before driver commit) = %v, want context canceled", writeErr)
	}
	driverCommitCause := errors.New("driver commit result lost")
	writeErr = classifyWriteCommitError(canceledWriteContext, driverCommitCause)
	if !errors.Is(writeErr, application.ErrCommitOutcomeUnknown) {
		t.Fatalf("classifyWriteCommitError(driver failure plus cancel) = %v, want outcome unknown", writeErr)
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	readErr := classifyReadCommitError(canceledContext, sql.ErrTxDone)
	if !errors.Is(readErr, context.Canceled) {
		t.Fatalf("classifyReadCommitError(canceled) = %v, want context canceled", readErr)
	}
	readErr = classifyReadCommitError(canceledContext, errors.New("driver read commit failed"))
	if !errors.Is(readErr, application.ErrRepositoryFailure) {
		t.Fatalf("classifyReadCommitError(driver failure plus cancel) = %v, want repository failure", readErr)
	}
	readErr = classifyReadCommitError(context.Background(), errors.New("transaction already done"))
	if !errors.Is(readErr, application.ErrRepositoryFailure) {
		t.Fatalf("classifyReadCommitError(background) = %v, want repository failure", readErr)
	}
}

func TestRestoreStrategyFailsClosed(t *testing.T) {
	t.Parallel()

	_, err := restoreStrategy(
		storedStrategy{ID: 1, Name: "stored"},
		[]storedAward{{ID: 1, Name: "\u00a0invalid", Weight: 1, Outcome: "reward"}},
	)
	if !errors.Is(err, application.ErrStoredStrategyInvalid) {
		t.Fatalf("restoreStrategy() error = %v, want invalid stored strategy", err)
	}
}
