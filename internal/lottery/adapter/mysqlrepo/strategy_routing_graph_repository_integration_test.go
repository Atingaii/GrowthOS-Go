package mysqlrepo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
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
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestStrategyRoutingGraphRepositoryMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-19-isolated-schema" {
		t.Skip("rule graph repository integration requires disposable-schema authorization")
	}
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_RULE_GRAPH_WRITES") != "lesson-28-isolated-rule-graph" {
		t.Skip("rule graph repository integration requires explicit lesson-28 write authorization")
	}

	graphConnection := repositoryIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_RULE_GRAPH")
	migrationConnection := repositoryIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	if graphConnection.Address != migrationConnection.Address ||
		graphConnection.Database != migrationConnection.Database {
		t.Fatal("rule graph and migration identities must target the same isolated schema")
	}
	if graphConnection.User == migrationConnection.User {
		t.Fatal("rule graph repository and migration identities must be distinct")
	}
	if apiUser, ok := os.LookupEnv("GROWTHOS_TEST_MYSQL_API_USER"); ok && graphConnection.User == apiUser {
		t.Fatal("rule graph repository identity must not reuse the API identity")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	applyAndVerifyRuleGraphSchemaV5(t, ctx, migrationConnection)

	graphDatabase, err := mysqlstore.Open(ctx, mysqlstore.Config{
		ConnectionConfig:      graphConnection,
		PingTimeout:           5 * time.Second,
		MaxOpenConnections:    8,
		MaxIdleConnections:    4,
		ConnectionMaxLifetime: time.Minute,
		ConnectionMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open rule graph repository database: %v", err)
	}
	t.Cleanup(func() { _ = graphDatabase.Close() })

	verificationDatabase, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: migrationConnection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open rule graph verification database: %v", err)
	}
	t.Cleanup(func() { _ = verificationDatabase.Close() })

	assertExactRuleGraphRepositoryGrants(t, ctx, graphDatabase, graphConnection.Database)
	assertRuleGraphRepositoryForbiddenSurfaces(t, ctx, graphDatabase)

	repository, err := NewStrategyRoutingGraphRepository(graphDatabase)
	if err != nil {
		t.Fatalf("construct rule graph repository: %v", err)
	}

	baseID := uint64(time.Now().UnixNano())
	graphIDs := []uint64{
		baseID,
		baseID + 1,
		baseID + 2,
		baseID + 3,
		baseID + 4,
		baseID + 5,
		math.MaxUint64,
	}
	strategyIDs := []uint64{baseID + 1000, baseID + 1001, math.MaxUint64}
	rollbackConstraint := "chk_l28_rg_" + strconv.FormatUint(baseID, 36)
	cleaned := false
	if err := cleanupRuleGraphRepositoryFixtures(
		verificationDatabase,
		graphIDs,
		strategyIDs,
		rollbackConstraint,
	); err != nil {
		t.Fatalf("initial rule graph fixture cleanup: %v", err)
	}
	defer func() {
		if cleaned {
			return
		}
		if err := cleanupRuleGraphRepositoryFixtures(
			verificationDatabase,
			graphIDs,
			strategyIDs,
			rollbackConstraint,
		); err != nil {
			t.Errorf("final deferred rule graph fixture cleanup: %v", err)
		}
	}()

	seedRuleGraphStrategies(t, ctx, verificationDatabase, strategyIDs)
	assertRuleGraphReadSnapshotOptionsReachMySQL(t, ctx, graphDatabase, verificationDatabase, baseID+5)

	roundTrip := mustIntegrationStrategyRoutingGraph(
		t,
		domain.StrategyRoutingGraphID(baseID),
		"membership-route-v1",
		10,
		20,
		30,
		domain.StrategyID(strategyIDs[0]),
		domain.StrategyID(strategyIDs[1]),
	)
	if err := repository.Create(ctx, roundTrip); err != nil {
		t.Fatalf("Create(round trip): %v", err)
	}
	loaded, err := repository.FindByIdentity(ctx, roundTrip.Identity())
	if err != nil {
		t.Fatalf("FindByIdentity(round trip): %v", err)
	}
	assertStrategyRoutingGraphEqual(t, loaded, roundTrip)

	secondRevision := mustIntegrationStrategyRoutingGraph(
		t,
		domain.StrategyRoutingGraphID(baseID),
		"membership-route-v2",
		10,
		20,
		30,
		domain.StrategyID(strategyIDs[1]),
		domain.StrategyID(strategyIDs[0]),
	)
	if err := repository.Create(ctx, secondRevision); err != nil {
		t.Fatalf("Create(same graph new revision): %v", err)
	}
	loaded, err = repository.FindByIdentity(ctx, secondRevision.Identity())
	if err != nil {
		t.Fatalf("FindByIdentity(second revision): %v", err)
	}
	assertStrategyRoutingGraphEqual(t, loaded, secondRevision)
	assertRuleGraphRowCounts(t, verificationDatabase, baseID, 2, 6, 4)

	if err := repository.Create(ctx, roundTrip); !errors.Is(err, application.ErrStrategyRoutingGraphAlreadyExists) {
		t.Fatalf("Create(duplicate revision) error = %v, want graph already exists", err)
	}
	assertRuleGraphRowCounts(t, verificationDatabase, baseID, 2, 6, 4)

	maximum := mustIntegrationStrategyRoutingGraph(
		t,
		domain.StrategyRoutingGraphID(math.MaxUint64),
		"max-uint64-v1",
		domain.StrategyRoutingNodeID(math.MaxUint64-2),
		domain.StrategyRoutingNodeID(math.MaxUint64-1),
		domain.StrategyRoutingNodeID(math.MaxUint64),
		domain.StrategyID(math.MaxUint64),
		domain.StrategyID(strategyIDs[0]),
	)
	if err := repository.Create(ctx, maximum); err != nil {
		t.Fatalf("Create(max uint64): %v", err)
	}
	loaded, err = repository.FindByIdentity(ctx, maximum.Identity())
	if err != nil {
		t.Fatalf("FindByIdentity(max uint64): %v", err)
	}
	assertStrategyRoutingGraphEqual(t, loaded, maximum)

	concurrent := mustIntegrationStrategyRoutingGraph(
		t,
		domain.StrategyRoutingGraphID(baseID+1),
		"concurrent-v1",
		10,
		20,
		30,
		domain.StrategyID(strategyIDs[0]),
		domain.StrategyID(strategyIDs[1]),
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
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, application.ErrStrategyRoutingGraphAlreadyExists):
			conflicts++
		default:
			t.Fatalf("concurrent Create() unexpected error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent Create() = %d success/%d conflict, want 1/1", successes, conflicts)
	}
	assertRuleGraphRowCounts(t, verificationDatabase, baseID+1, 1, 3, 2)

	installRuleGraphRollbackProbe(t, ctx, verificationDatabase, rollbackConstraint, baseID+2, 30)
	rollbackGraph := mustIntegrationStrategyRoutingGraph(
		t,
		domain.StrategyRoutingGraphID(baseID+2),
		"rollback-v1",
		10,
		20,
		30,
		domain.StrategyID(strategyIDs[0]),
		domain.StrategyID(strategyIDs[1]),
	)
	if err := repository.Create(ctx, rollbackGraph); !errors.Is(err, application.ErrRepositoryFailure) {
		t.Fatalf("Create(child constraint failure) error = %v, want repository failure", err)
	}
	dropRuleGraphRollbackProbe(t, ctx, verificationDatabase, rollbackConstraint)
	assertRuleGraphRowCounts(t, verificationDatabase, baseID+2, 0, 0, 0)

	missingIdentity := mustStrategyRoutingGraphIdentity(t, domain.StrategyRoutingGraphID(baseID+100), "missing-v1")
	missing, err := repository.FindByIdentity(ctx, missingIdentity)
	if !errors.Is(err, application.ErrStrategyRoutingGraphNotFound) {
		t.Fatalf("FindByIdentity(missing) error = %v, want graph not found", err)
	}
	assertZeroStrategyRoutingGraph(t, missing)

	insertDanglingLogicalRootFixture(
		t,
		ctx,
		verificationDatabase,
		baseID+3,
		strategyIDs[0],
		strategyIDs[1],
	)
	danglingIdentity := mustStrategyRoutingGraphIdentity(t, domain.StrategyRoutingGraphID(baseID+3), "dangling-root-v1")
	dangling, err := repository.FindByIdentity(ctx, danglingIdentity)
	if !errors.Is(err, application.ErrStoredStrategyRoutingGraphInvalid) {
		t.Fatalf("FindByIdentity(dangling root) error = %v, want stored graph invalid", err)
	}
	assertZeroStrategyRoutingGraph(t, dangling)

	canceledGraph := mustIntegrationStrategyRoutingGraph(
		t,
		domain.StrategyRoutingGraphID(baseID+4),
		"canceled-v1",
		10,
		20,
		30,
		domain.StrategyID(strategyIDs[0]),
		domain.StrategyID(strategyIDs[1]),
	)
	canceledContext, cancelImmediately := context.WithCancel(context.Background())
	cancelImmediately()
	if err := repository.Create(canceledContext, canceledGraph); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create(canceled) error = %v, want context canceled", err)
	}
	if _, err := repository.FindByIdentity(canceledContext, roundTrip.Identity()); !errors.Is(err, context.Canceled) {
		t.Fatalf("FindByIdentity(canceled) error = %v, want context canceled", err)
	}
	assertRuleGraphRowCounts(t, verificationDatabase, baseID+4, 0, 0, 0)

	assertRuleGraphRepositoryLookupPlans(t, ctx, verificationDatabase, roundTrip.Identity())
	if err := graphDatabase.PingContext(ctx); err != nil {
		t.Fatalf("rule graph repository pool unusable after operations: %v", err)
	}

	if err := cleanupRuleGraphRepositoryFixtures(
		verificationDatabase,
		graphIDs,
		strategyIDs,
		rollbackConstraint,
	); err != nil {
		t.Fatalf("final rule graph fixture cleanup: %v", err)
	}
	cleaned = true
}

