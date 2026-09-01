package migrations_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	projectmigrations "github.com/Atingaii/GrowthOS-Go/migrations"
)

const identitySchemaAuthorization = "lesson-32-isolated-schema"

var identityVersionElevenMigrations = []string{
	"000001_create_lottery_strategy.up.sql",
	"000002_create_lottery_strategy_award.up.sql",
	"000003_create_lottery_strategy_routing_graph.up.sql",
	"000004_create_lottery_strategy_routing_node.up.sql",
	"000005_create_lottery_strategy_routing_edge.up.sql",
	"000006_create_lottery_strategy_snapshot.up.sql",
	"000007_create_lottery_strategy_snapshot_award.up.sql",
	"000008_create_marketing_activity.up.sql",
	"000009_create_marketing_activity_publication.up.sql",
	"000010_create_marketing_activity_publication_strategy.up.sql",
	"000011_add_marketing_activity_active_publication_fk.up.sql",
}

var identityOldBusinessTables = []string{
	"lottery_strategy",
	"lottery_strategy_award",
	"lottery_strategy_routing_graph",
	"lottery_strategy_routing_node",
	"lottery_strategy_routing_edge",
	"lottery_strategy_snapshot",
	"lottery_strategy_snapshot_award",
	"marketing_activity",
	"marketing_activity_publication",
	"marketing_activity_publication_strategy",
}

var identityDisposableTables = []string{
	"identity_authentication_throttle",
	"identity_session",
	"identity_workforce_account",
	"marketing_activity_publication_strategy",
	"marketing_activity_publication",
	"marketing_activity",
	"lottery_strategy_snapshot_award",
	"lottery_strategy_snapshot",
	"lottery_strategy_routing_edge",
	"lottery_strategy_routing_node",
	"lottery_strategy_routing_graph",
	"lottery_strategy_award",
	"lottery_strategy",
	"schema_migrations",
}

func TestIdentitySchemaMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_IDENTITY_SCHEMA_CHANGES") != identitySchemaAuthorization {
		t.Skip("Identity schema integration requires exact disposable-schema authorization")
	}
	connection := identitySchemaConnection(t, "GROWTHOS_TEST_MYSQL_IDENTITY_MIGRATION")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	assertIdentityDatabaseStartsFresh(t, ctx, connection)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		cleanupIdentityDisposableSchema(t, cleanupCtx, connection)
	})

	// Phase one proves the real embedded set can create a fresh v14 schema and
	// that a second forward run is an exact no-op.
	fresh := runIdentityMigrations(t, ctx, connection, projectmigrations.Files, 14)
	if fresh.State != dbmigration.ResultApplied || fresh.Version != 14 {
		t.Fatalf("fresh migration result = %+v, want applied exact v14", fresh)
	}
	repeat := runIdentityMigrations(t, ctx, connection, projectmigrations.Files, 14)
	if repeat.State != dbmigration.ResultNoChange || repeat.Version != 14 {
		t.Fatalf("repeat migration result = %+v, want no_change exact v14", repeat)
	}

	// The opt-in schema was empty when connected and contains only the exact
	// migration-owned allowlist. Reset it so the second phase proves 11->14.
	cleanupIdentityDisposableSchema(t, ctx, connection)
	assertIdentityDatabaseStartsFresh(t, ctx, connection)

	baselineFS := identityVersionElevenFS(t)
	baseline := runIdentityMigrations(t, ctx, connection, baselineFS, 11)
	if baseline.State != dbmigration.ResultApplied || baseline.Version != 11 {
		t.Fatalf("v11 baseline result = %+v, want applied exact v11", baseline)
	}
	seedIdentityVersionElevenSchema(t, ctx, connection)
	before := captureIdentityOldSchemaFingerprint(t, ctx, connection)

	upgrade := runIdentityMigrations(t, ctx, connection, projectmigrations.Files, 14)
	if upgrade.State != dbmigration.ResultApplied || upgrade.Version != 14 {
		t.Fatalf("11->14 migration result = %+v, want applied exact v14", upgrade)
	}
	after := captureIdentityOldSchemaFingerprint(t, ctx, connection)
	if after != before {
		t.Fatalf("pre-Identity schema/data changed across 11->14: before=%+v after=%+v", before, after)
	}

	database := openIdentityMigrationDatabase(t, ctx, connection)
	defer database.Close()
	assertIdentityStrictMode(t, ctx, database)
	assertIdentityTables(t, ctx, database)
	assertIdentityColumns(t, ctx, database)
	assertIdentityIndexes(t, ctx, database)
	assertIdentityForeignKeys(t, ctx, database)
	assertIdentityChecks(t, ctx, database)
	assertIdentityBinarySemantics(t, ctx, database)
	assertIdentityRoundTripAndRejectedRows(t, ctx, database)
	assertIdentityDirtyFailsClosed(t, ctx, database, connection)
}

func identitySchemaConnection(t *testing.T, prefix string) mysqlstore.ConnectionConfig {
	t.Helper()
	requiredSuffixes := []string{"_ADDRESS", "_DATABASE", "_USER", "_PASSWORD"}
	values := make(map[string]string, len(requiredSuffixes))
	for _, suffix := range requiredSuffixes {
		key := prefix + suffix
		value, ok := os.LookupEnv(key)
		if !ok {
			t.Skipf("Identity schema integration requires explicit %s", key)
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
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   20 * time.Second,
	}
}

func openIdentityMigrationDatabase(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) *sql.DB {
	t.Helper()
	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 25 * time.Second,
		LockTimeout:      35 * time.Second,
	})
	if err != nil {
		t.Fatalf("open Identity migration database: %v", err)
	}
	return database
}

