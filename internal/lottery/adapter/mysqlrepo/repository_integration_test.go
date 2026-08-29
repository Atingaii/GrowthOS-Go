package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	projectmigrations "github.com/Atingaii/GrowthOS-Go/migrations"
)

const rollbackProbeAwardID = uint64(math.MaxUint64 - 1)

func TestRepositoryMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-19-isolated-schema" {
		t.Skip("repository integration requires explicit disposable-schema authorization")
	}
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES") != "lesson-19-isolated-repository" {
		t.Skip("repository integration requires explicit repository-write authorization")
	}

	applicationConnection := repositoryIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_API")
	migrationConnection := repositoryIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	if applicationConnection.Address != migrationConnection.Address ||
		applicationConnection.Database != migrationConnection.Database {
		t.Fatal("application and migration identities must target the same isolated schema")
	}
	if applicationConnection.User == migrationConnection.User {
		t.Fatal("application and migration identities must be distinct")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ensureRepositorySchema(t, ctx, migrationConnection)
	applicationDatabase, err := mysqlstore.Open(ctx, mysqlstore.Config{
		ConnectionConfig:      applicationConnection,
		PingTimeout:           5 * time.Second,
		MaxOpenConnections:    8,
		MaxIdleConnections:    4,
		ConnectionMaxLifetime: time.Minute,
		ConnectionMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open application database: %v", err)
	}
	t.Cleanup(func() { _ = applicationDatabase.Close() })

	verificationDatabase, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: migrationConnection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open verification database: %v", err)
	}
	t.Cleanup(func() { _ = verificationDatabase.Close() })

	repository, err := New(applicationDatabase)
	if err != nil {
		t.Fatalf("construct repository: %v", err)
	}

	baseID := uint64(time.Now().UnixNano())
	rollbackConstraint := "chk_l19_" + strconv.FormatUint(baseID+3, 36)
	testIDs := []uint64{
		baseID,
		baseID + 1,
		baseID + 2,
		baseID + 3,
		baseID + 4,
		baseID + 5,
		baseID + 6,
		baseID + 7,
		baseID + 8,
		baseID + 9,
		math.MaxUint64,
	}
	if err := cleanupRepositoryFixtures(verificationDatabase, testIDs, rollbackConstraint); err != nil {
		t.Fatalf("initial repository fixture cleanup: %v", err)
	}
	defer func() {
		if err := cleanupRepositoryFixtures(verificationDatabase, testIDs, rollbackConstraint); err != nil {
			t.Errorf("final repository fixture cleanup: %v", err)
		}
	}()

	if _, err := repository.FindByID(ctx, 0); !errors.Is(err, domain.ErrStrategyIDRequired) {
		t.Fatalf("FindByID(zero) error = %v, want strategy ID required", err)
	}
	if err := repository.Create(ctx, domain.Strategy{}); !errors.Is(err, domain.ErrStrategyIDRequired) {
		t.Fatalf("Create(zero strategy) error = %v, want strategy ID required", err)
	}
	if _, err := repository.FindByID(ctx, domain.StrategyID(baseID+100)); !errors.Is(err, application.ErrStrategyNotFound) {
		t.Fatalf("FindByID(missing) error = %v, want strategy not found", err)
	} else if err.Error() != application.ErrStrategyNotFound.Error() {
		t.Fatalf("FindByID(missing) rendered error = %q, want safe semantic class", err.Error())
	}

	roundTrip := mustStrategy(
		t,
		domain.StrategyID(baseID),
		"新手's ? -- wheel e\u0301",
		[]domain.Award{
			mustAward(t, 20, "Café 'try again'", 7, domain.AwardOutcomeNoReward),
			mustAward(t, 10, "Cafe e\u0301 ? -- reward", 3, domain.AwardOutcomeReward),
		},
	)
	if err := repository.Create(ctx, roundTrip); err != nil {
		t.Fatalf("Create(round trip): %v", err)
	}
	loaded, err := repository.FindByID(ctx, roundTrip.ID())
	if err != nil {
		t.Fatalf("FindByID(round trip): %v", err)
	}
	assertStrategyEqual(t, loaded, roundTrip)

	sameAwardID := mustStrategy(
		t,
		domain.StrategyID(baseID+1),
		"Second wheel",
		[]domain.Award{mustAward(t, 10, "Scoped identity", 1, domain.AwardOutcomeReward)},
	)
	if err := repository.Create(ctx, sameAwardID); err != nil {
		t.Fatalf("Create(strategy-scoped award identity): %v", err)
	}
	loaded, err = repository.FindByID(ctx, sameAwardID.ID())
	if err != nil {
		t.Fatalf("FindByID(strategy-scoped award identity): %v", err)
	}
	assertStrategyEqual(t, loaded, sameAwardID)

	maximum := mustStrategy(
		t,
		domain.StrategyID(math.MaxUint64),
		strings.Repeat("奖", domain.MaxStrategyNameRunes),
		[]domain.Award{mustAward(
			t,
			domain.AwardID(math.MaxUint64),
			strings.Repeat("品", domain.MaxAwardNameRunes),
			domain.Weight(math.MaxUint64),
			domain.AwardOutcomeReward,
		)},
	)
	if err := repository.Create(ctx, maximum); err != nil {
		t.Fatalf("Create(max uint64): %v", err)
	}
	loaded, err = repository.FindByID(ctx, maximum.ID())
	if err != nil {
		t.Fatalf("FindByID(max uint64): %v", err)
	}
	assertStrategyEqual(t, loaded, maximum)

	if err := repository.Create(ctx, roundTrip); !errors.Is(err, application.ErrStrategyAlreadyExists) {
		t.Fatalf("Create(duplicate) error = %v, want already exists", err)
	}
	assertAggregateRowCounts(t, verificationDatabase, uint64(roundTrip.ID()), 1, len(roundTrip.Awards()))

	concurrent := mustStrategy(
		t,
		domain.StrategyID(baseID+2),
		"Concurrent create",
		[]domain.Award{mustAward(t, 1, "Only outcome", 1, domain.AwardOutcomeReward)},
	)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- repository.Create(ctx, concurrent)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes, duplicates := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, application.ErrStrategyAlreadyExists):
			duplicates++
		default:
			t.Fatalf("concurrent Create() unexpected error: %v", result)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent Create() results = %d success, %d duplicate; want 1 and 1", successes, duplicates)
	}
	assertAggregateRowCounts(t, verificationDatabase, uint64(concurrent.ID()), 1, 1)

	installRollbackProbe(
		t,
		verificationDatabase,
		rollbackConstraint,
		baseID+3,
		rollbackProbeAwardID,
	)
	rollbackStrategy := mustStrategy(
		t,
		domain.StrategyID(baseID+3),
		"Rollback probe",
		[]domain.Award{mustAward(
			t,
			domain.AwardID(rollbackProbeAwardID),
			"Rejected child",
			1,
			domain.AwardOutcomeReward,
		)},
	)
	if err := repository.Create(ctx, rollbackStrategy); !errors.Is(err, application.ErrRepositoryFailure) {
		t.Fatalf("Create(constraint rejection) error = %v, want storage operation failed", err)
	}
	dropRollbackProbe(t, verificationDatabase, rollbackConstraint)
	assertAggregateRowCounts(t, verificationDatabase, uint64(rollbackStrategy.ID()), 0, 0)

	canceledContext, cancelImmediately := context.WithCancel(context.Background())
	cancelImmediately()
	if _, err := repository.FindByID(canceledContext, roundTrip.ID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindByID(canceled) error = %v, want context canceled", err)
	}
	canceledStrategy := mustStrategy(
		t,
		domain.StrategyID(baseID+8),
		"Canceled create",
		[]domain.Award{mustAward(t, 1, "Never stored", 1, domain.AwardOutcomeReward)},
	)
	if err := repository.Create(canceledContext, canceledStrategy); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create(canceled) error = %v, want context canceled", err)
	}
	assertAggregateRowCounts(t, verificationDatabase, uint64(canceledStrategy.ID()), 0, 0)

	blocker, err := verificationDatabase.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatalf("begin award gap-lock blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback() }()
	lockedRows, err := blocker.QueryContext(ctx, `
		SELECT award_id
		FROM lottery_strategy_award
		WHERE strategy_id = ?
		FOR UPDATE`, uint64(canceledStrategy.ID()))
	if err != nil {
		_ = blocker.Rollback()
		t.Fatalf("lock empty award range: %v", err)
	}
	if err := lockedRows.Close(); err != nil {
		_ = blocker.Rollback()
		t.Fatalf("close award gap-lock rows: %v", err)
	}
	blockedContext, cancelBlockedCreate := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelBlockedCreate()
	blockedAt := time.Now()
	if err := repository.Create(blockedContext, canceledStrategy); !errors.Is(err, context.DeadlineExceeded) {
		_ = blocker.Rollback()
		t.Fatalf("Create(blocked until deadline) error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(blockedAt); elapsed < 1500*time.Millisecond {
		_ = blocker.Rollback()
		t.Fatalf("blocked Create() returned after %s, want evidence it waited on the in-flight Award insert", elapsed)
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatalf("release award gap-lock blocker: %v", err)
	}
	assertAggregateRowCounts(t, verificationDatabase, uint64(canceledStrategy.ID()), 0, 0)

	insertCorruptSnapshots(t, verificationDatabase, baseID+4)
	for _, corruptID := range []uint64{baseID + 4, baseID + 5, baseID + 6, baseID + 7} {
		got, err := repository.FindByID(ctx, domain.StrategyID(corruptID))
		if !errors.Is(err, application.ErrStoredStrategyInvalid) {
			t.Fatalf("FindByID(corrupt %d) error = %v, want stored strategy invalid", corruptID, err)
		}
		if err.Error() != application.ErrStoredStrategyInvalid.Error() {
			t.Fatalf("FindByID(corrupt %d) rendered error = %q, want safe semantic class", corruptID, err.Error())
		}
		if got.ID() != 0 {
			t.Fatalf("FindByID(corrupt %d) returned partial aggregate %#v", corruptID, got)
		}
	}

	snapshotOld := mustStrategy(
		t,
		domain.StrategyID(baseID+9),
		"Snapshot old",
		[]domain.Award{mustAward(t, 1, "Existing award", 3, domain.AwardOutcomeReward)},
	)
	if err := repository.Create(ctx, snapshotOld); err != nil {
		t.Fatalf("Create(snapshot fixture): %v", err)
	}
	reader, err := applicationDatabase.BeginTxx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		t.Fatalf("begin snapshot reader: %v", err)
	}
	defer func() { _ = reader.Rollback() }()
	oldRoot, err := loadStoredStrategy(ctx, reader, snapshotOld.ID())
	if err != nil {
		t.Fatalf("load old snapshot root: %v", err)
	}
	writer, err := verificationDatabase.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin snapshot writer: %v", err)
	}
	if _, err := writer.ExecContext(
		ctx,
		"UPDATE lottery_strategy SET name = ? WHERE strategy_id = ?",
		"Snapshot new",
		uint64(snapshotOld.ID()),
	); err != nil {
		_ = writer.Rollback()
		t.Fatalf("update snapshot root: %v", err)
	}
	if _, err := writer.ExecContext(ctx, `
		INSERT INTO lottery_strategy_award
			(strategy_id, award_id, name, weight, outcome)
		VALUES (?, 2, 'New award', 5, 'no_reward')`, uint64(snapshotOld.ID())); err != nil {
		_ = writer.Rollback()
		t.Fatalf("insert snapshot award: %v", err)
	}
	if err := writer.Commit(); err != nil {
		t.Fatalf("commit snapshot writer: %v", err)
	}
	oldAwardRows, err := loadStoredAwards(ctx, reader, snapshotOld.ID())
	if err != nil {
		t.Fatalf("load old snapshot awards: %v", err)
	}
	if err := reader.Commit(); err != nil {
		t.Fatalf("commit snapshot reader: %v", err)
	}
	oldSnapshot, err := restoreStrategy(oldRoot, oldAwardRows)
	if err != nil {
		t.Fatalf("restore old snapshot: %v", err)
	}
	assertStrategyEqual(t, oldSnapshot, snapshotOld)
	newSnapshot, err := repository.FindByID(ctx, snapshotOld.ID())
	if err != nil {
		t.Fatalf("FindByID(new snapshot): %v", err)
	}
	if newSnapshot.Name() != "Snapshot new" || len(newSnapshot.Awards()) != 2 || newSnapshot.TotalWeight() != 8 {
		t.Fatalf("new snapshot = name %q, awards %d, total %d; want new/2/8",
			newSnapshot.Name(), len(newSnapshot.Awards()), newSnapshot.TotalWeight())
	}

	assertRepositoryLookupsUsePrimaryKeys(t, verificationDatabase, uint64(roundTrip.ID()))
	if err := applicationDatabase.PingContext(ctx); err != nil {
		t.Fatalf("shared pool was not usable after repository operations: %v", err)
	}
}

