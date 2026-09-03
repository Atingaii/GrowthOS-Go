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

const governanceSchemaAuthorization = "lesson-33-isolated-schema"

var governanceVersionFourteenMigrations = []string{
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
	"000012_create_identity_workforce_account.up.sql",
	"000013_create_identity_session.up.sql",
	"000014_create_identity_authentication_throttle.up.sql",
}

var governancePreexistingTables = []string{
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

var governanceDisposableTables = append([]string{
	"governance_authorization_audit_match",
	"governance_authorization_audit",
	"governance_active_policy",
	"governance_policy_activation",
	"governance_policy_role_binding",
	"governance_policy_role_permission",
	"governance_policy_role",
	"governance_policy_revision",
}, governancePreexistingTables...)

func TestGovernanceSchemaMySQLIntegration(t *testing.T) {
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_GOVERNANCE_SCHEMA_CHANGES") != governanceSchemaAuthorization {
		t.Skip("Governance schema integration requires exact disposable-schema authorization")
	}
	connection := schemaIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_GOVERNANCE_MIGRATION")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	assertGovernanceDatabaseStartsFresh(t, ctx, connection)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		cleanupGovernanceDisposableSchema(t, cleanupCtx, connection)
	})

	baseline := runGovernanceMigrations(
		t,
		ctx,
		connection,
		governanceVersionFourteenFS(t),
		14,
	)
	if baseline.State != dbmigration.ResultApplied || baseline.Version != 14 {
		t.Fatalf("fresh v14 baseline = %+v, want applied exact v14", baseline)
	}
	before := captureGovernancePreexistingFingerprint(t, ctx, connection)

	upgrade := runGovernanceMigrations(t, ctx, connection, projectmigrations.Files, 22)
	if upgrade.State != dbmigration.ResultApplied || upgrade.Version != 22 {
		t.Fatalf("14->22 migration result = %+v, want applied exact v22", upgrade)
	}
	after := captureGovernancePreexistingFingerprint(t, ctx, connection)
	if after != before {
		t.Fatalf("pre-Governance schema changed across 14->21: before=%+v after=%+v", before, after)
	}
	repeat := runGovernanceMigrations(t, ctx, connection, projectmigrations.Files, 22)
	if repeat.State != dbmigration.ResultNoChange || repeat.Version != 22 {
		t.Fatalf("repeat v22 migration result = %+v, want exact no_change", repeat)
	}

	database := openGovernanceMigrationDatabase(t, ctx, connection)
	defer database.Close()
	assertGovernanceStrictMode(t, ctx, database)
	assertGovernanceTables(t, ctx, database)
	assertGovernanceColumns(t, ctx, database)
	assertGovernanceForeignKeys(t, ctx, database)
	assertGovernanceChecks(t, ctx, database)
	assertGovernanceRoundTripAndRejectedRows(t, ctx, database)
}

func openGovernanceMigrationDatabase(
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
		t.Fatalf("open Governance migration database: %v", err)
	}
	return database
}

func runGovernanceMigrations(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
	files fs.FS,
	expectedLatest uint,
) dbmigration.Result {
	t.Helper()
	database := openGovernanceMigrationDatabase(t, ctx, connection)
	runner, err := dbmigration.New(ctx, files, database, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   20 * time.Second,
	})
	if err != nil {
		_ = database.Close()
		t.Fatalf("construct Governance migration runner: %v", err)
	}
	result, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("apply Governance migrations: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read Governance migration status: %v", err)
	}
	wantStatus := dbmigration.Status{
		State:   dbmigration.StatusClean,
		Version: expectedLatest,
		Latest:  expectedLatest,
	}
	if status != wantStatus {
		_ = runner.Close()
		t.Fatalf("Governance migration status = %+v, want %+v", status, wantStatus)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close Governance migration runner: %v", err)
	}
	return result
}

func governanceVersionFourteenFS(t *testing.T) fs.FS {
	t.Helper()
	baseline := fstest.MapFS{}
	for _, name := range governanceVersionFourteenMigrations {
		path := "sql/" + name
		contents, err := fs.ReadFile(projectmigrations.Files, path)
		if err != nil {
			t.Fatalf("read frozen v14 migration %s: %v", path, err)
		}
		baseline[path] = &fstest.MapFile{Data: contents, Mode: 0o444}
	}
	return baseline
}

