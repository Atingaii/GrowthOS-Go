package mysqlrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	"github.com/DATA-DOG/go-sqlmock"
	drivermysql "github.com/go-sql-driver/mysql"
)

func mustMaintenanceOperation(t *testing.T, now time.Time) identityapp.MaintenanceOperation {
	t.Helper()
	operation, err := identityapp.NewMaintenanceOperation(now)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func sessionMaintenanceRows(values ...sessionMaintenanceCandidate) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"session_ref", "cleanup_at"})
	for _, value := range values {
		rows.AddRow(value.reference, value.cleanupAt)
	}
	return rows
}

func throttleMaintenanceRows(values ...throttleMaintenanceCandidate) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"dimension", "subject_digest", "row_expires_at"})
	for _, value := range values {
		rows.AddRow(value.dimension, value.digest, value.cleanupAt)
	}
	return rows
}

func TestRunMaintenanceUsesClosedCutoffsStableOrderAndIndependentBudgets(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	cutoff := operation.SessionCutoff()
	repository, mock := newRepositoryMock(t, func() time.Time { return now })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(cutoff, identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows(
			sessionMaintenanceCandidate{reference: "session:d", cleanupAt: cutoff},
			sessionMaintenanceCandidate{reference: "session:c", cleanupAt: cutoff.Add(-time.Hour)},
			sessionMaintenanceCandidate{reference: "session:a", cleanupAt: cutoff.Add(-2 * time.Hour)},
		))
	mock.ExpectQuery(regexp.QuoteMeta(selectRevokedSessionMaintenanceCandidatesSQL)).
		WithArgs(cutoff, identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows(
			sessionMaintenanceCandidate{reference: "session:b", cleanupAt: cutoff.Add(-3 * time.Hour)},
			sessionMaintenanceCandidate{reference: "session:a", cleanupAt: cutoff.Add(-4 * time.Hour)},
		))
	sessionDelete, err := buildSessionMaintenanceDeleteSQL(4)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(sessionDelete)).
		WithArgs("session:a", "session:b", "session:c", "session:d", cutoff, cutoff).
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()

	digestTwo := bytes.Repeat([]byte{2}, 32)
	digestOne := bytes.Repeat([]byte{1}, 32)
	digestThree := bytes.Repeat([]byte{3}, 32)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectThrottleMaintenanceCandidatesSQL)).
		WithArgs(now, identityapp.MaintenanceThrottleBudget).
		WillReturnRows(throttleMaintenanceRows(
			throttleMaintenanceCandidate{dimension: "source", digest: digestThree, cleanupAt: now},
			throttleMaintenanceCandidate{dimension: "login", digest: digestTwo, cleanupAt: now.Add(-time.Hour)},
			throttleMaintenanceCandidate{dimension: "login", digest: digestOne, cleanupAt: now.Add(-time.Hour)},
		))
	throttleDelete, err := buildThrottleMaintenanceDeleteSQL(3)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(throttleDelete)).WithArgs(
		"login", digestOne,
		"login", digestTwo,
		"source", digestThree,
		now,
	).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	result, err := repository.RunMaintenance(context.Background(), operation)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionsDeleted() != 4 || result.ThrottlesDeleted() != 3 || result.TotalDeleted() != 7 {
		t.Fatalf("result=%#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunMaintenanceEmptyBatchesRollbackWithoutDelete(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows())
	mock.ExpectQuery(regexp.QuoteMeta(selectRevokedSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows())
	mock.ExpectRollback()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectThrottleMaintenanceCandidatesSQL)).
		WithArgs(now, identityapp.MaintenanceThrottleBudget).
		WillReturnRows(throttleMaintenanceRows())
	mock.ExpectRollback()

	result, err := repository.RunMaintenance(context.Background(), operation)
	if err != nil || result.TotalDeleted() != 0 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionMaintenanceFailureStopsBeforeThrottleStage(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	privateLockError := &drivermysql.MySQLError{Number: 1205, Message: "private lock owner"}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnError(privateLockError)
	mock.ExpectRollback()

	_, err := repository.RunMaintenance(context.Background(), operation)
	assertSafeDependencyError(t, err, identityapp.ErrDependencyUnavailable)
	if strings.Contains(err.Error(), "private") {
		t.Fatalf("error leaked storage detail: %v", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestMaintenanceDeleteRechecksEligibilityAndFailsClosedOnRace(t *testing.T) {
	t.Parallel()
	t.Run("session candidate disappeared", func(t *testing.T) {
		now := testInstant(0)
		operation := mustMaintenanceOperation(t, now)
		repository, mock := newRepositoryMock(t, func() time.Time { return now })
		candidate := sessionMaintenanceCandidate{reference: "session:race", cleanupAt: operation.SessionCutoff()}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
			WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
			WillReturnRows(sessionMaintenanceRows(candidate))
		mock.ExpectQuery(regexp.QuoteMeta(selectRevokedSessionMaintenanceCandidatesSQL)).
			WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
			WillReturnRows(sessionMaintenanceRows())
		query, err := buildSessionMaintenanceDeleteSQL(1)
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec(regexp.QuoteMeta(query)).
			WithArgs(candidate.reference, operation.SessionCutoff(), operation.SessionCutoff()).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		_, err = repository.RunMaintenance(context.Background(), operation)
		assertSafeDependencyError(t, err, identityapp.ErrDependencyUnavailable)
		if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
	})

	t.Run("throttle became inflight", func(t *testing.T) {
		now := testInstant(0)
		operation := mustMaintenanceOperation(t, now)
		repository, mock := newRepositoryMock(t, func() time.Time { return now })
		expectEmptySessionStage(mock, operation)
		digest := bytes.Repeat([]byte{9}, 32)
		candidate := throttleMaintenanceCandidate{dimension: "login", digest: digest, cleanupAt: now}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectThrottleMaintenanceCandidatesSQL)).
			WithArgs(now, identityapp.MaintenanceThrottleBudget).
			WillReturnRows(throttleMaintenanceRows(candidate))
		query, err := buildThrottleMaintenanceDeleteSQL(1)
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec(regexp.QuoteMeta(query)).
			WithArgs("login", digest, now).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		_, err = repository.RunMaintenance(context.Background(), operation)
		assertSafeDependencyError(t, err, identityapp.ErrDependencyUnavailable)
		if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
	})
}

func TestSessionMaintenanceCommitUnknownStopsSecondStageAndStaysRedacted(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	candidate := sessionMaintenanceCandidate{reference: "session:unknown", cleanupAt: operation.SessionCutoff()}
	privateCommit := errors.New("private mysql commit acknowledgement")

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows(candidate))
	mock.ExpectQuery(regexp.QuoteMeta(selectRevokedSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows())
	query, err := buildSessionMaintenanceDeleteSQL(1)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(query)).
		WithArgs(candidate.reference, operation.SessionCutoff(), operation.SessionCutoff()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(privateCommit)

	_, err = repository.RunMaintenance(context.Background(), operation)
	assertSafeDependencyError(t, err, identityapp.ErrCommitOutcomeUnknown)
	if strings.Contains(fmt.Sprintf("%v %#v", err, err), "private") {
		t.Fatalf("commit error leaked private detail: %v %#v", err, err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestThrottleMaintenanceFailuresAfterCommittedSessionStayExplicit(t *testing.T) {
	t.Parallel()

	t.Run("begin cancellation", func(t *testing.T) {
		now := testInstant(0)
		operation := mustMaintenanceOperation(t, now)
		repository, mock := newRepositoryMock(t, func() time.Time { return now })
		expectCommittedSessionMaintenanceStage(t, mock, operation)
		mock.ExpectBegin().WillReturnError(context.Canceled)

		result, err := repository.RunMaintenance(context.Background(), operation)
		if !errors.Is(err, context.Canceled) || result.TotalDeleted() != 0 {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
	})

	t.Run("selection deadline", func(t *testing.T) {
		now := testInstant(0)
		operation := mustMaintenanceOperation(t, now)
		repository, mock := newRepositoryMock(t, func() time.Time { return now })
		expectCommittedSessionMaintenanceStage(t, mock, operation)
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectThrottleMaintenanceCandidatesSQL)).
			WithArgs(now, identityapp.MaintenanceThrottleBudget).
			WillReturnError(context.DeadlineExceeded)
		mock.ExpectRollback()

		result, err := repository.RunMaintenance(context.Background(), operation)
		if !errors.Is(err, context.DeadlineExceeded) || result.TotalDeleted() != 0 {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
	})

	t.Run("conditional delete failure", func(t *testing.T) {
		now := testInstant(0)
		operation := mustMaintenanceOperation(t, now)
		repository, mock := newRepositoryMock(t, func() time.Time { return now })
		expectCommittedSessionMaintenanceStage(t, mock, operation)
		digest := bytes.Repeat([]byte{7}, 32)
		candidate := throttleMaintenanceCandidate{dimension: "login", digest: digest, cleanupAt: now}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectThrottleMaintenanceCandidatesSQL)).
			WithArgs(now, identityapp.MaintenanceThrottleBudget).
			WillReturnRows(throttleMaintenanceRows(candidate))
		query, err := buildThrottleMaintenanceDeleteSQL(1)
		if err != nil {
			t.Fatal(err)
		}
		privateDelete := &drivermysql.MySQLError{Number: 1205, Message: "private throttle lock owner"}
		mock.ExpectExec(regexp.QuoteMeta(query)).
			WithArgs("login", digest, now).
			WillReturnError(privateDelete)
		mock.ExpectRollback()

		result, err := repository.RunMaintenance(context.Background(), operation)
		assertSafeDependencyError(t, err, identityapp.ErrDependencyUnavailable)
		if result.TotalDeleted() != 0 || strings.Contains(err.Error(), "private") {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
	})

	t.Run("commit acknowledgement unknown", func(t *testing.T) {
		now := testInstant(0)
		operation := mustMaintenanceOperation(t, now)
		repository, mock := newRepositoryMock(t, func() time.Time { return now })
		expectCommittedSessionMaintenanceStage(t, mock, operation)
		digest := bytes.Repeat([]byte{8}, 32)
		candidate := throttleMaintenanceCandidate{dimension: "source", digest: digest, cleanupAt: now}
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(selectThrottleMaintenanceCandidatesSQL)).
			WithArgs(now, identityapp.MaintenanceThrottleBudget).
			WillReturnRows(throttleMaintenanceRows(candidate))
		query, err := buildThrottleMaintenanceDeleteSQL(1)
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec(regexp.QuoteMeta(query)).
			WithArgs("source", digest, now).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errors.New("private throttle commit acknowledgement"))

		result, err := repository.RunMaintenance(context.Background(), operation)
		assertSafeDependencyError(t, err, identityapp.ErrCommitOutcomeUnknown)
		if result.TotalDeleted() != 0 || strings.Contains(fmt.Sprintf("%v %#v", err, err), "private") {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
			t.Fatal(expectationErr)
		}
	})
}

func TestMaintenanceHonorsCancellationWithoutOpeningATransaction(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := repository.RunMaintenance(ctx, operation)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestMaintenanceSelectionCancellationRollsBackAndStops(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnError(context.Canceled)
	mock.ExpectRollback()
	_, err := repository.RunMaintenance(context.Background(), operation)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestMaintenanceRejectsCandidateOutsideClosedEligibilityBoundary(t *testing.T) {
	t.Parallel()
	now := testInstant(0)
	operation := mustMaintenanceOperation(t, now)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	// A still-current row cannot satisfy the real SELECT. If a driver or query
	// contract ever returns it anyway, the adapter refuses to issue any DELETE.
	active := sessionMaintenanceCandidate{
		reference: "session:active",
		cleanupAt: operation.SessionCutoff().Add(time.Microsecond),
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows(active))
	mock.ExpectRollback()
	_, err := repository.RunMaintenance(context.Background(), operation)
	assertSafeDependencyError(t, err, identityapp.ErrStoredIdentityInvalid)
	if expectationErr := mock.ExpectationsWereMet(); expectationErr != nil {
		t.Fatal(expectationErr)
	}
}

func TestMaintenanceCandidateAndDynamicSQLBounds(t *testing.T) {
	t.Parallel()
	cutoff := testInstant(0)
	expired := make([]sessionMaintenanceCandidate, identityapp.MaintenanceSessionBudget)
	revoked := make([]sessionMaintenanceCandidate, identityapp.MaintenanceSessionBudget)
	for index := 0; index < identityapp.MaintenanceSessionBudget; index++ {
		expired[index] = sessionMaintenanceCandidate{
			reference: fmt.Sprintf("expired:%03d", index),
			cleanupAt: cutoff.Add(time.Duration(index) * time.Microsecond),
		}
		revoked[index] = sessionMaintenanceCandidate{
			reference: fmt.Sprintf("revoked:%03d", index),
			cleanupAt: cutoff.Add(time.Duration(index) * time.Microsecond),
		}
	}
	merged, err := mergeSessionMaintenanceCandidates(
		expired,
		revoked,
		identityapp.MaintenanceSessionBudget,
	)
	if err != nil || len(merged) != identityapp.MaintenanceSessionBudget {
		t.Fatalf("len=%d err=%v", len(merged), err)
	}
	if _, err := buildSessionMaintenanceDeleteSQL(identityapp.MaintenanceSessionBudget); err != nil {
		t.Fatal(err)
	}
	if _, err := buildThrottleMaintenanceDeleteSQL(identityapp.MaintenanceThrottleBudget); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []int{0, -1, identityapp.MaintenanceSessionBudget + 1} {
		if _, err := buildSessionMaintenanceDeleteSQL(invalid); err == nil {
			t.Fatalf("session count %d accepted", invalid)
		}
		if _, err := buildThrottleMaintenanceDeleteSQL(invalid); err == nil {
			t.Fatalf("throttle count %d accepted", invalid)
		}
	}
}

func TestThrottleCandidateDigestsAreExplicitlyCleared(t *testing.T) {
	t.Parallel()
	candidates := []throttleMaintenanceCandidate{
		{digest: bytes.Repeat([]byte{1}, 32)},
		{digest: bytes.Repeat([]byte{2}, 32)},
	}
	references := [][]byte{candidates[0].digest, candidates[1].digest}
	clearThrottleMaintenanceCandidates(candidates)
	for index, reference := range references {
		if !bytes.Equal(reference, make([]byte, len(reference))) || candidates[index].digest != nil {
			t.Fatalf("digest %d was not cleared", index)
		}
	}
}

func TestMaintenanceSQLKeepsIndexedStableSelectionAndConditionalDeletes(t *testing.T) {
	t.Parallel()
	checks := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "expired session candidates",
			sql:  selectExpiredSessionMaintenanceCandidatesSQL,
			want: []string{
				"USE INDEX (idx_identity_session_absolute_cleanup)",
				"absolute_expires_at <= ?",
				"ORDER BY absolute_expires_at ASC, session_ref ASC",
				"LIMIT ?",
			},
		},
		{
			name: "revoked session candidates",
			sql:  selectRevokedSessionMaintenanceCandidatesSQL,
			want: []string{
				"USE INDEX (idx_identity_session_revoked_cleanup)",
				"revoked_at IS NOT NULL AND revoked_at <= ?",
				"ORDER BY revoked_at ASC, session_ref ASC",
				"LIMIT ?",
			},
		},
		{
			name: "throttle candidates",
			sql:  selectThrottleMaintenanceCandidatesSQL,
			want: []string{
				"USE INDEX (idx_identity_throttle_cleanup)",
				"row_expires_at <= ?",
				"inflight_count = 0",
				"inflight_expires_at IS NULL",
				"ORDER BY row_expires_at ASC, dimension ASC, subject_digest ASC",
				"LIMIT ?",
			},
		},
	}
	for _, check := range checks {
		for _, fragment := range check.want {
			if !strings.Contains(check.sql, fragment) {
				t.Fatalf("%s lost %q", check.name, fragment)
			}
		}
	}
	sessionDelete, err := buildSessionMaintenanceDeleteSQL(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"WHERE session_ref IN (?)",
		"absolute_expires_at <= ?",
		"revoked_at IS NOT NULL AND revoked_at <= ?",
	} {
		if !strings.Contains(sessionDelete, fragment) {
			t.Fatalf("session delete lost recheck %q", fragment)
		}
	}
	throttleDelete, err := buildThrottleMaintenanceDeleteSQL(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"WHERE (dimension, subject_digest) IN ((?, ?))",
		"row_expires_at <= ?",
		"inflight_count = 0",
		"inflight_expires_at IS NULL",
	} {
		if !strings.Contains(throttleDelete, fragment) {
			t.Fatalf("throttle delete lost recheck %q", fragment)
		}
	}
	for _, statement := range []string{sessionDelete, throttleDelete} {
		if !strings.Contains(statement, "WHERE") || strings.HasSuffix(strings.TrimSpace(statement), "DELETE FROM") {
			t.Fatalf("unbounded delete: %q", statement)
		}
	}
}

func expectEmptySessionStage(mock sqlmock.Sqlmock, operation identityapp.MaintenanceOperation) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows())
	mock.ExpectQuery(regexp.QuoteMeta(selectRevokedSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows())
	mock.ExpectRollback()
}

func expectCommittedSessionMaintenanceStage(
	t *testing.T,
	mock sqlmock.Sqlmock,
	operation identityapp.MaintenanceOperation,
) {
	t.Helper()
	candidate := sessionMaintenanceCandidate{
		reference: "session:committed",
		cleanupAt: operation.SessionCutoff(),
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectExpiredSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows(candidate))
	mock.ExpectQuery(regexp.QuoteMeta(selectRevokedSessionMaintenanceCandidatesSQL)).
		WithArgs(operation.SessionCutoff(), identityapp.MaintenanceSessionBudget).
		WillReturnRows(sessionMaintenanceRows())
	query, err := buildSessionMaintenanceDeleteSQL(1)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(query)).
		WithArgs(candidate.reference, operation.SessionCutoff(), operation.SessionCutoff()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}
