package mysqlrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/application"
	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
	"github.com/DATA-DOG/go-sqlmock"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestNewAndCallsRejectInvalidConfigurationAndArgumentsBeforeSQL(t *testing.T) {
	t.Parallel()

	repository, err := New(nil)
	if repository != nil || !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("New(nil) = %#v, %v; want nil/not configured", repository, err)
	}
	draft := mustTestDraft(t, 1)
	publicationTransition := mustTestPublicationTransition(t, draft)
	retirementTransition := mustTestRetirementTransition(t, publicationTransition.Next())
	var zero Repository
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{name: "create nil context", call: func() error { return zero.CreateDraft(nil, draft) }, want: application.ErrRepositoryInvalidArgument},
		{name: "create zero receiver", call: func() error { return zero.CreateDraft(context.Background(), draft) }, want: application.ErrRepositoryNotConfigured},
		{name: "root zero receiver", call: func() error { _, err := zero.FindActivityByID(context.Background(), 1); return err }, want: application.ErrRepositoryNotConfigured},
		{name: "current nil context", call: func() error { _, _, err := zero.FindCurrentActivity(nil, 1); return err }, want: application.ErrRepositoryInvalidArgument},
		{name: "history zero id", call: func() error { _, err := zero.FindPublicationByIdentity(context.Background(), 0, 1); return err }, want: application.ErrRepositoryNotConfigured},
		{name: "publication zero receiver", call: func() error { return zero.CompareAndSwapPublication(context.Background(), publicationTransition) }, want: application.ErrRepositoryNotConfigured},
		{name: "retirement nil context", call: func() error { return zero.CompareAndSwapRetirement(nil, retirementTransition) }, want: application.ErrRepositoryInvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	configured, mock := newRepositoryMock(t)
	if err := configured.CreateDraft(context.Background(), domain.Activity{}); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("CreateDraft(zero) error = %v, want invalid argument", err)
	}
	if err := configured.CreateDraft(context.Background(), publicationTransition.Next()); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("CreateDraft(published) error = %v, want invalid argument", err)
	}
	if _, err := configured.FindActivityByID(context.Background(), 0); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("FindActivityByID(0) error = %v, want invalid argument", err)
	}
	if _, err := configured.FindPublicationByIdentity(context.Background(), 1, 0); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("FindPublicationByIdentity(version 0) error = %v, want invalid argument", err)
	}
	if err := configured.CompareAndSwapPublication(context.Background(), retirementTransition); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("CompareAndSwapPublication(retirement) error = %v, want invalid argument", err)
	}
	if err := configured.CompareAndSwapRetirement(context.Background(), publicationTransition); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("CompareAndSwapRetirement(publication) error = %v, want invalid argument", err)
	}
	assertExpectations(t, mock)
}

func TestCreateDraftWritesOneTransactionAndClassifiesDuplicate(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		draft := mustTestDraft(t, 11)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(insertActivitySQL)).
			WithArgs(uint64(11), "Activity 11", "draft", uint64(0), nil, nil, nil).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		if err := repository.CreateDraft(context.Background(), draft); err != nil {
			t.Fatalf("CreateDraft() error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("duplicate is safe already exists", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		draft := mustTestDraft(t, 12)
		cause := &drivermysql.MySQLError{Number: 1062, Message: "secret activity key"}
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(insertActivitySQL)).
			WithArgs(uint64(12), "Activity 12", "draft", uint64(0), nil, nil, nil).
			WillReturnError(cause)
		mock.ExpectRollback()

		err := repository.CreateDraft(context.Background(), draft)
		assertSafeRepositoryError(t, err, application.ErrActivityAlreadyExists, cause)
		assertExpectations(t, mock)
	})

	t.Run("unexpected row count rolls back", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		draft := mustTestDraft(t, 13)
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(insertActivitySQL)).
			WithArgs(uint64(13), "Activity 13", "draft", uint64(0), nil, nil, nil).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		if err := repository.CreateDraft(context.Background(), draft); !errors.Is(err, application.ErrRepositoryFailure) {
			t.Fatalf("CreateDraft(rows=0) error = %v, want repository failure", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("commit response loss is unknown and not retryable", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		draft := mustTestDraft(t, 14)
		cause := errors.New("secret commit response was lost")
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(insertActivitySQL)).
			WithArgs(uint64(14), "Activity 14", "draft", uint64(0), nil, nil, nil).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(cause)

		err := repository.CreateDraft(context.Background(), draft)
		assertSafeRepositoryError(t, err, application.ErrCommitOutcomeUnknown, cause)
		if errors.Is(err, application.ErrRepositoryRetryable) {
			t.Fatal("unknown commit outcome was marked retryable")
		}
		assertExpectations(t, mock)
	})
}

