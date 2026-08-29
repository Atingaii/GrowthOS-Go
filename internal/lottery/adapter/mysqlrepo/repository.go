package mysqlrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	insertStrategySQL = `
		INSERT INTO lottery_strategy (strategy_id, name)
		VALUES (?, ?)`
	insertAwardSQL = `
		INSERT INTO lottery_strategy_award
			(strategy_id, award_id, name, weight, outcome)
		VALUES (?, ?, ?, ?, ?)`
	selectStrategySQL = `
		SELECT strategy_id, name
		FROM lottery_strategy
		WHERE strategy_id = ?`
)

var selectAwardsSQL = fmt.Sprintf(`
	SELECT award_id, name, weight, outcome
	FROM lottery_strategy_award
	WHERE strategy_id = ?
	ORDER BY award_id
	LIMIT %d`, domain.MaxAwardsPerStrategy+1)

var errNilDatabase = errors.New("mysql repository database is nil")
var errNilContext = errors.New("mysql repository context is nil")
var errUnexpectedRowsAffected = errors.New("mysql repository insert affected an unexpected row count")

// Repository is the MySQL Strategy adapter. The caller retains ownership of
// the shared pool and must close it at the application composition root.
type Repository struct {
	database *sqlx.DB
}

var _ application.StrategyCreator = (*Repository)(nil)
var _ application.StrategyReader = (*Repository)(nil)

// New constructs a repository without taking ownership of database.
func New(database *sqlx.DB) (*Repository, error) {
	if database == nil {
		return nil, application.WrapRepositoryError(application.ErrRepositoryNotConfigured, errNilDatabase)
	}
	return &Repository{database: database}, nil
}

// Create stores the root and every Award atomically. It is intentionally
// create-only: duplicate identity is not treated as idempotent success.
func (r *Repository) Create(ctx context.Context, strategy domain.Strategy) error {
	if ctx == nil {
		return application.WrapRepositoryError(application.ErrRepositoryInvalidArgument, errNilContext)
	}
	if r == nil || r.database == nil {
		return application.WrapRepositoryError(application.ErrRepositoryNotConfigured, errNilDatabase)
	}
	validatedStrategy, err := domain.RestoreStrategy(strategy.ID(), strategy.Name(), strategy.Awards())
	if err != nil {
		return err
	}
	strategy = validatedStrategy

	tx, err := r.database.BeginTxx(ctx, nil)
	if err != nil {
		return classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, insertStrategySQL, uint64(strategy.ID()), strategy.Name())
	if err != nil {
		return classifyRootInsertError(err)
	}
	if err := requireOneAffectedRow(result); err != nil {
		return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
	}

	awardStatement, err := tx.PrepareContext(ctx, insertAwardSQL)
	if err != nil {
		return classifyOperationError(err)
	}
	for _, award := range strategy.Awards() {
		result, err = awardStatement.ExecContext(
			ctx,
			uint64(strategy.ID()),
			uint64(award.ID()),
			award.Name(),
			uint64(award.Weight()),
			string(award.Outcome()),
		)
		if err != nil {
			_ = awardStatement.Close()
			return classifyOperationError(err)
		}
		if err := requireOneAffectedRow(result); err != nil {
			_ = awardStatement.Close()
			return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
		}
	}
	if err := awardStatement.Close(); err != nil {
		return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// FindByID reads one complete aggregate from a single repeatable-read snapshot.
// Domain reconstruction happens after the read-only transaction has ended.
func (r *Repository) FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
	if ctx == nil {
		return domain.Strategy{}, application.WrapRepositoryError(application.ErrRepositoryInvalidArgument, errNilContext)
	}
	if r == nil || r.database == nil {
		return domain.Strategy{}, application.WrapRepositoryError(application.ErrRepositoryNotConfigured, errNilDatabase)
	}
	if id == 0 {
		return domain.Strategy{}, domain.ErrStrategyIDRequired
	}

	tx, err := r.database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		return domain.Strategy{}, classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	strategyRow, err := loadStoredStrategy(ctx, tx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Strategy{}, application.WrapRepositoryError(application.ErrStrategyNotFound, err)
		}
		return domain.Strategy{}, classifyOperationError(err)
	}

	storedAwards, err := loadStoredAwards(ctx, tx, id)
	if err != nil {
		return domain.Strategy{}, classifyOperationError(err)
	}

	if err := ctx.Err(); err != nil {
		return domain.Strategy{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Strategy{}, classifyReadCommitError(ctx, err)
	}
	return restoreStrategy(strategyRow, storedAwards)
}

func readSnapshotOptions() *sql.TxOptions {
	return &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	}
}

