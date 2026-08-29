package main

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/migrations"
)

const (
	migrationPath  = "sql"
	migrationTable = "schema_migrations"
)

type migrationDatabaseOpener func(
	context.Context,
	mysqlstore.MigrationConfig,
) (*sql.DB, error)

type migrationRunnerConstructor func(
	context.Context,
	fs.FS,
	*sql.DB,
	dbmigration.Config,
) (*dbmigration.Runner, error)

func productionDependencies() runtimeDependencies {
	return runtimeDependencies{
		NewRunner: newProductionRunnerFactory(mysqlstore.OpenMigration, dbmigration.New),
	}
}

func newProductionRunnerFactory(
	openDatabase migrationDatabaseOpener,
	constructRunner migrationRunnerConstructor,
) runnerFactory {
	return func(ctx context.Context, config appconfig.MigrationMySQLConfig) (migrationRunner, error) {
		if openDatabase == nil || constructRunner == nil {
			return nil, errors.New("migration runtime dependency is unavailable")
		}

		database, err := openDatabase(ctx, mysqlMigrationConfig(config))
		if err != nil {
			if database != nil {
				_ = database.Close()
			}
			return nil, err
		}
		if database == nil {
			return nil, errors.New("migration database is unavailable")
		}

		runner, err := constructRunner(
			ctx,
			migrations.Files,
			database,
			migrationRunnerConfig(config),
		)
		if err != nil {
			if runner != nil {
				_ = runner.Close()
			}
			// New receives ownership of the database. Its contract closes that
			// database on every construction failure, so this layer must not
			// attempt a second close.
			return nil, err
		}
		if runner == nil {
			// A nil success violates the constructor contract before ownership
			// can be represented by a Runner. Close defensively to avoid a leak.
			_ = database.Close()
			return nil, errors.New("migration runner is unavailable")
		}
		return runner, nil
	}
}

func mysqlMigrationConfig(config appconfig.MigrationMySQLConfig) mysqlstore.MigrationConfig {
	return mysqlstore.MigrationConfig{
		ConnectionConfig: mysqlstore.ConnectionConfig{
			Address:        config.Address,
			Database:       config.Database,
			User:           config.User,
			Password:       config.Password,
			TLSMode:        mysqlstore.TLSMode(config.TLSMode),
			TLSCAFile:      config.TLSCAFile,
			ConnectTimeout: config.ConnectTimeout,
			ReadTimeout:    config.ReadTimeout,
			WriteTimeout:   config.WriteTimeout,
		},
		StatementTimeout: config.StatementTimeout,
		LockTimeout:      config.LockTimeout,
	}
}

func migrationRunnerConfig(config appconfig.MigrationMySQLConfig) dbmigration.Config {
	return dbmigration.Config{
		Path:               migrationPath,
		MigrationsTable:    migrationTable,
		LockTimeout:        config.LockTimeout,
		NetworkReadTimeout: config.ReadTimeout,
		StatementTimeout:   config.StatementTimeout,
	}
}