func TestFindActivityByIDUsesExactRepeatableReadAndStrictRestore(t *testing.T) {
	t.Parallel()

	t.Run("draft", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		draft := mustTestDraft(t, 21)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(21)).
			WillReturnRows(storedActivityRows(draft))
		mock.ExpectCommit()

		got, err := repository.FindActivityByID(context.Background(), 21)
		if err != nil {
			t.Fatalf("FindActivityByID() error = %v", err)
		}
		assertActivitiesEqual(t, got, draft)
		if options := readSnapshotOptions(); options == nil ||
			*options != (sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}) {
			t.Fatalf("read options = %#v, want repeatable-read/read-only", options)
		}
		assertExpectations(t, mock)
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(22)).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		got, err := repository.FindActivityByID(context.Background(), 22)
		if got != (domain.Activity{}) || !errors.Is(err, application.ErrActivityNotFound) {
			t.Fatalf("FindActivityByID(missing) = %#v, %v", got, err)
		}
		assertExpectations(t, mock)
	})

	t.Run("corrupt root", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(23)).
			WillReturnRows(sqlmock.NewRows(activityColumns()).
				AddRow(uint64(23), " Bad ", "draft", uint64(0), nil, nil, nil))
		mock.ExpectCommit()

		got, err := repository.FindActivityByID(context.Background(), 23)
		if got != (domain.Activity{}) || !errors.Is(err, application.ErrStoredActivityInvalid) {
			t.Fatalf("FindActivityByID(corrupt) = %#v, %v", got, err)
		}
		assertExpectations(t, mock)
	})
}

