package main

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"testing"
	"time"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
)

func TestProductionConfigMapsMigrationIdentityAndTimeouts(t *testing.T) {
	input := appconfig.MigrationMySQLConfig{
		MySQLConnectionConfig: appconfig.MySQLConnectionConfig{
			Address:        "mysql.internal:3307",
			Database:       "growthos_test",
			TLSMode:        appconfig.MySQLTLSVerifyIdentity,
			TLSCAFile:      "/run/secrets/mysql-ca.pem",
			ConnectTimeout: 3 * time.Second,
			ReadTimeout:    12 * time.Second,
			WriteTimeout:   5 * time.Second,
		},
		User:             "growthos_migrator_test",
		Password:         "migration-password",
		LockTimeout:      17 * time.Second,
		StatementTimeout: 7 * time.Second,
	}

	mysqlConfig := mysqlMigrationConfig(input)
	wantConnection := mysqlstore.ConnectionConfig{
		Address:        input.Address,
		Database:       input.Database,
		User:           input.User,
		Password:       input.Password,
		TLSMode:        mysqlstore.TLSMode(input.TLSMode),
		TLSCAFile:      input.TLSCAFile,
		ConnectTimeout: input.ConnectTimeout,
		ReadTimeout:    input.ReadTimeout,
		WriteTimeout:   input.WriteTimeout,
	}
	if mysqlConfig.ConnectionConfig != wantConnection {
		t.Fatal("mysqlMigrationConfig() did not map every validated connection field")
	}
	if mysqlConfig.StatementTimeout != input.StatementTimeout || mysqlConfig.LockTimeout != input.LockTimeout {
		t.Fatal("mysqlMigrationConfig() did not map the migration timeout hierarchy")
	}

	runnerConfig := migrationRunnerConfig(input)
	if runnerConfig.Path != migrationPath ||
		runnerConfig.MigrationsTable != migrationTable ||
		runnerConfig.LockTimeout != input.LockTimeout ||
		runnerConfig.NetworkReadTimeout != input.ReadTimeout ||
		runnerConfig.StatementTimeout != input.StatementTimeout {
		t.Fatalf("migrationRunnerConfig() = %#v", runnerConfig)
	}
}

func TestProductionFactoryPassesEmbeddedSourceAndValidatedConfiguration(t *testing.T) {
	database := unopenedTestDatabase(t)
	input := appconfig.MigrationMySQLConfig{
		MySQLConnectionConfig: appconfig.MySQLConnectionConfig{
			Address:        "127.0.0.1:3306",
			Database:       "growthos",
			TLSMode:        appconfig.MySQLTLSDisabled,
			ConnectTimeout: time.Second,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   3 * time.Second,
		},
		User:             "growthos_migrator",
		Password:         "migration-password",
		LockTimeout:      15 * time.Second,
		StatementTimeout: 5 * time.Second,
	}
	var gotMySQL mysqlstore.MigrationConfig
	var gotRunnerConfig dbmigration.Config
	var gotSource fs.FS
	var gotDatabase *sql.DB

	factory := newProductionRunnerFactory(
		func(_ context.Context, config mysqlstore.MigrationConfig) (*sql.DB, error) {
			gotMySQL = config
			return database, nil
		},
		func(_ context.Context, source fs.FS, owned *sql.DB, config dbmigration.Config) (*dbmigration.Runner, error) {
			gotSource = source
			gotDatabase = owned
			gotRunnerConfig = config
			return nil, nil
		},
	)

	if _, err := factory(context.Background(), input); err == nil {
		t.Fatal("factory returned nil error for nil constructed runner")
	}
	if gotMySQL != mysqlMigrationConfig(input) {
		t.Fatal("OpenMigration did not receive every validated migration connection field")
	}
	if gotRunnerConfig != migrationRunnerConfig(input) {
		t.Fatalf("dbmigration.New config = %#v, want %#v", gotRunnerConfig, migrationRunnerConfig(input))
	}
	if gotSource == nil {
		t.Fatal("dbmigration.New source is nil, want migrations.Files")
	}
	if _, err := fs.ReadFile(gotSource, "sql/README.md"); err != nil {
		t.Fatalf("embedded migration source is missing sql/README.md: %v", err)
	}
	if gotDatabase != database {
		t.Fatal("dbmigration.New did not receive the owned database")
	}
	if err := database.PingContext(context.Background()); err == nil {
		t.Fatal("nil runner success left the owned database open")
	}
}

