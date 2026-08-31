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

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	projectmigrations "github.com/Atingaii/GrowthOS-Go/migrations"
)

func TestActivityPublicationSchemaMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-30-isolated-schema" {
		t.Skip("Lesson 30 schema integration requires its disposable-schema authorization")
	}
	connection := schemaIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	assertLesson30DatabaseStartsFresh(t, ctx, connection)
	baseline := applyLesson30VersionFiveBaseline(t, ctx, connection)
	if baseline.State != dbmigration.ResultApplied || baseline.Version != 5 {
		t.Fatalf("v5 baseline result = %+v, want applied at exact version 5", baseline)
	}
	seedLesson30VersionFiveBaseline(t, ctx, connection)
	before := captureLesson30OldSchemaFingerprint(t, ctx, connection)

	first := applyLesson30Migrations(t, ctx, connection)
	if first.State != dbmigration.ResultApplied || first.Version != 11 {
		t.Fatalf("5->11 migration result = %+v, want applied at exact version 11", first)
	}
	after := captureLesson30OldSchemaFingerprint(t, ctx, connection)
	if after != before {
		t.Fatalf("old five-table schema fingerprint changed across 5->11: before=%+v after=%+v", before, after)
	}
	second := applyLesson30Migrations(t, ctx, connection)
	if second.State != dbmigration.ResultNoChange || second.Version != 11 {
		t.Fatalf("repeat migration result = %+v, want exact v11 no_change", second)
	}

	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open Lesson 30 verification database: %v", err)
	}
	defer database.Close()
	assertLesson30MigrationStatus(t, ctx, connection)
	assertLesson30DirtyFailsClosed(t, ctx, database, connection)
	assertLesson30Tables(t, ctx, database)
	assertLesson30Columns(t, ctx, database)
	assertLesson30PrimaryKeys(t, ctx, database)
	assertLesson30ForeignKeys(t, ctx, database)
	assertLesson30Checks(t, ctx, database)
}

var lesson30VersionFiveMigrations = []string{
	"000001_create_lottery_strategy.up.sql",
	"000002_create_lottery_strategy_award.up.sql",
	"000003_create_lottery_strategy_routing_graph.up.sql",
	"000004_create_lottery_strategy_routing_node.up.sql",
	"000005_create_lottery_strategy_routing_edge.up.sql",
}

var lesson30OldTables = []string{
	"lottery_strategy",
	"lottery_strategy_award",
	"lottery_strategy_routing_graph",
	"lottery_strategy_routing_node",
	"lottery_strategy_routing_edge",
}

type lesson30OldSchemaFingerprint struct {
	Tables           int
	Columns          int
	Constraints      int
	IndexColumns     int
	ShowCreateSHA256 string
	DataRows         int
	DataSHA256       string
}

