package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestNewStrategySnapshotRepositoryRejectsNilDatabase(t *testing.T) {
	t.Parallel()

	repository, err := NewStrategySnapshotRepository(nil)
	if repository != nil || !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("NewStrategySnapshotRepository(nil) = %#v, %v; want nil/not configured", repository, err)
	}
}

func TestStrategySnapshotRepositoryRejectsInvalidReceiverContextAndDomainBeforeSQL(t *testing.T) {
	t.Parallel()

	identity := mustStrategySnapshotIdentity(t, 51, "release-v1")
	var zero StrategySnapshotRepository
	if err := zero.CreateSnapshot(context.Background(), domain.StrategySnapshot{}); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero CreateSnapshot() error = %v, want not configured", err)
	}
	if _, err := zero.FindSnapshotByIdentity(context.Background(), identity); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero FindSnapshotByIdentity() error = %v, want not configured", err)
	}
	if err := zero.CreateSnapshot(nil, domain.StrategySnapshot{}); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("CreateSnapshot(nil) error = %v, want invalid argument", err)
	}
	if _, err := zero.FindSnapshotByIdentity(nil, domain.StrategySnapshotIdentity{}); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("FindSnapshotByIdentity(nil) error = %v, want invalid argument", err)
	}

	repository, mock := newStrategySnapshotRepositoryMock(t)
	if err := repository.CreateSnapshot(context.Background(), domain.StrategySnapshot{}); !errors.Is(err, domain.ErrStrategySnapshotInvalid) {
		t.Fatalf("CreateSnapshot(zero) error = %v, want snapshot invalid", err)
	}
	if _, err := repository.FindSnapshotByIdentity(context.Background(), domain.StrategySnapshotIdentity{}); !errors.Is(err, domain.ErrStrategySnapshotIdentityInvalid) {
		t.Fatalf("FindSnapshotByIdentity(zero) error = %v, want identity invalid", err)
	}
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotCreateWritesCanonicalAggregateInOneTransaction(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	snapshot := mustStrategySnapshot(t, 52, "release-v1")
	expectStrategySnapshotCreate(mock, snapshot)
	mock.ExpectCommit()

	if err := repository.CreateSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("CreateSnapshot() error = %v", err)
	}
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotCreateRejectsDuplicateExactIdentityWithoutConflation(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	snapshot := mustStrategySnapshot(t, 53, "duplicate-v1")
	strategy := snapshot.Strategy()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategySnapshotSQL)).
		WithArgs(uint64(53), "duplicate-v1", uint16(1), strategy.Name()).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "secret duplicate key"})
	mock.ExpectRollback()

	err := repository.CreateSnapshot(context.Background(), snapshot)
	if !errors.Is(err, application.ErrStrategySnapshotAlreadyExists) {
		t.Fatalf("CreateSnapshot(duplicate) error = %v, want snapshot already exists", err)
	}
	if errors.Is(err, application.ErrStrategyAlreadyExists) ||
		errors.Is(err, application.ErrStrategyRoutingGraphAlreadyExists) {
		t.Fatal("snapshot duplicate was conflated with another aggregate conflict")
	}
	if got := err.Error(); got != application.ErrStrategySnapshotAlreadyExists.Error() {
		t.Fatalf("duplicate rendered %q, want safe class", got)
	}
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotCreateChecksRowsAffectedAndRollsBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stage string
	}{
		{name: "header", stage: "header"},
		{name: "award", stage: "award"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, mock := newStrategySnapshotRepositoryMock(t)
			snapshot := mustStrategySnapshot(t, 54, test.stage+"-affected-v1")
			strategy := snapshot.Strategy()
			identity := snapshot.Identity()
			mock.ExpectBegin()
			headerAffected := int64(1)
			if test.stage == "header" {
				headerAffected = 0
			}
			mock.ExpectExec(regexp.QuoteMeta(insertStrategySnapshotSQL)).
				WithArgs(uint64(identity.ID()), string(identity.Revision()), uint16(1), strategy.Name()).
				WillReturnResult(sqlmock.NewResult(0, headerAffected))
			if test.stage == "award" {
				statement := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategySnapshotAwardSQL))
				award := strategy.Awards()[0]
				statement.ExpectExec().
					WithArgs(
						uint64(identity.ID()),
						string(identity.Revision()),
						uint64(award.ID()),
						award.Name(),
						uint64(award.Weight()),
						string(award.Outcome()),
					).
					WillReturnResult(sqlmock.NewResult(0, 0))
				statement.WillBeClosed()
			}
			mock.ExpectRollback()

			err := repository.CreateSnapshot(context.Background(), snapshot)
			if !errors.Is(err, application.ErrRepositoryFailure) {
				t.Fatalf("CreateSnapshot(%s affected) error = %v, want repository failure", test.stage, err)
			}
			assertStrategySnapshotExpectations(t, mock)
		})
	}
}