func runIdentityMigrations(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
	files fs.FS,
	expectedLatest uint,
) dbmigration.Result {
	t.Helper()
	database := openIdentityMigrationDatabase(t, ctx, connection)
	runner, err := dbmigration.New(ctx, files, database, dbmigration.Config{
		LockTimeout:        35 * time.Second,
		NetworkReadTimeout: 30 * time.Second,
		StatementTimeout:   25 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct Identity migration runner: %v", err)
	}
	result, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("apply Identity migrations: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read Identity migration status: %v", err)
	}
	wantStatus := dbmigration.Status{
		State:   dbmigration.StatusClean,
		Version: expectedLatest,
		Latest:  expectedLatest,
	}
	if status != wantStatus {
		_ = runner.Close()
		t.Fatalf("Identity migration status = %+v, want %+v", status, wantStatus)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close Identity migration runner: %v", err)
	}
	return result
}

func identityVersionElevenFS(t *testing.T) fs.FS {
	t.Helper()
	baseline := fstest.MapFS{}
	for _, name := range identityVersionElevenMigrations {
		path := "sql/" + name
		contents, err := fs.ReadFile(projectmigrations.Files, path)
		if err != nil {
			t.Fatalf("read frozen v11 migration %s: %v", path, err)
		}
		baseline[path] = &fstest.MapFile{Data: contents, Mode: 0o444}
	}
	return baseline
}

