// Command growth-identity-maintenance performs one bounded Identity cleanup
// operation through the normal runtime Identity database authority. It
// intentionally exposes no caller-controlled clock, cutoff, budget, loop, or
// retry policy.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/logging"
)

const (
	identityMaintenanceServiceName = "growth-identity-maintenance"
	maintenanceRunCommand          = "run"
)

// version can be replaced at build time with:
// go build -ldflags "-X main.version=<build-label>" ./cmd/growth-identity-maintenance
var version = "dev"

const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

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
	// The command grammar is deliberately resolved before configuration, file,
	// or database access. A usage error therefore cannot probe deployment state.
	if !parseMaintenanceCommand(args) {
		return exitUsage
	}
	if ctx == nil {
		ctx = context.Background()
	}

	bootstrapLogger, err := logging.New(
		output,
		"info",
		"json",
		slog.String("service", identityMaintenanceServiceName),
		slog.String("version", version),
	)
	if err != nil {
		return exitFailure
	}
	if dependencies.NewRuntime == nil {
		// Reject an unusable composition before configuration loading. In
		// particular, a broken injection must not read the Identity password file.
		bootstrapLogger.ErrorContext(
			ctx,
			"identity maintenance dependency is unavailable",
			slog.String("component", "identity_maintenance"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}

	config, err := appconfig.LoadIdentityMaintenance(lookup)
	if err != nil {
		// Configuration errors can contain variable names and file locations.
		// This operator-facing command records only the reviewed failure class.
		bootstrapLogger.ErrorContext(
			ctx,
			"identity maintenance configuration rejected",
			slog.String("component", "configuration"),
			slog.String("operation", "load"),
		)
		return exitFailure
	}

	logger, err := logging.New(
		output,
		string(config.Log.Level),
		string(config.Log.Format),
		slog.String("service", identityMaintenanceServiceName),
		slog.String("environment", string(config.Environment)),
		slog.String("version", version),
	)
	if err != nil {
		bootstrapLogger.ErrorContext(
			ctx,
			"identity maintenance logger configuration rejected",
			slog.String("component", "logging"),
			slog.String("operation", "configure"),
		)
		return exitFailure
	}

	runtime, err := constructMaintenanceRuntime(
		ctx,
		&config.MySQL,
		dependencies.NewRuntime,
	)
	if err != nil {
		// A factory normally returns nil with an error. Close a returned owner
		// exactly once if an injected implementation violates that convention.
		if !nilMaintenanceRuntime(runtime) {
			_ = runtime.Close()
		}
		logger.ErrorContext(
			ctx,
			"identity maintenance runtime construction failed",
			slog.String("component", "identity_maintenance"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}
	if nilMaintenanceRuntime(runtime) {
		logger.ErrorContext(
			ctx,
			"identity maintenance runtime construction failed",
			slog.String("component", "identity_maintenance"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}

	return executeMaintenance(ctx, config.OperationTimeout, runtime, logger)
}

// constructMaintenanceRuntime keeps the secret-bearing configuration copy
// alive only for the synchronous factory call. mysqlstore must necessarily
// retain its own driver credential while the pool is open, but this composition
// root releases its independent password reference immediately on both success
// and failure.
func constructMaintenanceRuntime(
	ctx context.Context,
	config *appconfig.IdentityMaintenanceMySQLConfig,
	factory runtimeFactory,
) (maintenanceRuntime, error) {
	if config == nil {
		return nil, errMaintenanceRuntimeDependency
	}
	defer func() { config.Password = "" }()
	if factory == nil {
		return nil, errMaintenanceRuntimeDependency
	}
	return factory(ctx, *config)
}

func executeMaintenance(
	ctx context.Context,
	timeout time.Duration,
	runtime maintenanceRuntime,
	logger *slog.Logger,
) int {
	if timeout <= 0 {
		logger.ErrorContext(
			ctx,
			"identity maintenance operation budget is invalid",
			slog.String("component", "identity_maintenance"),
			slog.String("operation", maintenanceRunCommand),
		)
		if err := runtime.Close(); err != nil {
			logMaintenanceRuntimeCloseFailure(ctx, logger)
		}
		return exitFailure
	}

	operationContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, runErr := runtime.Run(operationContext)
	if runErr == nil && (result.Validate() != nil || result.TotalDeleted() > identityapp.MaintenanceMaximumRows) {
		runErr = errMaintenanceRuntimeResult
	}

	// Closing the sole pool is part of success. No completed message is emitted
	// until this ownership boundary has shut down successfully.
	closeErr := runtime.Close()
	if runErr != nil {
		message := "identity maintenance failed"
		switch {
		case errors.Is(runErr, identityapp.ErrCommitOutcomeUnknown):
			// Commit uncertainty dominates a concurrently observed cancellation:
			// reporting cancellation could invite an unsafe blind retry.
			message = "identity maintenance outcome is unknown"
		case errors.Is(runErr, identityapp.ErrOperationCanceled),
			errors.Is(runErr, context.Canceled),
			errors.Is(runErr, context.DeadlineExceeded):
			message = "identity maintenance canceled"
		}
		logger.ErrorContext(
			ctx,
			message,
			slog.String("component", "identity_maintenance"),
			slog.String("operation", maintenanceRunCommand),
		)
	}
	if closeErr != nil {
		logMaintenanceRuntimeCloseFailure(ctx, logger)
	}
	if runErr != nil || closeErr != nil {
		return exitFailure
	}

	logger.InfoContext(
		ctx,
		"identity maintenance completed",
		slog.String("component", "identity_maintenance"),
		slog.String("operation", maintenanceRunCommand),
		slog.Int("sessions_deleted", result.SessionsDeleted()),
		slog.Int("throttles_deleted", result.ThrottlesDeleted()),
		slog.Int("total_deleted", result.TotalDeleted()),
	)
	return exitSuccess
}

func parseMaintenanceCommand(args []string) bool {
	return len(args) == 1 && args[0] == maintenanceRunCommand
}

func logMaintenanceRuntimeCloseFailure(ctx context.Context, logger *slog.Logger) {
	logger.ErrorContext(
		ctx,
		"identity maintenance runtime close failed",
		slog.String("component", "identity_maintenance"),
		slog.String("operation", "close"),
	)
}
