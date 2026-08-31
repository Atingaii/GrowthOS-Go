package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestStrategySnapshotRepositoryMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-30-isolated-schema" {
		t.Skip("Strategy snapshot integration requires Lesson 30 disposable-schema authorization")
	}
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SNAPSHOT_WRITES") != "lesson-30-isolated-strategy-snapshot" {
		t.Skip("Strategy snapshot integration requires explicit snapshot-write authorization")
	}
	snapshotConnection := repositoryIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_SNAPSHOT")
	migrationConnection := repositoryIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	assertSameIsolatedDatabaseDifferentIdentity(t, snapshotConnection, migrationConnection, "snapshot")
	if marketingUser := os.Getenv("GROWTHOS_TEST_MYSQL_MARKETING_USER"); marketingUser != "" &&
		marketingUser == snapshotConnection.User {
		t.Fatal("Strategy snapshot and Marketing repository identities must be distinct")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ensureRepositorySchema(t, ctx, migrationConnection)

	snapshotDatabase := openSnapshotIntegrationDatabase(t, ctx, snapshotConnection)
	verificationDatabase := openSnapshotVerificationDatabase(t, ctx, migrationConnection)
	assertExactSnapshotRepositoryGrants(t, ctx, snapshotDatabase, snapshotConnection.Database)
	assertSnapshotRepositoryForbiddenSurfaces(t, ctx, snapshotDatabase)

	repository, err := NewStrategySnapshotRepository(snapshotDatabase)
	if err != nil {
		t.Fatalf("construct Strategy snapshot repository: %v", err)
	}
	baseID := uint64(time.Now().UnixNano())
	strategyIDs := []uint64{baseID, baseID + 1, baseID + 2, baseID + 3}
	constraintName := "chk_l30_ss_" + strconv.FormatUint(baseID, 36)
	cleaned := false
	if err := cleanupSnapshotFixtures(verificationDatabase, strategyIDs, constraintName); err != nil {
		t.Fatalf("initial Strategy snapshot fixture cleanup: %v", err)
	}
	defer func() {
		if cleaned {
			return
		}
		if err := cleanupSnapshotFixtures(verificationDatabase, strategyIDs, constraintName); err != nil {
			t.Errorf("deferred Strategy snapshot fixture cleanup: %v", err)
		}
	}()
	seedSnapshotStrategyRoots(t, ctx, verificationDatabase, strategyIDs)

	roundTrip := mustStrategySnapshot(t, domain.StrategyID(baseID), "release:r1")
	if err := repository.CreateSnapshot(ctx, roundTrip); err != nil {
		t.Fatalf("CreateSnapshot(round trip): %v", err)
	}
	loaded, err := repository.FindSnapshotByIdentity(ctx, roundTrip.Identity())
	if err != nil {
		t.Fatalf("FindSnapshotByIdentity(round trip): %v", err)
	}
	assertStrategySnapshotsEqual(t, loaded, roundTrip)
	if err := repository.CreateSnapshot(ctx, roundTrip); !errors.Is(err, application.ErrStrategySnapshotAlreadyExists) {
		t.Fatalf("CreateSnapshot(duplicate) error = %v, want already exists", err)
	}
	assertSnapshotRowCounts(t, verificationDatabase, baseID, 1, 2)

	maximum := mustMaximumAwardSnapshot(t, domain.StrategyID(baseID+1), "capacity:r1000")
	if err := repository.CreateSnapshot(ctx, maximum); err != nil {
		t.Fatalf("CreateSnapshot(1000 awards): %v", err)
	}
	loaded, err = repository.FindSnapshotByIdentity(ctx, maximum.Identity())
	if err != nil {
		t.Fatalf("FindSnapshotByIdentity(1000 awards): %v", err)
	}
	assertStrategySnapshotsEqual(t, loaded, maximum)
	assertSnapshotRowCounts(t, verificationDatabase, baseID+1, 1, domain.MaxAwardsPerStrategy)

	concurrent := mustStrategySnapshot(t, domain.StrategyID(baseID+2), "concurrent:r1")
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- repository.CreateSnapshot(ctx, concurrent)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, application.ErrStrategySnapshotAlreadyExists):
			conflicts++
		default:
			t.Fatalf("concurrent CreateSnapshot error = %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent CreateSnapshot = %d success/%d conflict, want 1/1", successes, conflicts)
	}
	assertSnapshotRowCounts(t, verificationDatabase, baseID+2, 1, 2)

	installSnapshotChildFailureProbe(t, ctx, verificationDatabase, constraintName, baseID+3, 20)
	childFailure := mustStrategySnapshot(t, domain.StrategyID(baseID+3), "child-failure:r1")
	if err := repository.CreateSnapshot(ctx, childFailure); !errors.Is(err, application.ErrRepositoryFailure) {
		t.Fatalf("CreateSnapshot(child failure) error = %v, want repository failure", err)
	}
	dropSnapshotChildFailureProbe(t, ctx, verificationDatabase, constraintName)
	assertSnapshotRowCounts(t, verificationDatabase, baseID+3, 0, 0)

	if err := cleanupSnapshotFixtures(verificationDatabase, strategyIDs, constraintName); err != nil {
		t.Fatalf("final Strategy snapshot fixture cleanup: %v", err)
	}
	cleaned = true
}

