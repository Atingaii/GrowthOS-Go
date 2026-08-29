package mysqlrepo

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestFindByIDUsesOneOrderedTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	repository, err := New(sqlx.NewDb(database, "sqlmock"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(selectStrategySQL)).
		WithArgs(uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"strategy_id", "name"}).AddRow(uint64(7), "留存抽奖"))
	mock.ExpectQuery(regexp.QuoteMeta(selectAwardsSQL)).
		WithArgs(uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).
			AddRow(uint64(1), "未中奖", uint64(9), "no_reward").
			AddRow(uint64(2), "积分", uint64(1), "reward"))
	mock.ExpectCommit()

	strategy, err := repository.FindByID(context.Background(), domain.StrategyID(7))
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if strategy.ID() != 7 || strategy.Name() != "留存抽奖" || len(strategy.Awards()) != 2 {
		t.Fatalf("FindByID() = %#v, want complete strategy 7", strategy)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction expectations: %v", err)
	}
}

func TestCreateClassifiesDriverCommitFailureAsUnknownOutcome(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	repository, err := New(sqlx.NewDb(database, "sqlmock"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	laterAward, err := domain.NewAward(2, "未中奖", 9, domain.AwardOutcomeNoReward)
	if err != nil {
		t.Fatalf("NewAward() error = %v", err)
	}
	earlierAward, err := domain.NewAward(1, "积分", 1, domain.AwardOutcomeReward)
	if err != nil {
		t.Fatalf("NewAward() error = %v", err)
	}
	strategy, err := domain.NewStrategy(42, "提交语义", []domain.Award{laterAward, earlierAward})
	if err != nil {
		t.Fatalf("NewStrategy() error = %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(insertStrategySQL)).
		WithArgs(uint64(42), "提交语义").
		WillReturnResult(sqlmock.NewResult(0, 1))
	prepared := mock.ExpectPrepare(regexp.QuoteMeta(insertAwardSQL))
	prepared.ExpectExec().
		WithArgs(uint64(42), uint64(1), "积分", uint64(1), "reward").
		WillReturnResult(sqlmock.NewResult(0, 1))
	prepared.ExpectExec().
		WithArgs(uint64(42), uint64(2), "未中奖", uint64(9), "no_reward").
		WillReturnResult(sqlmock.NewResult(0, 1))
	prepared.WillBeClosed()
	commitCause := errors.New("driver lost the commit response")
	mock.ExpectCommit().WillReturnError(commitCause)

	err = repository.Create(context.Background(), strategy)
	if !errors.Is(err, application.ErrCommitOutcomeUnknown) {
		t.Fatalf("Create() error = %v, want unknown commit outcome", err)
	}
	if !errors.Is(err, commitCause) {
		t.Fatalf("Create() error = %v, want retained commit cause", err)
	}
	if got := err.Error(); got != application.ErrCommitOutcomeUnknown.Error() {
		t.Fatalf("Create() rendered error = %q, want safe class %q", got, application.ErrCommitOutcomeUnknown)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("transaction expectations: %v", err)
	}
}
