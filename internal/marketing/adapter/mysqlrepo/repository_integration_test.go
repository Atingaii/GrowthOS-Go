package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	lotterymysql "github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/mysqlrepo"
	lotteryapplication "github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	lotteryconfig "github.com/Atingaii/GrowthOS-Go/internal/marketing/adapter/lotteryconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/marketing/application"
	"github.com/Atingaii/GrowthOS-Go/internal/marketing/domain"
	projectmigrations "github.com/Atingaii/GrowthOS-Go/migrations"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestActivityRepositoryMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-30-isolated-schema" {
		t.Skip("Activity repository integration requires Lesson 30 disposable-schema authorization")
	}
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_ACTIVITY_WRITES") != "lesson-30-isolated-activity" {
		t.Skip("Activity repository integration requires explicit Activity-write authorization")
	}
	marketingConnection := marketingIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MARKETING")
	migrationConnection := marketingIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	assertMarketingConnectionsIsolated(t, marketingConnection, migrationConnection)
	if snapshotUser := os.Getenv("GROWTHOS_TEST_MYSQL_SNAPSHOT_USER"); snapshotUser != "" &&
		snapshotUser == marketingConnection.User {
		t.Fatal("Marketing and Strategy snapshot writers must use distinct identities")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ensureMarketingSchema(t, ctx, migrationConnection)
	marketingDatabase := openMarketingDatabase(t, ctx, marketingConnection)
	verificationDatabase := openMarketingVerificationDatabase(t, ctx, migrationConnection)
	lotteryReaderDatabase := openMarketingDatabase(t, ctx, migrationConnection)
	assertExactMarketingRepositoryGrants(t, ctx, marketingDatabase, marketingConnection.Database)
	assertMarketingRepositoryForbiddenSurfaces(t, ctx, marketingDatabase)

	repository, err := New(marketingDatabase)
	if err != nil {
		t.Fatalf("construct Activity repository: %v", err)
	}
	baseID := uint64(time.Now().UnixNano())
	activityIDs := []uint64{baseID, baseID + 1, baseID + 2, baseID + 3}
	constraintName := "chk_l30_ma_" + strconv.FormatUint(baseID, 36)
	cleaned := false
	if err := cleanupMarketingFixtures(verificationDatabase, activityIDs, constraintName); err != nil {
		t.Fatalf("initial Marketing fixture cleanup: %v", err)
	}
	defer func() {
		if cleaned {
			return
		}
		if err := cleanupMarketingFixtures(verificationDatabase, activityIDs, constraintName); err != nil {
			t.Errorf("deferred Marketing fixture cleanup: %v", err)
		}
	}()

	draft := mustIntegrationActivity(t, domain.ActivityID(baseID))
	if err := repository.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	current, zeroPublication, err := repository.FindCurrentActivity(ctx, draft.ID())
	if err != nil {
		t.Fatalf("FindCurrentActivity(draft): %v", err)
	}
	assertActivitiesEqual(t, current, draft)
	if !reflect.DeepEqual(zeroPublication, domain.ActivityPublication{}) {
		t.Fatalf("draft current publication = %#v, want zero", zeroPublication)
	}

	releaseOne := mustIntegrationPublish(t, draft, 9001, "missing-graph:r1", 7001, "missing-strategy:r1", "approval/release-1", testInstant())
	if err := repository.CompareAndSwapPublication(ctx, releaseOne); err != nil {
		t.Fatalf("publish release one: %v", err)
	}
	releaseOneRecord, _ := releaseOne.Record()
	current, active, err := repository.FindCurrentActivity(ctx, draft.ID())
	if err != nil {
		t.Fatalf("FindCurrentActivity(release one): %v", err)
	}
	assertActivitiesEqual(t, current, releaseOne.Next())
	assertPublicationsEqual(t, active, releaseOneRecord)
	assertBadCrossContextReferenceRejectedByVerifier(
		t,
		ctx,
		lotteryReaderDatabase,
		active,
	)

	releaseTwo := mustIntegrationPublish(t, current, 9002, "missing-graph:r2", 7002, "missing-strategy:r2", "approval/release-2", testInstant().Add(time.Minute))
	assertMarketingRepeatableReadAcrossReplacement(
		t,
		ctx,
		marketingDatabase,
		repository,
		releaseTwo,
	)
	current, active, err = repository.FindCurrentActivity(ctx, draft.ID())
	if err != nil {
		t.Fatalf("FindCurrentActivity(release two): %v", err)
	}
	assertActivitiesEqual(t, current, releaseTwo.Next())
	releaseTwoRecord, _ := releaseTwo.Record()
	assertPublicationsEqual(t, active, releaseTwoRecord)
	historicalOne, err := repository.FindPublicationByIdentity(ctx, draft.ID(), releaseOneRecord.Version())
	if err != nil {
		t.Fatalf("FindPublicationByIdentity(release one): %v", err)
	}
	assertPublicationsEqual(t, historicalOne, releaseOneRecord)

	rollback, err := domain.PlanRollback(
		current,
		releaseOneRecord,
		true,
		mustIntegrationEvidence(t, "approval/rollback-3"),
		testInstant().Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanRollback: %v", err)
	}
	if err := repository.CompareAndSwapPublication(ctx, rollback); err != nil {
		t.Fatalf("persist rollback: %v", err)
	}
	current, active, err = repository.FindCurrentActivity(ctx, draft.ID())
	if err != nil {
		t.Fatalf("FindCurrentActivity(rollback): %v", err)
	}
	assertActivitiesEqual(t, current, rollback.Next())
	rollbackRecord, _ := rollback.Record()
	assertPublicationsEqual(t, active, rollbackRecord)
	if rollbackOf, ok := active.RollbackOf(); !ok || rollbackOf != releaseOneRecord.Version() {
		t.Fatalf("rollback-of = %d/%v, want exact release one", rollbackOf, ok)
	}

	retirement, err := domain.PlanRetire(
		current,
		mustIntegrationEvidence(t, "retirement/change-4"),
		testInstant().Add(3*time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanRetire: %v", err)
	}
	if err := repository.CompareAndSwapRetirement(ctx, retirement); err != nil {
		t.Fatalf("persist retirement: %v", err)
	}
	current, active, err = repository.FindCurrentActivity(ctx, draft.ID())
	if err != nil {
		t.Fatalf("FindCurrentActivity(retired): %v", err)
	}
	assertActivitiesEqual(t, current, retirement.Next())
	assertPublicationsEqual(t, active, rollbackRecord)
	assertMarketingRowCounts(t, verificationDatabase, baseID, 1, 3, 3)

	assertConcurrentSameExpectedExactlyOneSuccess(
		t,
		ctx,
		repository,
		domain.ActivityID(baseID+1),
	)
	assertActivityChildFailureRollsBack(
		t,
		ctx,
		repository,
		verificationDatabase,
		domain.ActivityID(baseID+2),
		constraintName,
	)
	assertActivityCASLossRollsBackNewHistory(
		t,
		ctx,
		repository,
		verificationDatabase,
		domain.ActivityID(baseID+3),
	)

	if err := cleanupMarketingFixtures(verificationDatabase, activityIDs, constraintName); err != nil {
		t.Fatalf("final Marketing fixture cleanup: %v", err)
	}
	cleaned = true
}

func marketingIntegrationConnection(t *testing.T, prefix string) mysqlstore.ConnectionConfig {
	t.Helper()
	values := make(map[string]string)
	for _, suffix := range []string{"_ADDRESS", "_DATABASE", "_USER", "_PASSWORD"} {
		key := prefix + suffix
		value, ok := os.LookupEnv(key)
		if !ok {
			t.Skipf("integration test requires explicit %s", key)
		}
		values[key] = value
	}
	return mysqlstore.ConnectionConfig{
		Address:        values[prefix+"_ADDRESS"],
		Database:       values[prefix+"_DATABASE"],
		User:           values[prefix+"_USER"],
		Password:       values[prefix+"_PASSWORD"],
		TLSMode:        mysqlstore.TLSDisabled,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    25 * time.Second,
		WriteTimeout:   15 * time.Second,
	}
}

func assertMarketingConnectionsIsolated(
	t *testing.T,
	marketing mysqlstore.ConnectionConfig,
	migration mysqlstore.ConnectionConfig,
) {
	t.Helper()
	if marketing.Address != migration.Address || marketing.Database != migration.Database {
		t.Fatal("Marketing and migration identities must target the same isolated database")
	}
	if marketing.User == migration.User {
		t.Fatal("Marketing and migration identities must be distinct")
	}
}

func ensureMarketingSchema(
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
		t.Fatalf("open Marketing migration database: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct Marketing migration runner: %v", err)
	}
	result, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("apply Marketing schema: %v", err)
	}
	if result.Version != 14 {
		_ = runner.Close()
		t.Fatalf("Marketing schema result = %+v, want version 14", result)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close Marketing migration runner: %v", err)
	}
}