func assertIdentityDatabaseStartsFresh(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	database := openIdentityMigrationDatabase(t, ctx, connection)
	defer database.Close()
	var tables int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()`).Scan(&tables); err != nil {
		t.Fatalf("inspect disposable Identity database: %v", err)
	}
	if tables != 0 {
		t.Fatalf("Identity schema acceptance requires an empty disposable database; found %d tables", tables)
	}
}

func cleanupIdentityDisposableSchema(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	database := openIdentityMigrationDatabase(t, ctx, connection)
	defer database.Close()
	actual := queryIdentityStrings(t, ctx, database, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		ORDER BY table_name`)
	allowed := make(map[string]struct{}, len(identityDisposableTables))
	for _, table := range identityDisposableTables {
		allowed[table] = struct{}{}
	}
	for _, table := range actual {
		if _, ok := allowed[table]; !ok {
			t.Fatalf("refuse cleanup of unexpected table %q in disposable Identity schema", table)
		}
	}

	connectionHandle, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("pin Identity cleanup connection: %v", err)
	}
	defer connectionHandle.Close()
	if _, err := connectionHandle.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("disable Identity cleanup foreign-key checks: %v", err)
	}
	checksDisabled := true
	defer func() {
		if !checksDisabled {
			return
		}
		if _, err := connectionHandle.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
			t.Errorf("restore Identity cleanup foreign-key checks: %v", err)
		}
	}()
	for _, table := range identityDisposableTables {
		if _, err := connectionHandle.ExecContext(ctx, "DROP TABLE IF EXISTS `"+table+"`"); err != nil {
			t.Fatalf("drop disposable Identity table %s: %v", table, err)
		}
	}
	if _, err := connectionHandle.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
		t.Fatalf("restore Identity cleanup foreign-key checks: %v", err)
	}
	checksDisabled = false
	var remaining int
	if err := connectionHandle.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()`).Scan(&remaining); err != nil {
		t.Fatalf("verify disposable Identity cleanup: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("disposable Identity cleanup left %d tables", remaining)
	}
}

type identityOldSchemaFingerprint struct {
	PhysicalTables   int
	Columns          int
	Constraints      int
	IndexColumns     int
	ShowCreateSHA256 string
	DataRows         int
	DataSHA256       string
}

func captureIdentityOldSchemaFingerprint(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) identityOldSchemaFingerprint {
	t.Helper()
	database := openIdentityMigrationDatabase(t, ctx, connection)
	defer database.Close()
	tableList := "'" + strings.Join(identityOldBusinessTables, "','") + "'"
	queries := []string{
		"SELECT COUNT(*) + (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'schema_migrations') FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
	}
	counts := make([]int, len(queries))
	for index, query := range queries {
		if err := database.QueryRowContext(ctx, query).Scan(&counts[index]); err != nil {
			t.Fatalf("capture pre-Identity schema count %d: %v", index, err)
		}
	}
	if counts[0] != 11 {
		t.Fatalf("pre-Identity physical tables = %d, want ten business tables plus schema_migrations", counts[0])
	}

	showCreateTables := append([]string(nil), identityOldBusinessTables...)
	showCreateTables = append(showCreateTables, "schema_migrations")
	definitions := make([]string, 0, len(showCreateTables))
	for _, table := range showCreateTables {
		var returnedName, definition string
		if err := database.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&returnedName, &definition); err != nil {
			t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
		}
		if returnedName != table {
			t.Fatalf("SHOW CREATE TABLE returned %q, want %q", returnedName, table)
		}
		definitions = append(definitions, table+"\n"+definition)
	}
	sort.Strings(definitions)
	schemaChecksum := sha256.Sum256([]byte(strings.Join(definitions, "\n--next-table--\n")))
	dataRows, dataChecksum := captureIdentityOldDataFingerprint(t, ctx, database)
	return identityOldSchemaFingerprint{
		PhysicalTables:   counts[0],
		Columns:          counts[1],
		Constraints:      counts[2],
		IndexColumns:     counts[3],
		ShowCreateSHA256: hex.EncodeToString(schemaChecksum[:]),
		DataRows:         dataRows,
		DataSHA256:       dataChecksum,
	}
}

func seedIdentityVersionElevenSchema(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	database := openIdentityMigrationDatabase(t, ctx, connection)
	defer database.Close()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin v11 Identity baseline seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	statements := []struct {
		query string
		args  []any
	}{
		{
			query: "INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ?), (?, ?)",
			args:  []any{uint64(101), "v11 baseline Strategy", uint64(102), "v11 premium Strategy"},
		},
		{
			query: `INSERT INTO lottery_strategy_award
				(strategy_id, award_id, name, weight, outcome)
				VALUES (?, ?, ?, ?, ?)`,
			args: []any{uint64(101), uint64(1), "v11 reward", uint64(7), "reward"},
		},
		{
			query: `INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
				VALUES (?, ?, ?, ?)`,
			args: []any{uint64(201), "baseline:r1", uint16(1), uint64(10)},
		},
		{
			query: `INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
				VALUES
					(?, ?, ?, 'decision', 'lottery.membership_tier.route_strategy', NULL),
					(?, ?, ?, 'strategy_target', NULL, ?)`,
			args: []any{
				uint64(201), "baseline:r1", uint64(10),
				uint64(201), "baseline:r1", uint64(20), uint64(101),
			},
		},
		{
			query: `INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
				VALUES (?, ?, ?, 'baseline_default', ?, 1)`,
			args: []any{uint64(201), "baseline:r1", uint64(10), uint64(20)},
		},
		{
			query: `INSERT INTO lottery_strategy_snapshot
				(strategy_id, revision, schema_version, name)
				VALUES (?, ?, ?, ?)`,
			args: []any{uint64(101), "baseline:s1", uint16(1), "v11 baseline snapshot"},
		},
		{
			query: `INSERT INTO lottery_strategy_snapshot_award
				(strategy_id, revision, award_id, name, weight, outcome)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{uint64(101), "baseline:s1", uint64(1), "v11 snapshot reward", uint64(7), "reward"},
		},
		{
			query: `INSERT INTO marketing_activity
				(activity_id, name, lifecycle_state, state_version)
				VALUES (?, ?, 'draft', 0)`,
			args: []any{uint64(301), "v11 baseline activity"},
		},
		{
			query: `INSERT INTO marketing_activity_publication
				(activity_id, activity_version, schema_version, publication_kind,
				 rollback_of_version, graph_id, graph_revision, starts_at, ends_at,
				 published_at, approval_reference)
				VALUES (?, ?, 1, 'release', NULL, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				uint64(301), uint64(1), uint64(201), "baseline:r1",
				time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
				time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
				"approval:v11-baseline",
			},
		},
		{
			query: `INSERT INTO marketing_activity_publication_strategy
				(activity_id, activity_version, strategy_id, strategy_revision)
				VALUES (?, ?, ?, ?)`,
			args: []any{uint64(301), uint64(1), uint64(101), "baseline:s1"},
		},
	}
	for index, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("execute v11 Identity baseline seed %d: %v", index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v11 Identity baseline seed: %v", err)
	}
}

func captureIdentityOldDataFingerprint(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
) (int, string) {
	t.Helper()
	queries := []struct {
		table string
		query string
	}{
		{"lottery_strategy", "SELECT CONCAT(strategy_id, '|', HEX(name), '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f'), '|', DATE_FORMAT(updated_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy ORDER BY strategy_id"},
		{"lottery_strategy_award", "SELECT CONCAT(strategy_id, '|', award_id, '|', HEX(name), '|', weight, '|', outcome, '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f'), '|', DATE_FORMAT(updated_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy_award ORDER BY strategy_id, award_id"},
		{"lottery_strategy_routing_graph", "SELECT CONCAT(graph_id, '|', revision, '|', schema_version, '|', root_node_id, '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy_routing_graph ORDER BY graph_id, revision"},
		{"lottery_strategy_routing_node", "SELECT CONCAT(graph_id, '|', revision, '|', node_id, '|', node_kind, '|', COALESCE(rule_code, 'NULL'), '|', COALESCE(strategy_id, 'NULL'), '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy_routing_node ORDER BY graph_id, revision, node_id"},
		{"lottery_strategy_routing_edge", "SELECT CONCAT(graph_id, '|', revision, '|', from_node_id, '|', branch_code, '|', to_node_id, '|', is_default, '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy_routing_edge ORDER BY graph_id, revision, from_node_id, branch_code"},
		{"lottery_strategy_snapshot", "SELECT CONCAT(strategy_id, '|', revision, '|', schema_version, '|', HEX(name), '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy_snapshot ORDER BY strategy_id, revision"},
		{"lottery_strategy_snapshot_award", "SELECT CONCAT(strategy_id, '|', revision, '|', award_id, '|', HEX(name), '|', weight, '|', outcome, '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM lottery_strategy_snapshot_award ORDER BY strategy_id, revision, award_id"},
		{"marketing_activity", "SELECT CONCAT(activity_id, '|', HEX(name), '|', lifecycle_state, '|', state_version, '|', COALESCE(active_version, 'NULL'), '|', COALESCE(DATE_FORMAT(retired_at, '%Y-%m-%dT%H:%i:%s.%f'), 'NULL'), '|', COALESCE(retirement_reference, 'NULL'), '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f'), '|', DATE_FORMAT(updated_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM marketing_activity ORDER BY activity_id"},
		{"marketing_activity_publication", "SELECT CONCAT(activity_id, '|', activity_version, '|', schema_version, '|', publication_kind, '|', COALESCE(rollback_of_version, 'NULL'), '|', graph_id, '|', graph_revision, '|', DATE_FORMAT(starts_at, '%Y-%m-%dT%H:%i:%s.%f'), '|', DATE_FORMAT(ends_at, '%Y-%m-%dT%H:%i:%s.%f'), '|', DATE_FORMAT(published_at, '%Y-%m-%dT%H:%i:%s.%f'), '|', approval_reference, '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM marketing_activity_publication ORDER BY activity_id, activity_version"},
		{"marketing_activity_publication_strategy", "SELECT CONCAT(activity_id, '|', activity_version, '|', strategy_id, '|', strategy_revision, '|', DATE_FORMAT(created_at, '%Y-%m-%dT%H:%i:%s.%f')) FROM marketing_activity_publication_strategy ORDER BY activity_id, activity_version, strategy_id"},
	}
	rows := make([]string, 0, 12)
	for _, item := range queries {
		values := queryIdentityStrings(t, ctx, database, item.query)
		if len(values) == 0 {
			t.Fatalf("v11 baseline table %s is empty", item.table)
		}
		for _, value := range values {
			rows = append(rows, item.table+"|"+value)
		}
	}
	if len(rows) != 12 {
		t.Fatalf("v11 baseline data rows = %d, want exact 12", len(rows))
	}
	checksum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return len(rows), hex.EncodeToString(checksum[:])
}

func assertIdentityStrictMode(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var mode string
	if err := database.QueryRowContext(ctx, "SELECT @@SESSION.sql_mode").Scan(&mode); err != nil {
		t.Fatalf("read Identity MySQL sql_mode: %v", err)
	}
	if !strings.Contains(mode, "STRICT_TRANS_TABLES") && !strings.Contains(mode, "STRICT_ALL_TABLES") {
		t.Fatalf("Identity schema acceptance requires strict MySQL mode, got %q", mode)
	}
}

func assertIdentityTables(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := queryIdentityStrings(t, ctx, database, `
		SELECT CONCAT(table_name, '=', engine, '/', table_collation)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name LIKE 'identity\_%' ESCAPE '\\'
		ORDER BY table_name`)
	want := []string{
		"identity_authentication_throttle=InnoDB/ascii_bin",
		"identity_session=InnoDB/ascii_bin",
		"identity_workforce_account=InnoDB/ascii_bin",
	}
	assertIdentityStrings(t, "Identity tables", actual, want)
}

func assertIdentityColumns(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := queryIdentityStrings(t, ctx, database, `
		SELECT CONCAT(
			table_name, '#', LPAD(ordinal_position, 2, '0'), ':', column_name, '=',
			column_type, '/', is_nullable, '/',
			COALESCE(character_set_name, '-'), '/', COALESCE(collation_name, '-')
		)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'identity_workforce_account',
			'identity_session',
			'identity_authentication_throttle'
		  )
		ORDER BY table_name, ordinal_position`)
	want := []string{
		"identity_authentication_throttle#01:dimension=varchar(16)/NO/ascii/ascii_bin",
		"identity_authentication_throttle#02:subject_digest=binary(32)/NO/-/-",
		"identity_authentication_throttle#03:window_started_at=datetime(6)/NO/-/-",
		"identity_authentication_throttle#04:window_expires_at=datetime(6)/NO/-/-",
		"identity_authentication_throttle#05:failure_count=int unsigned/NO/-/-",
		"identity_authentication_throttle#06:inflight_count=int unsigned/NO/-/-",
		"identity_authentication_throttle#07:admission_epoch=bigint unsigned/NO/-/-",
		"identity_authentication_throttle#08:inflight_expires_at=datetime(6)/YES/-/-",
		"identity_authentication_throttle#09:blocked_until=datetime(6)/YES/-/-",
		"identity_authentication_throttle#10:updated_at=datetime(6)/NO/-/-",
		"identity_authentication_throttle#11:row_expires_at=datetime(6)/NO/-/-",
		"identity_session#01:session_ref=varchar(128)/NO/ascii/ascii_bin",
		"identity_session#02:issue_operation_ref=varchar(128)/NO/ascii/ascii_bin",
		"identity_session#03:account_id=varchar(128)/NO/ascii/ascii_bin",
		"identity_session#04:token_digest=binary(32)/NO/-/-",
		"identity_session#05:authentication_epoch=bigint unsigned/NO/-/-",
		"identity_session#06:issued_at=datetime(6)/NO/-/-",
		"identity_session#07:last_seen_at=datetime(6)/NO/-/-",
		"identity_session#08:idle_expires_at=datetime(6)/NO/-/-",
		"identity_session#09:absolute_expires_at=datetime(6)/NO/-/-",
		"identity_session#10:revoked_at=datetime(6)/YES/-/-",
		"identity_session#11:revoke_reason=varchar(32)/YES/ascii/ascii_bin",
		"identity_session#12:revoke_operation_ref=varchar(128)/YES/ascii/ascii_bin",
		"identity_session#13:updated_at=datetime(6)/NO/-/-",
		"identity_workforce_account#01:account_id=varchar(128)/NO/ascii/ascii_bin",
		"identity_workforce_account#02:login_name=varchar(64)/NO/ascii/ascii_bin",
		"identity_workforce_account#03:principal_id=varchar(128)/NO/ascii/ascii_bin",
		"identity_workforce_account#04:password_envelope=varchar(256)/NO/ascii/ascii_bin",
		"identity_workforce_account#05:account_status=varchar(16)/NO/ascii/ascii_bin",
		"identity_workforce_account#06:credential_version=bigint unsigned/NO/-/-",
		"identity_workforce_account#07:authentication_epoch=bigint unsigned/NO/-/-",
		"identity_workforce_account#08:created_at=datetime(6)/NO/-/-",
		"identity_workforce_account#09:updated_at=datetime(6)/NO/-/-",
	}
	assertIdentityStrings(t, "Identity columns", actual, want)
}

func assertIdentityIndexes(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := queryIdentityStrings(t, ctx, database, `
		SELECT CONCAT(
			table_name, '/', index_name, '/', non_unique,
			'#', LPAD(seq_in_index, 2, '0'), ':', column_name
		)
		FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'identity_workforce_account',
			'identity_session',
			'identity_authentication_throttle'
		  )`)
	want := []string{
		"identity_workforce_account/PRIMARY/0#01:account_id",
		"identity_workforce_account/uq_identity_workforce_account_login/0#01:login_name",
		"identity_workforce_account/uq_identity_workforce_account_principal/0#01:principal_id",
		"identity_session/PRIMARY/0#01:session_ref",
		"identity_session/uq_identity_session_issue_operation/0#01:issue_operation_ref",
		"identity_session/uq_identity_session_token_digest/0#01:token_digest",
		"identity_session/uq_identity_session_revoke_operation/0#01:revoke_operation_ref",
		"identity_session/idx_identity_session_account_active/1#01:account_id",
		"identity_session/idx_identity_session_account_active/1#02:authentication_epoch",
		"identity_session/idx_identity_session_account_active/1#03:revoked_at",
		"identity_session/idx_identity_session_account_active/1#04:absolute_expires_at",
		"identity_session/idx_identity_session_account_active/1#05:idle_expires_at",
		"identity_session/idx_identity_session_account_active/1#06:last_seen_at",
		"identity_session/idx_identity_session_account_active/1#07:issued_at",
		"identity_session/idx_identity_session_account_active/1#08:session_ref",
		"identity_session/idx_identity_session_absolute_cleanup/1#01:absolute_expires_at",
		"identity_session/idx_identity_session_absolute_cleanup/1#02:session_ref",
		"identity_session/idx_identity_session_revoked_cleanup/1#01:revoked_at",
		"identity_session/idx_identity_session_revoked_cleanup/1#02:session_ref",
		"identity_authentication_throttle/PRIMARY/0#01:dimension",
		"identity_authentication_throttle/PRIMARY/0#02:subject_digest",
		"identity_authentication_throttle/idx_identity_throttle_cleanup/1#01:row_expires_at",
		"identity_authentication_throttle/idx_identity_throttle_cleanup/1#02:dimension",
		"identity_authentication_throttle/idx_identity_throttle_cleanup/1#03:subject_digest",
	}
	assertIdentityStringSet(t, "Identity indexes", actual, want)
}

func assertIdentityForeignKeys(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	columns := queryIdentityStrings(t, ctx, database, `
		SELECT CONCAT(
			constraint_name, ':', table_name, '.', column_name,
			'->', referenced_table_name, '.', referenced_column_name
		)
		FROM information_schema.key_column_usage
		WHERE constraint_schema = DATABASE()
		  AND referenced_table_schema = DATABASE()
		  AND table_name LIKE 'identity\_%' ESCAPE '\\'
		ORDER BY constraint_name, ordinal_position`)
	assertIdentityStrings(t, "Identity FK columns", columns, []string{
		"fk_identity_session_account:identity_session.account_id->identity_workforce_account.account_id",
	})
	rules := queryIdentityStrings(t, ctx, database, `
		SELECT CONCAT(constraint_name, '=', update_rule, '/', delete_rule)
		FROM information_schema.referential_constraints
		WHERE constraint_schema = DATABASE()
		  AND constraint_name = 'fk_identity_session_account'`)
	assertIdentityStrings(t, "Identity FK rules", rules, []string{
		"fk_identity_session_account=RESTRICT/RESTRICT",
	})

	var crossBoundary int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.key_column_usage
		WHERE constraint_schema = DATABASE()
		  AND referenced_table_schema = DATABASE()
		  AND (
			(table_name LIKE 'identity\_%' ESCAPE '\\' AND referenced_table_name NOT LIKE 'identity\_%' ESCAPE '\\')
			OR
			(table_name NOT LIKE 'identity\_%' ESCAPE '\\' AND referenced_table_name LIKE 'identity\_%' ESCAPE '\\')
		  )`).Scan(&crossBoundary); err != nil {
		t.Fatalf("inspect Identity cross-boundary foreign keys: %v", err)
	}
	if crossBoundary != 0 {
		t.Fatalf("Identity cross-boundary foreign keys = %d, want none", crossBoundary)
	}
}

