package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	insertStrategySnapshotSQL = `
		INSERT INTO lottery_strategy_snapshot
			(strategy_id, revision, schema_version, name)
		VALUES (?, ?, ?, ?)`
	insertStrategySnapshotAwardSQL = `
		INSERT INTO lottery_strategy_snapshot_award
			(strategy_id, revision, award_id, name, weight, outcome)
		VALUES (?, ?, ?, ?, ?, ?)`
	selectStrategySnapshotSQL = `
		SELECT schema_version, name
		FROM lottery_strategy_snapshot
		WHERE strategy_id = ? AND revision = ?`
)

var selectStrategySnapshotAwardsSQL = fmt.Sprintf(`
	SELECT award_id, name, weight, outcome
	FROM lottery_strategy_snapshot_award
	WHERE strategy_id = ? AND revision = ?
	ORDER BY award_id
	LIMIT %d`, domain.MaxAwardsPerStrategy+1)

var errStoredStrategySnapshotAwardLimit = errors.New("stored strategy snapshot award limit exceeded")

// StrategySnapshotRepository persists complete, immutable Strategy
// configuration revisions. It intentionally exposes no update, upsert,
// delete, list, latest-revision, publication, or cache operation.
type StrategySnapshotRepository struct {
	database *sqlx.DB
}

var _ application.StrategySnapshotCreator = (*StrategySnapshotRepository)(nil)
var _ application.StrategySnapshotReader = (*StrategySnapshotRepository)(nil)

// NewStrategySnapshotRepository constructs the adapter without taking
// ownership of the shared database pool.
func NewStrategySnapshotRepository(
	database *sqlx.DB,
) (*StrategySnapshotRepository, error) {
	if database == nil {
		return nil, application.WrapRepositoryError(
			application.ErrRepositoryNotConfigured,
			errNilDatabase,
		)
	}
	return &StrategySnapshotRepository{database: database}, nil
}

