package mysqlstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
)

func TestMySQLPoolsIntegration(t *testing.T) {
	apiConnection := integrationConnection(t, "GROWTHOS_TEST_MYSQL_API")
	migrationConnection := integrationConnection(t, "GROWTHOS_TEST_MYSQL_MIGRATION")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	appDB, err := Open(ctx, Config{
		ConnectionConfig:      apiConnection,
		PingTimeout:           5 * time.Second,
		MaxOpenConnections:    3,
		MaxIdleConnections:    2,
		ConnectionMaxLifetime: time.Minute,
		ConnectionMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = appDB.Close() })

	var one int
	if err := appDB.GetContext(ctx, &one, "SELECT 1"); err != nil || one != 1 {
		t.Fatalf("application query: value=%d error=%v", one, err)
	}
	var session struct {
		TimeZone string `db:"time_zone"`
		Charset  string `db:"charset"`
	}
	if err := appDB.GetContext(
		ctx,
		&session,
		"SELECT @@session.time_zone AS time_zone, @@character_set_connection AS charset",
	); err != nil {
		t.Fatalf("read application session invariants: %v", err)
	}
	if session.TimeZone != "+00:00" || session.Charset != "utf8mb4" {
		t.Fatalf("application session invariants = timezone %q, charset %q", session.TimeZone, session.Charset)
	}
	if got := appDB.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("application MaxOpenConnections = %d", got)
	}

	migrationDB, err := OpenMigration(ctx, MigrationConfig{
		ConnectionConfig: migrationConnection,
		StatementTimeout: 10 * time.Second,
		LockTimeout:      25 * time.Second,
	})
	if err != nil {
		t.Fatalf("OpenMigration() error = %v", err)
	}
	t.Cleanup(func() { _ = migrationDB.Close() })
	if got := migrationDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("migration MaxOpenConnections = %d", got)
	}
	table := fmt.Sprintf("growthos_privilege_probe_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = migrationDB.ExecContext(cleanupCtx, "DROP TABLE IF EXISTS `"+table+"`")
	})
	create := "CREATE TABLE `" + table + "` (id BIGINT NOT NULL PRIMARY KEY)"
	if _, err := appDB.ExecContext(ctx, create); err == nil {
		_, _ = migrationDB.ExecContext(ctx, "DROP TABLE IF EXISTS `"+table+"`")
		t.Fatal("API identity unexpectedly has DDL permission")
	} else {
		var mysqlErr *drivermysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1142 {
			t.Fatalf("API DDL failed for an unexpected non-privilege reason: %v", err)
		}
	}

	multiStatementDDL := create + "; INSERT INTO `" + table + "` (id) VALUES (1); DROP TABLE `" + table + "`"
	if _, err := migrationDB.ExecContext(ctx, multiStatementDDL); err != nil {
		t.Fatalf("migration identity could not execute multi-statement DDL: %v", err)
	}
}

func integrationConnection(t *testing.T, prefix string) ConnectionConfig {
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

	mode := TLSDisabled
	if raw, ok := os.LookupEnv(prefix + "_TLS_MODE"); ok {
		mode = TLSMode(raw)
	}
	return ConnectionConfig{
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

func ExampleOpenMigration() {
	// A migration command maps its dedicated environment variables into a
	// MigrationConfig, transfers the returned pool to dbmigration.Runner, and
	// closes only the runner. The example intentionally contains no credential.
	_, _ = OpenMigration(context.Background(), MigrationConfig{})
	fmt.Println("migration pool ownership transfers to the runner")
	// Output: migration pool ownership transfers to the runner
}