func assertIdentityChecks(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := queryIdentityStrings(t, ctx, database, `
		SELECT CONCAT(tc.constraint_name, '=', tc.enforced)
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.check_constraints AS cc
		  ON cc.constraint_schema = tc.constraint_schema
		 AND cc.constraint_name = tc.constraint_name
		WHERE tc.constraint_schema = DATABASE()
		  AND tc.constraint_type = 'CHECK'
		  AND tc.table_name IN (
			'identity_workforce_account',
			'identity_session',
			'identity_authentication_throttle'
		  )`)
	want := []string{
		"chk_identity_workforce_account_id=YES",
		"chk_identity_workforce_account_login=YES",
		"chk_identity_workforce_account_principal=YES",
		"chk_identity_workforce_account_envelope=YES",
		"chk_identity_workforce_account_status=YES",
		"chk_identity_workforce_account_versions=YES",
		"chk_identity_workforce_account_times=YES",
		"chk_identity_session_ref=YES",
		"chk_identity_session_issue_operation=YES",
		"chk_identity_session_token_digest=YES",
		"chk_identity_session_epoch=YES",
		"chk_identity_session_times=YES",
		"chk_identity_session_revocation_shape=YES",
		"chk_identity_throttle_dimension=YES",
		"chk_identity_throttle_digest=YES",
		"chk_identity_throttle_window=YES",
		"chk_identity_throttle_epoch=YES",
		"chk_identity_throttle_aggregate=YES",
		"chk_identity_throttle_inflight_shape=YES",
		"chk_identity_throttle_block_shape=YES",
	}
	assertIdentityStringSet(t, "Identity enforced CHECKs", actual, want)
}