func assertSameIsolatedDatabaseDifferentIdentity(
	t *testing.T,
	left mysqlstore.ConnectionConfig,
	right mysqlstore.ConnectionConfig,
	name string,
) {
	t.Helper()
	if left.Address != right.Address || left.Database != right.Database {
		t.Fatalf("%s and migration identities must target the same isolated database", name)
	}
	if left.User == right.User {
		t.Fatalf("%s and migration identities must be distinct", name)
	}
}

func openSnapshotIntegrationDatabase(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) *sqlx.DB {
	t.Helper()
	database, err := mysqlstore.Open(ctx, mysqlstore.Config{
		ConnectionConfig:      connection,
		PingTimeout:           5 * time.Second,
		MaxOpenConnections:    8,
		MaxIdleConnections:    4,
		ConnectionMaxLifetime: time.Minute,
		ConnectionMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open Strategy snapshot database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func openSnapshotVerificationDatabase(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) *sql.DB {
	t.Helper()
	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open Strategy snapshot verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func assertExactSnapshotRepositoryGrants(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	databaseName string,
) {
	t.Helper()
	var currentAccount string
	if err := database.GetContext(ctx, &currentAccount, "SELECT CURRENT_USER()"); err != nil {
		t.Fatalf("read Strategy snapshot account: %v", err)
	}
	separator := strings.LastIndexByte(currentAccount, '@')
	if separator <= 0 || separator == len(currentAccount)-1 {
		t.Fatalf("snapshot CURRENT_USER() = %q, want user@host", currentAccount)
	}
	quote := func(value string) string { return "`" + strings.ReplaceAll(value, "`", "``") + "`" }
	account := quote(currentAccount[:separator]) + "@" + quote(currentAccount[separator+1:])
	quotedDatabase := quote(databaseName)
	want := []string{
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`lottery_strategy_snapshot` TO " + account,
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`lottery_strategy_snapshot_award` TO " + account,
		"GRANT USAGE ON *.* TO " + account,
	}
	var got []string
	if err := database.SelectContext(ctx, &got, "SHOW GRANTS FOR CURRENT_USER"); err != nil {
		t.Fatalf("read Strategy snapshot grants: %v", err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Strategy snapshot grants = %q, want exact %q", got, want)
	}
}

func assertSnapshotRepositoryForbiddenSurfaces(t *testing.T, ctx context.Context, database *sqlx.DB) {
	t.Helper()
	readProbes := []string{
		"SELECT COUNT(*) FROM lottery_strategy WHERE 1 = 0",
		"SELECT COUNT(*) FROM lottery_strategy_routing_graph WHERE 1 = 0",
		"SELECT COUNT(*) FROM marketing_activity WHERE 1 = 0",
		"SELECT version FROM schema_migrations LIMIT 1",
	}
	for _, query := range readProbes {
		var count int
		err := database.GetContext(ctx, &count, query)
		expectSnapshotMySQLErrorNumber(t, err, 1142)
	}
	writeProbes := []string{
		"UPDATE lottery_strategy_snapshot SET schema_version = schema_version WHERE strategy_id = 0",
		"DELETE FROM lottery_strategy_snapshot WHERE strategy_id = 0",
		"UPDATE lottery_strategy_snapshot_award SET weight = weight WHERE strategy_id = 0",
		"DELETE FROM lottery_strategy_snapshot_award WHERE strategy_id = 0",
		"INSERT INTO lottery_strategy (strategy_id, name) VALUES (1, 'forbidden')",
		"INSERT INTO lottery_strategy_routing_graph (graph_id, revision, schema_version, root_node_id) VALUES (1, 'forbidden:r1', 1, 1)",
		"INSERT INTO marketing_activity (activity_id, name, lifecycle_state, state_version) VALUES (1, 'forbidden', 'draft', 0)",
		"INSERT INTO schema_migrations (version, dirty) VALUES (2147483647, 0)",
		"UPDATE schema_migrations SET dirty = dirty",
	}
	for _, statement := range writeProbes {
		_, err := database.ExecContext(ctx, statement)
		expectSnapshotMySQLErrorNumber(t, err, 1142)
	}
}

func mustMaximumAwardSnapshot(
	t *testing.T,
	id domain.StrategyID,
	revision string,
) domain.StrategySnapshot {
	t.Helper()
	awards := make([]domain.Award, 0, domain.MaxAwardsPerStrategy)
	for index := 1; index <= domain.MaxAwardsPerStrategy; index++ {
		outcome := domain.AwardOutcomeReward
		if index%2 == 0 {
			outcome = domain.AwardOutcomeNoReward
		}
		award, err := domain.NewAward(
			domain.AwardID(index),
			"Capacity award "+strconv.Itoa(index),
			domain.Weight(index),
			outcome,
		)
		if err != nil {
			t.Fatalf("NewAward(%d): %v", index, err)
		}
		awards = append(awards, award)
	}
	strategy, err := domain.NewStrategy(id, "Maximum capacity Strategy", awards)
	if err != nil {
		t.Fatalf("NewStrategy(1000 awards): %v", err)
	}
	snapshot, err := domain.NewStrategySnapshot(revision, strategy)
	if err != nil {
		t.Fatalf("NewStrategySnapshot(1000 awards): %v", err)
	}
	return snapshot
}

func seedSnapshotStrategyRoots(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	strategyIDs []uint64,
) {
	t.Helper()
	for _, id := range strategyIDs {
		if _, err := database.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ?)",
			id,
			"Snapshot root "+strconv.FormatUint(id, 10),
		); err != nil {
			t.Fatalf("seed Strategy root %d: %v", id, err)
		}
	}
}

func assertSnapshotRowCounts(
	t *testing.T,
	database *sql.DB,
	strategyID uint64,
	wantHeaders int,
	wantAwards int,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var headers, awards int
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM lottery_strategy_snapshot WHERE strategy_id = ?",
		strategyID,
	).Scan(&headers); err != nil {
		t.Fatalf("count Strategy snapshot headers: %v", err)
	}
	if err := database.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM lottery_strategy_snapshot_award WHERE strategy_id = ?",
		strategyID,
	).Scan(&awards); err != nil {
		t.Fatalf("count Strategy snapshot awards: %v", err)
	}
	if headers != wantHeaders || awards != wantAwards {
		t.Fatalf("snapshot row counts = %d/%d, want %d/%d", headers, awards, wantHeaders, wantAwards)
	}
}

func installSnapshotChildFailureProbe(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	constraintName string,
	strategyID uint64,
	awardID uint64,
) {
	t.Helper()
	if err := removeSnapshotChildFailureProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("remove stale snapshot child failure probe: %v", err)
	}
	statement := fmt.Sprintf(
		"ALTER TABLE lottery_strategy_snapshot_award ADD CONSTRAINT `%s` CHECK (strategy_id <> %d OR award_id <> %d)",
		constraintName,
		strategyID,
		awardID,
	)
	if _, err := database.ExecContext(ctx, statement); err != nil {
		t.Fatalf("install snapshot child failure probe: %v", err)
	}
}