func TestStrategySnapshotCreateRollsBackAwardDriverFailure(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	snapshot := mustStrategySnapshot(t, 55, "rollback-v1")
	strategy := snapshot.Strategy()
	identity := snapshot.Identity()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategySnapshotSQL)).
		WithArgs(uint64(55), "rollback-v1", uint16(1), strategy.Name()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	statement := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategySnapshotAwardSQL))
	award := strategy.Awards()[0]
	statement.ExpectExec().
		WithArgs(
			uint64(identity.ID()),
			string(identity.Revision()),
			uint64(award.ID()),
			award.Name(),
			uint64(award.Weight()),
			string(award.Outcome()),
		).
		WillReturnError(&mysql.MySQLError{Number: 1452, Message: "secret child failure"})
	statement.WillBeClosed()
	mock.ExpectRollback()

	err := repository.CreateSnapshot(context.Background(), snapshot)
	if !errors.Is(err, application.ErrRepositoryFailure) {
		t.Fatalf("CreateSnapshot(child failure) error = %v, want repository failure", err)
	}
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotCreateCommitFailureIsUnknownOutcome(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	snapshot := mustStrategySnapshot(t, 56, "commit-v1")
	expectStrategySnapshotCreate(mock, snapshot)
	cause := errors.New("secret driver lost commit response")
	mock.ExpectCommit().WillReturnError(cause)

	err := repository.CreateSnapshot(context.Background(), snapshot)
	if !errors.Is(err, application.ErrCommitOutcomeUnknown) || !errors.Is(err, cause) {
		t.Fatalf("CreateSnapshot(commit failure) error = %v, want unknown outcome with cause", err)
	}
	if got := err.Error(); got != application.ErrCommitOutcomeUnknown.Error() {
		t.Fatalf("commit error rendered %q, want safe class", got)
	}
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotFindReadsExactRepeatableReadSnapshot(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	want := mustStrategySnapshot(t, 57, "read-v1")
	expectStrategySnapshotRead(mock, want.Identity(), validStoredStrategySnapshotHeader(), validStoredStrategySnapshotAwardRows())
	mock.ExpectCommit()

	got, err := repository.FindSnapshotByIdentity(context.Background(), want.Identity())
	if err != nil {
		t.Fatalf("FindSnapshotByIdentity() error = %v", err)
	}
	assertStrategySnapshotsEqual(t, got, want)
	if options := readSnapshotOptions(); options == nil ||
		*options != (sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}) {
		t.Fatalf("readSnapshotOptions() = %#v, want RR/read-only", options)
	}
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotFindMapsNotFoundAndReturnsZero(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	identity := mustStrategySnapshotIdentity(t, 58, "missing-v1")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategySnapshotSQL)).
		WithArgs(uint64(58), "missing-v1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	snapshot, err := repository.FindSnapshotByIdentity(context.Background(), identity)
	if !errors.Is(err, application.ErrStrategySnapshotNotFound) {
		t.Fatalf("FindSnapshotByIdentity(missing) error = %v, want snapshot not found", err)
	}
	if errors.Is(err, application.ErrStrategyNotFound) {
		t.Fatal("snapshot not-found was conflated with Strategy not-found")
	}
	assertZeroStoredStrategySnapshot(t, snapshot)
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotFindRejectsUnknownSchemaBeforeReadingAwards(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	identity := mustStrategySnapshotIdentity(t, 59, "future-v1")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategySnapshotSQL)).
		WithArgs(uint64(59), "future-v1").
		WillReturnRows(sqlmock.NewRows([]string{"schema_version", "name"}).AddRow(uint16(2), "Stored wheel"))
	mock.ExpectRollback()

	snapshot, err := repository.FindSnapshotByIdentity(context.Background(), identity)
	if !errors.Is(err, application.ErrStoredStrategySnapshotInvalid) ||
		!errors.Is(err, domain.ErrStrategySnapshotSchemaUnsupported) {
		t.Fatalf("FindSnapshotByIdentity(future schema) error = %v, want stored invalid/schema unsupported", err)
	}
	assertZeroStoredStrategySnapshot(t, snapshot)
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotFindRejectsOversizedStoredAwardsBeforeRestore(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	identity := mustStrategySnapshotIdentity(t, 60, "oversized-v1")
	rows := sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"})
	for index := 1; index <= domain.MaxAwardsPerStrategy+1; index++ {
		rows.AddRow(uint64(index), "Stored award", uint64(1), "reward")
	}
	expectStrategySnapshotRead(mock, identity, validStoredStrategySnapshotHeader(), rows)
	mock.ExpectRollback()

	snapshot, err := repository.FindSnapshotByIdentity(context.Background(), identity)
	if !errors.Is(err, application.ErrStoredStrategySnapshotInvalid) ||
		!errors.Is(err, errStoredStrategySnapshotAwardLimit) {
		t.Fatalf("FindSnapshotByIdentity(oversized) error = %v, want stored invalid/limit", err)
	}
	assertZeroStoredStrategySnapshot(t, snapshot)
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotFindFailsClosedForMalformedStoredAggregate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header *sqlmock.Rows
		awards *sqlmock.Rows
		want   error
	}{
		{
			name:   "non canonical name",
			header: sqlmock.NewRows([]string{"schema_version", "name"}).AddRow(uint16(1), " Stored wheel"),
			awards: validStoredStrategySnapshotAwardRows(),
			want:   domain.ErrStrategyNameInvalid,
		},
		{
			name:   "missing awards",
			header: validStoredStrategySnapshotHeader(),
			awards: sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}),
			want:   domain.ErrStrategyAwardsRequired,
		},
		{
			name:   "zero award weight",
			header: validStoredStrategySnapshotHeader(),
			awards: sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).AddRow(uint64(10), "Reward", uint64(0), "reward"),
			want:   domain.ErrAwardWeightRequired,
		},
		{
			name:   "unknown outcome",
			header: validStoredStrategySnapshotHeader(),
			awards: sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).AddRow(uint64(10), "Reward", uint64(1), "coupon"),
			want:   domain.ErrAwardOutcomeInvalid,
		},
	}
	for index, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository, mock := newStrategySnapshotRepositoryMock(t)
			identity := mustStrategySnapshotIdentity(t, domain.StrategyID(70+index), "stored-invalid-v1")
			expectStrategySnapshotRead(mock, identity, test.header, test.awards)
			mock.ExpectCommit()

			snapshot, err := repository.FindSnapshotByIdentity(context.Background(), identity)
			if !errors.Is(err, application.ErrStoredStrategySnapshotInvalid) || !errors.Is(err, test.want) {
				t.Fatalf("FindSnapshotByIdentity() error = %v, want stored invalid and %v", err, test.want)
			}
			assertZeroStoredStrategySnapshot(t, snapshot)
			assertStrategySnapshotExpectations(t, mock)
		})
	}
}