func assertIdentityBinarySemantics(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var equal int
	if err := database.QueryRowContext(ctx, `
		SELECT
			_ascii'alice.ops' COLLATE ascii_bin =
			_ascii'ALICE.OPS' COLLATE ascii_bin`).Scan(&equal); err != nil {
		t.Fatalf("verify ascii_bin case semantics: %v", err)
	}
	if equal != 0 {
		t.Fatal("ascii_bin unexpectedly treated different-case identifiers as equal")
	}
}

func assertIdentityRoundTripAndRejectedRows(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
) {
	t.Helper()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin Identity schema fixture transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("rollback Identity schema fixture: %v", err)
		}
	}()

	now := time.Date(2026, 9, 1, 10, 0, 0, 123000000, time.UTC)
	account := identityAccountFixture{
		accountID:           "identity.account.alice",
		loginName:           "alice.ops",
		principalID:         "principal.alice",
		passwordEnvelope:    "$argon2id$v=19$m=19456,t=2,p=1$c2FsdC1maXh0dXJlLTE$ZGlnaWVzdC1maXh0dXJlLTAwMDAwMDAwMDAwMDA",
		status:              "enabled",
		credentialVersion:   1,
		authenticationEpoch: 1,
		createdAt:           now,
		updatedAt:           now,
	}
	if err := insertIdentityAccount(ctx, tx, account); err != nil {
		t.Fatalf("insert valid Identity account: %v", err)
	}
	session := identitySessionFixture{
		sessionRef:          "session.alice.1",
		issueOperationRef:   "operation.issue.1",
		accountID:           account.accountID,
		tokenDigest:         identityDigest(0x11),
		authenticationEpoch: 1,
		issuedAt:            now,
		lastSeenAt:          now.Add(time.Minute),
		idleExpiresAt:       now.Add(16 * time.Minute),
		absoluteExpiresAt:   now.Add(8 * time.Hour),
		updatedAt:           now.Add(time.Minute),
	}
	if err := insertIdentitySession(ctx, tx, session); err != nil {
		t.Fatalf("insert valid Identity session: %v", err)
	}
	revokedSession := session
	revokedSession.sessionRef = "session.alice.revoked"
	revokedSession.issueOperationRef = "operation.issue.revoked"
	revokedSession.tokenDigest = identityDigest(0x12)
	revokedSession.revokedAt = nullableIdentityTime(now.Add(2 * time.Minute))
	revokedSession.revokeReason = sql.NullString{String: "logout", Valid: true}
	revokedSession.revokeOperationRef = sql.NullString{String: "operation.revoke.1", Valid: true}
	revokedSession.updatedAt = now.Add(2 * time.Minute)
	if err := insertIdentitySession(ctx, tx, revokedSession); err != nil {
		t.Fatalf("insert valid revoked Identity session union: %v", err)
	}
	throttle := identityThrottleFixture{
		dimension:         "login",
		subjectDigest:     identityDigest(0x22),
		windowStartedAt:   now,
		windowExpiresAt:   now.Add(15 * time.Minute),
		failureCount:      1,
		inflightCount:     1,
		admissionEpoch:    1,
		inflightExpiresAt: nullableIdentityTime(now.Add(time.Minute)),
		updatedAt:         now,
		rowExpiresAt:      now.Add(24 * time.Hour),
	}
	if err := insertIdentityThrottle(ctx, tx, throttle); err != nil {
		t.Fatalf("insert valid Identity throttle: %v", err)
	}
	assertIdentityFixtureRoundTrip(t, ctx, tx, account, session, throttle)

	invalidAccounts := []struct {
		name       string
		mutate     func(*identityAccountFixture)
		mysqlError uint16
	}{
		{"login grammar", func(value *identityAccountFixture) { value.loginName = "ab" }, 3819},
		{"status", func(value *identityAccountFixture) { value.status = "pending" }, 3819},
		{"credential version", func(value *identityAccountFixture) { value.credentialVersion = 0 }, 3819},
		{"authentication epoch", func(value *identityAccountFixture) { value.authenticationEpoch = 0 }, 3819},
		{"envelope", func(value *identityAccountFixture) { value.passwordEnvelope = "not-an-argon-envelope" }, 3819},
		{"time order", func(value *identityAccountFixture) { value.updatedAt = value.createdAt.Add(-time.Microsecond) }, 3819},
		{"duplicate login", func(value *identityAccountFixture) { value.loginName = account.loginName }, 1062},
		{"duplicate principal", func(value *identityAccountFixture) { value.principalID = account.principalID }, 1062},
	}
	for index, test := range invalidAccounts {
		value := account
		value.accountID = "identity.invalid.account." + identityOrdinal(index)
		value.loginName = "invalid." + identityOrdinal(index)
		value.principalID = "principal.invalid." + identityOrdinal(index)
		test.mutate(&value)
		expectIdentityMySQLError(t, test.name, test.mysqlError, insertIdentityAccount(ctx, tx, value))
	}

	invalidSessions := []struct {
		name       string
		mutate     func(*identitySessionFixture)
		mysqlError uint16
	}{
		{"zero digest", func(value *identitySessionFixture) { value.tokenDigest = make([]byte, 32) }, 3819},
		{"zero epoch", func(value *identitySessionFixture) { value.authenticationEpoch = 0 }, 3819},
		{"time order", func(value *identitySessionFixture) { value.idleExpiresAt = value.lastSeenAt }, 3819},
		{"revoked without union", func(value *identitySessionFixture) { value.revokedAt = nullableIdentityTime(value.lastSeenAt) }, 3819},
		{"reason without revoke", func(value *identitySessionFixture) {
			value.revokeReason = sql.NullString{String: "logout", Valid: true}
		}, 3819},
		{"orphan account", func(value *identitySessionFixture) { value.accountID = "identity.account.missing" }, 1452},
		{"duplicate digest", func(value *identitySessionFixture) { value.tokenDigest = session.tokenDigest }, 1062},
		{"duplicate issue operation", func(value *identitySessionFixture) { value.issueOperationRef = session.issueOperationRef }, 1062},
		{"duplicate revoke operation", func(value *identitySessionFixture) {
			value.revokedAt = revokedSession.revokedAt
			value.revokeReason = revokedSession.revokeReason
			value.revokeOperationRef = revokedSession.revokeOperationRef
			value.updatedAt = revokedSession.updatedAt
		}, 1062},
	}
	for index, test := range invalidSessions {
		value := session
		ordinal := identityOrdinal(index + 20)
		value.sessionRef = "session.invalid." + ordinal
		value.issueOperationRef = "operation.invalid." + ordinal
		value.tokenDigest = identityDigest(byte(index + 0x30))
		test.mutate(&value)
		expectIdentityMySQLError(t, test.name, test.mysqlError, insertIdentitySession(ctx, tx, value))
	}

	invalidThrottles := []struct {
		name   string
		mutate func(*identityThrottleFixture)
	}{
		{"dimension", func(value *identityThrottleFixture) { value.dimension = "account" }},
		{"zero digest", func(value *identityThrottleFixture) { value.subjectDigest = make([]byte, 32) }},
		{"window order", func(value *identityThrottleFixture) { value.windowExpiresAt = value.windowStartedAt }},
		{"zero admission epoch", func(value *identityThrottleFixture) { value.admissionEpoch = 0 }},
		{"aggregate overflow", func(value *identityThrottleFixture) { value.failureCount = ^uint32(0) }},
		{"inflight count without expiry", func(value *identityThrottleFixture) { value.inflightExpiresAt = sql.NullTime{} }},
		{"inflight expiry without count", func(value *identityThrottleFixture) { value.inflightCount = 0 }},
		{"blocked without failures", func(value *identityThrottleFixture) {
			value.failureCount = 0
			value.blockedUntil = nullableIdentityTime(value.windowStartedAt.Add(time.Minute))
		}},
	}
	for index, test := range invalidThrottles {
		value := throttle
		value.subjectDigest = identityDigest(byte(index + 0x50))
		test.mutate(&value)
		expectIdentityMySQLError(t, test.name, 3819, insertIdentityThrottle(ctx, tx, value))
	}
	expectIdentityMySQLError(t, "duplicate throttle primary key", 1062,
		insertIdentityThrottle(ctx, tx, throttle))

	_, deleteErr := tx.ExecContext(ctx,
		"DELETE FROM identity_workforce_account WHERE account_id = ?",
		account.accountID,
	)
	expectIdentityMySQLError(t, "parent delete RESTRICT", 1451, deleteErr)
}

