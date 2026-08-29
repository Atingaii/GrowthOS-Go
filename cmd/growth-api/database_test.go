package main

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

func TestComposeRuntimeSharesOnePoolAcrossReadinessRepositoryAndOwnership(t *testing.T) {
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	database := sqlx.NewDb(sqlDatabase, "mysql")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT strategy_id, name FROM lottery_strategy WHERE strategy_id = ?",
	)).WithArgs(uint64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"strategy_id", "name"}).AddRow(uint64(42), "Composed strategy"),
	)
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT award_id, name, weight, outcome FROM lottery_strategy_award WHERE strategy_id = ? ORDER BY award_id",
	)).WithArgs(uint64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).
			AddRow(uint64(7), "Only award", uint64(1), "reward"),
	)
	mock.ExpectCommit()
	mock.ExpectClose()

	components, err := composeRuntime(database)
	if err != nil {
		t.Fatalf("compose runtime: %v", err)
	}
	if components.database != database {
		t.Fatal("runtime readiness/ownership handle is not the pool supplied to Repository composition")
	}
	selection, err := components.selection.Select(context.Background(), domain.StrategyID(42))
	if err != nil {
		t.Fatalf("select through composed Repository: %v", err)
	}
	if selection.Strategy.ID() != 42 || selection.Award.ID() != 7 {
		t.Fatalf("selection = Strategy %d Award %d, want 42/7", selection.Strategy.ID(), selection.Award.ID())
	}
	if err := components.database.Close(); err != nil {
		t.Fatalf("close composed pool: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("runtime did not use and close exactly the supplied pool: %v", err)
	}
}

func TestComposeRuntimeRejectsNilPoolWithoutCreatingPartialRuntime(t *testing.T) {
	components, err := composeRuntime(nil)
	if !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("composeRuntime(nil) error = %v, want repository not configured", err)
	}
	if components.database != nil || components.selection != nil {
		t.Fatalf("partial runtime = %#v, want zero", components)
	}
}