func TestProductionFactoryClosesUnexpectedDatabaseReturnedWithOpenError(t *testing.T) {
	database := unopenedTestDatabase(t)
	openCause := errors.New("private mysql open failure")
	constructorCalled := false
	factory := newProductionRunnerFactory(
		func(context.Context, mysqlstore.MigrationConfig) (*sql.DB, error) {
			return database, openCause
		},
		func(context.Context, fs.FS, *sql.DB, dbmigration.Config) (*dbmigration.Runner, error) {
			constructorCalled = true
			return nil, nil
		},
	)

	if _, err := factory(context.Background(), appconfig.MigrationMySQLConfig{}); !errors.Is(err, openCause) {
		t.Fatalf("factory error = %v, want open cause", err)
	}
	if constructorCalled {
		t.Fatal("constructor was called after database open failure")
	}
	if err := database.PingContext(context.Background()); err == nil {
		t.Fatal("database returned with an open error was left open")
	}
}

func TestProductionFactoryConstructionFailureReliesOnOwnershipContractWithoutLeak(t *testing.T) {
	database := unopenedTestDatabase(t)
	constructorCause := errors.New("private migration SQL failed")
	factory := newProductionRunnerFactory(
		func(context.Context, mysqlstore.MigrationConfig) (*sql.DB, error) {
			return database, nil
		},
		func(_ context.Context, _ fs.FS, owned *sql.DB, _ dbmigration.Config) (*dbmigration.Runner, error) {
			// This fake enforces dbmigration.New's documented strong ownership
			// contract: after accepting owned, it closes on construction failure.
			_ = owned.Close()
			return nil, constructorCause
		},
	)

	if _, err := factory(context.Background(), appconfig.MigrationMySQLConfig{}); !errors.Is(err, constructorCause) {
		t.Fatalf("factory error = %v, want constructor cause", err)
	}
	if err := database.PingContext(context.Background()); err == nil {
		t.Fatal("constructor failure leaked owned database")
	}
}

func TestProductionFactoryWithEmbeddedMigrationsRequiresDatabaseAndReleasesOwnershipOnFailure(t *testing.T) {
	database := unopenedTestDatabase(t)
	factory := newProductionRunnerFactory(
		func(context.Context, mysqlstore.MigrationConfig) (*sql.DB, error) {
			return database, nil
		},
		dbmigration.New,
	)

	runner, err := factory(context.Background(), appconfig.MigrationMySQLConfig{
		LockTimeout:      40 * time.Second,
		StatementTimeout: 30 * time.Second,
	})
	if err == nil || runner != nil {
		t.Fatalf("factory = runner %v, error %v; real embedded migrations must open the database", runner, err)
	}
	if err := database.PingContext(context.Background()); err == nil {
		t.Fatal("migration construction failure did not release the owned database")
	}
}

func TestProductionFactoryRejectsEmptyDependencies(t *testing.T) {
	tests := []struct {
		name      string
		opener    migrationDatabaseOpener
		construct migrationRunnerConstructor
	}{
		{name: "both nil"},
		{name: "opener nil", construct: func(context.Context, fs.FS, *sql.DB, dbmigration.Config) (*dbmigration.Runner, error) {
			return nil, nil
		}},
		{name: "constructor nil", opener: func(context.Context, mysqlstore.MigrationConfig) (*sql.DB, error) {
			return nil, nil
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := newProductionRunnerFactory(test.opener, test.construct)
			if _, err := factory(context.Background(), appconfig.MigrationMySQLConfig{}); err == nil {
				t.Fatal("factory error = nil, want dependency failure")
			}
		})
	}
}

func unopenedTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("mysql", "test:test@tcp(127.0.0.1:1)/test")
	if err != nil {
		t.Fatalf("sql.Open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
