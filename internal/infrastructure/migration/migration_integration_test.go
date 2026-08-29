package dbmigration_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
)

func TestRunnerMySQLIntegration(t *testing.T) {
	connection := integrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := mysqlstore.OpenMigration(ctx, mysqlstore.MigrationConfig{
		ConnectionConfig: connection,
		StatementTimeout: 10 * time.Second,
		LockTimeout:      25 * time.Second,
	})
	if err != nil {
		t.Fatalf("OpenMigration() error = %v", err)
	}
	table := fmt.Sprintf("growthos_migration_test_%d", time.Now().UnixNano())
	table = strings.ReplaceAll(table, "-", "_")
	var runner *dbmigration.Runner
	t.Cleanup(func() {
		if runner != nil {
			_ = runner.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		cleanupDB, openErr := mysqlstore.OpenMigration(cleanupCtx, mysqlstore.MigrationConfig{
			ConnectionConfig: connection,
			StatementTimeout: 10 * time.Second,
			LockTimeout:      25 * time.Second,
		})
		if openErr != nil {
			t.Errorf("open isolated cleanup connection: %v", openErr)
			return
		}
		defer cleanupDB.Close()
		if _, dropErr := cleanupDB.ExecContext(cleanupCtx, "DROP TABLE IF EXISTS `"+table+"`"); dropErr != nil {
			t.Errorf("remove isolated version table: %v", dropErr)
		}
	})

	runner, err = dbmigration.New(ctx, fstest.MapFS{
		"sql/000001_integration_probe.up.sql": {Data: []byte("SET @growthos_migration_probe = 1; SET @growthos_migration_probe = 2")},
	}, db, dbmigration.Config{
		MigrationsTable:    table,
		LockTimeout:        25 * time.Second,
		NetworkReadTimeout: 20 * time.Second,
		StatementTimeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := runner.Up(ctx)
	if err != nil || result.State != dbmigration.ResultApplied || result.Version != 1 {
		t.Fatalf("first Up() = %+v, error = %v", result, err)
	}
	result, err = runner.Up(ctx)
	if err != nil || result.State != dbmigration.ResultNoChange {
		t.Fatalf("second Up() = %+v, error = %v", result, err)
	}
	status, err := runner.Status(ctx)
	if err != nil || status.State != dbmigration.StatusClean || status.Version != 1 || status.Latest != 1 {
		t.Fatalf("Status() = %+v, error = %v", status, err)
	}

	if err := runner.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func integrationConnection(t *testing.T, prefix string) mysqlstore.ConnectionConfig {
	t.Helper()
	requiredSuffixes := []string{
		"_ADDRESS",
		"_DATABASE",
		"_USER",
		"_PASSWORD",
	}
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
		ReadTimeout:    20 * time.Second,
		WriteTimeout:   10 * time.Second,
	}
}