func assertLesson30DatabaseStartsFresh(
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
		t.Fatalf("open fresh-database probe: %v", err)
	}
	defer database.Close()
	var tables int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()`).Scan(&tables); err != nil {
		t.Fatalf("inspect fresh disposable database: %v", err)
	}
	if tables != 0 {
		t.Fatalf("Lesson 30 schema acceptance requires an empty disposable database; found %d tables", tables)
	}
}

func applyLesson30VersionFiveBaseline(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) dbmigration.Result {
	t.Helper()
	baselineFiles := fstest.MapFS{}
	for _, name := range lesson30VersionFiveMigrations {
		path := "sql/" + name
		contents, err := fs.ReadFile(projectmigrations.Files, path)
		if err != nil {
			t.Fatalf("read frozen baseline migration %s: %v", path, err)
		}
		baselineFiles[path] = &fstest.MapFile{Data: contents, Mode: 0o444}
	}
	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open v5 baseline migration database: %v", err)
	}
	runner, err := dbmigration.New(ctx, baselineFiles, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct v5 baseline runner: %v", err)
	}
	result, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("apply v5 baseline: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read v5 baseline status: %v", err)
	}
	if status != (dbmigration.Status{State: dbmigration.StatusClean, Version: 5, Latest: 5}) {
		_ = runner.Close()
		t.Fatalf("v5 baseline status = %+v, want clean/5/5", status)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close v5 baseline runner: %v", err)
	}
	return result
}

func captureLesson30OldSchemaFingerprint(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) lesson30OldSchemaFingerprint {
	t.Helper()
	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open old-schema fingerprint database: %v", err)
	}
	defer database.Close()
	tableList := "'" + strings.Join(lesson30OldTables, "','") + "'"
	queries := []string{
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
	}
	counts := make([]int, len(queries))
	for index, query := range queries {
		if err := database.QueryRowContext(ctx, query).Scan(&counts[index]); err != nil {
			t.Fatalf("capture old-schema count %d: %v", index, err)
		}
	}
	if counts[0] != len(lesson30OldTables) {
		t.Fatalf("old baseline tables = %d, want %d", counts[0], len(lesson30OldTables))
	}
	definitions := make([]string, 0, len(lesson30OldTables))
	for _, table := range lesson30OldTables {
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
	checksum := sha256.Sum256([]byte(strings.Join(definitions, "\n--next-table--\n")))
	dataRows, dataChecksum := captureLesson30OldDataFingerprint(t, ctx, database)
	return lesson30OldSchemaFingerprint{
		Tables:           counts[0],
		Columns:          counts[1],
		Constraints:      counts[2],
		IndexColumns:     counts[3],
		ShowCreateSHA256: hex.EncodeToString(checksum[:]),
		DataRows:         dataRows,
		DataSHA256:       dataChecksum,
	}
}

func seedLesson30VersionFiveBaseline(
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
		t.Fatalf("open v5 seed database: %v", err)
	}
	defer database.Close()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin v5 seed transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	statements := []struct {
		query string
		args  []any
	}{
		{
			query: "INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ?), (?, ?)",
			args:  []any{uint64(101), "v5 baseline Strategy", uint64(102), "v5 premium Strategy"},
		},
		{
			query: `INSERT INTO lottery_strategy_award
				(strategy_id, award_id, name, weight, outcome)
				VALUES (?, ?, ?, ?, ?)`,
			args: []any{uint64(101), uint64(1), "v5 reward", uint64(7), "reward"},
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
			args: []any{uint64(201), "baseline:r1", uint64(10), uint64(201), "baseline:r1", uint64(20), uint64(101)},
		},
		{
			query: `INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
				VALUES (?, ?, ?, 'baseline_default', ?, 1)`,
			args: []any{uint64(201), "baseline:r1", uint64(10), uint64(20)},
		},
	}
	for index, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("execute v5 seed statement %d: %v", index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v5 seed transaction: %v", err)
	}
}

func captureLesson30OldDataFingerprint(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
) (int, string) {
	t.Helper()
	queries := []struct {
		table string
		query string
	}{
		{
			table: "lottery_strategy",
			query: "SELECT CONCAT(strategy_id, '|', HEX(name)) FROM lottery_strategy ORDER BY strategy_id",
		},
		{
			table: "lottery_strategy_award",
			query: "SELECT CONCAT(strategy_id, '|', award_id, '|', HEX(name), '|', weight, '|', outcome) FROM lottery_strategy_award ORDER BY strategy_id, award_id",
		},
		{
			table: "lottery_strategy_routing_graph",
			query: "SELECT CONCAT(graph_id, '|', revision, '|', schema_version, '|', root_node_id) FROM lottery_strategy_routing_graph ORDER BY graph_id, revision",
		},
		{
			table: "lottery_strategy_routing_node",
			query: "SELECT CONCAT(graph_id, '|', revision, '|', node_id, '|', node_kind, '|', COALESCE(rule_code, 'NULL'), '|', COALESCE(strategy_id, 'NULL')) FROM lottery_strategy_routing_node ORDER BY graph_id, revision, node_id",
		},
		{
			table: "lottery_strategy_routing_edge",
			query: "SELECT CONCAT(graph_id, '|', revision, '|', from_node_id, '|', branch_code, '|', to_node_id, '|', is_default) FROM lottery_strategy_routing_edge ORDER BY graph_id, revision, from_node_id, branch_code",
		},
	}
	rows := make([]string, 0, 7)
	for _, item := range queries {
		values := querySchemaStrings(t, ctx, database, item.query)
		if len(values) == 0 {
			t.Fatalf("v5 baseline table %s is empty; want non-empty seed", item.table)
		}
		for _, value := range values {
			rows = append(rows, item.table+"|"+value)
		}
	}
	if len(rows) != 7 {
		t.Fatalf("v5 baseline data rows = %d, want exact 7", len(rows))
	}
	checksum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return len(rows), hex.EncodeToString(checksum[:])
}

func applyLesson30Migrations(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) dbmigration.Result {
	t.Helper()
	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open Lesson 30 migration database: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct Lesson 30 migration runner: %v", err)
	}
	result, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("apply Lesson 30 migrations: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read Lesson 30 migration status: %v", err)
	}
	if status.State != dbmigration.StatusClean || status.Version != 11 || status.Latest != 11 {
		_ = runner.Close()
		t.Fatalf("migration status = %+v, want clean exact v11", status)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close Lesson 30 migration runner: %v", err)
	}
	return result
}

func assertLesson30MigrationStatus(
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
		t.Fatalf("open migration status database: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct migration status runner: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read exact migration status: %v", err)
	}
	if status != (dbmigration.Status{State: dbmigration.StatusClean, Version: 11, Latest: 11}) {
		_ = runner.Close()
		t.Fatalf("migration status = %+v, want clean/11/11", status)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close migration status runner: %v", err)
	}
}

func assertLesson30DirtyFailsClosed(
	t *testing.T,
	ctx context.Context,
	verification *sql.DB,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	if _, err := verification.ExecContext(ctx, "UPDATE schema_migrations SET dirty = 1 WHERE version = 11"); err != nil {
		t.Fatalf("install dirty migration probe: %v", err)
	}
	dirty := true
	defer func() {
		if !dirty {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := verification.ExecContext(cleanupCtx, "UPDATE schema_migrations SET dirty = 0 WHERE version = 11"); err != nil {
			t.Errorf("remove dirty migration probe: %v", err)
		}
	}()

	database, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 20 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open dirty migration database: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct dirty migration runner: %v", err)
	}
	if _, err := runner.Status(ctx); !errors.Is(err, dbmigration.ErrDirty) {
		_ = runner.Close()
		t.Fatalf("dirty Status error = %v, want ErrDirty", err)
	}
	if _, err := runner.Up(ctx); !errors.Is(err, dbmigration.ErrDirty) {
		_ = runner.Close()
		t.Fatalf("dirty Up error = %v, want ErrDirty", err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close dirty migration runner: %v", err)
	}
	if _, err := verification.ExecContext(ctx, "UPDATE schema_migrations SET dirty = 0 WHERE version = 11"); err != nil {
		t.Fatalf("remove dirty migration probe: %v", err)
	}
	dirty = false
}

func assertLesson30Tables(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	rows := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(table_name, '=', engine, '/', table_collation)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'lottery_strategy_snapshot',
			'lottery_strategy_snapshot_award',
			'marketing_activity',
			'marketing_activity_publication',
			'marketing_activity_publication_strategy'
		  )
		ORDER BY table_name`)
	want := []string{
		"lottery_strategy_snapshot=InnoDB/utf8mb4_0900_bin",
		"lottery_strategy_snapshot_award=InnoDB/utf8mb4_0900_bin",
		"marketing_activity=InnoDB/utf8mb4_0900_bin",
		"marketing_activity_publication=InnoDB/utf8mb4_0900_bin",
		"marketing_activity_publication_strategy=InnoDB/utf8mb4_0900_bin",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Lesson 30 tables = %q, want exact InnoDB/binary set %q", rows, want)
	}
}

