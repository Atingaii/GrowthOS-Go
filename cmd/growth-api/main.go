// Command growth-api runs the GrowthOS modular monolith HTTP process.
package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpserver"
	lotteryhttp "github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/logging"
	"github.com/gin-gonic/gin"
)

const serviceName = "growth-api"

// version can be replaced at build time with:
// go build -ldflags "-X main.version=<build-label>" ./cmd/growth-api
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exitCode := run(ctx, os.LookupEnv, os.Stdout)
	stop()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func run(ctx context.Context, lookup appconfig.LookupFunc, output io.Writer) int {
	return runWithDependencies(ctx, lookup, output, productionDependencies())
}

func runWithDependencies(
	ctx context.Context,
	lookup appconfig.LookupFunc,
	output io.Writer,
	dependencies runtimeDependencies,
) int {
	if ctx == nil {
		ctx = context.Background()
	}
	bootstrapLogger, err := logging.New(
		output,
		"info",
		"json",
		slog.String("service", serviceName),
		slog.String("version", version),
	)
	if err != nil {
		return 1
	}

	config, err := appconfig.Load(lookup)
	if err != nil {
		bootstrapLogger.ErrorContext(ctx, "configuration rejected", slog.Any("error", err))
		return 1
	}

	logger, err := logging.New(
		output,
		string(config.Log.Level),
		string(config.Log.Format),
		slog.String("service", serviceName),
		slog.String("environment", string(config.Environment)),
		slog.String("version", version),
	)
	if err != nil {
		bootstrapLogger.ErrorContext(ctx, "logger configuration rejected", slog.Any("error", err))
		return 1
	}
	if dependencies.OpenRuntime == nil {
		logger.ErrorContext(ctx, "runtime dependency is unavailable", slog.String("component", "application"))
		return 1
	}

	components, err := dependencies.OpenRuntime(ctx, runtimeConfig(config), newStrategyCacheObserver(logger))
	cacheMissing := nilStrategyCacheRuntime(components.cache)
	cacheContractInvalid := (config.Lottery.StrategyCache.Enabled && cacheMissing) ||
		(!config.Lottery.StrategyCache.Enabled && !cacheMissing)
	if err != nil || nilDatabaseRuntime(components.database) ||
		nilDatabaseRuntime(components.identityDatabase) ||
		sameDatabaseRuntime(components.database, components.identityDatabase) ||
		components.readiness == nil || components.readiness.Validate() != nil ||
		nilIdentitySessionRuntime(components.identity) || components.identity.Validate() != nil ||
		cacheContractInvalid || components.selection == nil || components.selection.Validate() != nil {
		closePartialRuntime(components)
		// Runtime errors can contain driver topology, SQL, entropy adapter, or
		// composition details. Keep the process log to a stable phase.
		logger.ErrorContext(ctx, "runtime startup failed", slog.String("component", "application"))
		return 1
	}
	database := components.database
	identityDatabase := components.identityDatabase
	cache := components.cache
	databaseClosed := false
	identityDatabaseClosed := false
	cacheClosed := false
	defer func() {
		if !cacheClosed && !nilStrategyCacheRuntime(cache) {
			_ = cache.Close()
		}
		if !identityDatabaseClosed {
			_ = identityDatabase.Close()
		}
		if !databaseClosed {
			_ = database.Close()
		}
	}()

	// Gin's debug mode writes its own unstructured route table to stdout. The
	// product process owns logging through slog, so keep framework diagnostics
	// quiet in every deployment environment.
	gin.SetMode(gin.ReleaseMode)
	router := httpapi.NewRouter(httpapi.RouterOptions{
		Version:          version,
		Clock:            httpapi.ClockFunc(time.Now),
		Logger:           logger,
		ReadinessChecker: components.readiness,
		ReadinessTimeout: dualMySQLReadinessTimeout(
			config.MySQL.PingTimeout,
			config.IdentityMySQL.PingTimeout,
		),
	})
	if err := components.identity.RegisterRoutes(router, logger, config.Identity.HandlerTimeout); err != nil {
		logger.ErrorContext(ctx, "identity HTTP adapter startup failed", slog.String("component", "identity"))
		return 1
	}
	if config.Lottery.EphemeralSelectionEnabled {
		if err := lotteryhttp.RegisterRoutes(router, components.selection, lotteryhttp.Options{
			Logger:  logger,
			Timeout: config.Lottery.SelectionTimeout,
		}); err != nil {
			logger.ErrorContext(ctx, "lottery HTTP adapter startup failed", slog.String("component", "lottery"))
			return 1
		}
	}
	server := httpserver.New(router, httpServerConfig(config.HTTP, logger))

	logger.InfoContext(
		ctx,
		"service starting",
		slog.String("http_address", config.HTTP.Address),
		slog.Int64("shutdown_timeout_ms", config.HTTP.ShutdownTimeout.Milliseconds()),
	)
	serverErr := server.Run(ctx)
	var cacheCloseErr error
	if !nilStrategyCacheRuntime(cache) {
		cacheCloseErr = cache.Close()
	}
	cacheClosed = true
	identityDatabaseCloseErr := identityDatabase.Close()
	identityDatabaseClosed = true
	databaseCloseErr := database.Close()
	databaseClosed = true
	if serverErr != nil {
		logger.ErrorContext(ctx, "service stopped with an error", slog.Any("error", serverErr))
	}
	if databaseCloseErr != nil {
		logger.ErrorContext(ctx, "database shutdown failed", slog.String("component", "mysql"))
	}
	if identityDatabaseCloseErr != nil {
		logger.ErrorContext(ctx, "identity database shutdown failed", slog.String("component", "identity_mysql"))
	}
	if cacheCloseErr != nil {
		logger.ErrorContext(ctx, "cache shutdown failed", slog.String("component", "redis"))
	}
	if serverErr != nil || identityDatabaseCloseErr != nil || databaseCloseErr != nil || cacheCloseErr != nil {
		return 1
	}
	logger.InfoContext(ctx, "service stopped")
	return 0
}

func closePartialRuntime(components runtimeComponents) {
	if !nilStrategyCacheRuntime(components.cache) {
		_ = components.cache.Close()
	}
	if !nilDatabaseRuntime(components.identityDatabase) {
		_ = components.identityDatabase.Close()
	}
	if !nilDatabaseRuntime(components.database) &&
		!sameDatabaseRuntime(components.database, components.identityDatabase) {
		// Defend the dependency boundary even when an injected or future opener
		// returns owners alongside an error or aliases one owner unexpectedly.
		_ = components.database.Close()
	}
}

func httpServerConfig(config appconfig.HTTPConfig, logger *slog.Logger) httpserver.Config {
	return httpserver.Config{
		Address:           config.Address,
		ShutdownTimeout:   config.ShutdownTimeout,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		ErrorLogger:       logger,
	}
}
