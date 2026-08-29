package mysqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var errNilContext = errors.New("nil context")

// Open creates and verifies the application pool. Pool limits are installed
// before the first bounded ping. On any failure the partially opened pool is
// closed before the safe stage error is returned.
func Open(ctx context.Context, cfg Config) (*sqlx.DB, error) {
	if ctx == nil {
		return nil, newError(StageConfigInvalid, errNilContext)
	}
	pool, err := normalizePool(cfg)
	if err != nil {
		return nil, newError(StageConfigInvalid, err)
	}
	driverCfg, err := driverConfig(cfg.ConnectionConfig, false)
	if err != nil {
		return nil, err
	}

	connector, err := drivermysql.NewConnector(driverCfg)
	if err != nil {
		return nil, newError(StageConnector, err)
	}
	sqlDB := sql.OpenDB(connector)
	sqlDB.SetMaxOpenConns(pool.maxOpen)
	sqlDB.SetMaxIdleConns(pool.maxIdle)
	sqlDB.SetConnMaxLifetime(pool.maxLifetime)
	sqlDB.SetConnMaxIdleTime(pool.maxIdleTime)

	pingCtx, cancel := boundedContext(ctx, pool.pingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, newError(StagePing, err)
	}

	return sqlx.NewDb(sqlDB, "mysql"), nil
}

// OpenMigration creates a single-connection pool using only the dedicated
// migration identity. Multi-statements are enabled exclusively here because a
// versioned migration may contain more than one SQL statement. Ownership of the
// returned pool transfers to the caller (normally dbmigration.Runner), which
// must close it.
func OpenMigration(ctx context.Context, cfg MigrationConfig) (*sql.DB, error) {
	if ctx == nil {
		return nil, newError(StageConfigInvalid, errNilContext)
	}
	driverCfg, err := migrationDriverConfig(cfg)
	if err != nil {
		return nil, err
	}
	connector, err := drivermysql.NewConnector(driverCfg)
	if err != nil {
		return nil, newError(StageConnector, err)
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(defaultConnMaxLifetime)
	db.SetConnMaxIdleTime(defaultConnMaxIdleTime)

	pingCtx, cancel := boundedContext(ctx, driverCfg.Timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, newError(StagePing, err)
	}
	return db, nil
}

func boundedContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= timeout {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}