func assertGovernanceDatabaseStartsFresh(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	database := openGovernanceMigrationDatabase(t, ctx, connection)
	defer database.Close()
	var tables int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()`).Scan(&tables); err != nil {
		t.Fatalf("inspect disposable Governance database: %v", err)
	}
	if tables != 0 {
		t.Fatalf("Governance schema acceptance requires an empty disposable database; found %d tables", tables)
	}
}

func cleanupGovernanceDisposableSchema(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) {
	t.Helper()
	database := openGovernanceMigrationDatabase(t, ctx, connection)
	defer database.Close()
	actual := querySchemaStrings(t, ctx, database, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		ORDER BY table_name`)
	allowed := make(map[string]struct{}, len(governanceDisposableTables))
	for _, table := range governanceDisposableTables {
		allowed[table] = struct{}{}
	}
	for _, table := range actual {
		if _, ok := allowed[table]; !ok {
			t.Fatalf("refuse cleanup of unexpected table %q in disposable Governance schema", table)
		}
	}

	connectionHandle, err := database.Conn(ctx)
	if err != nil {
		t.Fatalf("pin Governance cleanup connection: %v", err)
	}
	defer connectionHandle.Close()
	if _, err := connectionHandle.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("disable Governance cleanup foreign-key checks: %v", err)
	}
	checksDisabled := true
	defer func() {
		if !checksDisabled {
			return
		}
		if _, err := connectionHandle.ExecContext(
			context.Background(),
			"SET FOREIGN_KEY_CHECKS = 1",
		); err != nil {
			t.Errorf("restore Governance cleanup foreign-key checks: %v", err)
		}
	}()
	for _, table := range governanceDisposableTables {
		if _, err := connectionHandle.ExecContext(ctx, "DROP TABLE IF EXISTS `"+table+"`"); err != nil {
			t.Fatalf("drop disposable Governance table %s: %v", table, err)
		}
	}
	if _, err := connectionHandle.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
		t.Fatalf("restore Governance cleanup foreign-key checks: %v", err)
	}
	checksDisabled = false
	var remaining int
	if err := connectionHandle.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()`).Scan(&remaining); err != nil {
		t.Fatalf("verify disposable Governance cleanup: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("disposable Governance cleanup left %d tables", remaining)
	}
}

type governancePreexistingFingerprint struct {
	Tables           int
	Columns          int
	Constraints      int
	IndexColumns     int
	ShowCreateSHA256 string
}

func captureGovernancePreexistingFingerprint(
	t *testing.T,
	ctx context.Context,
	connection mysqlstore.ConnectionConfig,
) governancePreexistingFingerprint {
	t.Helper()
	database := openGovernanceMigrationDatabase(t, ctx, connection)
	defer database.Close()
	tableList := "'" + strings.Join(governancePreexistingTables, "','") + "'"
	queries := []string{
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name IN (" + tableList + ")",
		"SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name IN (" + tableList + ")",
	}
	counts := make([]int, len(queries))
	for index, query := range queries {
		if err := database.QueryRowContext(ctx, query).Scan(&counts[index]); err != nil {
			t.Fatalf("capture pre-Governance schema count %d: %v", index, err)
		}
	}
	if counts[0] != len(governancePreexistingTables) {
		t.Fatalf(
			"pre-Governance tables = %d, want exact %d",
			counts[0],
			len(governancePreexistingTables),
		)
	}

	definitions := make([]string, 0, len(governancePreexistingTables))
	for _, table := range governancePreexistingTables {
		var returnedName, definition string
		if err := database.QueryRowContext(
			ctx,
			"SHOW CREATE TABLE `"+table+"`",
		).Scan(&returnedName, &definition); err != nil {
			t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
		}
		if returnedName != table {
			t.Fatalf("SHOW CREATE TABLE returned %q, want %q", returnedName, table)
		}
		definitions = append(definitions, table+"\n"+definition)
	}
	sort.Strings(definitions)
	checksum := sha256.Sum256([]byte(strings.Join(definitions, "\n--next-table--\n")))
	return governancePreexistingFingerprint{
		Tables:           counts[0],
		Columns:          counts[1],
		Constraints:      counts[2],
		IndexColumns:     counts[3],
		ShowCreateSHA256: hex.EncodeToString(checksum[:]),
	}
}

func assertGovernanceStrictMode(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var mode string
	if err := database.QueryRowContext(ctx, "SELECT @@SESSION.sql_mode").Scan(&mode); err != nil {
		t.Fatalf("read Governance MySQL sql_mode: %v", err)
	}
	if !strings.Contains(mode, "STRICT_TRANS_TABLES") && !strings.Contains(mode, "STRICT_ALL_TABLES") {
		t.Fatalf("Governance schema acceptance requires strict MySQL mode, got %q", mode)
	}
}

func assertGovernanceTables(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(table_name, '=', engine, '/', table_collation)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		ORDER BY table_name`)
	want := []string{
		"governance_active_policy=InnoDB/ascii_bin",
		"governance_authorization_audit=InnoDB/ascii_bin",
		"governance_authorization_audit_match=InnoDB/ascii_bin",
		"governance_policy_activation=InnoDB/ascii_bin",
		"governance_policy_revision=InnoDB/ascii_bin",
		"governance_policy_role=InnoDB/ascii_bin",
		"governance_policy_role_binding=InnoDB/ascii_bin",
		"governance_policy_role_permission=InnoDB/ascii_bin",
	}
	assertGovernanceStrings(t, "Governance tables", actual, want)
}