func assertLesson30Columns(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	rows := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(
			table_name, '#', LPAD(ordinal_position, 2, '0'), ':', column_name, '=',
			column_type, '/', is_nullable, '/',
			COALESCE(character_set_name, '-'), '/', COALESCE(collation_name, '-'), '/',
			COALESCE(NULLIF(extra, ''), '-')
		)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'lottery_strategy_snapshot',
			'lottery_strategy_snapshot_award',
			'marketing_activity',
			'marketing_activity_publication',
			'marketing_activity_publication_strategy'
		  )
		ORDER BY table_name, ordinal_position`)
	want := []string{
		"lottery_strategy_snapshot#01:strategy_id=bigint unsigned/NO/-/-/-",
		"lottery_strategy_snapshot#02:revision=varchar(128)/NO/ascii/ascii_bin/-",
		"lottery_strategy_snapshot#03:schema_version=smallint unsigned/NO/-/-/-",
		"lottery_strategy_snapshot#04:name=varchar(128)/NO/utf8mb4/utf8mb4_0900_bin/-",
		"lottery_strategy_snapshot#05:created_at=datetime(6)/NO/-/-/DEFAULT_GENERATED",
		"lottery_strategy_snapshot_award#01:strategy_id=bigint unsigned/NO/-/-/-",
		"lottery_strategy_snapshot_award#02:revision=varchar(128)/NO/ascii/ascii_bin/-",
		"lottery_strategy_snapshot_award#03:award_id=bigint unsigned/NO/-/-/-",
		"lottery_strategy_snapshot_award#04:name=varchar(128)/NO/utf8mb4/utf8mb4_0900_bin/-",
		"lottery_strategy_snapshot_award#05:weight=bigint unsigned/NO/-/-/-",
		"lottery_strategy_snapshot_award#06:outcome=varchar(16)/NO/ascii/ascii_bin/-",
		"lottery_strategy_snapshot_award#07:created_at=datetime(6)/NO/-/-/DEFAULT_GENERATED",
		"marketing_activity#01:activity_id=bigint unsigned/NO/-/-/-",
		"marketing_activity#02:name=varchar(128)/NO/utf8mb4/utf8mb4_0900_bin/-",
		"marketing_activity#03:lifecycle_state=varchar(16)/NO/ascii/ascii_bin/-",
		"marketing_activity#04:state_version=bigint unsigned/NO/-/-/-",
		"marketing_activity#05:active_version=bigint unsigned/YES/-/-/-",
		"marketing_activity#06:retired_at=datetime(6)/YES/-/-/-",
		"marketing_activity#07:retirement_reference=varchar(128)/YES/ascii/ascii_bin/-",
		"marketing_activity#08:created_at=datetime(6)/NO/-/-/DEFAULT_GENERATED",
		"marketing_activity#09:updated_at=datetime(6)/NO/-/-/DEFAULT_GENERATED on update CURRENT_TIMESTAMP(6)",
		"marketing_activity_publication#01:activity_id=bigint unsigned/NO/-/-/-",
		"marketing_activity_publication#02:activity_version=bigint unsigned/NO/-/-/-",
		"marketing_activity_publication#03:schema_version=smallint unsigned/NO/-/-/-",
		"marketing_activity_publication#04:publication_kind=varchar(16)/NO/ascii/ascii_bin/-",
		"marketing_activity_publication#05:rollback_of_version=bigint unsigned/YES/-/-/-",
		"marketing_activity_publication#06:graph_id=bigint unsigned/NO/-/-/-",
		"marketing_activity_publication#07:graph_revision=varchar(128)/NO/ascii/ascii_bin/-",
		"marketing_activity_publication#08:starts_at=datetime(6)/NO/-/-/-",
		"marketing_activity_publication#09:ends_at=datetime(6)/NO/-/-/-",
		"marketing_activity_publication#10:published_at=datetime(6)/NO/-/-/-",
		"marketing_activity_publication#11:approval_reference=varchar(128)/NO/ascii/ascii_bin/-",
		"marketing_activity_publication#12:created_at=datetime(6)/NO/-/-/DEFAULT_GENERATED",
		"marketing_activity_publication_strategy#01:activity_id=bigint unsigned/NO/-/-/-",
		"marketing_activity_publication_strategy#02:activity_version=bigint unsigned/NO/-/-/-",
		"marketing_activity_publication_strategy#03:strategy_id=bigint unsigned/NO/-/-/-",
		"marketing_activity_publication_strategy#04:strategy_revision=varchar(128)/NO/ascii/ascii_bin/-",
		"marketing_activity_publication_strategy#05:created_at=datetime(6)/NO/-/-/DEFAULT_GENERATED",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Lesson 30 columns =\n%s\nwant exact columns\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
}

func assertLesson30PrimaryKeys(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	rows := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(table_name, '#', LPAD(ordinal_position, 2, '0'), ':', column_name)
		FROM information_schema.key_column_usage
		WHERE constraint_schema = DATABASE()
		  AND constraint_name = 'PRIMARY'
		  AND table_name IN (
			'lottery_strategy_snapshot',
			'lottery_strategy_snapshot_award',
			'marketing_activity',
			'marketing_activity_publication',
			'marketing_activity_publication_strategy'
		  )
		ORDER BY table_name, ordinal_position`)
	want := []string{
		"lottery_strategy_snapshot#01:strategy_id",
		"lottery_strategy_snapshot#02:revision",
		"lottery_strategy_snapshot_award#01:strategy_id",
		"lottery_strategy_snapshot_award#02:revision",
		"lottery_strategy_snapshot_award#03:award_id",
		"marketing_activity#01:activity_id",
		"marketing_activity_publication#01:activity_id",
		"marketing_activity_publication#02:activity_version",
		"marketing_activity_publication_strategy#01:activity_id",
		"marketing_activity_publication_strategy#02:activity_version",
		"marketing_activity_publication_strategy#03:strategy_id",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Lesson 30 primary keys = %q, want %q", rows, want)
	}
}