func openMarketingDatabase(
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
		t.Fatalf("open Marketing database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func openMarketingVerificationDatabase(
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
		t.Fatalf("open Marketing verification database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func assertExactMarketingRepositoryGrants(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	databaseName string,
) {
	t.Helper()
	var currentAccount string
	if err := database.GetContext(ctx, &currentAccount, "SELECT CURRENT_USER()"); err != nil {
		t.Fatalf("read Marketing account: %v", err)
	}
	separator := strings.LastIndexByte(currentAccount, '@')
	if separator <= 0 || separator == len(currentAccount)-1 {
		t.Fatalf("Marketing CURRENT_USER() = %q, want user@host", currentAccount)
	}
	quote := func(value string) string { return "`" + strings.ReplaceAll(value, "`", "``") + "`" }
	account := quote(currentAccount[:separator]) + "@" + quote(currentAccount[separator+1:])
	quotedDatabase := quote(databaseName)
	want := []string{
		"GRANT SELECT, INSERT, UPDATE ON " + quotedDatabase + ".`marketing_activity` TO " + account,
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`marketing_activity_publication` TO " + account,
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`marketing_activity_publication_strategy` TO " + account,
		"GRANT USAGE ON *.* TO " + account,
	}
	var got []string
	if err := database.SelectContext(ctx, &got, "SHOW GRANTS FOR CURRENT_USER"); err != nil {
		t.Fatalf("read Marketing grants: %v", err)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Marketing grants = %q, want exact %q", got, want)
	}
}

func assertMarketingRepositoryForbiddenSurfaces(t *testing.T, ctx context.Context, database *sqlx.DB) {
	t.Helper()
	readProbes := []string{
		"SELECT COUNT(*) FROM lottery_strategy WHERE 1 = 0",
		"SELECT COUNT(*) FROM lottery_strategy_snapshot WHERE 1 = 0",
		"SELECT COUNT(*) FROM lottery_strategy_routing_graph WHERE 1 = 0",
		"SELECT version FROM schema_migrations LIMIT 1",
	}
	for _, query := range readProbes {
		var count int
		err := database.GetContext(ctx, &count, query)
		expectMarketingMySQLErrorNumber(t, err, 1142)
	}
	writeProbes := []string{
		"DELETE FROM marketing_activity WHERE activity_id = 0",
		"UPDATE marketing_activity_publication SET schema_version = schema_version WHERE activity_id = 0",
		"DELETE FROM marketing_activity_publication WHERE activity_id = 0",
		"UPDATE marketing_activity_publication_strategy SET strategy_revision = strategy_revision WHERE activity_id = 0",
		"DELETE FROM marketing_activity_publication_strategy WHERE activity_id = 0",
		"INSERT INTO lottery_strategy (strategy_id, name) VALUES (1, 'forbidden')",
		"INSERT INTO lottery_strategy_snapshot (strategy_id, revision, schema_version, name) VALUES (1, 'forbidden:r1', 1, 'forbidden')",
		"INSERT INTO lottery_strategy_routing_graph (graph_id, revision, schema_version, root_node_id) VALUES (1, 'forbidden:r1', 1, 1)",
		"INSERT INTO schema_migrations (version, dirty) VALUES (2147483647, 0)",
		"UPDATE schema_migrations SET dirty = dirty",
	}
	for _, statement := range writeProbes {
		_, err := database.ExecContext(ctx, statement)
		expectMarketingMySQLErrorNumber(t, err, 1142)
	}
}

func mustIntegrationActivity(t *testing.T, id domain.ActivityID) domain.Activity {
	t.Helper()
	activity, err := domain.NewActivity(id, "Integration Activity "+strconv.FormatUint(uint64(id), 10))
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	return activity
}

func mustIntegrationPublish(
	t *testing.T,
	current domain.Activity,
	graphID domain.LotteryGraphID,
	graphRevision string,
	strategyID domain.LotteryStrategyID,
	strategyRevision string,
	approval string,
	publishedAt time.Time,
) domain.ActivityTransition {
	t.Helper()
	graph, err := domain.NewLotteryGraphReference(graphID, graphRevision)
	if err != nil {
		t.Fatalf("NewLotteryGraphReference: %v", err)
	}
	strategy, err := domain.NewLotteryStrategyRevisionReference(strategyID, strategyRevision)
	if err != nil {
		t.Fatalf("NewLotteryStrategyRevisionReference: %v", err)
	}
	transition, err := domain.PlanPublish(
		current,
		testInstant(),
		testInstant().Add(24*time.Hour),
		graph,
		[]domain.LotteryStrategyRevisionReference{strategy},
		mustIntegrationEvidence(t, approval),
		publishedAt,
	)
	if err != nil {
		t.Fatalf("PlanPublish: %v", err)
	}
	return transition
}

func mustIntegrationEvidence(t *testing.T, value string) domain.EvidenceReference {
	t.Helper()
	reference, err := domain.NewEvidenceReference(value)
	if err != nil {
		t.Fatalf("NewEvidenceReference: %v", err)
	}
	return reference
}

func assertBadCrossContextReferenceRejectedByVerifier(
	t *testing.T,
	ctx context.Context,
	lotteryDatabase *sqlx.DB,
	publication domain.ActivityPublication,
) {
	t.Helper()
	graphs, err := lotterymysql.NewStrategyRoutingGraphRepository(lotteryDatabase)
	if err != nil {
		t.Fatalf("construct Lottery graph reader: %v", err)
	}
	strategies, err := lotterymysql.NewStrategySnapshotRepository(lotteryDatabase)
	if err != nil {
		t.Fatalf("construct Lottery snapshot reader: %v", err)
	}
	verifier, err := lotteryconfig.NewVerifier(graphs, strategies)
	if err != nil {
		t.Fatalf("construct Lottery ACL verifier: %v", err)
	}
	candidate, err := application.NewActivityPublicationCandidate(publication)
	if err != nil {
		t.Fatalf("construct publication candidate: %v", err)
	}
	if err := verifier.VerifyPublication(ctx, candidate); !errors.Is(err, application.ErrLotteryPublicationInvalid) {
		t.Fatalf("verify persisted dangling reference error = %v, want Lottery publication invalid", err)
	}
	// The repository test deliberately bypasses the application service to prove
	// that the database has no cross-context FK. Application service unit tests
	// independently prove that this verifier runs before the writer port.
	var _ lotteryapplication.StrategyRoutingGraphReader = graphs
}

func assertMarketingRepeatableReadAcrossReplacement(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	repository *Repository,
	replacement domain.ActivityTransition,
) {
	t.Helper()
	tx, err := database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		t.Fatalf("begin Marketing RR proof: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	var before uint64
	if err := tx.GetContext(
		ctx,
		&before,
		"SELECT state_version FROM marketing_activity WHERE activity_id = ?",
		uint64(replacement.Next().ID()),
	); err != nil {
		t.Fatalf("read Marketing RR before replacement: %v", err)
	}
	if err := repository.CompareAndSwapPublication(ctx, replacement); err != nil {
		t.Fatalf("persist replacement during Marketing RR proof: %v", err)
	}
	var during uint64
	if err := tx.GetContext(
		ctx,
		&during,
		"SELECT state_version FROM marketing_activity WHERE activity_id = ?",
		uint64(replacement.Next().ID()),
	); err != nil {
		t.Fatalf("reread Marketing RR after replacement: %v", err)
	}
	if before != 1 || during != before {
		t.Fatalf("Marketing RR versions = %d then %d, want stable old version 1", before, during)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit Marketing RR proof: %v", err)
	}
}

func assertConcurrentSameExpectedExactlyOneSuccess(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	id domain.ActivityID,
) {
	t.Helper()
	draft := mustIntegrationActivity(t, id)
	if err := repository.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("create concurrent draft: %v", err)
	}
	transition := mustIntegrationPublish(t, draft, 9101, "concurrent-graph:r1", 7101, "concurrent-strategy:r1", "approval/concurrent", testInstant())
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- repository.CompareAndSwapPublication(ctx, transition)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	successes, conflicts, retryableLosers := 0, 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, application.ErrActivityStateConflict):
			conflicts++
		case errors.Is(result, application.ErrRepositoryRetryable):
			// InnoDB may detect the insert-parent/update-parent lock conversion
			// cycle before the losing transaction reaches the zero-row CAS. The
			// adapter never replays this high-risk command automatically.
			retryableLosers++
		default:
			t.Fatalf("concurrent Activity publication error = %v", result)
		}
	}
	if successes != 1 || conflicts+retryableLosers != 1 {
		t.Fatalf(
			"concurrent Activity publication = %d success/%d conflict/%d retryable, want exactly one success and one rejected transaction",
			successes,
			conflicts,
			retryableLosers,
		)
	}
	current, _, err := repository.FindCurrentActivity(ctx, id)
	if err != nil || current.StateVersion() != 1 {
		t.Fatalf("concurrent Activity read-back = %#v, %v; want one durable version", current, err)
	}
	if retryableLosers == 1 {
		if err := repository.CompareAndSwapPublication(ctx, transition); !errors.Is(err, application.ErrActivityStateConflict) {
			t.Fatalf("stale transition after exact read-back error = %v, want state conflict", err)
		}
	}
}