func TestFindCurrentActivityReadsRootAndExactActivePublicationInOneSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("draft returns zero publication without history query", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		draft := mustTestDraft(t, 31)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(31)).WillReturnRows(storedActivityRows(draft))
		mock.ExpectCommit()

		got, publication, err := repository.FindCurrentActivity(context.Background(), 31)
		if err != nil || !reflect.DeepEqual(publication, domain.ActivityPublication{}) {
			t.Fatalf("FindCurrentActivity(draft) = %#v, %#v, %v", got, publication, err)
		}
		assertActivitiesEqual(t, got, draft)
		assertExpectations(t, mock)
	})

	t.Run("published resolves exact active version and bounded manifest", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 32))
		published := transition.Next()
		record, _ := transition.Record()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(32)).WillReturnRows(storedActivityRows(published))
		expectPublicationRead(mock, record)
		mock.ExpectCommit()

		got, publication, err := repository.FindCurrentActivity(context.Background(), 32)
		if err != nil {
			t.Fatalf("FindCurrentActivity(published) error = %v", err)
		}
		assertActivitiesEqual(t, got, published)
		assertPublicationsEqual(t, publication, record)
		assertExpectations(t, mock)
	})

	t.Run("retired retains and resolves its exact last active publication", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		publishedTransition := mustTestPublicationTransition(t, mustTestDraft(t, 36))
		publication, _ := publishedTransition.Record()
		retirementTransition := mustTestRetirementTransition(t, publishedTransition.Next())
		retired := retirementTransition.Next()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(36)).WillReturnRows(storedActivityRows(retired))
		expectPublicationRead(mock, publication)
		mock.ExpectCommit()

		got, active, err := repository.FindCurrentActivity(context.Background(), 36)
		if err != nil {
			t.Fatalf("FindCurrentActivity(retired) error = %v", err)
		}
		assertActivitiesEqual(t, got, retired)
		assertPublicationsEqual(t, active, publication)
		assertExpectations(t, mock)
	})

	t.Run("missing active history is stored corruption, not historical not-found", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 33))
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(33)).WillReturnRows(storedActivityRows(transition.Next()))
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
			WithArgs(uint64(33), uint64(1)).WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		_, _, err := repository.FindCurrentActivity(context.Background(), 33)
		if !errors.Is(err, application.ErrStoredActivityPublicationInvalid) ||
			errors.Is(err, application.ErrActivityPublicationNotFound) {
			t.Fatalf("FindCurrentActivity(missing active) error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("future header fails closed before bindings", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 34))
		record, _ := transition.Record()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(34)).WillReturnRows(storedActivityRows(transition.Next()))
		rows := storedPublicationRows(record)
		rows = sqlmock.NewRows(publicationColumns()).AddRow(
			uint16(2), "release", nil, uint64(71), "graph:r1",
			record.StartsAt(), record.EndsAt(), record.PublishedAt(), "approval/release-1",
		)
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
			WithArgs(uint64(34), uint64(1)).WillReturnRows(rows)
		mock.ExpectRollback()

		_, _, err := repository.FindCurrentActivity(context.Background(), 34)
		if !errors.Is(err, application.ErrStoredActivityPublicationInvalid) {
			t.Fatalf("FindCurrentActivity(future schema) error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("129 bindings breach restore budget", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 35))
		record, _ := transition.Record()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(35)).WillReturnRows(storedActivityRows(transition.Next()))
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
			WithArgs(uint64(35), uint64(1)).WillReturnRows(storedPublicationRows(record))
		rows := sqlmock.NewRows([]string{"strategy_id", "strategy_revision"})
		for index := 1; index <= domain.MaxStrategyRevisionManifestEntries+1; index++ {
			rows.AddRow(uint64(index), "strategy:r1")
		}
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationStrategiesSQL)).
			WithArgs(uint64(35), uint64(1)).WillReturnRows(rows)
		mock.ExpectRollback()

		_, _, err := repository.FindCurrentActivity(context.Background(), 35)
		if !errors.Is(err, application.ErrStoredActivityPublicationInvalid) {
			t.Fatalf("FindCurrentActivity(over limit) error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("malformed exact Strategy binding fails closed", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 37))
		record, _ := transition.Record()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
			WithArgs(uint64(37)).WillReturnRows(storedActivityRows(transition.Next()))
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
			WithArgs(uint64(37), uint64(1)).WillReturnRows(storedPublicationRows(record))
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationStrategiesSQL)).
			WithArgs(uint64(37), uint64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"strategy_id", "strategy_revision"}).
				AddRow(uint64(0), "strategy:r1"))
		mock.ExpectRollback()

		_, _, err := repository.FindCurrentActivity(context.Background(), 37)
		if !errors.Is(err, application.ErrStoredActivityPublicationInvalid) {
			t.Fatalf("FindCurrentActivity(malformed binding) error = %v", err)
		}
		assertExpectations(t, mock)
	})
}

func TestFindPublicationByIdentityNeverSubstitutesAnotherVersion(t *testing.T) {
	t.Parallel()

	t.Run("exact success", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 41))
		want, _ := transition.Record()
		mock.ExpectBegin()
		expectPublicationRead(mock, want)
		mock.ExpectCommit()

		got, err := repository.FindPublicationByIdentity(context.Background(), 41, 1)
		if err != nil {
			t.Fatalf("FindPublicationByIdentity() error = %v", err)
		}
		assertPublicationsEqual(t, got, want)
		assertExpectations(t, mock)
	})

	t.Run("exact missing", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
			WithArgs(uint64(42), uint64(9)).WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		got, err := repository.FindPublicationByIdentity(context.Background(), 42, 9)
		if !reflect.DeepEqual(got, domain.ActivityPublication{}) || !errors.Is(err, application.ErrActivityPublicationNotFound) {
			t.Fatalf("FindPublicationByIdentity(missing) = %#v, %v", got, err)
		}
		assertExpectations(t, mock)
	})

	t.Run("driver detail is classified and hidden", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		cause := errors.New("secret publication scan failure")
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
			WithArgs(uint64(43), uint64(1)).WillReturnError(cause)
		mock.ExpectRollback()

		_, err := repository.FindPublicationByIdentity(context.Background(), 43, 1)
		assertSafeRepositoryError(t, err, application.ErrRepositoryFailure, cause)
		assertExpectations(t, mock)
	})
}

