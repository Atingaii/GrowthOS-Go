package main

import (
	"context"
	"errors"
	"reflect"

	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/redisstore"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/mysqlrepo"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/randomsource"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/strategycache"
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

type strategyCacheRuntime interface {
	strategycache.Store
	Close() error
}

var (
	errStrategyCacheRuntimeRequired   = errors.New("strategy cache runtime is required")
	errUnexpectedStrategyCacheRuntime = errors.New("strategy cache runtime must be absent when disabled")
)

// runtimeComponents is the fully composed product runtime. The concrete sqlx
// pool is retained while constructing the Repository, then exposed only through
// its readiness/ownership boundary. Repository and readiness therefore share
// exactly one pool without a type assertion or a second connection opener.
type runtimeComponents struct {
	database  databaseRuntime
	cache     strategyCacheRuntime
	selection *application.EphemeralSelectionService
}

type runtimeConfiguration struct {
	Environment   appconfig.Environment
	MySQL         appconfig.MySQLConfig
	Redis         appconfig.RedisConfig
	StrategyCache appconfig.StrategyCacheConfig
}

type runtimeDependencies struct {
	OpenRuntime func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error)
}

func productionDependencies() runtimeDependencies {
	return runtimeDependencies{OpenRuntime: openRuntime}
}

func openRuntime(
	ctx context.Context,
	config runtimeConfiguration,
	observer strategycache.Observer,
) (runtimeComponents, error) {
	database, err := mysqlstore.Open(ctx, mysqlRuntimeConfig(config.MySQL))
	if err != nil {
		return runtimeComponents{}, err
	}
	keepDatabase := false
	var cache *redisstore.Client
	keepCache := false
	defer func() {
		if cache != nil && !keepCache {
			_ = cache.Close()
		}
		if !keepDatabase {
			_ = database.Close()
		}
	}()

	if config.StrategyCache.Enabled {
		cache, err = redisstore.Open(redisRuntimeConfig(config.Redis))
		if err != nil {
			return runtimeComponents{}, err
		}
	}

	components, err := composeRuntime(ctx, database, cache, config, observer)
	if err != nil {
		return runtimeComponents{}, err
	}

	keepDatabase = true
	keepCache = cache != nil
	return components, nil
}

func composeRuntime(
	lifecycle context.Context,
	database *sqlx.DB,
	cache strategyCacheRuntime,
	config runtimeConfiguration,
	observer strategycache.Observer,
) (runtimeComponents, error) {
	repository, err := mysqlrepo.New(database)
	if err != nil {
		return runtimeComponents{}, err
	}
	var strategies application.StrategyReader = repository
	if config.StrategyCache.Enabled {
		if nilStrategyCacheRuntime(cache) {
			return runtimeComponents{}, errStrategyCacheRuntimeRequired
		}
		cachedStrategies, err := strategycache.New(repository, cache, strategyCacheOptions(lifecycle, config, observer))
		if err != nil {
			return runtimeComponents{}, err
		}
		strategies = cachedStrategies
	} else if !nilStrategyCacheRuntime(cache) {
		return runtimeComponents{}, errUnexpectedStrategyCacheRuntime
	}
	selector, err := domain.NewWeightedSelector(randomsource.NewCryptoSource())
	if err != nil {
		return runtimeComponents{}, err
	}
	selection, err := application.NewEphemeralSelectionService(strategies, selector)
	if err != nil {
		return runtimeComponents{}, err
	}

	return runtimeComponents{database: database, cache: cache, selection: selection}, nil
}

func runtimeConfig(config appconfig.Config) runtimeConfiguration {
	return runtimeConfiguration{
		Environment:   config.Environment,
		MySQL:         config.MySQL,
		Redis:         config.Redis,
		StrategyCache: config.Lottery.StrategyCache,
	}
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

func redisRuntimeConfig(config appconfig.RedisConfig) redisstore.Config {
	return redisstore.Config{
		Address:               config.Address,
		Username:              config.Username,
		Password:              config.Password,
		Database:              config.Database,
		TLSMode:               redisstore.TLSMode(config.TLSMode),
		TLSCAFile:             config.TLSCAFile,
		DialTimeout:           config.DialTimeout,
		ReadTimeout:           config.ReadTimeout,
		WriteTimeout:          config.WriteTimeout,
		PoolTimeout:           config.PoolTimeout,
		PoolSize:              config.PoolSize,
		MinIdleConnections:    config.MinIdleConnections,
		ConnectionMaxLifetime: config.ConnectionMaxLifetime,
		ConnectionMaxIdleTime: config.ConnectionMaxIdleTime,
	}
}

func strategyCacheOptions(
	lifecycle context.Context,
	config runtimeConfiguration,
	observer strategycache.Observer,
) strategycache.Options {
	return strategycache.Options{
		Namespace:     "growthos:" + string(config.Environment),
		TTL:           config.StrategyCache.TTL,
		LookupTimeout: config.StrategyCache.LookupTimeout,
		WriteTimeout:  config.StrategyCache.WriteTimeout,
		FillTimeout:   config.StrategyCache.FillTimeout,
		Lifecycle:     lifecycle,
		Observer:      observer,
	}
}

func nilStrategyCacheRuntime(cache strategyCacheRuntime) bool {
	if cache == nil {
		return true
	}
	value := reflect.ValueOf(cache)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