// CreateSnapshot stores the header and every Award in one transaction. A
// duplicate exact identity is a conflict, never idempotent success.
func (repository *StrategySnapshotRepository) CreateSnapshot(
	ctx context.Context,
	snapshot domain.StrategySnapshot,
) error {
	if ctx == nil {
		return application.WrapRepositoryError(
			application.ErrRepositoryInvalidArgument,
			errNilContext,
		)
	}
	if repository == nil || repository.database == nil {
		return application.WrapRepositoryError(
			application.ErrRepositoryNotConfigured,
			errNilDatabase,
		)
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}

	identity := snapshot.Identity()
	strategy := snapshot.Strategy()
	strategyID := uint64(identity.ID())
	revision := string(identity.Revision())
	tx, err := repository.database.BeginTxx(ctx, nil)
	if err != nil {
		return classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		ctx,
		insertStrategySnapshotSQL,
		strategyID,
		revision,
		uint16(snapshot.SchemaVersion()),
		strategy.Name(),
	)
	if err != nil {
		return classifyStrategySnapshotRootInsertError(err)
	}
	if err := requireOneAffectedRow(result); err != nil {
		return application.WrapRepositoryError(application.ErrRepositoryFailure, err)
	}

	awardStatement, err := tx.PrepareContext(ctx, insertStrategySnapshotAwardSQL)
	if err != nil {
		return classifyOperationError(err)
	}
	for _, award := range strategy.Awards() {
		result, err = awardStatement.ExecContext(
			ctx,
			strategyID,
			revision,
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

// FindSnapshotByIdentity reads one complete immutable revision from a
// repeatable-read, read-only snapshot and restores it only after that database
// snapshot has ended.
func (repository *StrategySnapshotRepository) FindSnapshotByIdentity(
	ctx context.Context,
	identity domain.StrategySnapshotIdentity,
) (domain.StrategySnapshot, error) {
	if ctx == nil {
		return domain.StrategySnapshot{}, application.WrapRepositoryError(
			application.ErrRepositoryInvalidArgument,
			errNilContext,
		)
	}
	if repository == nil || repository.database == nil {
		return domain.StrategySnapshot{}, application.WrapRepositoryError(
			application.ErrRepositoryNotConfigured,
			errNilDatabase,
		)
	}
	if err := identity.Validate(); err != nil {
		return domain.StrategySnapshot{}, err
	}

	tx, err := repository.database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		return domain.StrategySnapshot{}, classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	header, err := loadStoredStrategySnapshot(ctx, tx, identity)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.StrategySnapshot{}, application.WrapRepositoryError(
				application.ErrStrategySnapshotNotFound,
				err,
			)
		}
		return domain.StrategySnapshot{}, classifyOperationError(err)
	}
	if err := ctx.Err(); err != nil {
		return domain.StrategySnapshot{}, err
	}
	if domain.StrategySnapshotSchemaVersion(header.SchemaVersion) !=
		domain.StrategySnapshotSchemaVersionV1 {
		return domain.StrategySnapshot{}, storedStrategySnapshotInvalid(
			domain.ErrStrategySnapshotSchemaUnsupported,
		)
	}

	awardRows, err := loadStoredStrategySnapshotAwards(ctx, tx, identity)
	if err != nil {
		if errors.Is(err, errStoredStrategySnapshotAwardLimit) {
			return domain.StrategySnapshot{}, storedStrategySnapshotInvalid(err)
		}
		return domain.StrategySnapshot{}, classifyOperationError(err)
	}
	if err := ctx.Err(); err != nil {
		return domain.StrategySnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.StrategySnapshot{}, classifyReadCommitError(ctx, err)
	}
	return restoreStoredStrategySnapshot(identity, header, awardRows)
}

type storedStrategySnapshot struct {
	SchemaVersion uint16 `db:"schema_version"`
	Name          string `db:"name"`
}

type storedStrategySnapshotAward struct {
	ID      uint64 `db:"award_id"`
	Name    string `db:"name"`
	Weight  uint64 `db:"weight"`
	Outcome string `db:"outcome"`
}

func loadStoredStrategySnapshot(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	identity domain.StrategySnapshotIdentity,
) (storedStrategySnapshot, error) {
	var header storedStrategySnapshot
	err := sqlx.GetContext(
		ctx,
		queryer,
		&header,
		selectStrategySnapshotSQL,
		uint64(identity.ID()),
		string(identity.Revision()),
	)
	return header, err
}

func loadStoredStrategySnapshotAwards(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	identity domain.StrategySnapshotIdentity,
) ([]storedStrategySnapshotAward, error) {
	rows, err := queryer.QueryxContext(
		ctx,
		selectStrategySnapshotAwardsSQL,
		uint64(identity.ID()),
		string(identity.Revision()),
	)
	if err != nil {
		return nil, err
	}
	awardRows := make([]storedStrategySnapshotAward, 0)
	for rows.Next() {
		var row storedStrategySnapshotAward
		if err := rows.StructScan(&row); err != nil {
			_ = rows.Close()
			return nil, err
		}
		awardRows = append(awardRows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(awardRows) > domain.MaxAwardsPerStrategy {
		return nil, errStoredStrategySnapshotAwardLimit
	}
	return awardRows, nil
}

func restoreStoredStrategySnapshot(
	identity domain.StrategySnapshotIdentity,
	header storedStrategySnapshot,
	awardRows []storedStrategySnapshotAward,
) (domain.StrategySnapshot, error) {
	awards := make([]domain.Award, 0, len(awardRows))
	for _, row := range awardRows {
		award, err := domain.RestoreAward(
			domain.AwardID(row.ID),
			row.Name,
			domain.Weight(row.Weight),
			domain.AwardOutcome(row.Outcome),
		)
		if err != nil {
			return domain.StrategySnapshot{}, storedStrategySnapshotInvalid(err)
		}
		awards = append(awards, award)
	}

	snapshot, err := domain.RestoreStrategySnapshot(
		identity,
		domain.StrategySnapshotSchemaVersion(header.SchemaVersion),
		header.Name,
		awards,
	)
	if err != nil {
		return domain.StrategySnapshot{}, storedStrategySnapshotInvalid(err)
	}
	return snapshot, nil
}

func classifyStrategySnapshotRootInsertError(err error) error {
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return application.WrapRepositoryError(
			application.ErrStrategySnapshotAlreadyExists,
			err,
		)
	}
	return classifyOperationError(err)
}

func storedStrategySnapshotInvalid(cause error) error {
	return application.WrapRepositoryError(
		application.ErrStoredStrategySnapshotInvalid,
		cause,
	)
}