func applyAndVerifyRuleGraphSchemaV5(
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
		t.Fatalf("open migration database for rule graph repository integration: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		_ = database.Close()
		t.Fatalf("construct migration runner for rule graph repository integration: %v", err)
	}
	if _, err := runner.Up(ctx); err != nil {
		_ = runner.Close()
		t.Fatalf("apply migrations for rule graph repository integration: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read rule graph migration status: %v", err)
	}
	if status.State != dbmigration.StatusClean || status.Version != 11 || status.Latest != 11 {
		_ = runner.Close()
		t.Fatalf("migration status = %+v, want clean exact v5", status)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close rule graph migration runner: %v", err)
	}
}

func assertExactRuleGraphRepositoryGrants(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	databaseName string,
) {
	t.Helper()

	var currentAccount string
	if err := database.GetContext(ctx, &currentAccount, "SELECT CURRENT_USER()"); err != nil {
		t.Fatalf("read rule graph account identity: %v", err)
	}
	separator := strings.LastIndexByte(currentAccount, '@')
	if separator <= 0 || separator == len(currentAccount)-1 {
		t.Fatalf("rule graph CURRENT_USER() = %q, want user@host", currentAccount)
	}
	quoteIdentifier := func(value string) string {
		return "`" + strings.ReplaceAll(value, "`", "``") + "`"
	}
	account := quoteIdentifier(currentAccount[:separator]) + "@" + quoteIdentifier(currentAccount[separator+1:])
	quotedDatabase := quoteIdentifier(databaseName)
	expected := []string{
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`lottery_strategy_routing_edge` TO " + account,
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`lottery_strategy_routing_graph` TO " + account,
		"GRANT SELECT, INSERT ON " + quotedDatabase + ".`lottery_strategy_routing_node` TO " + account,
		"GRANT USAGE ON *.* TO " + account,
	}
	var actual []string
	if err := database.SelectContext(ctx, &actual, "SHOW GRANTS FOR CURRENT_USER"); err != nil {
		t.Fatalf("read rule graph repository grants: %v", err)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("rule graph grants = %q, want exact allowlist %q", actual, expected)
	}
	var mandatoryRoles string
	if err := database.GetContext(ctx, &mandatoryRoles, "SELECT @@GLOBAL.mandatory_roles"); err != nil {
		t.Fatalf("read MySQL mandatory roles: %v", err)
	}
	if mandatoryRoles != "" {
		t.Fatalf("mandatory roles expand rule graph repository privileges: %q", mandatoryRoles)
	}
}