func TestCompareAndSwapPublicationOrdersAppendBeforeExactRootCAS(t *testing.T) {
	t.Parallel()

	t.Run("initial release", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 51))
		record, _ := transition.Record()
		mock.ExpectBegin()
		expectPublicationInsert(mock, record)
		mock.ExpectExec(regexp.QuoteMeta(updateActivityWithoutActiveSQL)).
			WithArgs("published", uint64(1), uint64(1), nil, nil, uint64(51), "draft", uint64(0)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		if err := repository.CompareAndSwapPublication(context.Background(), transition); err != nil {
			t.Fatalf("CompareAndSwapPublication() error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("lost CAS rolls inserted history back", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 52))
		record, _ := transition.Record()
		mock.ExpectBegin()
		expectPublicationInsert(mock, record)
		mock.ExpectExec(regexp.QuoteMeta(updateActivityWithoutActiveSQL)).
			WithArgs("published", uint64(1), uint64(1), nil, nil, uint64(52), "draft", uint64(0)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		err := repository.CompareAndSwapPublication(context.Background(), transition)
		if !errors.Is(err, application.ErrActivityStateConflict) {
			t.Fatalf("CompareAndSwapPublication(CAS lost) error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("duplicate publication is conflict and does not replay", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 53))
		record, _ := transition.Record()
		cause := &drivermysql.MySQLError{Number: 1062, Message: "secret publication key"}
		mock.ExpectBegin()
		expectPublicationHeaderError(mock, record, cause)
		mock.ExpectRollback()

		err := repository.CompareAndSwapPublication(context.Background(), transition)
		assertSafeRepositoryError(t, err, application.ErrActivityStateConflict, cause)
		assertExpectations(t, mock)
	})

	t.Run("binding failure rolls header back", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 54))
		record, _ := transition.Record()
		cause := errors.New("secret binding write failed")
		mock.ExpectBegin()
		expectPublicationHeader(mock, record)
		first := record.StrategyRevisionManifest()[0]
		mock.ExpectExec(regexp.QuoteMeta(insertPublicationStrategySQL)).
			WithArgs(uint64(54), uint64(1), uint64(first.StrategyID()), string(first.Revision())).
			WillReturnError(cause)
		mock.ExpectRollback()

		err := repository.CompareAndSwapPublication(context.Background(), transition)
		assertSafeRepositoryError(t, err, application.ErrRepositoryFailure, cause)
		assertExpectations(t, mock)
	})

	t.Run("commit response loss is never converted into retry", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		transition := mustTestPublicationTransition(t, mustTestDraft(t, 55))
		record, _ := transition.Record()
		cause := driver.ErrBadConn
		mock.ExpectBegin()
		expectPublicationInsert(mock, record)
		mock.ExpectExec(regexp.QuoteMeta(updateActivityWithoutActiveSQL)).
			WithArgs("published", uint64(1), uint64(1), nil, nil, uint64(55), "draft", uint64(0)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(cause)

		err := repository.CompareAndSwapPublication(context.Background(), transition)
		assertSafeRepositoryError(t, err, application.ErrCommitOutcomeUnknown, cause)
		if errors.Is(err, application.ErrRepositoryRetryable) {
			t.Fatal("commit response loss was marked retryable")
		}
		assertExpectations(t, mock)
	})
}