func TestStrategySnapshotFindClassifiesReadCommitFailure(t *testing.T) {
	t.Parallel()

	repository, mock := newStrategySnapshotRepositoryMock(t)
	identity := mustStrategySnapshotIdentity(t, 80, "read-commit-v1")
	expectStrategySnapshotRead(mock, identity, validStoredStrategySnapshotHeader(), validStoredStrategySnapshotAwardRows())
	cause := errors.New("secret read commit failure")
	mock.ExpectCommit().WillReturnError(cause)

	snapshot, err := repository.FindSnapshotByIdentity(context.Background(), identity)
	if !errors.Is(err, application.ErrRepositoryFailure) || !errors.Is(err, cause) {
		t.Fatalf("FindSnapshotByIdentity(commit failure) error = %v, want repository failure with cause", err)
	}
	assertZeroStoredStrategySnapshot(t, snapshot)
	assertStrategySnapshotExpectations(t, mock)
}

func TestStrategySnapshotSQLSurfaceHasNoMutationOrImplicitRevisionLookup(t *testing.T) {
	t.Parallel()

	surface := strings.ToLower(strings.Join([]string{
		insertStrategySnapshotSQL,
		insertStrategySnapshotAwardSQL,
		selectStrategySnapshotSQL,
		selectStrategySnapshotAwardsSQL,
	}, "\n"))
	for _, forbidden := range []string{" update ", " delete ", " replace ", " on duplicate ", " max(", " latest ", " order by revision"} {
		if strings.Contains(surface, forbidden) {
			t.Fatalf("Strategy snapshot SQL contains forbidden %q", forbidden)
		}
	}
	if !strings.Contains(surface, "where strategy_id = ? and revision = ?") {
		t.Fatal("Strategy snapshot reads are not scoped by exact StrategyID/revision")
	}
}

func newStrategySnapshotRepositoryMock(
	t *testing.T,
) (*StrategySnapshotRepository, sqlmock.Sqlmock) {
	t.Helper()

	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository, err := NewStrategySnapshotRepository(sqlx.NewDb(database, "sqlmock"))
	if err != nil {
		t.Fatalf("NewStrategySnapshotRepository() error = %v", err)
	}
	return repository, mock
}