func assertRuleGraphRepositoryForbiddenSurfaces(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
) {
	t.Helper()

	readProbes := []string{
		"SELECT COUNT(*) FROM lottery_strategy WHERE 1 = 0",
		"SELECT COUNT(*) FROM lottery_strategy_award WHERE 1 = 0",
		"SELECT version FROM schema_migrations LIMIT 1",
	}
	for _, query := range readProbes {
		var rows int
		err := database.GetContext(ctx, &rows, query)
		expectRuleGraphMySQLErrorNumber(t, err, 1142)
	}
	writeProbes := []string{
		"UPDATE lottery_strategy_routing_graph SET root_node_id = root_node_id WHERE graph_id = 0",
		"UPDATE lottery_strategy_routing_node SET node_id = node_id WHERE graph_id = 0",
		"UPDATE lottery_strategy_routing_edge SET to_node_id = to_node_id WHERE graph_id = 0",
		"DELETE FROM lottery_strategy_routing_graph WHERE graph_id = 0",
		"DELETE FROM lottery_strategy_routing_node WHERE graph_id = 0",
		"DELETE FROM lottery_strategy_routing_edge WHERE graph_id = 0",
		"INSERT INTO lottery_strategy (strategy_id, name) VALUES (1, 'forbidden')",
		"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (1, 1, 'forbidden', 1, 'reward')",
		"INSERT INTO schema_migrations (version, dirty) VALUES (2147483647, 0)",
		"UPDATE schema_migrations SET dirty = dirty",
	}
	for _, statement := range writeProbes {
		_, err := database.ExecContext(ctx, statement)
		expectRuleGraphMySQLErrorNumber(t, err, 1142)
	}
}