type identityAccountFixture struct {
	accountID           string
	loginName           string
	principalID         string
	passwordEnvelope    string
	status              string
	credentialVersion   uint64
	authenticationEpoch uint64
	createdAt           time.Time
	updatedAt           time.Time
}

func insertIdentityAccount(ctx context.Context, tx *sql.Tx, value identityAccountFixture) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO identity_workforce_account (
			account_id, login_name, principal_id, password_envelope,
			account_status, credential_version, authentication_epoch,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.accountID,
		value.loginName,
		value.principalID,
		value.passwordEnvelope,
		value.status,
		value.credentialVersion,
		value.authenticationEpoch,
		value.createdAt,
		value.updatedAt,
	)
	return err
}

type identitySessionFixture struct {
	sessionRef          string
	issueOperationRef   string
	accountID           string
	tokenDigest         []byte
	authenticationEpoch uint64
	issuedAt            time.Time
	lastSeenAt          time.Time
	idleExpiresAt       time.Time
	absoluteExpiresAt   time.Time
	revokedAt           sql.NullTime
	revokeReason        sql.NullString
	revokeOperationRef  sql.NullString
	updatedAt           time.Time
}

func insertIdentitySession(ctx context.Context, tx *sql.Tx, value identitySessionFixture) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO identity_session (
			session_ref, issue_operation_ref, account_id, token_digest,
			authentication_epoch, issued_at, last_seen_at, idle_expires_at,
			absolute_expires_at, revoked_at, revoke_reason,
			revoke_operation_ref, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.sessionRef,
		value.issueOperationRef,
		value.accountID,
		value.tokenDigest,
		value.authenticationEpoch,
		value.issuedAt,
		value.lastSeenAt,
		value.idleExpiresAt,
		value.absoluteExpiresAt,
		value.revokedAt,
		value.revokeReason,
		value.revokeOperationRef,
		value.updatedAt,
	)
	return err
}

