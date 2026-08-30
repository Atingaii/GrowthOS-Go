package migrations_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	projectmigrations "github.com/Atingaii/GrowthOS-Go/migrations"
)

func TestStrategyRoutingGraphSchemaMySQLIntegration(t *testing.T) {
	migrationConnection := schemaIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	applicationConnection := schemaIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_API")
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-19-isolated-schema" {
		t.Skip("schema integration requires explicit disposable-schema authorization")
	}
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES") != "lesson-19-isolated-repository" {
		t.Skip("schema integration requires explicit repository-write authorization")
	}
	if migrationConnection.Address != applicationConnection.Address ||
		migrationConnection.Database != applicationConnection.Database {
		t.Fatal("application and migration integration identities must target the same isolated schema")
	}
	if migrationConnection.User == applicationConnection.User {
		t.Fatal("application and migration integration identities must be distinct")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	migrationDatabase, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: migrationConnection,
		StatementTimeout: 15 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	runner, err := dbmigration.New(ctx, projectmigrations.Files, migrationDatabase, dbmigration.Config{
		LockTimeout:        30 * time.Second,
		NetworkReadTimeout: 25 * time.Second,
		StatementTimeout:   15 * time.Second,
	})
	if err != nil {
		t.Fatalf("construct migration runner: %v", err)
	}
	if _, err := runner.Up(ctx); err != nil {
		_ = runner.Close()
		t.Fatalf("apply embedded migrations: %v", err)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read migration status: %v", err)
	}
	if status.State != dbmigration.StatusClean || status.Version != 11 || status.Latest != 11 {
		_ = runner.Close()
		t.Fatalf("migration status = %+v, want clean at exact current version 11", status)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("close migration runner: %v", err)
	}

	applicationDatabase, err := mysqlstore.Open(ctx, mysqlstore.Config{
		ConnectionConfig:      applicationConnection,
		PingTimeout:           5 * time.Second,
		MaxOpenConnections:    2,
		MaxIdleConnections:    1,
		ConnectionMaxLifetime: time.Minute,
		ConnectionMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open application database: %v", err)
	}
	defer applicationDatabase.Close()

	for _, table := range []string{
		"lottery_strategy_routing_graph",
		"lottery_strategy_routing_node",
		"lottery_strategy_routing_edge",
	} {
		var rows int
		if err := applicationDatabase.GetContext(
			ctx,
			&rows,
			"SELECT COUNT(*) FROM "+table+" WHERE 1 = 0",
		); err == nil {
			t.Fatalf("application identity unexpectedly read %s", table)
		} else {
			expectMySQLErrorNumber(t, err, 1142)
		}
	}
	if _, err := applicationDatabase.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		 VALUES (1, 'permission-probe-v1', 1, 1)`,
	); err == nil {
		t.Fatal("application identity unexpectedly inserted a routing graph")
	} else {
		expectMySQLErrorNumber(t, err, 1142)
	}
	assertExactApplicationGrants(t, ctx, applicationDatabase, applicationConnection.Database)

	verificationDatabase, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: migrationConnection,
		StatementTimeout: 15 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open schema verification database: %v", err)
	}
	defer verificationDatabase.Close()

	var matchingTables int
	if err := verificationDatabase.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'lottery_strategy_routing_graph',
			'lottery_strategy_routing_node',
			'lottery_strategy_routing_edge'
		  )
		  AND engine = 'InnoDB'
		  AND table_collation = 'utf8mb4_0900_bin'
	`).Scan(&matchingTables); err != nil {
		t.Fatalf("inspect routing graph tables: %v", err)
	}
	if matchingTables != 3 {
		t.Fatalf("matching InnoDB routing graph tables = %d, want 3", matchingTables)
	}

	var updatedAtColumns int
	if err := verificationDatabase.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'lottery_strategy_routing_graph',
			'lottery_strategy_routing_node',
			'lottery_strategy_routing_edge'
		  )
		  AND column_name = 'updated_at'
	`).Scan(&updatedAtColumns); err != nil {
		t.Fatalf("inspect routing graph update timestamps: %v", err)
	}
	if updatedAtColumns != 0 {
		t.Fatalf("routing graph updated_at columns = %d, want create-only schema", updatedAtColumns)
	}

	tokenColumns := querySchemaStrings(t, ctx, verificationDatabase, `
		SELECT CONCAT(
			table_name, '.', column_name, '=',
			character_set_name, '/', collation_name
		)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND (
			(table_name = 'lottery_strategy_routing_graph'
			 AND column_name = 'revision')
			OR
			(table_name = 'lottery_strategy_routing_node'
			 AND column_name IN ('revision', 'node_kind', 'rule_code'))
			OR
			(table_name = 'lottery_strategy_routing_edge'
			 AND column_name IN ('revision', 'branch_code'))
		  )
		ORDER BY table_name, column_name
	`)
	wantTokenColumns := []string{
		"lottery_strategy_routing_edge.branch_code=ascii/ascii_bin",
		"lottery_strategy_routing_edge.revision=ascii/ascii_bin",
		"lottery_strategy_routing_graph.revision=ascii/ascii_bin",
		"lottery_strategy_routing_node.node_kind=ascii/ascii_bin",
		"lottery_strategy_routing_node.revision=ascii/ascii_bin",
		"lottery_strategy_routing_node.rule_code=ascii/ascii_bin",
	}
	if strings.Join(tokenColumns, "\n") != strings.Join(wantTokenColumns, "\n") {
		t.Fatalf("routing token columns = %q, want exact ASCII/binary columns %q", tokenColumns, wantTokenColumns)
	}

	var routingForeignKeys int
	if err := verificationDatabase.QueryRowContext(ctx, `
		SELECT COUNT(*)
			FROM information_schema.referential_constraints
			WHERE constraint_schema = DATABASE()
			  AND constraint_name IN (
			'fk_lottery_strategy_routing_node_graph',
			'fk_lottery_strategy_routing_node_strategy',
			'fk_lottery_strategy_routing_edge_from_node',
			'fk_lottery_strategy_routing_edge_to_node'
		  )
		  AND update_rule = 'RESTRICT'
		  AND delete_rule = 'RESTRICT'
	`).Scan(&routingForeignKeys); err != nil {
		t.Fatalf("inspect routing graph foreign keys: %v", err)
	}
	if routingForeignKeys != 4 {
		t.Fatalf("named routing RESTRICT foreign keys = %d, want 4", routingForeignKeys)
	}

	routingForeignKeyColumns := querySchemaStrings(t, ctx, verificationDatabase, `
		SELECT CONCAT(
			constraint_name, '#', LPAD(ordinal_position, 2, '0'), ':',
			table_name, '.', column_name, '->',
			referenced_table_name, '.', referenced_column_name
		)
			FROM information_schema.key_column_usage
			WHERE constraint_schema = DATABASE()
			  AND referenced_table_schema = DATABASE()
			  AND constraint_name IN (
			'fk_lottery_strategy_routing_node_graph',
			'fk_lottery_strategy_routing_node_strategy',
			'fk_lottery_strategy_routing_edge_from_node',
			'fk_lottery_strategy_routing_edge_to_node'
		  )
		ORDER BY constraint_name, ordinal_position
	`)
	wantRoutingForeignKeyColumns := []string{
		"fk_lottery_strategy_routing_edge_from_node#01:lottery_strategy_routing_edge.graph_id->lottery_strategy_routing_node.graph_id",
		"fk_lottery_strategy_routing_edge_from_node#02:lottery_strategy_routing_edge.revision->lottery_strategy_routing_node.revision",
		"fk_lottery_strategy_routing_edge_from_node#03:lottery_strategy_routing_edge.from_node_id->lottery_strategy_routing_node.node_id",
		"fk_lottery_strategy_routing_edge_to_node#01:lottery_strategy_routing_edge.graph_id->lottery_strategy_routing_node.graph_id",
		"fk_lottery_strategy_routing_edge_to_node#02:lottery_strategy_routing_edge.revision->lottery_strategy_routing_node.revision",
		"fk_lottery_strategy_routing_edge_to_node#03:lottery_strategy_routing_edge.to_node_id->lottery_strategy_routing_node.node_id",
		"fk_lottery_strategy_routing_node_graph#01:lottery_strategy_routing_node.graph_id->lottery_strategy_routing_graph.graph_id",
		"fk_lottery_strategy_routing_node_graph#02:lottery_strategy_routing_node.revision->lottery_strategy_routing_graph.revision",
		"fk_lottery_strategy_routing_node_strategy#01:lottery_strategy_routing_node.strategy_id->lottery_strategy.strategy_id",
	}
	if strings.Join(routingForeignKeyColumns, "\n") != strings.Join(wantRoutingForeignKeyColumns, "\n") {
		t.Fatalf("routing foreign-key columns = %q, want exact ordered references %q", routingForeignKeyColumns, wantRoutingForeignKeyColumns)
	}

	var rootNodeForeignKeys int
	if err := verificationDatabase.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE()
		  AND table_name = 'lottery_strategy_routing_graph'
		  AND column_name = 'root_node_id'
		  AND referenced_table_name IS NOT NULL
	`).Scan(&rootNodeForeignKeys); err != nil {
		t.Fatalf("inspect routing graph root reference: %v", err)
	}
	if rootNodeForeignKeys != 0 {
		t.Fatalf("root_node_id foreign keys = %d, want logical reference validated after restore", rootNodeForeignKeys)
	}

	tx, err := verificationDatabase.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin routing schema verification transaction: %v", err)
	}
	transactionOpen := true
	// This defer only contains fixtures after an already-failed assertion. A
	// passing test must reach the explicit rollback and outside probes below.
	defer func() {
		if !transactionOpen {
			return
		}
		if err := tx.Rollback(); err != nil {
			t.Errorf("rollback routing schema verification transaction after failure: %v", err)
		}
	}()

	strategyID := uint64(time.Now().UnixNano())
	premiumStrategyID := strategyID + 1
	graphID := strategyID + 2
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, 'routing baseline'), (?, 'routing premium')",
		strategyID,
		premiumStrategyID,
	); err != nil {
		t.Fatalf("insert routing target strategies: %v", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		 VALUES (?, 'membership-route-v1', 1, 10)`,
		graphID,
	); err != nil {
		t.Fatalf("insert routing graph header before its root node: %v", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_node
			(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
		 VALUES
			(?, 'membership-route-v1', 10, 'decision', 'lottery.membership_tier.route_strategy', NULL),
			(?, 'membership-route-v1', 20, 'strategy_target', NULL, ?),
			(?, 'membership-route-v1', 30, 'strategy_target', NULL, ?)`,
		graphID,
		graphID,
		premiumStrategyID,
		graphID,
		strategyID,
	); err != nil {
		t.Fatalf("insert valid routing nodes: %v", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_edge
			(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
		 VALUES
			(?, 'membership-route-v1', 10, 'premium_override', 20, 0),
			(?, 'membership-route-v1', 10, 'baseline_default', 30, 1)`,
		graphID,
		graphID,
	); err != nil {
		t.Fatalf("insert valid routing edges: %v", err)
	}

	var edgeCount int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM lottery_strategy_routing_edge
		 WHERE graph_id = ? AND revision = 'membership-route-v1'`,
		graphID,
	).Scan(&edgeCount); err != nil {
		t.Fatalf("read valid routing edges: %v", err)
	}
	if edgeCount != 2 {
		t.Fatalf("stored routing edges = %d, want 2", edgeCount)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		 VALUES (?, 'staged-route-v2', 1, 999)`,
		graphID,
	); err != nil {
		t.Fatalf("insert same graph id at another revision before its root node: %v", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_node
			(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
		 VALUES (?, 'staged-route-v2', 10, 'decision', 'lottery.membership_tier.route_strategy', NULL)`,
		graphID,
	); err != nil {
		t.Fatalf("insert source node in staged revision: %v", err)
	}
	expectSchemaError(t, 1452, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
			VALUES (?, 'staged-route-v2', 10, 'premium_override', 20, 0)
		`, graphID)
		return err
	})
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_routing_graph
			(graph_id, revision, schema_version, root_node_id)
		 VALUES (?, 'Membership-route-v1', 1, 10)`,
		graphID,
	); err != nil {
		t.Fatalf("insert case-distinct graph revision: %v", err)
	}
	var caseDistinctRevisions int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM lottery_strategy_routing_graph
		 WHERE graph_id = ?
		   AND revision IN ('membership-route-v1', 'Membership-route-v1')`,
		graphID,
	).Scan(&caseDistinctRevisions); err != nil {
		t.Fatalf("count case-distinct graph revisions: %v", err)
	}
	if caseDistinctRevisions != 2 {
		t.Fatalf("case-distinct graph revisions = %d, want 2 under ascii_bin identity", caseDistinctRevisions)
	}

	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
			VALUES (0, 'invalid-v1', 1, 1)
		`)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
			VALUES (?, '-invalid-v1', 1, 1)
		`, graphID+1)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
			VALUES (?, 'invalid-v2', 2, 1)
		`, graphID+2)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
			VALUES (?, 'invalid-root-v1', 1, 0)
		`, graphID+3)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
			VALUES (?, 'invalid-trailing-space ', 1, 1)
		`, graphID+4)
		return err
	})
	expectSchemaError(t, 1062, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_graph
				(graph_id, revision, schema_version, root_node_id)
			VALUES (?, 'membership-route-v1', 1, 10)
		`, graphID)
		return err
	})

	expectSchemaError(t, 1452, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'missing-graph-v1', 1, 'decision', 'lottery.membership_tier.route_strategy', NULL)
		`, graphID)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'membership-route-v1', 0, 'decision', 'lottery.membership_tier.route_strategy', NULL)
		`, graphID)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'membership-route-v1', 44, 'decision', NULL, NULL)
		`, graphID)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'membership-route-v1', 40, 'decision', 'lottery.membership_tier.route_strategy', ?)
		`, graphID, strategyID)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'membership-route-v1', 41, 'strategy_target', 'lottery.membership_tier.route_strategy', ?)
		`, graphID, strategyID)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'membership-route-v1', 42, 'strategy_target', NULL, NULL)
		`, graphID)
		return err
	})
	expectSchemaError(t, 1452, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_node
				(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
			VALUES (?, 'membership-route-v1', 43, 'strategy_target', NULL, ?)
		`, graphID, premiumStrategyID+1000)
		return err
	})

	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
			VALUES (?, 'membership-route-v1', 10, 'premium_override', 30, 1)
		`, graphID)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
			VALUES (?, 'membership-route-v1', 10, 'unknown_branch', 30, 0)
		`, graphID)
		return err
	})
	expectSchemaError(t, 1452, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
			VALUES (?, 'membership-route-v1', 99, 'premium_override', 20, 0)
		`, graphID)
		return err
	})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lottery_strategy_routing_node
			(graph_id, revision, node_id, node_kind, rule_code, strategy_id)
		VALUES (?, 'membership-route-v1', 11, 'decision', 'lottery.membership_tier.route_strategy', NULL)
	`, graphID); err != nil {
		t.Fatalf("insert decision node for target foreign-key probe: %v", err)
	}
	expectSchemaError(t, 1452, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
			VALUES (?, 'membership-route-v1', 11, 'premium_override', 99, 0)
		`, graphID)
		return err
	})
	expectSchemaError(t, 1062, func() error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO lottery_strategy_routing_edge
				(graph_id, revision, from_node_id, branch_code, to_node_id, is_default)
			VALUES (?, 'membership-route-v1', 10, 'premium_override', 30, 0)
		`, graphID)
		return err
	})
	expectSchemaError(t, 1451, func() error {
		_, err := tx.ExecContext(ctx, `
			DELETE FROM lottery_strategy_routing_node
			WHERE graph_id = ? AND revision = 'membership-route-v1' AND node_id = 20
		`, graphID)
		return err
	})
	expectSchemaError(t, 1451, func() error {
		_, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy WHERE strategy_id = ?", premiumStrategyID)
		return err
	})
	expectSchemaError(t, 1451, func() error {
		_, err := tx.ExecContext(ctx, `
			DELETE FROM lottery_strategy_routing_graph
			WHERE graph_id = ? AND revision = 'membership-route-v1'
		`, graphID)
		return err
	})

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback routing schema verification transaction: %v", err)
	}
	transactionOpen = false

	residualChecks := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name:  "strategies",
			query: "SELECT COUNT(*) FROM lottery_strategy WHERE strategy_id IN (?, ?)",
			args:  []any{strategyID, premiumStrategyID},
		},
		{
			name:  "graph headers",
			query: "SELECT COUNT(*) FROM lottery_strategy_routing_graph WHERE graph_id = ?",
			args:  []any{graphID},
		},
		{
			name:  "graph nodes",
			query: "SELECT COUNT(*) FROM lottery_strategy_routing_node WHERE graph_id = ?",
			args:  []any{graphID},
		},
		{
			name:  "graph edges",
			query: "SELECT COUNT(*) FROM lottery_strategy_routing_edge WHERE graph_id = ?",
			args:  []any{graphID},
		},
	}
	for _, check := range residualChecks {
		var rows int
		if err := verificationDatabase.QueryRowContext(ctx, check.query, check.args...).Scan(&rows); err != nil {
			t.Fatalf("inspect rolled-back routing %s: %v", check.name, err)
		}
		if rows != 0 {
			t.Fatalf("rolled-back routing %s = %d rows, want zero residual fixture rows", check.name, rows)
		}
	}
}

func querySchemaStrings(t *testing.T, ctx context.Context, database *sql.DB, query string) []string {
	t.Helper()

	rows, err := database.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("query routing schema metadata: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close routing schema metadata rows: %v", err)
		}
	}()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan routing schema metadata: %v", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate routing schema metadata: %v", err)
	}
	return values
}