func expectStrategySnapshotCreate(mock sqlmock.Sqlmock, snapshot domain.StrategySnapshot) {
	identity := snapshot.Identity()
	strategy := snapshot.Strategy()
	strategyID := uint64(identity.ID())
	revision := string(identity.Revision())
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategySnapshotSQL)).
		WithArgs(strategyID, revision, uint16(snapshot.SchemaVersion()), strategy.Name()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	statement := mock.ExpectPrepare(regexp.QuoteMeta(insertStrategySnapshotAwardSQL))
	for _, award := range strategy.Awards() {
		statement.ExpectExec().
			WithArgs(
				strategyID,
				revision,
				uint64(award.ID()),
				award.Name(),
				uint64(award.Weight()),
				string(award.Outcome()),
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	statement.WillBeClosed()
}

func expectStrategySnapshotRead(
	mock sqlmock.Sqlmock,
	identity domain.StrategySnapshotIdentity,
	header *sqlmock.Rows,
	awards *sqlmock.Rows,
) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategySnapshotSQL)).
		WithArgs(uint64(identity.ID()), string(identity.Revision())).
		WillReturnRows(header)
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategySnapshotAwardsSQL)).
		WithArgs(uint64(identity.ID()), string(identity.Revision())).
		WillReturnRows(awards)
}

func validStoredStrategySnapshotHeader() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"schema_version", "name"}).
		AddRow(uint16(1), "Versioned wheel")
}

func validStoredStrategySnapshotAwardRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).
		AddRow(uint64(10), "Reward", uint64(4), "reward").
		AddRow(uint64(20), "Try again", uint64(6), "no_reward")
}

func mustStrategySnapshot(
	t *testing.T,
	id domain.StrategyID,
	revision string,
) domain.StrategySnapshot {
	t.Helper()

	reward, err := domain.NewAward(10, "Reward", 4, domain.AwardOutcomeReward)
	if err != nil {
		t.Fatalf("NewAward(reward) error = %v", err)
	}
	miss, err := domain.NewAward(20, "Try again", 6, domain.AwardOutcomeNoReward)
	if err != nil {
		t.Fatalf("NewAward(miss) error = %v", err)
	}
	strategy, err := domain.NewStrategy(id, "Versioned wheel", []domain.Award{miss, reward})
	if err != nil {
		t.Fatalf("NewStrategy() error = %v", err)
	}
	snapshot, err := domain.NewStrategySnapshot(revision, strategy)
	if err != nil {
		t.Fatalf("NewStrategySnapshot() error = %v", err)
	}
	return snapshot
}

func mustStrategySnapshotIdentity(
	t *testing.T,
	id domain.StrategyID,
	revision string,
) domain.StrategySnapshotIdentity {
	t.Helper()

	identity, err := domain.NewStrategySnapshotIdentity(id, revision)
	if err != nil {
		t.Fatalf("NewStrategySnapshotIdentity() error = %v", err)
	}
	return identity
}

func assertStrategySnapshotsEqual(
	t *testing.T,
	got domain.StrategySnapshot,
	want domain.StrategySnapshot,
) {
	t.Helper()

	if got.Identity() != want.Identity() || got.SchemaVersion() != want.SchemaVersion() {
		t.Fatalf("snapshot envelope = %#v, want %#v", got, want)
	}
	gotStrategy, wantStrategy := got.Strategy(), want.Strategy()
	if gotStrategy.ID() != wantStrategy.ID() ||
		gotStrategy.Name() != wantStrategy.Name() ||
		gotStrategy.TotalWeight() != wantStrategy.TotalWeight() {
		t.Fatalf("snapshot Strategy = %#v, want %#v", gotStrategy, wantStrategy)
	}
	gotAwards, wantAwards := gotStrategy.Awards(), wantStrategy.Awards()
	if len(gotAwards) != len(wantAwards) {
		t.Fatalf("snapshot awards = %d, want %d", len(gotAwards), len(wantAwards))
	}
	for index := range gotAwards {
		if gotAwards[index] != wantAwards[index] {
			t.Fatalf("snapshot award[%d] = %#v, want %#v", index, gotAwards[index], wantAwards[index])
		}
	}
}

func assertZeroStoredStrategySnapshot(t *testing.T, snapshot domain.StrategySnapshot) {
	t.Helper()

	if snapshot.Identity() != (domain.StrategySnapshotIdentity{}) ||
		snapshot.SchemaVersion() != 0 ||
		snapshot.Strategy().ID() != 0 ||
		len(snapshot.Strategy().Awards()) != 0 {
		t.Fatalf("operation returned partial Strategy snapshot %#v", snapshot)
	}
}

func assertStrategySnapshotExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("Strategy snapshot SQL expectations: %v", err)
	}
}
