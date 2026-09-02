package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/logging"
)

func TestRunParsesExactCommandBeforeOutputConfigurationAndRuntime(t *testing.T) {
	tests := [][]string{
		nil,
		{},
		{"RUN"},
		{"run", "--cutoff", "2026-09-01T00:00:00Z"},
		{"run", "--batch", "500"},
		{"run", "--loop"},
		{"run", "--retry"},
		{"status"},
		{" run"},
		{"run "},
	}
	for _, args := range tests {
		lookupCalls := 0
		factoryCalls := 0
		var output bytes.Buffer
		exitCode := runWithDependencies(
			context.Background(),
			args,
			func(string) (string, bool) {
				lookupCalls++
				return "", false
			},
			&output,
			runtimeDependencies{NewRuntime: func(context.Context, appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
				factoryCalls++
				return nil, nil
			}},
		)
		if exitCode != exitUsage || lookupCalls != 0 || factoryCalls != 0 || output.Len() != 0 {
			t.Fatalf("invalid grammar %#v crossed a boundary: exit=%d lookup=%d factory=%d output=%q", args, exitCode, lookupCalls, factoryCalls, output.String())
		}
	}
	if !parseMaintenanceCommand([]string{"run"}) {
		t.Fatal("parseMaintenanceCommand rejected the sole supported grammar")
	}
}

func TestRunExecutesOnceWithIdentityConfigAndClosesBeforeSuccess(t *testing.T) {
	const databasePassword = "PRIVATE_IDENTITY_DATABASE_PASSWORD"
	result := mustMaintenanceResult(t, 250, 250)
	runtime := &fakeMaintenanceRuntime{result: result}
	var gotConfig appconfig.IdentityMaintenanceMySQLConfig
	var factoryContext context.Context
	var output bytes.Buffer

	exitCode := runWithDependencies(
		context.Background(),
		[]string{"run"},
		maintenanceLookup(map[string]string{
			"GROWTHOS_IDENTITY_MYSQL_USER":     "growthos_identity_test",
			"GROWTHOS_IDENTITY_MYSQL_PASSWORD": databasePassword,
		}),
		&output,
		runtimeDependencies{NewRuntime: func(ctx context.Context, config appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
			factoryContext = ctx
			gotConfig = config
			return runtime, nil
		}},
	)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want success\n%s", exitCode, output.String())
	}
	if factoryContext == nil || gotConfig.User != "growthos_identity_test" || gotConfig.Password != databasePassword {
		t.Fatal("factory did not receive the validated runtime Identity credential")
	}
	if runtime.runCalls != 1 || runtime.closeCalls != 1 || strings.Join(runtime.order, ",") != "run,close" {
		t.Fatalf("runtime lifecycle calls run=%d close=%d order=%v", runtime.runCalls, runtime.closeCalls, runtime.order)
	}
	if runtime.runContext == nil || runtime.deadlineRemaining <= 0 || runtime.deadlineRemaining > 4*time.Second {
		t.Fatalf("runtime operation context/deadline = %v / %s", runtime.runContext, runtime.deadlineRemaining)
	}

	record := decodeMaintenanceLog(t, output.String())
	assertMaintenanceLogFields(t, record, map[string]any{
		"level":             "INFO",
		"msg":               "identity maintenance completed",
		"service":           identityMaintenanceServiceName,
		"environment":       "test",
		"version":           version,
		"component":         "identity_maintenance",
		"operation":         maintenanceRunCommand,
		"sessions_deleted":  float64(250),
		"throttles_deleted": float64(250),
		"total_deleted":     float64(500),
	})
	if strings.Contains(output.String(), databasePassword) || strings.Contains(output.String(), "growthos_identity_test") {
		t.Fatalf("success log leaked configuration: %s", output.String())
	}
}

func TestRunConfigurationFailurePrecedesFactoryAndRedactsRejectedValue(t *testing.T) {
	const privateValue = "PRIVATE_INVALID_MAINTENANCE_TIMEOUT"
	factoryCalls := 0
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"run"},
		maintenanceLookup(map[string]string{
			"GROWTHOS_IDENTITY_MAINTENANCE_OPERATION_TIMEOUT": privateValue,
		}),
		&output,
		runtimeDependencies{NewRuntime: func(context.Context, appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
			factoryCalls++
			return nil, nil
		}},
	)
	if exitCode != exitFailure || factoryCalls != 0 {
		t.Fatalf("configuration failure exit=%d factory=%d", exitCode, factoryCalls)
	}
	if strings.Contains(output.String(), privateValue) {
		t.Fatalf("configuration log leaked rejected value: %s", output.String())
	}
	record := decodeMaintenanceLog(t, output.String())
	if record["component"] != "configuration" || record["operation"] != "load" {
		t.Fatalf("configuration record = %#v", record)
	}
}