func repositoryIntegrationConnection(t *testing.T, prefix string) mysqlstore.ConnectionConfig {
	t.Helper()

	requiredSuffixes := []string{"_ADDRESS", "_DATABASE", "_USER", "_PASSWORD"}
	values := make(map[string]string, len(requiredSuffixes))
	for _, suffix := range requiredSuffixes {
		key := prefix + suffix
		value, ok := os.LookupEnv(key)
		if !ok {
			t.Skipf("integration test requires explicit %s", key)
		}
		values[key] = value
	}
	mode := mysqlstore.TLSDisabled
	if raw, ok := os.LookupEnv(prefix + "_TLS_MODE"); ok {
		mode = mysqlstore.TLSMode(raw)
	}
	return mysqlstore.ConnectionConfig{
		Address:        values[prefix+"_ADDRESS"],
		Database:       values[prefix+"_DATABASE"],
		User:           values[prefix+"_USER"],
		Password:       values[prefix+"_PASSWORD"],
		TLSMode:        mode,
		TLSCAFile:      os.Getenv(prefix + "_TLS_CA_FILE"),
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    25 * time.Second,
		WriteTimeout:   15 * time.Second,
	}
}

func ensureRepositorySchema(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()

	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open migration database for repository integration: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct migration runner for repository integration: %v", err)
	}
	if _, err := runner.Up(ctx); err != nil {
		_ = runner.Close()
		t.Fatalf("apply schema for repository integration: %v", err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close repository integration migration runner: %v", err)
	}
}