func assertGovernanceColumns(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	counts := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(table_name, '=', COUNT(*))
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		GROUP BY table_name
		ORDER BY table_name`)
	wantCounts := []string{
		"governance_active_policy=7",
		"governance_authorization_audit=25",
		"governance_authorization_audit_match=12",
		"governance_policy_activation=7",
		"governance_policy_revision=6",
		"governance_policy_role=4",
		"governance_policy_role_binding=15",
		"governance_policy_role_permission=7",
	}
	assertGovernanceStrings(t, "Governance column counts", counts, wantCounts)

	nullableSource := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(table_name, '.', column_name)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		  AND is_nullable = 'YES'
		  AND extra NOT LIKE '%GENERATED%'
		ORDER BY table_name, ordinal_position`)
	wantNullable := []string{
		"governance_authorization_audit.resource_id",
		"governance_authorization_audit.resource_tenant_id",
		"governance_authorization_audit.resource_owner_kind",
		"governance_authorization_audit.resource_owner_id",
		"governance_policy_role_binding.scope_tenant_id",
		"governance_policy_role_binding.scope_resource_type",
		"governance_policy_role_binding.scope_resource_id",
	}
	assertGovernanceStrings(t, "Governance nullable union columns", nullableSource, wantNullable)

	generatedScopeKeys := querySchemaStrings(t, ctx, database, `
		SELECT CONCAT(column_name, '=', extra)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'governance_policy_role_binding'
		  AND column_name IN (
			'scope_tenant_key',
			'scope_resource_type_key',
			'scope_resource_id_key'
		  )
		ORDER BY ordinal_position`)
	wantGeneratedScopeKeys := []string{
		"scope_tenant_key=STORED GENERATED",
		"scope_resource_type_key=STORED GENERATED",
		"scope_resource_id_key=STORED GENERATED",
	}
	assertGovernanceStrings(
		t,
		"Governance normalized scope keys",
		generatedScopeKeys,
		wantGeneratedScopeKeys,
	)

	var nonASCIIColumns int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		  AND character_set_name IS NOT NULL
		  AND (character_set_name <> 'ascii' OR collation_name <> 'ascii_bin')`).Scan(&nonASCIIColumns); err != nil {
		t.Fatalf("inspect Governance character columns: %v", err)
	}
	if nonASCIIColumns != 0 {
		t.Fatalf("Governance non-binary ASCII character columns = %d, want zero", nonASCIIColumns)
	}
}

func assertGovernanceForeignKeys(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := querySchemaStrings(t, ctx, database, `
		SELECT constraint_name
		FROM information_schema.referential_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		ORDER BY constraint_name`)
	want := []string{
		"fk_governance_active_policy_activation",
		"fk_governance_audit_match_audit",
		"fk_governance_audit_match_binding",
		"fk_governance_audit_match_permission",
		"fk_governance_authorization_audit_activation",
		"fk_governance_policy_activation_revision",
		"fk_governance_policy_role_revision",
		"fk_governance_role_binding_role",
		"fk_governance_role_permission_role",
	}
	assertGovernanceStrings(t, "Governance foreign keys", actual, want)

	var nonRestrict int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.referential_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		  AND (update_rule <> 'RESTRICT' OR delete_rule <> 'RESTRICT')`).Scan(&nonRestrict); err != nil {
		t.Fatalf("inspect Governance foreign-key rules: %v", err)
	}
	if nonRestrict != 0 {
		t.Fatalf("Governance non-RESTRICT foreign keys = %d, want zero", nonRestrict)
	}
}