func TestRunMissingFactoryDoesNotReadConfigurationOrSecretFile(t *testing.T) {
	lookupCalls := 0
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"run"},
		func(string) (string, bool) {
			lookupCalls++
			panic("configuration lookup must not run without a runtime factory")
		},
		&output,
		runtimeDependencies{},
	)
	if exitCode != exitFailure || lookupCalls != 0 {
		t.Fatalf("missing factory exit=%d lookup=%d", exitCode, lookupCalls)
	}
	if !strings.Contains(output.String(), "identity maintenance dependency is unavailable") ||
		!strings.Contains(output.String(), "identity_maintenance") {
		t.Fatalf("missing factory log = %s", output.String())
	}
}

func TestRunClosesUnexpectedRuntimeFromFactoryFailureWithoutLoggingCause(t *testing.T) {
	const privateCause = "PRIVATE_DSN_FACTORY_CAUSE"
	runtime := &fakeMaintenanceRuntime{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"run"},
		maintenanceLookup(nil),
		&output,
		runtimeDependencies{NewRuntime: fixedMaintenanceRuntimeFactory(runtime, errors.New(privateCause))},
	)
	if exitCode != exitFailure || runtime.runCalls != 0 || runtime.closeCalls != 1 {
		t.Fatalf("factory failure exit=%d run=%d close=%d", exitCode, runtime.runCalls, runtime.closeCalls)
	}
	if strings.Contains(output.String(), privateCause) {
		t.Fatalf("factory log leaked cause: %s", output.String())
	}
}

func TestConstructMaintenanceRuntimeReleasesCompositionPasswordOnEveryFactoryResult(t *testing.T) {
	const privatePassword = "PRIVATE_COMPOSITION_PASSWORD_REFERENCE"
	tests := []struct {
		name       string
		factoryErr error
	}{
		{name: "success"},
		{name: "failure", factoryErr: errors.New("factory failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := appconfig.IdentityMaintenanceMySQLConfig{Password: privatePassword}
			factoryCalls := 0
			_, err := constructMaintenanceRuntime(
				context.Background(),
				&config,
				func(_ context.Context, got appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
					factoryCalls++
					if got.Password != privatePassword {
						t.Fatal("factory did not receive the validated credential before release")
					}
					return &fakeMaintenanceRuntime{}, test.factoryErr
				},
			)
			if factoryCalls != 1 || config.Password != "" || !errors.Is(err, test.factoryErr) {
				t.Fatalf("factory calls=%d retained password=%t error=%v", factoryCalls, config.Password != "", err)
			}
		})
	}

	if runtime, err := constructMaintenanceRuntime(context.Background(), nil, fixedMaintenanceRuntimeFactory(nil, nil)); runtime != nil || !errors.Is(err, errMaintenanceRuntimeDependency) {
		t.Fatalf("nil config = runtime %v error %v", runtime, err)
	}
	config := appconfig.IdentityMaintenanceMySQLConfig{Password: privatePassword}
	if runtime, err := constructMaintenanceRuntime(context.Background(), &config, nil); runtime != nil || !errors.Is(err, errMaintenanceRuntimeDependency) || config.Password != "" {
		t.Fatalf("nil factory = runtime %v error %v retained-password=%t", runtime, err, config.Password != "")
	}
}

func TestRunDefendsMissingNilAndTypedNilRuntime(t *testing.T) {
	tests := []struct {
		name         string
		dependencies runtimeDependencies
	}{
		{name: "nil factory"},
		{name: "nil runtime", dependencies: runtimeDependencies{NewRuntime: fixedMaintenanceRuntimeFactory(nil, nil)}},
		{name: "typed nil runtime", dependencies: runtimeDependencies{NewRuntime: func(context.Context, appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
			var runtime *fakeMaintenanceRuntime
			return runtime, nil
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				[]string{"run"},
				maintenanceLookup(nil),
				&output,
				test.dependencies,
			)
			if exitCode != exitFailure || !strings.Contains(output.String(), "identity_maintenance") {
				t.Fatalf("dependency defense exit=%d output=%s", exitCode, output.String())
			}
		})
	}
}