func cleanupRepositoryFixtures(database *sql.DB, ids []uint64, constraintName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := removeRollbackProbe(ctx, database, constraintName); err != nil {
		return fmt.Errorf("drop rollback probe constraint: %w", err)
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fixture cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy_award WHERE strategy_id = ?", id); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete awards for strategy %d: %w", id, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy WHERE strategy_id = ?", id); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete strategy %d: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fixture cleanup: %w", err)
	}
	for _, id := range ids {
		var roots, awards int
		if err := database.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM lottery_strategy WHERE strategy_id = ?",
			id,
		).Scan(&roots); err != nil {
			return fmt.Errorf("verify strategy cleanup for %d: %w", id, err)
		}
		if err := database.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM lottery_strategy_award WHERE strategy_id = ?",
			id,
		).Scan(&awards); err != nil {
			return fmt.Errorf("verify award cleanup for %d: %w", id, err)
		}
		if roots != 0 || awards != 0 {
			return fmt.Errorf("fixture cleanup left strategy %d with %d roots and %d awards", id, roots, awards)
		}
	}
	var remainingConstraint int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'lottery_strategy_award'
		  AND constraint_name = ?`, constraintName).Scan(&remainingConstraint); err != nil {
		return fmt.Errorf("verify rollback constraint cleanup: %w", err)
	}
	if remainingConstraint != 0 {
		return fmt.Errorf("rollback constraint %q remains after cleanup", constraintName)
	}
	return nil
}

func mustAward(
	t *testing.T,
	id domain.AwardID,
	name string,
	weight domain.Weight,
	outcome domain.AwardOutcome,
) domain.Award {
	t.Helper()

	award, err := domain.NewAward(id, name, weight, outcome)
	if err != nil {
		t.Fatalf("NewAward(): %v", err)
	}
	return award
}

func mustStrategy(
	t *testing.T,
	id domain.StrategyID,
	name string,
	awards []domain.Award,
) domain.Strategy {
	t.Helper()

	strategy, err := domain.NewStrategy(id, name, awards)
	if err != nil {
		t.Fatalf("NewStrategy(): %v", err)
	}
	return strategy
}

func assertStrategyEqual(t *testing.T, got, want domain.Strategy) {
	t.Helper()

	if got.ID() != want.ID() || got.Name() != want.Name() || got.TotalWeight() != want.TotalWeight() {
		t.Fatalf("strategy root = (%d, %q, %d), want (%d, %q, %d)",
			got.ID(), got.Name(), got.TotalWeight(), want.ID(), want.Name(), want.TotalWeight())
	}
	gotAwards, wantAwards := got.Awards(), want.Awards()
	if len(gotAwards) != len(wantAwards) {
		t.Fatalf("award count = %d, want %d", len(gotAwards), len(wantAwards))
	}
	for index := range gotAwards {
		if gotAwards[index] != wantAwards[index] {
			t.Fatalf("award[%d] = %#v, want %#v", index, gotAwards[index], wantAwards[index])
		}
	}
}

func assertAggregateRowCounts(t *testing.T, database *sql.DB, strategyID uint64, wantRoot, wantAwards int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var roots, awards int
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM lottery_strategy WHERE strategy_id = ?",
		strategyID,
	).Scan(&roots); err != nil {
		t.Fatalf("count strategy rows: %v", err)
	}
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM lottery_strategy_award WHERE strategy_id = ?",
		strategyID,
	).Scan(&awards); err != nil {
		t.Fatalf("count award rows: %v", err)
	}
	if roots != wantRoot || awards != wantAwards {
		t.Fatalf("stored rows = %d root/%d awards, want %d/%d", roots, awards, wantRoot, wantAwards)
	}
}

func installRollbackProbe(
	t *testing.T,
	database *sql.DB,
	constraintName string,
	strategyID uint64,
	awardID uint64,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := removeRollbackProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("remove stale rollback probe constraint: %v", err)
	}
	statement := fmt.Sprintf(
		"ALTER TABLE lottery_strategy_award ADD CONSTRAINT `%s` CHECK (strategy_id <> %d OR award_id <> %d)",
		constraintName,
		strategyID,
		awardID,
	)
	if _, err := database.ExecContext(ctx, statement); err != nil {
		t.Fatalf("install rollback probe constraint: %v", err)
	}
}

func dropRollbackProbe(t *testing.T, database *sql.DB, constraintName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := removeRollbackProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("drop rollback probe constraint: %v", err)
	}
}

func removeRollbackProbe(ctx context.Context, database *sql.DB, constraintName string) error {
	var exists int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'lottery_strategy_award'
		  AND constraint_name = ?`, constraintName).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := database.ExecContext(
		ctx,
		"ALTER TABLE lottery_strategy_award DROP CHECK `"+constraintName+"`",
	)
	return err
}