func TestCompareAndSwapPublicationPersistsRollbackAsNewExactVersion(t *testing.T) {
	t.Parallel()

	repository, mock := newRepositoryMock(t)
	draft := mustTestDraft(t, 61)
	first := mustTestPublicationTransition(t, draft)
	target, _ := first.Record()
	second, err := domain.PlanPublish(
		first.Next(),
		testInstant().Add(time.Hour),
		testInstant().Add(48*time.Hour),
		mustTestGraphReference(t, 72, "graph:r2"),
		[]domain.LotteryStrategyRevisionReference{mustTestStrategyReference(t, 33, "strategy:r2")},
		mustTestEvidence(t, "approval/release-2"),
		testInstant().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanPublish(second): %v", err)
	}
	rollback, err := domain.PlanRollback(
		second.Next(),
		target,
		true,
		mustTestEvidence(t, "approval/rollback-3"),
		testInstant().Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanRollback: %v", err)
	}
	record, _ := rollback.Record()
	mock.ExpectBegin()
	expectPublicationInsert(mock, record)
	mock.ExpectExec(regexp.QuoteMeta(updateActivityWithActiveSQL)).
		WithArgs("published", uint64(3), uint64(3), nil, nil, uint64(61), "published", uint64(2), uint64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repository.CompareAndSwapPublication(context.Background(), rollback); err != nil {
		t.Fatalf("CompareAndSwapPublication(rollback) error = %v", err)
	}
	assertExpectations(t, mock)
}

func TestCompareAndSwapRetirementOnlyChangesRootWithExactCAS(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		published := mustTestPublicationTransition(t, mustTestDraft(t, 71)).Next()
		transition := mustTestRetirementTransition(t, published)
		next := transition.Next()
		retiredAt, _ := next.RetiredAt()
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(updateActivityWithActiveSQL)).
			WithArgs("retired", uint64(2), uint64(1), retiredAt, "retirement/change-1", uint64(71), "published", uint64(1), uint64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		if err := repository.CompareAndSwapRetirement(context.Background(), transition); err != nil {
			t.Fatalf("CompareAndSwapRetirement() error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("lost CAS", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		published := mustTestPublicationTransition(t, mustTestDraft(t, 72)).Next()
		transition := mustTestRetirementTransition(t, published)
		next := transition.Next()
		retiredAt, _ := next.RetiredAt()
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(updateActivityWithActiveSQL)).
			WithArgs("retired", uint64(2), uint64(1), retiredAt, "retirement/change-1", uint64(72), "published", uint64(1), uint64(1)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		if err := repository.CompareAndSwapRetirement(context.Background(), transition); !errors.Is(err, application.ErrActivityStateConflict) {
			t.Fatalf("CompareAndSwapRetirement(CAS lost) error = %v", err)
		}
		assertExpectations(t, mock)
	})

	t.Run("commit response loss is unknown", func(t *testing.T) {
		t.Parallel()
		repository, mock := newRepositoryMock(t)
		published := mustTestPublicationTransition(t, mustTestDraft(t, 73)).Next()
		transition := mustTestRetirementTransition(t, published)
		next := transition.Next()
		retiredAt, _ := next.RetiredAt()
		cause := errors.New("secret retirement commit response was lost")
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(updateActivityWithActiveSQL)).
			WithArgs("retired", uint64(2), uint64(1), retiredAt, "retirement/change-1", uint64(73), "published", uint64(1), uint64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(cause)

		err := repository.CompareAndSwapRetirement(context.Background(), transition)
		assertSafeRepositoryError(t, err, application.ErrCommitOutcomeUnknown, cause)
		assertExpectations(t, mock)
	})
}

func TestReadCommitFailureIsStorageFailureNotUnknownWriteOutcome(t *testing.T) {
	t.Parallel()

	repository, mock := newRepositoryMock(t)
	draft := mustTestDraft(t, 81)
	cause := errors.New("secret read commit failure")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectActivitySQL)).
		WithArgs(uint64(81)).WillReturnRows(storedActivityRows(draft))
	mock.ExpectCommit().WillReturnError(cause)

	_, err := repository.FindActivityByID(context.Background(), 81)
	assertSafeRepositoryError(t, err, application.ErrRepositoryFailure, cause)
	if errors.Is(err, application.ErrCommitOutcomeUnknown) {
		t.Fatal("read-only commit was classified as an unknown write outcome")
	}
	assertExpectations(t, mock)
}