func seedRuleGraphStrategies(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	strategyIDs []uint64,
) {
	t.Helper()

	if len(strategyIDs) != 3 {
		t.Fatalf("strategy fixture IDs = %d, want 3", len(strategyIDs))
	}
	_, err := database.ExecContext(ctx, `
		INSERT INTO lottery_strategy (strategy_id, name)
		VALUES
			(?, 'rule graph baseline'),
			(?, 'rule graph premium'),
			(?, 'rule graph max target')`,
		strategyIDs[0],
		strategyIDs[1],
		strategyIDs[2],
	)
	if err != nil {
		t.Fatalf("seed rule graph target Strategies: %v", err)
	}
}

func mustIntegrationStrategyRoutingGraph(
	t *testing.T,
	graphID domain.StrategyRoutingGraphID,
	revision string,
	rootNodeID domain.StrategyRoutingNodeID,
	baselineNodeID domain.StrategyRoutingNodeID,
	premiumNodeID domain.StrategyRoutingNodeID,
	baselineStrategyID domain.StrategyID,
	premiumStrategyID domain.StrategyID,
) domain.StrategyRoutingGraph {
	t.Helper()

	root, err := domain.NewStrategyRoutingDecisionNode(rootNodeID, domain.MembershipStrategyRoutingRuleCode)
	if err != nil {
		t.Fatalf("construct integration root: %v", err)
	}
	baseline, err := domain.NewStrategyRoutingTargetNode(baselineNodeID, baselineStrategyID)
	if err != nil {
		t.Fatalf("construct integration baseline target: %v", err)
	}
	premium, err := domain.NewStrategyRoutingTargetNode(premiumNodeID, premiumStrategyID)
	if err != nil {
		t.Fatalf("construct integration premium target: %v", err)
	}
	baselineEdge, err := domain.NewStrategyRoutingEdge(
		rootNodeID,
		baselineNodeID,
		domain.MembershipRoutingBranchBaselineDefault,
	)
	if err != nil {
		t.Fatalf("construct integration baseline edge: %v", err)
	}
	premiumEdge, err := domain.NewStrategyRoutingEdge(
		rootNodeID,
		premiumNodeID,
		domain.MembershipRoutingBranchPremiumOverride,
	)
	if err != nil {
		t.Fatalf("construct integration premium edge: %v", err)
	}
	graph, err := domain.NewStrategyRoutingGraph(
		graphID,
		revision,
		rootNodeID,
		[]domain.StrategyRoutingNode{premium, root, baseline},
		[]domain.StrategyRoutingEdge{premiumEdge, baselineEdge},
	)
	if err != nil {
		t.Fatalf("construct integration graph: %v", err)
	}
	return graph
}