func TestRunMapsStableOperationClassesWithoutLoggingPrivateDetails(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{name: "unknown commit", err: identityapp.ErrCommitOutcomeUnknown, message: "identity maintenance outcome is unknown"},
		{name: "wrapped unknown commit", err: fmt.Errorf("PRIVATE_COMMIT_CAUSE: %w", identityapp.ErrCommitOutcomeUnknown), message: "identity maintenance outcome is unknown"},
		{name: "unknown dominates cancel", err: errors.Join(context.Canceled, identityapp.ErrCommitOutcomeUnknown), message: "identity maintenance outcome is unknown"},
		{name: "stable canceled", err: identityapp.ErrOperationCanceled, message: "identity maintenance canceled"},
		{name: "context canceled", err: context.Canceled, message: "identity maintenance canceled"},
		{name: "deadline", err: context.DeadlineExceeded, message: "identity maintenance canceled"},
		{name: "dependency", err: errors.New("PRIVATE_DATABASE_HOST_AND_PASSWORD"), message: "identity maintenance failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeMaintenanceRuntime{runErr: test.err}
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				[]string{"run"},
				maintenanceLookup(nil),
				&output,
				runtimeDependencies{NewRuntime: fixedMaintenanceRuntimeFactory(runtime, nil)},
			)
			if exitCode != exitFailure || runtime.runCalls != 1 || runtime.closeCalls != 1 {
				t.Fatalf("operation failure exit=%d run=%d close=%d", exitCode, runtime.runCalls, runtime.closeCalls)
			}
			if !strings.Contains(output.String(), test.message) || strings.Contains(output.String(), "PRIVATE_") {
				t.Fatalf("operation log = %s", output.String())
			}
		})
	}
}

func TestExecuteMaintenanceCloseFailureDowngradesSuccess(t *testing.T) {
	runtime := &fakeMaintenanceRuntime{
		result:   mustMaintenanceResult(t, 1, 2),
		closeErr: errors.New("PRIVATE_CLOSE_DSN"),
	}
	var output bytes.Buffer
	exitCode := executeMaintenance(
		context.Background(),
		time.Second,
		runtime,
		newMaintenanceTestLogger(t, &output),
	)
	if exitCode != exitFailure || runtime.runCalls != 1 || runtime.closeCalls != 1 {
		t.Fatalf("close failure exit=%d run=%d close=%d", exitCode, runtime.runCalls, runtime.closeCalls)
	}
	if strings.Contains(output.String(), "PRIVATE_CLOSE_DSN") ||
		!strings.Contains(output.String(), "runtime close failed") ||
		strings.Contains(output.String(), "maintenance completed") {
		t.Fatalf("close failure log = %s", output.String())
	}
}

func TestExecuteMaintenanceReportsRunAndCloseFailuresWithoutCauses(t *testing.T) {
	runtime := &fakeMaintenanceRuntime{
		runErr:   errors.New("PRIVATE_RUN_DATABASE_CAUSE"),
		closeErr: errors.New("PRIVATE_CLOSE_DATABASE_CAUSE"),
	}
	var output bytes.Buffer
	exitCode := executeMaintenance(
		context.Background(),
		time.Second,
		runtime,
		newMaintenanceTestLogger(t, &output),
	)
	if exitCode != exitFailure || runtime.runCalls != 1 || runtime.closeCalls != 1 {
		t.Fatalf("combined failure exit=%d run=%d close=%d", exitCode, runtime.runCalls, runtime.closeCalls)
	}
	if !strings.Contains(output.String(), "identity maintenance failed") ||
		!strings.Contains(output.String(), "identity maintenance runtime close failed") ||
		strings.Contains(output.String(), "PRIVATE_") ||
		strings.Contains(output.String(), "identity maintenance completed") {
		t.Fatalf("combined failure log = %s", output.String())
	}
	if got := len(nonemptyMaintenanceLogLines(output.String())); got != 2 {
		t.Fatalf("combined failure log lines = %d, want run and close failures", got)
	}
}

func TestExecuteMaintenanceEnforcesDeadlineAndStillCloses(t *testing.T) {
	runtime := &fakeMaintenanceRuntime{waitForContext: true}
	var output bytes.Buffer
	started := time.Now()
	exitCode := executeMaintenance(
		context.Background(),
		5*time.Millisecond,
		runtime,
		newMaintenanceTestLogger(t, &output),
	)
	if exitCode != exitFailure || runtime.closeCalls != 1 || !errors.Is(runtime.observedContextError, context.DeadlineExceeded) {
		t.Fatalf("deadline exit=%d close=%d error=%v", exitCode, runtime.closeCalls, runtime.observedContextError)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("operation deadline took %s", elapsed)
	}
}