func assertActivityChildFailureRollsBack(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	verification *sql.DB,
	id domain.ActivityID,
	constraintName string,
) {
	t.Helper()
	draft := mustIntegrationActivity(t, id)
	if err := repository.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("create child-failure draft: %v", err)
	}
	transition := mustIntegrationPublish(t, draft, 9201, "child-graph:r1", 7201, "child-strategy:r1", "approval/child", testInstant())
	installMarketingChildFailureProbe(t, ctx, verification, constraintName, uint64(id), 7201)
	if err := repository.CompareAndSwapPublication(ctx, transition); !errors.Is(err, application.ErrRepositoryFailure) {
		t.Fatalf("child failure error = %v, want repository failure", err)
	}
	dropMarketingChildFailureProbe(t, ctx, verification, constraintName)
	assertMarketingRowCounts(t, verification, uint64(id), 1, 0, 0)
	stored, err := repository.FindActivityByID(ctx, id)
	if err != nil || stored.Lifecycle() != domain.ActivityLifecycleDraft {
		t.Fatalf("root after child failure = %#v, %v; want intact draft", stored, err)
	}
}

func assertActivityCASLossRollsBackNewHistory(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	verification *sql.DB,
	id domain.ActivityID,
) {
	t.Helper()
	draft := mustIntegrationActivity(t, id)
	if err := repository.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("create CAS-loss draft: %v", err)
	}
	inMemoryFirst := mustIntegrationPublish(t, draft, 9301, "unpersisted-graph:r1", 7301, "unpersisted-strategy:r1", "approval/unpersisted-1", testInstant())
	staleSecond := mustIntegrationPublish(t, inMemoryFirst.Next(), 9302, "stale-graph:r2", 7302, "stale-strategy:r2", "approval/stale-2", testInstant().Add(time.Minute))
	if err := repository.CompareAndSwapPublication(ctx, staleSecond); !errors.Is(err, application.ErrActivityStateConflict) {
		t.Fatalf("CAS-loss publication error = %v, want state conflict", err)
	}
	assertMarketingRowCounts(t, verification, uint64(id), 1, 0, 0)
	stored, err := repository.FindActivityByID(ctx, id)
	if err != nil || stored.Lifecycle() != domain.ActivityLifecycleDraft {
		t.Fatalf("root after CAS loss = %#v, %v; want intact draft", stored, err)
	}
}

