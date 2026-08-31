package mysqlrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/marketing/application"
	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	insertActivitySQL = `
		INSERT INTO marketing_activity
			(activity_id, name, lifecycle_state, state_version, active_version,
			 retired_at, retirement_reference)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	selectActivitySQL = `
		SELECT activity_id, name, lifecycle_state, state_version, active_version,
		       retired_at, retirement_reference
		FROM marketing_activity
		WHERE activity_id = ?`
	insertPublicationSQL = `
		INSERT INTO marketing_activity_publication
			(activity_id, activity_version, schema_version, publication_kind,
			 rollback_of_version, graph_id, graph_revision, starts_at, ends_at,
			 published_at, approval_reference)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	insertPublicationStrategySQL = `
		INSERT INTO marketing_activity_publication_strategy
			(activity_id, activity_version, strategy_id, strategy_revision)
		VALUES (?, ?, ?, ?)`
	selectPublicationSQL = `
		SELECT schema_version, publication_kind, rollback_of_version, graph_id,
		       graph_revision, starts_at, ends_at, published_at, approval_reference
		FROM marketing_activity_publication
		WHERE activity_id = ? AND activity_version = ?`
	updateActivityWithoutActiveSQL = `
		UPDATE marketing_activity
		SET lifecycle_state = ?, state_version = ?, active_version = ?,
		    retired_at = ?, retirement_reference = ?
		WHERE activity_id = ?
		  AND lifecycle_state = ?
		  AND state_version = ?
		  AND active_version IS NULL`
	updateActivityWithActiveSQL = `
		UPDATE marketing_activity
		SET lifecycle_state = ?, state_version = ?, active_version = ?,
		    retired_at = ?, retirement_reference = ?
		WHERE activity_id = ?
		  AND lifecycle_state = ?
		  AND state_version = ?
		  AND active_version = ?`
)

var selectPublicationStrategiesSQL = fmt.Sprintf(`
	SELECT strategy_id, strategy_revision
	FROM marketing_activity_publication_strategy
	WHERE activity_id = ? AND activity_version = ?
	ORDER BY strategy_id
	LIMIT %d`, domain.MaxStrategyRevisionManifestEntries+1)

var (
	errNilDatabase                 = errors.New("marketing mysql repository database is nil")
	errNilContext                  = errors.New("marketing mysql repository context is nil")
	errUnexpectedRowsAffected      = errors.New("marketing mysql repository affected an unexpected row count")
	errStoredPublicationLimit      = errors.New("stored activity publication Strategy manifest limit exceeded")
	errDraftActivityNeeded         = errors.New("activity root must be a draft")
	errPublicationTransitionNeeded = errors.New("activity transition must append a publication")
	errRetirementTransitionNeeded  = errors.New("activity transition must retire without a publication")
)

// Repository is the MySQL adapter for the complete Activity persistence port.
// The composition root retains ownership of the shared connection pool.
type Repository struct {
	database *sqlx.DB
}

var _ application.ActivityDraftCreator = (*Repository)(nil)
var _ application.ActivityReader = (*Repository)(nil)
var _ application.ActivityCurrentReader = (*Repository)(nil)
var _ application.ActivityPublicationReader = (*Repository)(nil)
var _ application.ActivityPublicationWriter = (*Repository)(nil)
var _ application.ActivityRetirer = (*Repository)(nil)

// New constructs an Activity repository without probing or taking ownership
// of the database pool.
func New(database *sqlx.DB) (*Repository, error) {
	if database == nil {
		return nil, repositoryError(application.ErrRepositoryNotConfigured, errNilDatabase)
	}
	return &Repository{database: database}, nil
}