func TestExecuteMaintenanceRejectsInvalidBudgetWithoutRunningAndStillCloses(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		runtime := &fakeMaintenanceRuntime{}
		var output bytes.Buffer
		exitCode := executeMaintenance(
			context.Background(),
			timeout,
			runtime,
			newMaintenanceTestLogger(t, &output),
		)
		if exitCode != exitFailure || runtime.runCalls != 0 || runtime.closeCalls != 1 {
			t.Fatalf("invalid budget %s exit=%d run=%d close=%d", timeout, exitCode, runtime.runCalls, runtime.closeCalls)
		}
	}
}

func TestRunRejectsNilAndTypedNilOutputBeforeConfiguration(t *testing.T) {
	for _, output := range []io.Writer{nil, (*bytes.Buffer)(nil)} {
		lookupCalls := 0
		exitCode := runWithDependencies(
			context.Background(),
			[]string{"run"},
			func(string) (string, bool) {
				lookupCalls++
				return "", false
			},
			output,
			runtimeDependencies{},
		)
		if exitCode != exitFailure || lookupCalls != 0 {
			t.Fatalf("nil output exit=%d lookup=%d", exitCode, lookupCalls)
		}
	}
}

func TestRunReplacesNilContext(t *testing.T) {
	runtime := &fakeMaintenanceRuntime{result: mustMaintenanceResult(t, 0, 0)}
	var factoryContext context.Context
	var output bytes.Buffer
	exitCode := runWithDependencies(
		nil,
		[]string{"run"},
		maintenanceLookup(nil),
		&output,
		runtimeDependencies{NewRuntime: func(ctx context.Context, _ appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
			factoryContext = ctx
			return runtime, nil
		}},
	)
	if exitCode != exitSuccess || factoryContext == nil || runtime.runContext == nil {
		t.Fatalf("nil context exit=%d factory=%v run=%v", exitCode, factoryContext, runtime.runContext)
	}
}

type fakeMaintenanceRuntime struct {
	result               identityapp.MaintenanceResult
	runErr               error
	closeErr             error
	runCalls             int
	closeCalls           int
	runContext           context.Context
	deadlineRemaining    time.Duration
	waitForContext       bool
	observedContextError error
	order                []string
}

func (runtime *fakeMaintenanceRuntime) Run(ctx context.Context) (identityapp.MaintenanceResult, error) {
	runtime.runCalls++
	runtime.order = append(runtime.order, "run")
	runtime.runContext = ctx
	if deadline, ok := ctx.Deadline(); ok {
		runtime.deadlineRemaining = time.Until(deadline)
	}
	if runtime.waitForContext {
		<-ctx.Done()
		runtime.observedContextError = ctx.Err()
		return identityapp.MaintenanceResult{}, ctx.Err()
	}
	return runtime.result, runtime.runErr
}

func (runtime *fakeMaintenanceRuntime) Close() error {
	runtime.closeCalls++
	runtime.order = append(runtime.order, "close")
	return runtime.closeErr
}

func fixedMaintenanceRuntimeFactory(runtime maintenanceRuntime, err error) runtimeFactory {
	return func(context.Context, appconfig.IdentityMaintenanceMySQLConfig) (maintenanceRuntime, error) {
		return runtime, err
	}
}

func maintenanceLookup(overrides map[string]string) appconfig.LookupFunc {
	values := map[string]string{
		"GROWTHOS_ENVIRONMENT":             "test",
		"GROWTHOS_LOG_LEVEL":               "info",
		"GROWTHOS_LOG_FORMAT":              "json",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD": "identity-database-password",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func mustMaintenanceResult(t *testing.T, sessionsDeleted, throttlesDeleted int) identityapp.MaintenanceResult {
	t.Helper()
	result, err := identityapp.NewMaintenanceResult(sessionsDeleted, throttlesDeleted)
	if err != nil {
		t.Fatalf("NewMaintenanceResult(%d, %d) error = %v", sessionsDeleted, throttlesDeleted, err)
	}
	return result
}

func decodeMaintenanceLog(t *testing.T, output string) map[string]any {
	t.Helper()
	lines := nonemptyMaintenanceLogLines(output)
	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1\n%s", len(lines), output)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode JSON log: %v\n%s", err, output)
	}
	return record
}

func nonemptyMaintenanceLogLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func assertMaintenanceLogFields(t *testing.T, record, want map[string]any) {
	t.Helper()
	for key, expected := range want {
		if got := record[key]; got != expected {
			t.Fatalf("log field %q = %#v, want %#v\n%#v", key, got, expected, record)
		}
	}
}

func newMaintenanceTestLogger(t *testing.T, output io.Writer) *slog.Logger {
	t.Helper()
	logger, err := logging.New(output, "info", "json")
	if err != nil {
		t.Fatal(err)
	}
	return logger
}