func assertMarketingRowCounts(
	t *testing.T,
	database *sql.DB,
	activityID uint64,
	wantRoots int,
	wantPublications int,
	wantBindings int,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queries := []string{
		"SELECT COUNT(*) FROM marketing_activity WHERE activity_id = ?",
		"SELECT COUNT(*) FROM marketing_activity_publication WHERE activity_id = ?",
		"SELECT COUNT(*) FROM marketing_activity_publication_strategy WHERE activity_id = ?",
	}
	want := []int{wantRoots, wantPublications, wantBindings}
	for index, query := range queries {
		var got int
		if err := database.QueryRowContext(ctx, query, activityID).Scan(&got); err != nil {
			t.Fatalf("count Marketing rows: %v", err)
		}
		if got != want[index] {
			t.Fatalf("Marketing row count[%d] = %d, want %d", index, got, want[index])
		}
	}
}

func installMarketingChildFailureProbe(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	constraintName string,
	activityID uint64,
	strategyID uint64,
) {
	t.Helper()
	if err := removeMarketingChildFailureProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("remove stale Marketing child failure probe: %v", err)
	}
	statement := fmt.Sprintf(
		"ALTER TABLE marketing_activity_publication_strategy ADD CONSTRAINT `%s` CHECK (activity_id <> %d OR strategy_id <> %d)",
		constraintName,
		activityID,
		strategyID,
	)
	if _, err := database.ExecContext(ctx, statement); err != nil {
		t.Fatalf("install Marketing child failure probe: %v", err)
	}
}