func dropSnapshotChildFailureProbe(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	constraintName string,
) {
	t.Helper()
	if err := removeSnapshotChildFailureProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("drop snapshot child failure probe: %v", err)
	}
}

func removeSnapshotChildFailureProbe(
	ctx context.Context,
	database *sql.DB,
	constraintName string,
) error {
	if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(constraintName) {
		return fmt.Errorf("invalid snapshot constraint name %q", constraintName)
	}
	var exists int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'lottery_strategy_snapshot_award'
		  AND constraint_name = ?`, constraintName).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := database.ExecContext(
		ctx,
		"ALTER TABLE lottery_strategy_snapshot_award DROP CHECK `"+constraintName+"`",
	)
	return err
}

func cleanupSnapshotFixtures(
	database *sql.DB,
	strategyIDs []uint64,
	constraintName string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := removeSnapshotChildFailureProbe(ctx, database, constraintName); err != nil {
		return err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range strategyIDs {
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy_snapshot_award WHERE strategy_id = ?", id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy_snapshot WHERE strategy_id = ?", id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy WHERE strategy_id = ?", id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func expectSnapshotMySQLErrorNumber(t *testing.T, err error, number uint16) {
	t.Helper()
	if err == nil {
		t.Fatalf("operation unexpectedly succeeded; want MySQL error %d", number)
	}
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != number {
		t.Fatalf("MySQL error = %v, want number %d", err, number)
	}
}