type identityThrottleFixture struct {
	dimension         string
	subjectDigest     []byte
	windowStartedAt   time.Time
	windowExpiresAt   time.Time
	failureCount      uint32
	inflightCount     uint32
	admissionEpoch    uint64
	inflightExpiresAt sql.NullTime
	blockedUntil      sql.NullTime
	updatedAt         time.Time
	rowExpiresAt      time.Time
}

func insertIdentityThrottle(ctx context.Context, tx *sql.Tx, value identityThrottleFixture) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO identity_authentication_throttle (
			dimension, subject_digest, window_started_at, window_expires_at,
			failure_count, inflight_count, admission_epoch, inflight_expires_at,
			blocked_until, updated_at, row_expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.dimension,
		value.subjectDigest,
		value.windowStartedAt,
		value.windowExpiresAt,
		value.failureCount,
		value.inflightCount,
		value.admissionEpoch,
		value.inflightExpiresAt,
		value.blockedUntil,
		value.updatedAt,
		value.rowExpiresAt,
	)
	return err
}

func assertIdentityFixtureRoundTrip(
	t *testing.T,
	ctx context.Context,
	tx *sql.Tx,
	account identityAccountFixture,
	session identitySessionFixture,
	throttle identityThrottleFixture,
) {
	t.Helper()
	var accountValue string
	if err := tx.QueryRowContext(ctx, `
		SELECT CONCAT(account_id, '|', login_name, '|', principal_id, '|',
			account_status, '|', credential_version, '|', authentication_epoch)
		FROM identity_workforce_account WHERE account_id = ?`, account.accountID).Scan(&accountValue); err != nil {
		t.Fatalf("round-trip Identity account: %v", err)
	}
	if accountValue != "identity.account.alice|alice.ops|principal.alice|enabled|1|1" {
		t.Fatalf("round-trip Identity account = %q", accountValue)
	}

	var sessionValue string
	if err := tx.QueryRowContext(ctx, `
		SELECT CONCAT(session_ref, '|', issue_operation_ref, '|', account_id,
			'|', HEX(token_digest), '|', authentication_epoch)
		FROM identity_session WHERE session_ref = ?`, session.sessionRef).Scan(&sessionValue); err != nil {
		t.Fatalf("round-trip Identity session: %v", err)
	}
	wantSession := "session.alice.1|operation.issue.1|identity.account.alice|" +
		strings.ToUpper(hex.EncodeToString(session.tokenDigest)) + "|1"
	if sessionValue != wantSession {
		t.Fatalf("round-trip Identity session = %q, want %q", sessionValue, wantSession)
	}

	var throttleValue string
	if err := tx.QueryRowContext(ctx, `
		SELECT CONCAT(dimension, '|', HEX(subject_digest), '|', failure_count,
			'|', inflight_count, '|', admission_epoch)
		FROM identity_authentication_throttle
		WHERE dimension = ? AND subject_digest = ?`,
		throttle.dimension, throttle.subjectDigest).Scan(&throttleValue); err != nil {
		t.Fatalf("round-trip Identity throttle: %v", err)
	}
	wantThrottle := "login|" + strings.ToUpper(hex.EncodeToString(throttle.subjectDigest)) + "|1|1|1"
	if throttleValue != wantThrottle {
		t.Fatalf("round-trip Identity throttle = %q, want %q", throttleValue, wantThrottle)
	}
}