func dropMarketingChildFailureProbe(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	constraintName string,
) {
	t.Helper()
	if err := removeMarketingChildFailureProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("drop Marketing child failure probe: %v", err)
	}
}

func removeMarketingChildFailureProbe(
	ctx context.Context,
	database *sql.DB,
	constraintName string,
) error {
	if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(constraintName) {
		return fmt.Errorf("invalid Marketing constraint name %q", constraintName)
	}
	var exists int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'marketing_activity_publication_strategy'
		  AND constraint_name = ?`, constraintName).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := database.ExecContext(
		ctx,
		"ALTER TABLE marketing_activity_publication_strategy DROP CHECK `"+constraintName+"`",
	)
	return err
}

func cleanupMarketingFixtures(
	database *sql.DB,
	activityIDs []uint64,
	constraintName string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := removeMarketingChildFailureProbe(ctx, database, constraintName); err != nil {
		return err
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range activityIDs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE marketing_activity
			SET lifecycle_state = 'draft', state_version = 0, active_version = NULL,
			    retired_at = NULL, retirement_reference = NULL
			WHERE activity_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM marketing_activity_publication_strategy WHERE activity_id = ?", id); err != nil {
			return err
		}
		rows, err := tx.QueryContext(
			ctx,
			"SELECT activity_version FROM marketing_activity_publication WHERE activity_id = ? ORDER BY activity_version DESC",
			id,
		)
		if err != nil {
			return err
		}
		var versions []uint64
		for rows.Next() {
			var version uint64
			if err := rows.Scan(&version); err != nil {
				_ = rows.Close()
				return err
			}
			versions = append(versions, version)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, version := range versions {
			if _, err := tx.ExecContext(
				ctx,
				"DELETE FROM marketing_activity_publication WHERE activity_id = ? AND activity_version = ?",
				id,
				version,
			); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM marketing_activity WHERE activity_id = ?", id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func expectMarketingMySQLErrorNumber(t *testing.T, err error, number uint16) {
	t.Helper()
	if err == nil {
		t.Fatalf("operation unexpectedly succeeded; want MySQL error %d", number)
	}
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != number {
		t.Fatalf("MySQL error = %v, want number %d", err, number)
	}
}
