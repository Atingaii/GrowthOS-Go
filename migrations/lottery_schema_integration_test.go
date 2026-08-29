package migrations_test

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	projectmigrations "github.com/Atingaii/GrowthOS-Go/migrations"
	drivermysql "github.com/go-sql-driver/mysql"
)

func TestLotterySchemaMySQLIntegration(t *testing.T) {
	migrationConnection := schemaIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	applicationConnection := schemaIntegrationConnection(t, "GROWTHOS_TEST_MYSQL_API")
	if os.Getenv("GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES") != "lesson-18-isolated-schema" {
		t.Skip("schema integration requires explicit disposable-schema authorization")
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
	result, err := runner.Up(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("apply embedded migrations: %v", err)
	}
	if result.State != dbmigration.ResultApplied && result.State != dbmigration.ResultNoChange {
		_ = runner.Close()
		t.Fatalf("migration result = %+v, want applied or no_change", result)
	}
	status, err := runner.Status(ctx)
	if err != nil {
		_ = runner.Close()
		t.Fatalf("read migration status: %v", err)
	}
	if status.State != dbmigration.StatusClean || status.Version < 2 || status.Version != status.Latest {
		_ = runner.Close()
		t.Fatalf("migration status = %+v, want clean at latest version >= 2", status)
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

	var visibleRows int
	if err := applicationDatabase.GetContext(
		ctx,
		&visibleRows,
		"SELECT COUNT(*) FROM lottery_strategy WHERE 1 = 0",
	); err != nil {
		t.Fatalf("application identity cannot read lottery_strategy: %v", err)
	}
	if err := applicationDatabase.GetContext(
		ctx,
		&visibleRows,
		"SELECT COUNT(*) FROM lottery_strategy_award WHERE 1 = 0",
	); err != nil {
		t.Fatalf("application identity cannot read lottery_strategy_award: %v", err)
	}
	applicationWriteProbe, err := applicationDatabase.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin application write-permission probe: %v", err)
	}
	_, writeProbeErr := applicationWriteProbe.ExecContext(
		ctx,
		"INSERT INTO lottery_strategy (strategy_id, name) VALUES (1, 'not permitted in lesson 18')",
	)
	if err := applicationWriteProbe.Rollback(); err != nil {
		t.Fatalf("rollback application write-permission probe: %v", err)
	}
	if writeProbeErr == nil {
		t.Fatal("application identity unexpectedly wrote a business table before the repository lesson")
	} else {
		expectMySQLErrorNumber(t, writeProbeErr, 1142)
	}
	if _, err := applicationDatabase.ExecContext(
		ctx,
		"SELECT version FROM schema_migrations",
	); err == nil {
		t.Fatal("application identity unexpectedly read schema_migrations")
	} else {
		expectMySQLErrorNumber(t, err, 1142)
	}
	if _, err := applicationDatabase.ExecContext(
		ctx,
		"UPDATE schema_migrations SET dirty = dirty",
	); err == nil {
		t.Fatal("application identity unexpectedly modified schema_migrations")
	} else {
		expectMySQLErrorNumber(t, err, 1142)
	}

	verificationDatabase, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: migrationConnection,
		StatementTimeout: 15 * time.Second,
		LockTimeout:      30 * time.Second,
	})
	if err != nil {
		t.Fatalf("open schema verification database: %v", err)
	}
	defer verificationDatabase.Close()

	var sqlMode string
	if err := verificationDatabase.QueryRowContext(ctx, "SELECT @@session.sql_mode").Scan(&sqlMode); err != nil {
		t.Fatalf("read SQL mode: %v", err)
	}
	if !strings.Contains(sqlMode, "STRICT_TRANS_TABLES") && !strings.Contains(sqlMode, "STRICT_ALL_TABLES") {
		t.Fatalf("schema verification requires a strict MySQL mode, got %q", sqlMode)
	}

	var matchingTables int
	if err := verificationDatabase.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name IN ('lottery_strategy', 'lottery_strategy_award')
		  AND engine = 'InnoDB'
		  AND table_collation = 'utf8mb4_0900_bin'
	`).Scan(&matchingTables); err != nil {
		t.Fatalf("inspect lottery tables: %v", err)
	}
	if matchingTables != 2 {
		t.Fatalf("matching InnoDB lottery tables = %d, want 2", matchingTables)
	}

	tx, err := verificationDatabase.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin schema verification transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	strategyID := uint64(time.Now().UnixNano())
	secondStrategyID := strategyID + 1
	maxStrategyID := uint64(math.MaxUint64)
	longName := strings.Repeat("奖", 128)
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ?), (?, ?), (?, ?)",
		strategyID,
		longName,
		secondStrategyID,
		"Unicode identity probe",
		maxStrategyID,
		"maximum unsigned identifier",
	); err != nil {
		t.Fatalf("insert valid strategies: %v", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (?, ?, ?, ?, ?)",
		maxStrategyID,
		uint64(math.MaxUint64),
		longName,
		uint64(math.MaxUint64),
		"reward",
	); err != nil {
		t.Fatalf("insert max-uint64 award: %v", err)
	}
	decomposedName := "e\u0301"
	precomposedName := "é"
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_award
			(strategy_id, award_id, name, weight, outcome)
		 VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)`,
		secondStrategyID,
		uint64(1),
		decomposedName,
		uint64(1),
		"reward",
		secondStrategyID,
		uint64(2),
		precomposedName,
		uint64(1),
		"no_reward",
	); err != nil {
		t.Fatalf("insert distinct Unicode names: %v", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO lottery_strategy_award
			(strategy_id, award_id, name, weight, outcome)
		 VALUES (?, ?, ?, ?, ?)`,
		strategyID,
		uint64(1),
		"same award id in another strategy",
		uint64(1),
		"no_reward",
	); err != nil {
		t.Fatalf("reuse strategy-scoped award id: %v", err)
	}

	var storedAwardID uint64
	var storedName string
	var storedWeight uint64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT award_id, name, weight FROM lottery_strategy_award WHERE strategy_id = ? AND award_id = ?",
		maxStrategyID,
		uint64(math.MaxUint64),
	).Scan(&storedAwardID, &storedName, &storedWeight); err != nil {
		t.Fatalf("read max-uint64 award: %v", err)
	}
	if storedAwardID != uint64(math.MaxUint64) || storedName != longName || storedWeight != uint64(math.MaxUint64) {
		t.Fatalf("stored award = id %d, name length %d, weight %d", storedAwardID, len([]rune(storedName)), storedWeight)
	}

	var decomposedMatches int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM lottery_strategy_award WHERE strategy_id = ? AND name = ?",
		secondStrategyID,
		decomposedName,
	).Scan(&decomposedMatches); err != nil {
		t.Fatalf("compare Unicode names: %v", err)
	}
	if decomposedMatches != 1 {
		t.Fatalf("decomposed-name matches = %d, want 1", decomposedMatches)
	}

	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, "INSERT INTO lottery_strategy (strategy_id, name) VALUES (0, 'invalid')")
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, "INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ' ')", strategyID+10)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, "INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ' invalid')", strategyID+11)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(ctx, "INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, 'invalid ')", strategyID+12)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (?, 0, 'invalid', 1, 'reward')",
			secondStrategyID,
		)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (?, 10, 'invalid', 0, 'reward')",
			secondStrategyID,
		)
		return err
	})
	expectSchemaError(t, 3819, func() error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (?, 11, 'invalid', 1, 'REWARD')",
			secondStrategyID,
		)
		return err
	})
	expectSchemaError(t, 1452, func() error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (?, 1, 'orphan', 1, 'reward')",
			strategyID+100,
		)
		return err
	})
	expectSchemaError(t, 1062, func() error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy_award (strategy_id, award_id, name, weight, outcome) VALUES (?, 1, 'duplicate', 1, 'reward')",
			secondStrategyID,
		)
		return err
	})
	expectSchemaError(t, 1451, func() error {
		_, err := tx.ExecContext(ctx, "DELETE FROM lottery_strategy WHERE strategy_id = ?", secondStrategyID)
		return err
	})
	expectSchemaError(t, 1406, func() error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO lottery_strategy (strategy_id, name) VALUES (?, ?)",
			strategyID+200,
			strings.Repeat("奖", 129),
		)
		return err
	})
}

func expectSchemaError(t *testing.T, number uint16, operation func() error) {
	t.Helper()
	err := operation()
	if err == nil {
		t.Fatalf("schema operation unexpectedly succeeded; want MySQL error %d", number)
	}
	expectMySQLErrorNumber(t, err, number)
}

func expectMySQLErrorNumber(t *testing.T, err error, number uint16) {
	t.Helper()
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) {
		t.Fatalf("error = %v, want MySQL error %d", err, number)
	}
	if mysqlError.Number != number {
		t.Fatalf("MySQL error number = %d, want %d", mysqlError.Number, number)
	}
}

func schemaIntegrationConnection(t *testing.T, prefix string) mysqlstore.ConnectionConfig {
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