func nullableIdentityTime(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: true}
}

func identityDigest(fill byte) []byte {
	digest := make([]byte, 32)
	for index := range digest {
		digest[index] = fill
	}
	return digest
}

func identityOrdinal(value int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value < 0 || value >= len(alphabet) {
		return "x"
	}
	return string(alphabet[value])
}

func expectIdentityMySQLError(t *testing.T, name string, number uint16, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s unexpectedly succeeded; want MySQL error %d", name, number)
	}
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) {
		t.Fatalf("%s error = %v, want MySQL error %d", name, err, number)
	}
	if mysqlError.Number != number {
		t.Fatalf("%s MySQL error = %d, want %d", name, mysqlError.Number, number)
	}
}

func assertIdentityDirtyFailsClosed(
	t *testing.T,
	ctx context.Context,
	verification *sql.DB,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	if _, err := verification.ExecContext(ctx,
		"UPDATE schema_migrations SET dirty = 1 WHERE version = 14"); err != nil {
		t.Fatalf("install Identity dirty probe: %v", err)
	}
	dirty := true
	defer func() {
		if !dirty {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := verification.ExecContext(cleanupCtx,
			"UPDATE schema_migrations SET dirty = 0 WHERE version = 14"); err != nil {
			t.Errorf("remove Identity dirty probe: %v", err)
		}
	}()

	database := openIdentityMigrationDatabase(t, ctx, connection)
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        35 * time.Second,
		NetworkReadTimeout: 30 * time.Second,
		StatementTimeout:   25 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct dirty Identity runner: %v", err)
	}
	if _, err := runner.Status(ctx); !errors.Is(err, dbmigration.ErrDirty) {
		_ = runner.Close()
		t.Fatalf("dirty Identity Status error = %v, want ErrDirty", err)
	}
	if _, err := runner.Up(ctx); !errors.Is(err, dbmigration.ErrDirty) {
		_ = runner.Close()
		t.Fatalf("dirty Identity Up error = %v, want ErrDirty", err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close dirty Identity runner: %v", err)
	}
	if _, err := verification.ExecContext(ctx,
		"UPDATE schema_migrations SET dirty = 0 WHERE version = 14"); err != nil {
		t.Fatalf("remove Identity dirty probe: %v", err)
	}
	dirty = false
}

func queryIdentityStrings(
	t *testing.T,
	ctx context.Context,
	database interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	query string,
	args ...any,
) []string {
	t.Helper()
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("query Identity schema metadata: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close Identity schema metadata rows: %v", err)
		}
	}()
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan Identity schema metadata: %v", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate Identity schema metadata: %v", err)
	}
	return values
}

func assertIdentityStrings(t *testing.T, label string, actual, want []string) {
	t.Helper()
	if strings.Join(actual, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s =\n%s\nwant\n%s", label, strings.Join(actual, "\n"), strings.Join(want, "\n"))
	}
}

func assertIdentityStringSet(t *testing.T, label string, actual, want []string) {
	t.Helper()
	actualCopy := append([]string(nil), actual...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(actualCopy)
	sort.Strings(wantCopy)
	assertIdentityStrings(t, label, actualCopy, wantCopy)
}