func installRuleGraphRollbackProbe(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	constraintName string,
	graphID uint64,
	nodeID uint64,
) {
	t.Helper()

	if err := removeRuleGraphRollbackProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("remove stale rule graph rollback probe: %v", err)
	}
	statement := fmt.Sprintf(
		"ALTER TABLE lottery_strategy_routing_node ADD CONSTRAINT `%s` CHECK (graph_id <> %d OR node_id <> %d)",
		constraintName,
		graphID,
		nodeID,
	)
	if _, err := database.ExecContext(ctx, statement); err != nil {
		t.Fatalf("install rule graph rollback probe: %v", err)
	}
}

func dropRuleGraphRollbackProbe(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	constraintName string,
) {
	t.Helper()

	if err := removeRuleGraphRollbackProbe(ctx, database, constraintName); err != nil {
		t.Fatalf("drop rule graph rollback probe: %v", err)
	}
}

func removeRuleGraphRollbackProbe(
	ctx context.Context,
	database *sql.DB,
	constraintName string,
) error {
	if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(constraintName) {
		return fmt.Errorf("invalid rollback constraint name %q", constraintName)
	}
	var exists int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'lottery_strategy_routing_node'
		  AND constraint_name = ?`, constraintName).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := database.ExecContext(
		ctx,
		"ALTER TABLE lottery_strategy_routing_node DROP CHECK `"+constraintName+"`",
	)
	return err
}

func insertDanglingLogicalRootFixture(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	graphID uint64,
	baselineStrategyID uint64,
	premiumStrategyID uint64,
) {
	t.Helper()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin dangling root fixture: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		VALUES (?, 'dangling-root-v1', 1, 999)`, graphID); err != nil {
		t.Fatalf("insert dangling root header: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy_routing_node
			(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
		VALUES
			(?, 'dangling-root-v1', 10, 'decision', 'lottery.membership_tier.route_strategy', NULL),
			(?, 'dangling-root-v1', 20, 'strategy_target', NULL, ?),
			(?, 'dangling-root-v1', 30, 'strategy_target', NULL, ?)`,
		graphID,
		graphID,
		baselineStrategyID,
		graphID,
		premiumStrategyID,
	); err != nil {
		t.Fatalf("insert dangling root nodes: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy_routing_edge
			(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
		VALUES
			(?, 'dangling-root-v1', 10, 'baseline_default', 20, 1),
			(?, 'dangling-root-v1', 10, 'premium_override', 30, 0)`,
		graphID,
		graphID,
	); err != nil {
		t.Fatalf("insert dangling root edges: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit dangling root fixture: %v", err)
	}
}

func assertRuleGraphReadSnapshotOptionsReachMySQL(
	t *testing.T,
	ctx context.Context,
	database *sqlx.DB,
	verificationDatabase *sql.DB,
	probeGraphID uint64,
) {
	t.Helper()

	tx, err := database.BeginTxx(ctx, readSnapshotOptions())
	if err != nil {
		t.Fatalf("begin rule graph read-only snapshot probe: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	var isolation string
	if err := tx.GetContext(ctx, &isolation, "SELECT @@transaction_isolation"); err != nil {
		t.Fatalf("read rule graph snapshot isolation: %v", err)
	}
	if isolation != "REPEATABLE-READ" {
		t.Fatalf("rule graph snapshot isolation = %q, want REPEATABLE-READ", isolation)
	}
	_, writeErr := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		VALUES (?, 'read-only-probe-v1', 1, 1)`, probeGraphID)
	if !errors.Is(writeErr, driver.ErrBadConn) {
		t.Fatalf("read-only graph snapshot write error = %v, want driver bad connection mapped from MySQL 1792", writeErr)
	}
	_ = tx.Rollback()
	assertRuleGraphRowCounts(t, verificationDatabase, probeGraphID, 0, 0, 0)
}

func assertRuleGraphRepositoryLookupPlans(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	identity domain.StrategyRoutingGraphIdentity,
) {
	t.Helper()

	arguments := []any{uint64(identity.ID()), string(identity.Revision())}
	headerPlan := explainRuleGraphLookup(t, ctx, database, selectStrategyRoutingGraphSQL, arguments...)
	if headerPlan["type"] != "const" || headerPlan["key"] != "PRIMARY" {
		t.Fatalf("graph header EXPLAIN = %#v, want const PRIMARY lookup", headerPlan)
	}
	for name, query := range map[string]string{
		"nodes": selectStrategyRoutingNodesSQL,
		"edges": selectStrategyRoutingEdgesSQL,
	} {
		plan := explainRuleGraphLookup(t, ctx, database, query, arguments...)
		if plan["key"] != "PRIMARY" {
			t.Fatalf("graph %s EXPLAIN = %#v, want PRIMARY scoped lookup", name, plan)
		}
		if strings.Contains(strings.ToLower(plan["extra"]), "using filesort") {
			t.Fatalf("graph %s EXPLAIN = %#v, unexpected filesort", name, plan)
		}
	}
}

func explainRuleGraphLookup(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	query string,
	arguments ...any,
) map[string]string {
	t.Helper()

	rows, err := database.QueryContext(ctx, "EXPLAIN "+query, arguments...)
	if err != nil {
		t.Fatalf("explain rule graph lookup: %v", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatalf("read rule graph EXPLAIN columns: %v", err)
	}
	values := make([]sql.NullString, len(columns))
	destinations := make([]any, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	if !rows.Next() {
		t.Fatal("rule graph EXPLAIN returned no row")
	}
	if err := rows.Scan(destinations...); err != nil {
		t.Fatalf("scan rule graph EXPLAIN: %v", err)
	}
	plan := make(map[string]string, len(columns))
	for index, column := range columns {
		if values[index].Valid {
			plan[strings.ToLower(column)] = values[index].String
		}
	}
	if rows.Next() {
		t.Fatal("rule graph EXPLAIN returned an unexpected additional row")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rule graph EXPLAIN: %v", err)
	}
	return plan
}

func assertRuleGraphRowCounts(
	t *testing.T,
	database *sql.DB,
	graphID uint64,
	wantGraphs int,
	wantNodes int,
	wantEdges int,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queries := []struct {
		query string
		want  int
	}{
		{query: "SELECT COUNT(*) FROM lottery_strategy_routing_graph WHERE graph_id = ?", want: wantGraphs},
		{query: "SELECT COUNT(*) FROM lottery_strategy_routing_node WHERE graph_id = ?", want: wantNodes},
		{query: "SELECT COUNT(*) FROM lottery_strategy_routing_edge WHERE graph_id = ?", want: wantEdges},
	}
	for _, check := range queries {
		var rows int
		if err := database.QueryRowContext(ctx, check.query, graphID).Scan(&rows); err != nil {
			t.Fatalf("count rule graph rows: %v", err)
		}
		if rows != check.want {
			t.Fatalf("rule graph %d query %q = %d rows, want %d", graphID, check.query, rows, check.want)
		}
	}
}

func cleanupRuleGraphRepositoryFixtures(
	database *sql.DB,
	graphIDs []uint64,
	strategyIDs []uint64,
	constraintName string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := removeRuleGraphRollbackProbe(ctx, database, constraintName); err != nil {
		return fmt.Errorf("drop rule graph rollback probe: %w", err)
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rule graph cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, graphID := range graphIDs {
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy_routing_edge WHERE graph_id = ?", graphID); err != nil {
			return fmt.Errorf("delete graph %d edges: %w", graphID, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy_routing_node WHERE graph_id = ?", graphID); err != nil {
			return fmt.Errorf("delete graph %d nodes: %w", graphID, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy_routing_graph WHERE graph_id = ?", graphID); err != nil {
			return fmt.Errorf("delete graph %d headers: %w", graphID, err)
		}
	}
	for _, strategyID := range strategyIDs {
		if _, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy WHERE strategy_id = ?", strategyID); err != nil {
			return fmt.Errorf("delete target Strategy %d: %w", strategyID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rule graph cleanup: %w", err)
	}

	for _, graphID := range graphIDs {
		for _, table := range []string{
			"lottery_strategy_routing_graph",
			"lottery_strategy_routing_node",
			"lottery_strategy_routing_edge",
		} {
			var rows int
			query := "SELECT COUNT(*) FROM " + table + " WHERE graph_id = ?"
			if err := database.QueryRowContext(ctx, query, graphID).Scan(&rows); err != nil {
				return fmt.Errorf("verify graph %d cleanup in %s: %w", graphID, table, err)
			}
			if rows != 0 {
				return fmt.Errorf("graph %d cleanup left %d rows in %s", graphID, rows, table)
			}
		}
	}
	for _, strategyID := range strategyIDs {
		var rows int
		if err := database.QueryRowContext(
			ctx,
			"SELECT COUNT(*) FROM lottery_strategy WHERE strategy_id = ?",
			strategyID,
		).Scan(&rows); err != nil {
			return fmt.Errorf("verify target Strategy %d cleanup: %w", strategyID, err)
		}
		if rows != 0 {
			return fmt.Errorf("target Strategy %d cleanup left %d rows", strategyID, rows)
		}
	}
	var constraints int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'lottery_strategy_routing_node'
		  AND constraint_name = ?`, constraintName).Scan(&constraints); err != nil {
		return fmt.Errorf("verify rule graph constraint cleanup: %w", err)
	}
	if constraints != 0 {
		return fmt.Errorf("rule graph rollback constraint %q remains", constraintName)
	}
	return nil
}

func expectRuleGraphMySQLErrorNumber(t *testing.T, err error, number uint16) {
	t.Helper()

	if err == nil {
		t.Fatalf("operation unexpectedly succeeded, want MySQL error %d", number)
	}
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) {
		t.Fatalf("error = %v, want MySQL error %d", err, number)
	}
	if mysqlError.Number != number {
		t.Fatalf("MySQL error number = %d, want %d", mysqlError.Number, number)
	}
}
