package main

import (
	"context"
	"reflect"

	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/mysqlrepo"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/randomsource"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/jmoiron/sqlx"
)

func nilDatabaseRuntime(database databaseRuntime) bool {
	if database == nil {
		return true
	}
	value := reflect.ValueOf(database)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type databaseRuntime interface {
	PingContext(context.Context) error
	Close() error
}

// runtimeComponents is the fully composed product runtime. The concrete sqlx
// pool is retained while constructing the Repository, then exposed only through
// its readiness/ownership boundary. Repository and readiness therefore share
// exactly one pool without a type assertion or a second connection opener.
type runtimeComponents struct {
	database  databaseRuntime
	selection *application.EphemeralSelectionService
}

type runtimeDependencies struct {
	OpenRuntime func(context.Context, appconfig.MySQLConfig) (runtimeComponents, error)
}

func productionDependencies() runtimeDependencies {
	return runtimeDependencies{OpenRuntime: openRuntime}
}

func openRuntime(ctx context.Context, config appconfig.MySQLConfig) (runtimeComponents, error) {
	database, err := mysqlstore.Open(ctx, mysqlRuntimeConfig(config))
	if err != nil {
		return runtimeComponents{}, err
	}
	keepDatabase := false
	defer func() {
		if !keepDatabase {
			_ = database.Close()
		}
	}()

	components, err := composeRuntime(database)
	if err != nil {
		return runtimeComponents{}, err
	}

	keepDatabase = true
	return components, nil
}

func composeRuntime(database *sqlx.DB) (runtimeComponents, error) {
	repository, err := mysqlrepo.New(database)
	if err != nil {
		return runtimeComponents{}, err
	}
	selector, err := domain.NewWeightedSelector(randomsource.NewCryptoSource())
	if err != nil {
		return runtimeComponents{}, err
	}
	selection, err := application.NewEphemeralSelectionService(repository, selector)
	if err != nil {
		return runtimeComponents{}, err
	}

	return runtimeComponents{database: database, selection: selection}, nil
}

func mysqlRuntimeConfig(config appconfig.MySQLConfig) mysqlstore.Config {
	return mysqlstore.Config{
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
		PingTimeout:           config.PingTimeout,
		MaxOpenConnections:    config.MaxOpenConnections,
		MaxIdleConnections:    config.MaxIdleConnections,
		ConnectionMaxLifetime: config.ConnectionMaxLifetime,
		ConnectionMaxIdleTime: config.ConnectionMaxIdleTime,
	}
}