func insertCorruptSnapshots(t *testing.T, database *sql.DB, firstID uint64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin corrupt snapshot fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy (strategy_id, name)
		VALUES (?, ?), (?, ?), (?, ?), (?, ?)`,
		firstID, "No awards",
		firstID+1, "\u00a0non canonical",
		firstID+2, "Control-character award",
		firstID+3, "Overflowing total",
	); err != nil {
		t.Fatalf("insert corrupt strategy roots: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy_award
			(strategy_id, award_id, name, weight, outcome)
		VALUES
			(?, 1, 'Valid award', 1, 'reward'),
			(?, 1, ?, 1, 'reward'),
			(?, 1, 'Largest', ?, 'reward'),
			(?, 2, 'Overflow', 1, 'no_reward')`,
		firstID+1,
		firstID+2, "invalid\nname",
		firstID+3, uint64(math.MaxUint64),
		firstID+3,
	); err != nil {
		t.Fatalf("insert corrupt strategy awards: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit corrupt snapshot fixture: %v", err)
	}
}

func assertRepositoryLookupsUsePrimaryKeys(t *testing.T, database *sql.DB, strategyID uint64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rootPlan := explainSingleTableLookup(t, ctx, database, selectStrategySQL, strategyID)
	if rootPlan["type"] != "const" || rootPlan["key"] != "PRIMARY" {
		t.Fatalf("strategy lookup EXPLAIN = %#v, want const access through PRIMARY", rootPlan)
	}
	awardPlan := explainSingleTableLookup(t, ctx, database, selectAwardsSQL, strategyID)
	if awardPlan["key"] != "PRIMARY" || awardPlan["key_len"] != "8" {
		t.Fatalf("award lookup EXPLAIN = %#v, want PRIMARY strategy_id left-prefix access", awardPlan)
	}
	if strings.Contains(strings.ToLower(awardPlan["extra"]), "using filesort") {
		t.Fatalf("award lookup EXPLAIN = %#v, unexpected filesort", awardPlan)
	}
}

func explainSingleTableLookup(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	query string,
	strategyID uint64,
) map[string]string {
	t.Helper()

	rows, err := database.QueryContext(ctx, "EXPLAIN "+query, strategyID)
	if err != nil {
		t.Fatalf("explain repository lookup: %v", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("read repository EXPLAIN columns: %v", err)
	}
	values := make([]sql.NullString, len(columns))
	destinations := make([]any, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	if !rows.Next() {
		t.Fatal("repository EXPLAIN returned no row")
	}
	if err := rows.Scan(destinations...); err != nil {
		t.Fatalf("scan repository EXPLAIN: %v", err)
	}
	plan := make(map[string]string, len(columns))
	for index, column := range columns {
		if values[index].Valid {
			plan[strings.ToLower(column)] = values[index].String
		}
	}
	if rows.Next() {
		t.Fatalf("repository EXPLAIN returned an unexpected additional row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate repository EXPLAIN: %v", err)
	}
	return plan
}