func assertGovernanceChecks(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	actual := querySchemaStrings(t, ctx, database, `
		SELECT constraint_name
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name LIKE 'governance\_%' ESCAPE '\\'
		  AND constraint_type = 'CHECK'
		ORDER BY constraint_name`)
	want := []string{
		"chk_governance_active_policy_slot",
		"chk_governance_active_policy_state",
		"chk_governance_authorization_audit_activation",
		"chk_governance_authorization_audit_authentication",
		"chk_governance_authorization_audit_capability",
		"chk_governance_authorization_audit_decision",
		"chk_governance_authorization_audit_principal",
		"chk_governance_authorization_audit_refs",
		"chk_governance_authorization_audit_resource_shape",
		"chk_governance_authorization_audit_tenant",
		"chk_governance_policy_activation_shape",
		"chk_governance_policy_revision_content",
		"chk_governance_policy_revision_identity",
		"chk_governance_policy_revision_publication",
		"chk_governance_policy_role_id",
		"chk_governance_role_binding_identity",
		"chk_governance_role_binding_scope_shape",
		"chk_governance_role_permission_capability",
		"chk_governance_role_permission_ceiling",
	}
	assertGovernanceStrings(t, "Governance checks", actual, want)
}

func assertGovernanceRoundTripAndRejectedRows(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
) {
	t.Helper()
	digest := make([]byte, 32)
	digest[0] = 1
	publishedAt := time.Date(2026, 9, 3, 1, 0, 0, 123456000, time.UTC)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin Governance round trip: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	statements := []struct {
		query string
		args  []any
	}{
		{
			query: `INSERT INTO governance_policy_revision
				(policy_id, policy_revision, schema_version, content_digest,
				 publication_reference, published_at)
				VALUES (?, ?, ?, ?, ?, ?)`,
			args: []any{"workforce-policy", uint64(1), uint16(1), digest, "publication-1", publishedAt},
		},
		{
			query: `INSERT INTO governance_policy_role
				(policy_id, policy_revision, role_id)
				VALUES
					('workforce-policy', 1, 'platform_administrator'),
					('workforce-policy', 1, 'growth_member')`,
		},
		{
			query: `INSERT INTO governance_policy_role_permission
				(policy_id, policy_revision, role_id, resource_kind, resource_type, action)
				VALUES
					('workforce-policy', 1, 'platform_administrator', 'object', 'lottery.strategy', 'read'),
					('workforce-policy', 1, 'platform_administrator', 'object', 'lottery.strategy', 'simulate')`,
		},
		{
			query: `INSERT INTO governance_policy_role_binding
				(policy_id, policy_revision, binding_id, principal_kind, principal_id,
				 role_id, scope_kind, binding_effect)
				VALUES
					('workforce-policy', 1, 'binding-admin', 'human', 'principal-admin',
					 'platform_administrator', 'system', 'allow'),
					('workforce-policy', 1, 'binding-other', 'human', 'principal-other',
					 'platform_administrator', 'system', 'allow')`,
		},
		{
			query: `INSERT INTO governance_policy_activation
				(policy_slot, state_version, activation_reference, policy_id,
				 policy_revision, policy_content_digest, activated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				"workforce_http", uint64(1), "activation-1", "workforce-policy",
				uint64(1), digest, publishedAt.Add(time.Second),
			},
		},
		{
			query: `INSERT INTO governance_active_policy
				(policy_slot, state_version, activation_reference, policy_id,
				 policy_revision, policy_content_digest, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				"workforce_http", uint64(1), "activation-1", "workforce-policy",
				uint64(1), digest, publishedAt.Add(time.Second),
			},
		},
		{
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				"evaluation-allow-1", "correlation-1", "workforce_http", uint64(1),
				"activation-1", "workforce-policy", uint64(1), digest,
				"human", "principal-admin", "workforce_session", "session-1", uint64(7),
				"object", "lottery.strategy", "strategy-42",
				"simulate", "allow", "explicit_allow", uint16(1), publishedAt.Add(2 * time.Second),
			},
		},
		{
			query: `INSERT INTO governance_authorization_audit_match
				(evaluation_reference, policy_id, policy_revision, binding_id,
				 principal_kind, principal_id, role_id, binding_effect, scope_kind,
				 resource_kind, resource_type, action)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				"evaluation-allow-1", "workforce-policy", uint64(1), "binding-admin",
				"human", "principal-admin", "platform_administrator", "allow", "system",
				"object", "lottery.strategy", "simulate",
			},
		},
		{
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			args: []any{
				"evaluation-deny-1", "correlation-2", "workforce_http", uint64(1),
				"activation-1", "workforce-policy", uint64(1), digest,
				"human", "principal-unbound", "workforce_session", "session-2", uint64(3),
				"object", "lottery.strategy", "strategy-42",
				"simulate", "deny", "no_binding", uint16(0), publishedAt.Add(3 * time.Second),
			},
		},
	}
	for index, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("execute Governance round-trip statement %d: %v", index, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit Governance round trip: %v", err)
	}

	var policies, roles, permissions, bindings, activations, active, audits, matches int
	for index, target := range []*int{
		&policies, &roles, &permissions, &bindings, &activations, &active, &audits, &matches,
	} {
		table := []string{
			"governance_policy_revision",
			"governance_policy_role",
			"governance_policy_role_permission",
			"governance_policy_role_binding",
			"governance_policy_activation",
			"governance_active_policy",
			"governance_authorization_audit",
			"governance_authorization_audit_match",
		}[index]
		if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(target); err != nil {
			t.Fatalf("count Governance round-trip table %s: %v", table, err)
		}
	}
	if policies != 1 || roles != 2 || permissions != 2 || bindings != 2 ||
		activations != 1 || active != 1 || audits != 2 || matches != 1 {
		t.Fatalf(
			"Governance round-trip counts = %d/%d/%d/%d/%d/%d/%d/%d, want 1/2/2/2/1/1/2/1",
			policies, roles, permissions, bindings, activations, active, audits, matches,
		)
	}
	var tenantKey, resourceTypeKey, resourceIDKey string
	if err := database.QueryRowContext(ctx, `
		SELECT scope_tenant_key, scope_resource_type_key, scope_resource_id_key
		FROM governance_policy_role_binding
		WHERE policy_id = 'workforce-policy'
		  AND policy_revision = 1
		  AND binding_id = 'binding-admin'`).Scan(
		&tenantKey,
		&resourceTypeKey,
		&resourceIDKey,
	); err != nil {
		t.Fatalf("read generated Governance scope keys: %v", err)
	}
	if tenantKey != "" || resourceTypeKey != "" || resourceIDKey != "" {
		t.Fatalf(
			"system scope normalized keys = %q/%q/%q, want three empty sentinels",
			tenantKey,
			resourceTypeKey,
			resourceIDKey,
		)
	}

	assertGovernanceActivePolicyCAS(t, ctx, database, digest, publishedAt)
	assertGovernanceRejectedRows(t, ctx, database, digest, publishedAt)
}

func assertGovernanceActivePolicyCAS(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	digest []byte,
	base time.Time,
) {
	t.Helper()
	badDigest := append([]byte(nil), digest...)
	badDigest[0] = 2

	_, err := database.ExecContext(ctx, `
		INSERT INTO governance_policy_activation
			(policy_slot, state_version, activation_reference, policy_id,
			 policy_revision, policy_content_digest, activated_at)
		VALUES ('workforce_http', 2, 'activation-1', 'workforce-policy', 1, ?, ?)`,
		digest,
		base.Add(4*time.Second),
	)
	assertGovernanceMySQLErrorCode(t, err, 1062)

	_, err = database.ExecContext(ctx, `
		INSERT INTO governance_policy_activation
			(policy_slot, state_version, activation_reference, policy_id,
			 policy_revision, policy_content_digest, activated_at)
		VALUES ('workforce_http', 2, 'activation-bad-digest', 'workforce-policy', 1, ?, ?)`,
		badDigest,
		base.Add(4*time.Second),
	)
	assertGovernanceMySQLErrorCode(t, err, 1452)

	if _, err := database.ExecContext(ctx, `
		INSERT INTO governance_policy_activation
			(policy_slot, state_version, activation_reference, policy_id,
			 policy_revision, policy_content_digest, activated_at)
		VALUES ('workforce_http', 2, 'activation-2', 'workforce-policy', 1, ?, ?)`,
		digest,
		base.Add(4*time.Second),
	); err != nil {
		t.Fatalf("append second Governance activation event: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		UPDATE governance_active_policy
		SET state_version = 2,
		    activation_reference = 'activation-1',
		    policy_id = 'workforce-policy',
		    policy_revision = 1,
		    policy_content_digest = ?,
		    updated_at = ?
		WHERE policy_slot = 'workforce_http'
		  AND state_version = 1`, digest, base.Add(4*time.Second))
	assertGovernanceMySQLErrorCode(t, err, 1452)

	_, err = database.ExecContext(ctx, `
		UPDATE governance_active_policy
		SET state_version = 2,
		    activation_reference = 'activation-2',
		    policy_id = 'workforce-policy',
		    policy_revision = 1,
		    policy_content_digest = ?,
		    updated_at = ?
		WHERE policy_slot = 'workforce_http'
		  AND state_version = 1`, badDigest, base.Add(4*time.Second))
	assertGovernanceMySQLErrorCode(t, err, 1452)

	result, err := database.ExecContext(ctx, `
		UPDATE governance_active_policy
		SET state_version = 2,
		    activation_reference = 'activation-2',
		    policy_id = 'workforce-policy',
		    policy_revision = 1,
		    policy_content_digest = ?,
		    updated_at = ?
		WHERE policy_slot = 'workforce_http'
		  AND state_version = 1`, digest, base.Add(4*time.Second))
	if err != nil {
		t.Fatalf("advance active Governance policy with expected state version: %v", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("active Governance CAS affected = %d/%v, want 1/nil", affected, err)
	}
	stale, err := database.ExecContext(ctx, `
		UPDATE governance_active_policy
		SET state_version = 2,
		    activation_reference = 'activation-2',
		    policy_id = 'workforce-policy',
		    policy_revision = 1,
		    policy_content_digest = ?,
		    updated_at = ?
		WHERE policy_slot = 'workforce_http'
		  AND state_version = 1`, digest, base.Add(5*time.Second))
	if err != nil {
		t.Fatalf("execute stale active Governance CAS: %v", err)
	}
	if affected, err := stale.RowsAffected(); err != nil || affected != 0 {
		t.Fatalf("stale active Governance CAS affected = %d/%v, want 0/nil", affected, err)
	}

	var activationEvents, activeVersion int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM governance_policy_activation
		WHERE policy_slot = 'workforce_http'`).Scan(&activationEvents); err != nil {
		t.Fatalf("count retained Governance activation events: %v", err)
	}
	if err := database.QueryRowContext(ctx, `
		SELECT state_version
		FROM governance_active_policy
		WHERE policy_slot = 'workforce_http'`).Scan(&activeVersion); err != nil {
		t.Fatalf("read active Governance state version: %v", err)
	}
	if activationEvents != 2 || activeVersion != 2 {
		t.Fatalf(
			"Governance activation history/current = %d/%d, want 2/2",
			activationEvents,
			activeVersion,
		)
	}
	_, err = database.ExecContext(ctx, `
		DELETE FROM governance_policy_activation
		WHERE policy_slot = 'workforce_http'
		  AND state_version = 1`)
	assertGovernanceMySQLErrorCode(t, err, 1451)
}

func assertGovernanceRejectedRows(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	digest []byte,
	base time.Time,
) {
	t.Helper()
	zeroDigest := make([]byte, 32)
	badDigest := append([]byte(nil), digest...)
	badDigest[0] = 2
	tests := []struct {
		name     string
		wantCode uint16
		query    string
		args     []any
	}{
		{
			name:     "non-canonical policy identity",
			wantCode: 3819,
			query: `INSERT INTO governance_policy_revision
				(policy_id, policy_revision, schema_version, content_digest,
				 publication_reference, published_at)
				VALUES ('Uppercase', 2, 1, ?, 'publication-uppercase', ?)`,
			args: []any{digest, base},
		},
		{
			name:     "zero policy digest",
			wantCode: 3819,
			query: `INSERT INTO governance_policy_revision
				(policy_id, policy_revision, schema_version, content_digest,
				 publication_reference, published_at)
				VALUES ('zero-digest', 1, 1, ?, 'publication-zero-digest', ?)`,
			args: []any{zeroDigest, base},
		},
		{
			name:     "growth member capability exceeds ceiling",
			wantCode: 3819,
			query: `INSERT INTO governance_policy_role_permission
				(policy_id, policy_revision, role_id, resource_kind, resource_type, action)
				VALUES ('workforce-policy', 1, 'growth_member', 'object', 'lottery.strategy', 'read')`,
		},
		{
			name:     "collection simulate is outside capability catalog",
			wantCode: 3819,
			query: `INSERT INTO governance_policy_role_permission
				(policy_id, policy_revision, role_id, resource_kind, resource_type, action)
				VALUES ('workforce-policy', 1, 'platform_administrator', 'collection', 'lottery.strategy', 'simulate')`,
		},
		{
			name:     "tenant scope lacks tenant",
			wantCode: 3819,
			query: `INSERT INTO governance_policy_role_binding
				(policy_id, policy_revision, binding_id, principal_kind, principal_id,
				 role_id, scope_kind, binding_effect)
				VALUES ('workforce-policy', 1, 'binding-no-tenant', 'human', 'principal-2',
				 'platform_administrator', 'tenant', 'allow')`,
		},
		{
			name:     "system scope carries tenant",
			wantCode: 3819,
			query: `INSERT INTO governance_policy_role_binding
				(policy_id, policy_revision, binding_id, principal_kind, principal_id,
				 role_id, scope_kind, scope_tenant_id, binding_effect)
				VALUES ('workforce-policy', 1, 'binding-system-tenant', 'human', 'principal-3',
				 'platform_administrator', 'system', 'tenant-1', 'allow')`,
		},
		{
			name:     "semantic duplicate binding uses a different id",
			wantCode: 1062,
			query: `INSERT INTO governance_policy_role_binding
				(policy_id, policy_revision, binding_id, principal_kind, principal_id,
				 role_id, scope_kind, binding_effect)
				VALUES ('workforce-policy', 1, 'binding-admin-alias', 'human', 'principal-admin',
				 'platform_administrator', 'system', 'allow')`,
		},
		{
			name:     "active policy rejects zero state version",
			wantCode: 3819,
			query: `INSERT INTO governance_active_policy
				(policy_slot, state_version, activation_reference, policy_id,
				 policy_revision, policy_content_digest, updated_at)
				VALUES ('other', 0, 'activation-invalid', 'workforce-policy', 1, ?, ?)`,
			args: []any{digest, base},
		},
		{
			name:     "default deny cannot declare matches",
			wantCode: 3819,
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES ('evaluation-bad-count', 'correlation-3', 'workforce_http',
				 1, 'activation-1', 'workforce-policy', 1, ?, 'human', 'principal-unbound',
				 'workforce_session', 'session-bad-count', 1, 'object', 'lottery.strategy',
				 'strategy-42', 'simulate', 'deny', 'no_binding', 1, ?)`,
			args: []any{digest, base},
		},
		{
			name:     "collection audit cannot carry object id",
			wantCode: 3819,
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES ('evaluation-bad-resource', 'correlation-4', 'workforce_http',
				 1, 'activation-1', 'workforce-policy', 1, ?, 'human', 'principal-unbound',
				 'workforce_session', 'session-bad-resource', 1, 'collection', 'lottery.strategy',
				 'strategy-42', 'read', 'deny', 'no_binding', 0, ?)`,
			args: []any{digest, base},
		},
		{
			name:     "audit rejects unregistered capability tuple",
			wantCode: 3819,
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES ('evaluation-bad-capability', 'correlation-5', 'workforce_http',
				 1, 'activation-1', 'workforce-policy', 1, ?, 'human', 'principal-unbound',
				 'workforce_session', 'session-bad-capability', 1, 'object',
				 'lottery.routing_graph', 'graph-1', 'simulate', 'deny', 'no_binding', 0, ?)`,
			args: []any{digest, base},
		},
		{
			name:     "human audit rejects service credential provenance",
			wantCode: 3819,
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES ('evaluation-bad-auth', 'correlation-6', 'workforce_http',
				 1, 'activation-1', 'workforce-policy', 1, ?, 'human', 'principal-unbound',
				 'service_credential', 'credential-1', 1, 'object', 'lottery.strategy',
				 'strategy-42', 'simulate', 'deny', 'no_binding', 0, ?)`,
			args: []any{digest, base},
		},
		{
			name:     "audit cannot replace observed policy digest",
			wantCode: 1452,
			query: `INSERT INTO governance_authorization_audit
				(evaluation_reference, correlation_reference, policy_slot,
				 activation_state_version, activation_reference, policy_id, policy_revision,
				 policy_content_digest, principal_kind, principal_id, authentication_kind,
				 authentication_reference, authentication_epoch, resource_kind, resource_type,
				 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES ('evaluation-bad-digest', 'correlation-7', 'workforce_http',
				 1, 'activation-1', 'workforce-policy', 1, ?, 'human', 'principal-unbound',
				 'workforce_session', 'session-bad-digest', 1, 'object', 'lottery.strategy',
				 'strategy-42', 'simulate', 'deny', 'no_binding', 0, ?)`,
			args: []any{badDigest, base},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := database.ExecContext(ctx, test.query, test.args...)
			assertGovernanceMySQLErrorCode(t, err, test.wantCode)
		})
	}
	assertGovernanceRejectedAuditMatches(t, ctx, database, digest, base)
}

func assertGovernanceRejectedAuditMatches(
	t *testing.T,
	ctx context.Context,
	database *sql.DB,
	digest []byte,
	base time.Time,
) {
	t.Helper()
	tests := []struct {
		name                string
		evaluationReference string
		bindingID           string
		principalID         string
		bindingEffect       string
		scopeKind           string
		action              string
	}{
		{
			name:                "audit match cannot replace evaluated capability",
			evaluationReference: "evaluation-match-capability",
			bindingID:           "binding-admin",
			principalID:         "principal-admin",
			bindingEffect:       "allow",
			scopeKind:           "system",
			action:              "read",
		},
		{
			name:                "audit match cannot forge binding effect",
			evaluationReference: "evaluation-match-effect",
			bindingID:           "binding-admin",
			principalID:         "principal-admin",
			bindingEffect:       "deny",
			scopeKind:           "system",
			action:              "simulate",
		},
		{
			name:                "audit match cannot forge binding scope",
			evaluationReference: "evaluation-match-scope",
			bindingID:           "binding-admin",
			principalID:         "principal-admin",
			bindingEffect:       "allow",
			scopeKind:           "tenant",
			action:              "simulate",
		},
		{
			name:                "audit match cannot cross principals",
			evaluationReference: "evaluation-match-principal",
			bindingID:           "binding-other",
			principalID:         "principal-other",
			bindingEffect:       "allow",
			scopeKind:           "system",
			action:              "simulate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx, err := database.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("begin rejected audit-match probe: %v", err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO governance_authorization_audit
					(evaluation_reference, correlation_reference, policy_slot,
					 activation_state_version, activation_reference, policy_id, policy_revision,
					 policy_content_digest, principal_kind, principal_id, authentication_kind,
					 authentication_reference, authentication_epoch, resource_kind, resource_type,
					 resource_id, action, decision_outcome, decision_reason, match_count, evaluated_at)
				VALUES (?, 'correlation-match-probe', 'workforce_http',
					1, 'activation-1', 'workforce-policy', 1, ?, 'human', 'principal-admin',
					'workforce_session', 'session-match-probe', 1, 'object', 'lottery.strategy',
					'strategy-42', 'simulate', 'allow', 'explicit_allow', 1, ?)`,
				test.evaluationReference,
				digest,
				base,
			); err != nil {
				t.Fatalf("insert rejected audit-match parent: %v", err)
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO governance_authorization_audit_match
					(evaluation_reference, policy_id, policy_revision, binding_id,
					 principal_kind, principal_id, role_id, binding_effect, scope_kind,
					 resource_kind, resource_type, action)
				VALUES (?, 'workforce-policy', 1, ?, 'human', ?,
					'platform_administrator', ?, ?, 'object', 'lottery.strategy', ?)`,
				test.evaluationReference,
				test.bindingID,
				test.principalID,
				test.bindingEffect,
				test.scopeKind,
				test.action,
			)
			assertGovernanceMySQLErrorCode(t, err, 1452)
		})
	}
}

func assertGovernanceMySQLErrorCode(t *testing.T, err error, want uint16) {
	t.Helper()
	if err == nil {
		t.Fatalf("write succeeded, want MySQL error %d", want)
	}
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) {
		t.Fatalf("error = %T %v, want MySQL error %d", err, err, want)
	}
	if mysqlError.Number != want {
		t.Fatalf("MySQL error = %d (%v), want %d", mysqlError.Number, err, want)
	}
}

func assertGovernanceStrings(t *testing.T, label string, actual, want []string) {
	t.Helper()
	if strings.Join(actual, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s = %q, want %q", label, actual, want)
	}
}