// CreateDraft creates exactly one draft root. It is neither an upsert nor an
// idempotency endpoint.
func (repository *Repository) CreateDraft(ctx context.Context, activity domain.Activity) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if err := activity.Validate(); err != nil {
		return repositoryError(application.ErrRepositoryInvalidArgument, err)
	}
	if activity.Lifecycle() != domain.ActivityLifecycleDraft {
		return repositoryError(application.ErrRepositoryInvalidArgument, errDraftActivityNeeded)
	}

	tx, err := repository.database.BeginTxx(ctx, nil)
	if err != nil {
		return classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		ctx,
		insertActivitySQL,
		uint64(activity.ID()),
		activity.Name().String(),
		string(activity.Lifecycle()),
		uint64(activity.StateVersion()),
		nil,
		nil,
		nil,
	)
	if err != nil {
		return classifyDraftInsertError(err)
	}
	if err := requireOneAffectedRow(result); err != nil {
		return repositoryError(application.ErrRepositoryFailure, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// FindActivityByID restores one root from a read-only repeatable-read
// transaction. It never loads or guesses a publication.
func (repository *Repository) FindActivityByID(
	ctx context.Context,
	id domain.ActivityID,
) (domain.Activity, error) {
	if err := repository.validateIdentityCall(ctx, id); err != nil {
		return domain.Activity{}, err
	}
	tx, err := repository.database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		return domain.Activity{}, classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	row, err := loadStoredActivity(ctx, tx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Activity{}, repositoryError(application.ErrActivityNotFound, err)
		}
		return domain.Activity{}, classifyOperationError(err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Activity{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Activity{}, classifyReadCommitError(ctx, err)
	}
	return restoreStoredActivity(row)
}

// FindCurrentActivity restores the root and, when present, its exact active
// publication from one database snapshot. A draft returns a zero publication.
func (repository *Repository) FindCurrentActivity(
	ctx context.Context,
	id domain.ActivityID,
) (domain.Activity, domain.ActivityPublication, error) {
	if err := repository.validateIdentityCall(ctx, id); err != nil {
		return domain.Activity{}, domain.ActivityPublication{}, err
	}
	tx, err := repository.database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		return domain.Activity{}, domain.ActivityPublication{}, classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	rootRow, err := loadStoredActivity(ctx, tx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Activity{}, domain.ActivityPublication{}, repositoryError(application.ErrActivityNotFound, err)
		}
		return domain.Activity{}, domain.ActivityPublication{}, classifyOperationError(err)
	}
	activity, err := restoreStoredActivity(rootRow)
	if err != nil {
		return domain.Activity{}, domain.ActivityPublication{}, err
	}
	if activity.Lifecycle() == domain.ActivityLifecycleDraft {
		if err := ctx.Err(); err != nil {
			return domain.Activity{}, domain.ActivityPublication{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.Activity{}, domain.ActivityPublication{}, classifyReadCommitError(ctx, err)
		}
		return activity, domain.ActivityPublication{}, nil
	}

	publication, err := loadAndRestorePublication(
		ctx,
		tx,
		activity.ID(),
		activity.ActivePublicationVersion(),
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Activity{}, domain.ActivityPublication{}, storedPublicationInvalid(err)
		}
		return domain.Activity{}, domain.ActivityPublication{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.Activity{}, domain.ActivityPublication{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Activity{}, domain.ActivityPublication{}, classifyReadCommitError(ctx, err)
	}
	return activity, publication, nil
}

// FindPublicationByIdentity restores exactly the requested immutable history
// record. It has no current/latest fallback.
func (repository *Repository) FindPublicationByIdentity(
	ctx context.Context,
	activityID domain.ActivityID,
	version domain.ActivityPublicationVersion,
) (domain.ActivityPublication, error) {
	if err := repository.validateIdentityCall(ctx, activityID); err != nil {
		return domain.ActivityPublication{}, err
	}
	if version == 0 {
		return domain.ActivityPublication{}, repositoryError(
			application.ErrRepositoryInvalidArgument,
			errors.New("activity publication version is required"),
		)
	}
	tx, err := repository.database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		return domain.ActivityPublication{}, classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	publication, err := loadAndRestorePublication(ctx, tx, activityID, version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ActivityPublication{}, repositoryError(
				application.ErrActivityPublicationNotFound,
				err,
			)
		}
		return domain.ActivityPublication{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ActivityPublication{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ActivityPublication{}, classifyReadCommitError(ctx, err)
	}
	return publication, nil
}

// CompareAndSwapPublication inserts one immutable publication and all exact
// Strategy bindings, then changes the root through an exact optimistic CAS.
func (repository *Repository) CompareAndSwapPublication(
	ctx context.Context,
	transition domain.ActivityTransition,
) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if err := transition.Validate(); err != nil {
		return repositoryError(application.ErrRepositoryInvalidArgument, err)
	}
	record, appends := transition.Record()
	if !appends || !transition.AppendsPublication() {
		return repositoryError(application.ErrRepositoryInvalidArgument, errPublicationTransitionNeeded)
	}

	tx, err := repository.database.BeginTxx(ctx, nil)
	if err != nil {
		return classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertPublication(ctx, tx, record); err != nil {
		return err
	}
	for _, reference := range record.StrategyRevisionManifest() {
		result, execErr := tx.ExecContext(
			ctx,
			insertPublicationStrategySQL,
			uint64(record.ActivityID()),
			uint64(record.Version()),
			uint64(reference.StrategyID()),
			string(reference.Revision()),
		)
		if execErr != nil {
			return classifyOperationError(execErr)
		}
		if err := requireOneAffectedRow(result); err != nil {
			return repositoryError(application.ErrRepositoryFailure, err)
		}
	}
	if err := compareAndSwapActivity(ctx, tx, transition); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// CompareAndSwapRetirement changes only the root. It never appends, updates,
// or deletes historical publication rows.
func (repository *Repository) CompareAndSwapRetirement(
	ctx context.Context,
	transition domain.ActivityTransition,
) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if err := transition.Validate(); err != nil {
		return repositoryError(application.ErrRepositoryInvalidArgument, err)
	}
	if _, appends := transition.Record(); appends || transition.AppendsPublication() {
		return repositoryError(application.ErrRepositoryInvalidArgument, errRetirementTransitionNeeded)
	}

	tx, err := repository.database.BeginTxx(ctx, nil)
	if err != nil {
		return classifyOperationError(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := compareAndSwapActivity(ctx, tx, transition); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

type storedActivity struct {
	ID                  uint64           `db:"activity_id"`
	Name                string           `db:"name"`
	Lifecycle           string           `db:"lifecycle_state"`
	StateVersion        uint64           `db:"state_version"`
	ActiveVersion       sql.Null[uint64] `db:"active_version"`
	RetiredAt           sql.NullTime     `db:"retired_at"`
	RetirementReference sql.NullString   `db:"retirement_reference"`
}

type storedPublication struct {
	SchemaVersion     uint16           `db:"schema_version"`
	Kind              string           `db:"publication_kind"`
	RollbackOfVersion sql.Null[uint64] `db:"rollback_of_version"`
	GraphID           uint64           `db:"graph_id"`
	GraphRevision     string           `db:"graph_revision"`
	StartsAt          time.Time        `db:"starts_at"`
	EndsAt            time.Time        `db:"ends_at"`
	PublishedAt       time.Time        `db:"published_at"`
	ApprovalReference string           `db:"approval_reference"`
}

type storedPublicationStrategy struct {
	StrategyID       uint64 `db:"strategy_id"`
	StrategyRevision string `db:"strategy_revision"`
}

func (repository *Repository) validateCall(ctx context.Context) error {
	if ctx == nil {
		return repositoryError(application.ErrRepositoryInvalidArgument, errNilContext)
	}
	if repository == nil || repository.database == nil {
		return repositoryError(application.ErrRepositoryNotConfigured, errNilDatabase)
	}
	return nil
}

func (repository *Repository) validateIdentityCall(ctx context.Context, id domain.ActivityID) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if id == 0 {
		return repositoryError(
			application.ErrRepositoryInvalidArgument,
			errors.New("activity id is required"),
		)
	}
	return nil
}

func readSnapshotOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
}

func loadStoredActivity(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	id domain.ActivityID,
) (storedActivity, error) {
	var row storedActivity
	err := sqlx.GetContext(ctx, queryer, &row, selectActivitySQL, uint64(id))
	return row, err
}

func restoreStoredActivity(row storedActivity) (domain.Activity, error) {
	var activeVersion domain.ActivityPublicationVersion
	if row.ActiveVersion.Valid {
		activeVersion = domain.ActivityPublicationVersion(row.ActiveVersion.V)
	}
	var retiredAt time.Time
	if row.RetiredAt.Valid {
		retiredAt = row.RetiredAt.Time
	}
	var retirementReference domain.EvidenceReference
	if row.RetirementReference.Valid {
		retirementReference = domain.EvidenceReference(row.RetirementReference.String)
	}
	activity, err := domain.RestoreActivity(
		domain.ActivityID(row.ID),
		domain.ActivityName(row.Name),
		domain.ActivityLifecycle(row.Lifecycle),
		domain.ActivityStateVersion(row.StateVersion),
		activeVersion,
		retiredAt,
		retirementReference,
	)
	if err != nil {
		return domain.Activity{}, repositoryError(application.ErrStoredActivityInvalid, err)
	}
	return activity, nil
}

func loadAndRestorePublication(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	activityID domain.ActivityID,
	version domain.ActivityPublicationVersion,
) (domain.ActivityPublication, error) {
	var row storedPublication
	if err := sqlx.GetContext(
		ctx,
		queryer,
		&row,
		selectPublicationSQL,
		uint64(activityID),
		uint64(version),
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ActivityPublication{}, err
		}
		return domain.ActivityPublication{}, classifyOperationError(err)
	}
	if domain.ActivityPublicationSchemaVersion(row.SchemaVersion) !=
		domain.ActivityPublicationSchemaVersionV1 {
		return domain.ActivityPublication{}, storedPublicationInvalid(
			domain.ErrActivityPublicationSchemaUnsupported,
		)
	}

	strategyRows, err := loadStoredPublicationStrategies(ctx, queryer, activityID, version)
	if err != nil {
		if errors.Is(err, errStoredPublicationLimit) {
			return domain.ActivityPublication{}, storedPublicationInvalid(err)
		}
		return domain.ActivityPublication{}, classifyOperationError(err)
	}
	return restoreStoredPublication(activityID, version, row, strategyRows)
}

func loadStoredPublicationStrategies(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	activityID domain.ActivityID,
	version domain.ActivityPublicationVersion,
) ([]storedPublicationStrategy, error) {
	rows, err := queryer.QueryxContext(
		ctx,
		selectPublicationStrategiesSQL,
		uint64(activityID),
		uint64(version),
	)
	if err != nil {
		return nil, err
	}
	strategyRows := make([]storedPublicationStrategy, 0)
	for rows.Next() {
		var row storedPublicationStrategy
		if err := rows.StructScan(&row); err != nil {
			_ = rows.Close()
			return nil, err
		}
		strategyRows = append(strategyRows, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(strategyRows) > domain.MaxStrategyRevisionManifestEntries {
		return nil, errStoredPublicationLimit
	}
	return strategyRows, nil
}

func restoreStoredPublication(
	activityID domain.ActivityID,
	version domain.ActivityPublicationVersion,
	row storedPublication,
	strategyRows []storedPublicationStrategy,
) (domain.ActivityPublication, error) {
	graphReference, err := domain.NewLotteryGraphReference(
		domain.LotteryGraphID(row.GraphID),
		row.GraphRevision,
	)
	if err != nil {
		return domain.ActivityPublication{}, storedPublicationInvalid(err)
	}
	manifest := make([]domain.LotteryStrategyRevisionReference, 0, len(strategyRows))
	for _, strategyRow := range strategyRows {
		reference, err := domain.NewLotteryStrategyRevisionReference(
			domain.LotteryStrategyID(strategyRow.StrategyID),
			strategyRow.StrategyRevision,
		)
		if err != nil {
			return domain.ActivityPublication{}, storedPublicationInvalid(err)
		}
		manifest = append(manifest, reference)
	}
	var rollbackOf domain.ActivityPublicationVersion
	if row.RollbackOfVersion.Valid {
		rollbackOf = domain.ActivityPublicationVersion(row.RollbackOfVersion.V)
	}
	publication, err := domain.RestoreActivityPublication(
		activityID,
		version,
		domain.ActivityPublicationSchemaVersion(row.SchemaVersion),
		domain.ActivityPublicationKind(row.Kind),
		rollbackOf,
		row.StartsAt,
		row.EndsAt,
		row.PublishedAt,
		graphReference,
		manifest,
		domain.EvidenceReference(row.ApprovalReference),
	)
	if err != nil {
		return domain.ActivityPublication{}, storedPublicationInvalid(err)
	}
	return publication, nil
}

func insertPublication(ctx context.Context, tx *sqlx.Tx, publication domain.ActivityPublication) error {
	rollbackOf, rollback := publication.RollbackOf()
	var rollbackArgument any
	if rollback {
		rollbackArgument = uint64(rollbackOf)
	}
	graphReference := publication.GraphReference()
	result, err := tx.ExecContext(
		ctx,
		insertPublicationSQL,
		uint64(publication.ActivityID()),
		uint64(publication.Version()),
		uint16(publication.SchemaVersion()),
		string(publication.Kind()),
		rollbackArgument,
		uint64(graphReference.ID()),
		string(graphReference.Revision()),
		publication.StartsAt(),
		publication.EndsAt(),
		publication.PublishedAt(),
		publication.ApprovalEvidenceReference().String(),
	)
	if err != nil {
		return classifyPublicationInsertError(err)
	}
	if err := requireOneAffectedRow(result); err != nil {
		return repositoryError(application.ErrRepositoryFailure, err)
	}
	return nil
}

func compareAndSwapActivity(
	ctx context.Context,
	tx *sqlx.Tx,
	transition domain.ActivityTransition,
) error {
	next := transition.Next()
	retiredAt, hasRetiredAt := next.RetiredAt()
	var retiredAtArgument any
	if hasRetiredAt {
		retiredAtArgument = retiredAt
	}
	retirementReference, hasRetirementReference := next.RetirementReference()
	var retirementReferenceArgument any
	if hasRetirementReference {
		retirementReferenceArgument = retirementReference.String()
	}
	arguments := []any{
		string(next.Lifecycle()),
		uint64(next.StateVersion()),
		uint64(next.ActivePublicationVersion()),
		retiredAtArgument,
		retirementReferenceArgument,
		uint64(next.ID()),
		string(transition.ExpectedLifecycle()),
		uint64(transition.ExpectedStateVersion()),
	}
	statement := updateActivityWithoutActiveSQL
	if transition.ExpectedActivePublicationVersion() != 0 {
		statement = updateActivityWithActiveSQL
		arguments = append(arguments, uint64(transition.ExpectedActivePublicationVersion()))
	}
	result, err := tx.ExecContext(ctx, statement, arguments...)
	if err != nil {
		return classifyOperationError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return repositoryError(application.ErrRepositoryFailure, err)
	}
	if affected == 0 {
		return repositoryError(application.ErrActivityStateConflict, errUnexpectedRowsAffected)
	}
	if affected != 1 {
		return repositoryError(application.ErrRepositoryFailure, errUnexpectedRowsAffected)
	}
	return nil
}

func requireOneAffectedRow(result sql.Result) error {
	if result == nil {
		return errUnexpectedRowsAffected
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errUnexpectedRowsAffected
	}
	return nil
}

func repositoryError(class, cause error) error {
	return application.WrapRepositoryError(class, cause)
}

func storedPublicationInvalid(cause error) error {
	return repositoryError(application.ErrStoredActivityPublicationInvalid, cause)
}

func classifyDraftInsertError(err error) error {
	if isDuplicateEntry(err) {
		return repositoryError(application.ErrActivityAlreadyExists, err)
	}
	return classifyOperationError(err)
}

func classifyPublicationInsertError(err error) error {
	if isDuplicateEntry(err) {
		return repositoryError(application.ErrActivityStateConflict, err)
	}
	return classifyOperationError(err)
}

func isDuplicateEntry(err error) bool {
	var mysqlError *drivermysql.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
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
			return repositoryError(application.ErrRepositoryRetryable, err)
		}
	}
	if errors.Is(err, driver.ErrBadConn) {
		return repositoryError(application.ErrRepositoryRetryable, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return repositoryError(application.ErrRepositoryRetryable, err)
	}
	return repositoryError(application.ErrRepositoryFailure, err)
}

func classifyWriteCommitError(ctx context.Context, err error) error {
	if contextError := canceledTransactionError(ctx, err); contextError != nil {
		return contextError
	}
	return repositoryError(application.ErrCommitOutcomeUnknown, err)
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
