// Command growth-migrate applies or inspects forward-only GrowthOS database
// migrations. Destructive rollback, drop, and force operations are
// intentionally not exposed by this production command.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"reflect"
	"syscall"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/logging"
)

const migrationServiceName = "growth-migrate"

// version can be replaced at build time with:
// go build -ldflags "-X main.version=<build-label>" ./cmd/growth-migrate
var version = "dev"

type command string

const (
	commandUp     command = "up"
	commandStatus command = "status"
)

const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

type migrationRunner interface {
	Up(context.Context) (dbmigration.Result, error)
	Status(context.Context) (dbmigration.Status, error)
	Close() error
}

type runnerFactory func(context.Context, appconfig.MigrationMySQLConfig) (migrationRunner, error)

type runtimeDependencies struct {
	NewRunner runnerFactory
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exitCode := run(ctx, os.Args[1:], os.LookupEnv, os.Stdout)
	stop()
	if exitCode != exitSuccess {
		os.Exit(exitCode)
	}
}

func run(ctx context.Context, args []string, lookup appconfig.LookupFunc, output io.Writer) int {
	return runWithDependencies(ctx, args, lookup, output, productionDependencies())
}

func runWithDependencies(
	ctx context.Context,
	args []string,
	lookup appconfig.LookupFunc,
	output io.Writer,
	dependencies runtimeDependencies,
) int {
	selectedCommand, ok := parseCommand(args)
	if !ok {
		return exitUsage
	}
	if ctx == nil {
		ctx = context.Background()
	}

	bootstrapLogger, err := logging.New(
		output,
		"info",
		"json",
		slog.String("service", migrationServiceName),
		slog.String("version", version),
	)
	if err != nil {
		return exitFailure
	}

	config, err := appconfig.LoadMigration(lookup)
	if err != nil {
		bootstrapLogger.ErrorContext(
			ctx,
			"migration configuration rejected",
			slog.String("component", "configuration"),
			slog.String("operation", "load"),
			slog.Any("error", err),
		)
		return exitFailure
	}

	logger, err := logging.New(
		output,
		string(config.Log.Level),
		string(config.Log.Format),
		slog.String("service", migrationServiceName),
		slog.String("environment", string(config.Environment)),
		slog.String("version", version),
	)
	if err != nil {
		bootstrapLogger.ErrorContext(
			ctx,
			"migration logger configuration rejected",
			slog.String("component", "logging"),
			slog.String("operation", "configure"),
		)
		return exitFailure
	}

	if dependencies.NewRunner == nil {
		logger.ErrorContext(
			ctx,
			"migration dependency is unavailable",
			slog.String("component", "migration"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}

	runner, err := dependencies.NewRunner(ctx, config.MySQL)
	if err != nil {
		// A factory must normally return a nil runner with an error. If an
		// injected implementation violates that convention, close the returned
		// owner exactly once before failing closed.
		if !nilMigrationRunner(runner) {
			_ = runner.Close()
		}
		logger.ErrorContext(
			ctx,
			"migration runner construction failed",
			slog.String("component", "migration"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}
	if nilMigrationRunner(runner) {
		logger.ErrorContext(
			ctx,
			"migration runner construction failed",
			slog.String("component", "migration"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}

	return executeCommand(ctx, selectedCommand, runner, logger)
}

func executeCommand(
	ctx context.Context,
	selectedCommand command,
	runner migrationRunner,
	logger *slog.Logger,
) (exitCode int) {
	exitCode = exitFailure
	defer func() {
		if err := runner.Close(); err != nil {
			logger.ErrorContext(
				ctx,
				"migration runner close failed",
				slog.String("component", "migration"),
				slog.String("operation", "close"),
			)
			exitCode = exitFailure
		}
	}()

	switch selectedCommand {
	case commandUp:
		result, err := runner.Up(ctx)
		if err != nil {
			logger.ErrorContext(
				ctx,
				"migration command failed",
				slog.String("component", "migration"),
				slog.String("operation", string(commandUp)),
			)
			return exitFailure
		}
		if !validResultState(result.State) {
			logger.ErrorContext(
				ctx,
				"migration command returned an invalid result",
				slog.String("component", "migration"),
				slog.String("operation", string(commandUp)),
			)
			return exitFailure
		}
		logger.InfoContext(
			ctx,
			"migration command completed",
			slog.String("component", "migration"),
			slog.String("operation", string(commandUp)),
			slog.String("result", string(result.State)),
			slog.Uint64("migration_version", uint64(result.Version)),
		)
		return exitSuccess

	case commandStatus:
		status, err := runner.Status(ctx)
		if err != nil {
			message := "migration status failed"
			if errors.Is(err, dbmigration.ErrDirty) {
				message = "migration status is dirty"
			}
			logger.ErrorContext(
				ctx,
				message,
				slog.String("component", "migration"),
				slog.String("operation", string(commandStatus)),
			)
			return exitFailure
		}
		if !validStatusState(status.State) {
			logger.ErrorContext(
				ctx,
				"migration command returned an invalid status",
				slog.String("component", "migration"),
				slog.String("operation", string(commandStatus)),
			)
			return exitFailure
		}
		logger.InfoContext(
			ctx,
			"migration status completed",
			slog.String("component", "migration"),
			slog.String("operation", string(commandStatus)),
			slog.String("result", string(status.State)),
			slog.Uint64("migration_version", uint64(status.Version)),
			slog.Uint64("latest_migration_version", uint64(status.Latest)),
		)
		return exitSuccess

	default:
		logger.ErrorContext(
			ctx,
			"migration command is unavailable",
			slog.String("component", "migration"),
			slog.String("operation", "dispatch"),
		)
		return exitFailure
	}
}

func parseCommand(args []string) (command, bool) {
	if len(args) != 1 {
		return "", false
	}
	selected := command(args[0])
	switch selected {
	case commandUp, commandStatus:
		return selected, true
	default:
		return "", false
	}
}

func validResultState(state dbmigration.ResultState) bool {
	switch state {
	case dbmigration.ResultNoMigrations, dbmigration.ResultNoChange, dbmigration.ResultApplied:
		return true
	default:
		return false
	}
}

func validStatusState(state dbmigration.StatusState) bool {
	switch state {
	case dbmigration.StatusNoMigrations, dbmigration.StatusUninitialized, dbmigration.StatusPending, dbmigration.StatusClean:
		return true
	default:
		return false
	}
}

func nilMigrationRunner(runner migrationRunner) bool {
	if runner == nil {
		return true
	}
	value := reflect.ValueOf(runner)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