func TestRepositoryErrorClassificationPreservesOnlyReviewedClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cause error
		want  error
	}{
		{name: "canceled", cause: context.Canceled, want: context.Canceled},
		{name: "deadline", cause: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "lock timeout", cause: &drivermysql.MySQLError{Number: 1205, Message: "secret lock"}, want: application.ErrRepositoryRetryable},
		{name: "deadlock", cause: &drivermysql.MySQLError{Number: 1213, Message: "secret deadlock"}, want: application.ErrRepositoryRetryable},
		{name: "bad connection", cause: driver.ErrBadConn, want: application.ErrRepositoryRetryable},
		{name: "other", cause: errors.New("secret storage detail"), want: application.ErrRepositoryFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyOperationError(test.cause)
			if !errors.Is(err, test.want) {
				t.Fatalf("classifyOperationError(%T) = %v, want %v", test.cause, err, test.want)
			}
			if test.want == context.Canceled || test.want == context.DeadlineExceeded {
				if err != test.want {
					t.Fatalf("context error = %v, want raw %v", err, test.want)
				}
				return
			}
			assertSafeRepositoryError(t, err, test.want, test.cause)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := classifyWriteCommitError(ctx, sql.ErrTxDone); err != context.Canceled {
		t.Fatalf("canceled write transaction error = %v, want context canceled", err)
	}
	if err := classifyReadCommitError(ctx, context.Canceled); err != context.Canceled {
		t.Fatalf("canceled read transaction error = %v, want context canceled", err)
	}
}

func TestSQLStoplineKeepsHistoryAppendOnlyExactAndBounded(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "repository.go"))
	if err != nil {
		t.Fatalf("ReadFile(repository.go): %v", err)
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)UPDATE\s+marketing_activity_publication(?:\s|_)`),
		regexp.MustCompile(`(?is)DELETE\s+FROM\s+marketing_activity_publication(?:\s|_)`),
		regexp.MustCompile(`(?is)MAX\s*\(\s*activity_version\s*\)`),
		regexp.MustCompile(`(?is)ORDER\s+BY\s+activity_version\s+DESC`),
		regexp.MustCompile(`(?is)ON\s+DUPLICATE\s+KEY`),
		regexp.MustCompile(`(?is)REPLACE\s+INTO`),
	} {
		if forbidden.Match(source) {
			t.Fatalf("repository crosses SQL stopline %q", forbidden)
		}
	}
	if !strings.Contains(selectPublicationSQL, "activity_id = ? AND activity_version = ?") {
		t.Fatalf("publication selector is not exact: %s", selectPublicationSQL)
	}
	if !strings.Contains(selectPublicationStrategiesSQL, "LIMIT 129") {
		t.Fatalf("publication manifest selector is not bounded: %s", selectPublicationStrategiesSQL)
	}
	if strings.Contains(strings.ToLower(selectPublicationSQL), "latest") ||
		strings.Contains(strings.ToLower(selectPublicationSQL), "max(") {
		t.Fatalf("publication selector guesses a revision: %s", selectPublicationSQL)
	}
}

func newRepositoryMock(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository, err := New(sqlx.NewDb(database, "sqlmock"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return repository, mock
}

func mustTestDraft(t *testing.T, id domain.ActivityID) domain.Activity {
	t.Helper()
	activity, err := domain.NewActivity(id, "Activity "+testIDString(uint64(id)))
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	return activity
}

func mustTestPublicationTransition(t *testing.T, draft domain.Activity) domain.ActivityTransition {
	t.Helper()
	transition, err := domain.PlanPublish(
		draft,
		testInstant(),
		testInstant().Add(24*time.Hour),
		mustTestGraphReference(t, 71, "graph:r1"),
		[]domain.LotteryStrategyRevisionReference{
			mustTestStrategyReference(t, 22, "strategy:22-r1"),
			mustTestStrategyReference(t, 11, "strategy:11-r1"),
		},
		mustTestEvidence(t, "approval/release-1"),
		testInstant().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanPublish: %v", err)
	}
	return transition
}

func mustTestRetirementTransition(t *testing.T, published domain.Activity) domain.ActivityTransition {
	t.Helper()
	transition, err := domain.PlanRetire(
		published,
		mustTestEvidence(t, "retirement/change-1"),
		testInstant().Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanRetire: %v", err)
	}
	return transition
}

func mustTestGraphReference(
	t *testing.T,
	id domain.LotteryGraphID,
	revision string,
) domain.LotteryGraphReference {
	t.Helper()
	reference, err := domain.NewLotteryGraphReference(id, revision)
	if err != nil {
		t.Fatalf("NewLotteryGraphReference: %v", err)
	}
	return reference
}

func mustTestStrategyReference(
	t *testing.T,
	id domain.LotteryStrategyID,
	revision string,
) domain.LotteryStrategyRevisionReference {
	t.Helper()
	reference, err := domain.NewLotteryStrategyRevisionReference(id, revision)
	if err != nil {
		t.Fatalf("NewLotteryStrategyRevisionReference: %v", err)
	}
	return reference
}

func mustTestEvidence(t *testing.T, value string) domain.EvidenceReference {
	t.Helper()
	reference, err := domain.NewEvidenceReference(value)
	if err != nil {
		t.Fatalf("NewEvidenceReference: %v", err)
	}
	return reference
}

func testInstant() time.Time {
	return time.Date(2026, time.August, 30, 8, 0, 0, 123456000, time.UTC)
}

func testIDString(id uint64) string {
	const digits = "0123456789"
	if id == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for id > 0 {
		index--
		buffer[index] = digits[id%10]
		id /= 10
	}
	return string(buffer[index:])
}

func activityColumns() []string {
	return []string{
		"activity_id", "name", "lifecycle_state", "state_version", "active_version",
		"retired_at", "retirement_reference",
	}
}

func storedActivityRows(activity domain.Activity) *sqlmock.Rows {
	var active any
	if activity.ActivePublicationVersion() != 0 {
		active = uint64(activity.ActivePublicationVersion())
	}
	var retiredAt any
	if value, ok := activity.RetiredAt(); ok {
		retiredAt = value
	}
	var retirementReference any
	if value, ok := activity.RetirementReference(); ok {
		retirementReference = value.String()
	}
	return sqlmock.NewRows(activityColumns()).AddRow(
		uint64(activity.ID()),
		activity.Name().String(),
		string(activity.Lifecycle()),
		uint64(activity.StateVersion()),
		active,
		retiredAt,
		retirementReference,
	)
}

func publicationColumns() []string {
	return []string{
		"schema_version", "publication_kind", "rollback_of_version", "graph_id",
		"graph_revision", "starts_at", "ends_at", "published_at", "approval_reference",
	}
}

func storedPublicationRows(publication domain.ActivityPublication) *sqlmock.Rows {
	rollbackOf, rollback := publication.RollbackOf()
	var rollbackValue any
	if rollback {
		rollbackValue = uint64(rollbackOf)
	}
	graph := publication.GraphReference()
	return sqlmock.NewRows(publicationColumns()).AddRow(
		uint16(publication.SchemaVersion()),
		string(publication.Kind()),
		rollbackValue,
		uint64(graph.ID()),
		string(graph.Revision()),
		publication.StartsAt(),
		publication.EndsAt(),
		publication.PublishedAt(),
		publication.ApprovalEvidenceReference().String(),
	)
}

func storedStrategyRows(publication domain.ActivityPublication) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"strategy_id", "strategy_revision"})
	for _, reference := range publication.StrategyRevisionManifest() {
		rows.AddRow(uint64(reference.StrategyID()), string(reference.Revision()))
	}
	return rows
}

func expectPublicationRead(mock sqlmock.Sqlmock, publication domain.ActivityPublication) {
	mock.ExpectQuery(regexp.QuoteMeta(selectPublicationSQL)).
		WithArgs(uint64(publication.ActivityID()), uint64(publication.Version())).
		WillReturnRows(storedPublicationRows(publication))
	mock.ExpectQuery(regexp.QuoteMeta(selectPublicationStrategiesSQL)).
		WithArgs(uint64(publication.ActivityID()), uint64(publication.Version())).
		WillReturnRows(storedStrategyRows(publication))
}

func expectPublicationHeader(mock sqlmock.Sqlmock, publication domain.ActivityPublication) {
	rollbackOf, rollback := publication.RollbackOf()
	var rollbackArgument any
	if rollback {
		rollbackArgument = uint64(rollbackOf)
	}
	graph := publication.GraphReference()
	mock.ExpectExec(regexp.QuoteMeta(insertPublicationSQL)).
		WithArgs(
			uint64(publication.ActivityID()),
			uint64(publication.Version()),
			uint16(publication.SchemaVersion()),
			string(publication.Kind()),
			rollbackArgument,
			uint64(graph.ID()),
			string(graph.Revision()),
			publication.StartsAt(),
			publication.EndsAt(),
			publication.PublishedAt(),
			publication.ApprovalEvidenceReference().String(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectPublicationHeaderError(
	mock sqlmock.Sqlmock,
	publication domain.ActivityPublication,
	cause error,
) {
	rollbackOf, rollback := publication.RollbackOf()
	var rollbackArgument any
	if rollback {
		rollbackArgument = uint64(rollbackOf)
	}
	graph := publication.GraphReference()
	mock.ExpectExec(regexp.QuoteMeta(insertPublicationSQL)).
		WithArgs(
			uint64(publication.ActivityID()),
			uint64(publication.Version()),
			uint16(publication.SchemaVersion()),
			string(publication.Kind()),
			rollbackArgument,
			uint64(graph.ID()),
			string(graph.Revision()),
			publication.StartsAt(),
			publication.EndsAt(),
			publication.PublishedAt(),
			publication.ApprovalEvidenceReference().String(),
		).
		WillReturnError(cause)
}

func expectPublicationInsert(mock sqlmock.Sqlmock, publication domain.ActivityPublication) {
	expectPublicationHeader(mock, publication)
	for _, reference := range publication.StrategyRevisionManifest() {
		mock.ExpectExec(regexp.QuoteMeta(insertPublicationStrategySQL)).
			WithArgs(
				uint64(publication.ActivityID()),
				uint64(publication.Version()),
				uint64(reference.StrategyID()),
				string(reference.Revision()),
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

func assertActivitiesEqual(t *testing.T, got, want domain.Activity) {
	t.Helper()
	if got.ID() != want.ID() || got.Name() != want.Name() || got.Lifecycle() != want.Lifecycle() ||
		got.StateVersion() != want.StateVersion() ||
		got.ActivePublicationVersion() != want.ActivePublicationVersion() {
		t.Fatalf("Activity = %#v, want %#v", got, want)
	}
	gotRetiredAt, gotRetired := got.RetiredAt()
	wantRetiredAt, wantRetired := want.RetiredAt()
	gotReference, gotHasReference := got.RetirementReference()
	wantReference, wantHasReference := want.RetirementReference()
	if gotRetired != wantRetired || gotRetiredAt != wantRetiredAt ||
		gotHasReference != wantHasReference || gotReference != wantReference {
		t.Fatalf("Activity retirement = %v/%v/%q/%v, want %v/%v/%q/%v",
			gotRetiredAt, gotRetired, gotReference, gotHasReference,
			wantRetiredAt, wantRetired, wantReference, wantHasReference)
	}
}

func assertPublicationsEqual(t *testing.T, got, want domain.ActivityPublication) {
	t.Helper()
	gotRollback, gotIsRollback := got.RollbackOf()
	wantRollback, wantIsRollback := want.RollbackOf()
	if got.ActivityID() != want.ActivityID() || got.Version() != want.Version() ||
		got.SchemaVersion() != want.SchemaVersion() || got.Kind() != want.Kind() ||
		gotRollback != wantRollback || gotIsRollback != wantIsRollback ||
		got.StartsAt() != want.StartsAt() || got.EndsAt() != want.EndsAt() ||
		got.PublishedAt() != want.PublishedAt() || got.GraphReference() != want.GraphReference() ||
		got.ApprovalEvidenceReference() != want.ApprovalEvidenceReference() {
		t.Fatalf("publication = %#v, want %#v", got, want)
	}
	gotManifest := got.StrategyRevisionManifest()
	wantManifest := want.StrategyRevisionManifest()
	if len(gotManifest) != len(wantManifest) {
		t.Fatalf("manifest length = %d, want %d", len(gotManifest), len(wantManifest))
	}
	for index := range wantManifest {
		if gotManifest[index] != wantManifest[index] {
			t.Fatalf("manifest[%d] = %#v, want %#v", index, gotManifest[index], wantManifest[index])
		}
	}
}

func assertSafeRepositoryError(t *testing.T, err, class, cause error) {
	t.Helper()
	if !errors.Is(err, class) {
		t.Fatalf("error = %v, want class %v", err, class)
	}
	if got := err.Error(); got != class.Error() {
		t.Fatalf("rendered error = %q, want %q", got, class)
	}
	if errors.Is(err, cause) {
		t.Fatalf("ordinary error chain exposes cause %v", cause)
	}
	var repositoryError *application.RepositoryError
	if !errors.As(err, &repositoryError) || repositoryError.Cause() != cause {
		t.Fatalf("trusted cause = %#v, want %#v", repositoryError, cause)
	}
}

func assertExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sqlmock expectations: %v", err)
	}
}
