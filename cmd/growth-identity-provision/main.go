// Command growth-identity-provision creates one local workforce authentication
// account through a dedicated, short-lived, INSERT-only database identity.
// It intentionally exposes no account mutation or authorization-binding path.
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

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlprovisioner"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/logging"
)

const identityProvisionServiceName = "growth-identity-provision"

// version can be replaced at build time with:
// go build -ldflags "-X main.version=<build-label>" ./cmd/growth-identity-provision
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
	command, ok := parseProvisionCommand(args)
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
		slog.String("service", identityProvisionServiceName),
		slog.String("version", version),
	)
	if err != nil {
		return exitFailure
	}

	config, err := appconfig.LoadIdentityProvisioner(lookup)
	if err != nil {
		bootstrapLogger.ErrorContext(
			ctx,
			"identity provisioner configuration rejected",
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
		slog.String("service", identityProvisionServiceName),
		slog.String("environment", string(config.Environment)),
		slog.String("version", version),
	)
	if err != nil {
		bootstrapLogger.ErrorContext(
			ctx,
			"identity provisioner logger configuration rejected",
			slog.String("component", "logging"),
			slog.String("operation", "configure"),
		)
		return exitFailure
	}

	if dependencies.NewRuntime == nil {
		logger.ErrorContext(
			ctx,
			"identity provisioner dependency is unavailable",
			slog.String("component", "identity_provisioning"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}

	password, err := readEnrollmentPassword(command.passwordFile)
	if err != nil {
		logger.ErrorContext(
			ctx,
			"identity enrollment password file rejected",
			slog.String("component", "credential_input"),
			slog.String("operation", "read"),
		)
		return exitFailure
	}
	defer clear(password)

	runtime, err := dependencies.NewRuntime(ctx, config)
	if err != nil {
		// A factory normally returns nil with an error. Close a returned owner
		// exactly once if an injected implementation violates that convention.
		if !nilProvisionRuntime(runtime) {
			_ = runtime.Close()
		}
		logger.ErrorContext(
			ctx,
			"identity provisioner runtime construction failed",
			slog.String("component", "identity_provisioning"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}
	if nilProvisionRuntime(runtime) {
		logger.ErrorContext(
			ctx,
			"identity provisioner runtime construction failed",
			slog.String("component", "identity_provisioning"),
			slog.String("operation", "construct"),
		)
		return exitFailure
	}

	return executeProvisionCommand(
		ctx,
		config.OperationTimeout,
		command,
		password,
		runtime,
		logger,
	)
}

func executeProvisionCommand(
	ctx context.Context,
	timeout time.Duration,
	command provisionCommand,
	password []byte,
	runtime provisionRuntime,
	logger *slog.Logger,
) int {
	if timeout <= 0 {
		clear(password)
		logger.ErrorContext(
			ctx,
			"identity provisioner operation budget is invalid",
			slog.String("component", "identity_provisioning"),
			slog.String("operation", provisionCreateCommand),
		)
		if err := runtime.Close(); err != nil {
			logProvisionRuntimeCloseFailure(ctx, logger)
		}
		return exitFailure
	}
	operationContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := runtime.Create(operationContext, command, password)
	// Create has no permission to retain this caller-owned plaintext. Clear it
	// before classification and runtime shutdown; the outer defer is a second
	// fail-safe for construction paths that never reach this operation.
	clear(password)

	message := ""
	if err != nil {
		message = "identity account provision failed"
		switch {
		case errors.Is(err, mysqlprovisioner.ErrCommitOutcomeUnknown):
			message = "identity account provision outcome is unknown"
		case errors.Is(err, mysqlprovisioner.ErrAlreadyExists):
			message = "identity account already exists"
		case errors.Is(err, mysqlprovisioner.ErrOperationCanceled),
			errors.Is(err, context.Canceled),
			errors.Is(err, context.DeadlineExceeded):
			message = "identity account provision canceled"
		}
	}

	closeErr := runtime.Close()
	if message != "" {
		logger.ErrorContext(
			ctx,
			message,
			slog.String("component", "identity_provisioning"),
			slog.String("operation", provisionCreateCommand),
		)
	}
	if closeErr != nil {
		logProvisionRuntimeCloseFailure(ctx, logger)
	}
	if err != nil || closeErr != nil {
		return exitFailure
	}

	logger.InfoContext(
		ctx,
		"identity account provision completed",
		slog.String("component", "identity_provisioning"),
		slog.String("operation", provisionCreateCommand),
		slog.String("result", "created"),
	)
	return exitSuccess
}

func logProvisionRuntimeCloseFailure(ctx context.Context, logger *slog.Logger) {
	logger.ErrorContext(
		ctx,
		"identity provisioner runtime close failed",
		slog.String("component", "identity_provisioning"),
		slog.String("operation", "close"),
	)
}
