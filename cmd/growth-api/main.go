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
	if dependencies.OpenDatabase == nil {
		logger.ErrorContext(ctx, "database dependency is unavailable", slog.String("component", "mysql"))
		return 1
	}

	database, err := dependencies.OpenDatabase(ctx, config.MySQL)
	if err != nil || nilDatabaseRuntime(database) {
		if !nilDatabaseRuntime(database) {
			// Defend the dependency boundary even when an injected or future
			// opener violates the conventional nil-on-error contract.
			_ = database.Close()
		}
		// Database errors can originate in a driver and may contain topology,
		// account, or query details. Keep the process log to a stable phase and
		// retain the underlying error only in the adapter's error chain.
		logger.ErrorContext(ctx, "database startup failed", slog.String("component", "mysql"))
		return 1
	}
	databaseClosed := false
	defer func() {
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
		ReadinessChecker: database,
		ReadinessTimeout: config.MySQL.PingTimeout,
	})
	server := httpserver.New(router, httpServerConfig(config.HTTP, logger))

	logger.InfoContext(
		ctx,
		"service starting",
		slog.String("http_address", config.HTTP.Address),
		slog.Int64("shutdown_timeout_ms", config.HTTP.ShutdownTimeout.Milliseconds()),
	)
	serverErr := server.Run(ctx)
	databaseCloseErr := database.Close()
	databaseClosed = true
	if serverErr != nil {
		logger.ErrorContext(ctx, "service stopped with an error", slog.Any("error", serverErr))
	}
	if databaseCloseErr != nil {
		logger.ErrorContext(ctx, "database shutdown failed", slog.String("component", "mysql"))
	}
	if serverErr != nil || databaseCloseErr != nil {
		return 1
	}
	logger.InfoContext(ctx, "service stopped")
	return 0
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
