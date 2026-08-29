package main

import (
	"context"
	"reflect"

	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
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

type runtimeDependencies struct {
	OpenDatabase func(context.Context, appconfig.MySQLConfig) (databaseRuntime, error)
}

func productionDependencies() runtimeDependencies {
	return runtimeDependencies{OpenDatabase: openDatabase}
}

func openDatabase(ctx context.Context, config appconfig.MySQLConfig) (databaseRuntime, error) {
	return mysqlstore.Open(ctx, mysqlRuntimeConfig(config))
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