func assertLesson30ForeignKeys(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	rows := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(
			constraint_name, '#', LPAD(ordinal_position, 2, '0'), ':',
			table_name, '.', column_name, '->', referenced_table_name, '.', referenced_column_name
		)
		FROM information_schema.key_column_usage
		WHERE constraint_schema = DATABASE()
		  AND referenced_table_schema = DATABASE()
		  AND table_name IN (
			'lottery_strategy_snapshot',
			'lottery_strategy_snapshot_award',
			'marketing_activity',
			'marketing_activity_publication',
			'marketing_activity_publication_strategy'
		  )
		ORDER BY constraint_name, ordinal_position`)
	want := []string{
		"fk_lottery_strategy_snapshot_award_snapshot#01:lottery_strategy_snapshot_award.strategy_id->lottery_strategy_snapshot.strategy_id",
		"fk_lottery_strategy_snapshot_award_snapshot#02:lottery_strategy_snapshot_award.revision->lottery_strategy_snapshot.revision",
		"fk_lottery_strategy_snapshot_strategy#01:lottery_strategy_snapshot.strategy_id->lottery_strategy.strategy_id",
		"fk_marketing_activity_active_publication#01:marketing_activity.activity_id->marketing_activity_publication.activity_id",
		"fk_marketing_activity_active_publication#02:marketing_activity.active_version->marketing_activity_publication.activity_version",
		"fk_marketing_publication_activity#01:marketing_activity_publication.activity_id->marketing_activity.activity_id",
		"fk_marketing_publication_rollback#01:marketing_activity_publication.activity_id->marketing_activity_publication.activity_id",
		"fk_marketing_publication_rollback#02:marketing_activity_publication.rollback_of_version->marketing_activity_publication.activity_version",
		"fk_marketing_publication_strategy_publication#01:marketing_activity_publication_strategy.activity_id->marketing_activity_publication.activity_id",
		"fk_marketing_publication_strategy_publication#02:marketing_activity_publication_strategy.activity_version->marketing_activity_publication.activity_version",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Lesson 30 foreign-key columns = %q, want %q", rows, want)
	}

	rules := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(constraint_name, '=', update_rule, '/', delete_rule)
		FROM information_schema.referential_constraints
		WHERE constraint_schema = DATABASE()
		  AND constraint_name IN (
			'fk_lottery_strategy_snapshot_award_snapshot',
			'fk_lottery_strategy_snapshot_strategy',
			'fk_marketing_activity_active_publication',
			'fk_marketing_publication_activity',
			'fk_marketing_publication_rollback',
			'fk_marketing_publication_strategy_publication'
		  )
		ORDER BY constraint_name`)
	wantRules := []string{
		"fk_lottery_strategy_snapshot_award_snapshot=RESTRICT/RESTRICT",
		"fk_lottery_strategy_snapshot_strategy=RESTRICT/RESTRICT",
		"fk_marketing_activity_active_publication=RESTRICT/RESTRICT",
		"fk_marketing_publication_activity=RESTRICT/RESTRICT",
		"fk_marketing_publication_rollback=RESTRICT/RESTRICT",
		"fk_marketing_publication_strategy_publication=RESTRICT/RESTRICT",
	}
	if strings.Join(rules, "\n") != strings.Join(wantRules, "\n") {
		t.Fatalf("Lesson 30 FK rules = %q, want all exact RESTRICT %q", rules, wantRules)
	}

	var crossContext int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.key_column_usage
		WHERE constraint_schema = DATABASE()
		  AND table_name LIKE 'marketing\_%' ESCAPE '\\'
		  AND referenced_table_name LIKE 'lottery\_%' ESCAPE '\\'`).Scan(&crossContext); err != nil {
		t.Fatalf("inspect Marketing-to-Lottery foreign keys: %v", err)
	}
	if crossContext != 0 {
		t.Fatalf("Marketing-to-Lottery foreign keys = %d, want none across bounded contexts", crossContext)
	}
}

func assertLesson30Checks(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	rows := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(tc.constraint_name, '=', tc.enforced)
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.check_constraints AS cc
		  ON cc.constraint_schema = tc.constraint_schema
		 AND cc.constraint_name = tc.constraint_name
		WHERE tc.constraint_schema = DATABASE()
		  AND tc.constraint_type = 'CHECK'
		  AND tc.table_name IN (
			'lottery_strategy_snapshot',
			'lottery_strategy_snapshot_award',
			'marketing_activity',
			'marketing_activity_publication',
			'marketing_activity_publication_strategy'
		  )
		ORDER BY tc.constraint_name`)
	want := []string{
		"chk_lottery_strategy_snapshot_award_ids_positive=YES",
		"chk_lottery_strategy_snapshot_award_name_basic=YES",
		"chk_lottery_strategy_snapshot_award_outcome=YES",
		"chk_lottery_strategy_snapshot_award_revision=YES",
		"chk_lottery_strategy_snapshot_award_weight_positive=YES",
		"chk_lottery_strategy_snapshot_id_positive=YES",
		"chk_lottery_strategy_snapshot_name_basic=YES",
		"chk_lottery_strategy_snapshot_revision=YES",
		"chk_lottery_strategy_snapshot_schema_version=YES",
		"chk_marketing_activity_id_positive=YES",
		"chk_marketing_activity_name_basic=YES",
		"chk_marketing_activity_retirement_ref=YES",
		"chk_marketing_activity_state_shape=YES",
		"chk_marketing_publication_approval_ref=YES",
		"chk_marketing_publication_graph_identity=YES",
		"chk_marketing_publication_identity=YES",
		"chk_marketing_publication_kind_shape=YES",
		"chk_marketing_publication_schema_version=YES",
		"chk_marketing_publication_strategy_identity=YES",
		"chk_marketing_publication_window=YES",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Lesson 30 CHECK constraints = %q, want exact enforced set %q", rows, want)
	}
}