func loadStoredStrategy(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	id domain.StrategyID,
) (storedStrategy, error) {
	var row storedStrategy
	err := sqlx.GetContext(ctx, queryer, &row, selectStrategySQL, uint64(id))
	return row, err
}

func loadStoredAwards(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	id domain.StrategyID,
) ([]storedAward, error) {
	rows, err := queryer.QueryxContext(ctx, selectAwardsSQL, uint64(id))
	if err != nil {
		return nil, err
	}
	storedAwards := make([]storedAward, 0)
	for rows.Next() {
		var awardRow storedAward
		if err := rows.StructScan(&awardRow); err != nil {
			_ = rows.Close()
			return nil, err
		}
		storedAwards = append(storedAwards, awardRow)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return storedAwards, nil
}

type storedStrategy struct {
	ID   uint64 `db:"strategy_id"`
	Name string `db:"name"`
}

type storedAward struct {
	ID      uint64 `db:"award_id"`
	Name    string `db:"name"`
	Weight  uint64 `db:"weight"`
	Outcome string `db:"outcome"`
}

func restoreStrategy(strategyRow storedStrategy, awardRows []storedAward) (domain.Strategy, error) {
	awards := make([]domain.Award, 0, len(awardRows))
	for _, row := range awardRows {
		award, err := domain.RestoreAward(
			domain.AwardID(row.ID),
			row.Name,
			domain.Weight(row.Weight),
			domain.AwardOutcome(row.Outcome),
		)
		if err != nil {
			return domain.Strategy{}, application.WrapRepositoryError(application.ErrStoredStrategyInvalid, err)
		}
		awards = append(awards, award)
	}

	strategy, err := domain.RestoreStrategy(domain.StrategyID(strategyRow.ID), strategyRow.Name, awards)
	if err != nil {
		return domain.Strategy{}, application.WrapRepositoryError(application.ErrStoredStrategyInvalid, err)
	}
	return strategy, nil
}

func requireOneAffectedRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errUnexpectedRowsAffected
	}
	return nil
}

func classifyOperationError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}

	var mysqlError *drivermysql.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1205, 1213:
			return application.WrapRepositoryError(application.ErrRepositoryRetryable, err)
		}
	}
	if errors.Is(err, driver.ErrBadConn) {
		return application.WrapRepositoryError(application.ErrRepositoryRetryable, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return application.WrapRepositoryError(application.ErrRepositoryRetryable, err)
	}
	return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
}

func classifyRootInsertError(err error) error {
	var mysqlError *drivermysql.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return application.WrapRepositoryError(application.ErrStrategyAlreadyExists, err)
	}
	return classifyOperationError(err)
}

func classifyWriteCommitError(ctx context.Context, err error) error {
	if contextError := canceledTransactionError(ctx, err); contextError != nil {
		return contextError
	}
	return application.WrapRepositoryError(application.ErrCommitOutcomeUnknown, err)
}

func classifyReadCommitError(ctx context.Context, err error) error {
	if contextError := canceledTransactionError(ctx, err); contextError != nil {
		return contextError
	}
	return classifyOperationError(err)
}

func canceledTransactionError(ctx context.Context, transactionError error) error {
	contextError := ctx.Err()
	if contextError == nil {
		return nil
	}
	if errors.Is(transactionError, contextError) || errors.Is(transactionError, sql.ErrTxDone) {
		return contextError
	}
	return nil
}
