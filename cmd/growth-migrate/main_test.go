package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	dbmigration "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/migration"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
)

func TestRunParsesCommandBeforeLoggerConfigurationAndEnvironment(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "extra", args: []string{"up", "status"}},
		{name: "down", args: []string{"down"}},
		{name: "drop", args: []string{"drop"}},
		{name: "force", args: []string{"force"}},
		{name: "uppercase", args: []string{"UP"}},
		{name: "whitespace", args: []string{" up"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookupCalls := 0
			factoryCalls := 0
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				test.args,
				func(string) (string, bool) {
					lookupCalls++
					return "", false
				},
				&output,
				runtimeDependencies{NewRunner: func(context.Context, appconfig.MigrationMySQLConfig) (migrationRunner, error) {
					factoryCalls++
					return nil, nil
				}},
			)

			if exitCode != exitUsage {
				t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
			}
			if lookupCalls != 0 || factoryCalls != 0 {
				t.Fatalf("invalid command reached dependencies: lookup=%d factory=%d", lookupCalls, factoryCalls)
			}
			if output.Len() != 0 {
				t.Fatalf("invalid command bootstrapped logger: %q", output.String())
			}
		})
	}
}

func TestRunUpUsesMigrationOnlyConfigurationAndClosesOnce(t *testing.T) {
	var output bytes.Buffer
	runner := &fakeMigrationRunner{
		upResult: dbmigration.Result{State: dbmigration.ResultApplied, Version: 3},
	}
	var gotConfig appconfig.MigrationMySQLConfig
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"up"},
		migrationLookup(map[string]string{
			"GROWTHOS_ENVIRONMENT":              "test",
			"GROWTHOS_LOG_LEVEL":                "info",
			"GROWTHOS_LOG_FORMAT":               "json",
			"GROWTHOS_MYSQL_MIGRATION_USER":     "lesson13_migrator",
			"GROWTHOS_MYSQL_MIGRATION_PASSWORD": "migration-password",
			// These belong to the API process and must not participate in the
			// independent migration command's configuration boundary.
			"GROWTHOS_HTTP_ADDRESS":         "not-an-address",
			"GROWTHOS_MYSQL_USER":           " invalid-api-user ",
			"GROWTHOS_MYSQL_PASSWORD":       "",
			"GROWTHOS_MYSQL_MAX_OPEN_CONNS": "not-a-number",
		}),
		&output,
		runtimeDependencies{NewRunner: func(_ context.Context, config appconfig.MigrationMySQLConfig) (migrationRunner, error) {
			gotConfig = config
			return runner, nil
		}},
	)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d\n%s", exitCode, exitSuccess, output.String())
	}
	if gotConfig.User != "lesson13_migrator" || gotConfig.Password != "migration-password" {
		t.Fatal("factory did not receive the separately configured migration identity")
	}
	if runner.upCalls != 1 || runner.statusCalls != 0 || runner.closeCalls != 1 {
		t.Fatalf("runner calls: up=%d status=%d close=%d", runner.upCalls, runner.statusCalls, runner.closeCalls)
	}

	record := decodeMigrationLog(t, output.String())
	assertMigrationLogFields(t, record, map[string]any{
		"level":             "INFO",
		"msg":               "migration command completed",
		"service":           migrationServiceName,
		"environment":       "test",
		"version":           version,
		"migration_version": float64(3),
		"component":         "migration",
		"operation":         "up",
		"result":            string(dbmigration.ResultApplied),
	})
}

func TestRunTreatsIdempotentUpResultsAsSuccess(t *testing.T) {
	for _, state := range []dbmigration.ResultState{
		dbmigration.ResultNoMigrations,
		dbmigration.ResultNoChange,
	} {
		t.Run(string(state), func(t *testing.T) {
			runner := &fakeMigrationRunner{upResult: dbmigration.Result{State: state}}
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				[]string{"up"},
				migrationLookup(nil),
				&output,
				runtimeDependencies{NewRunner: fixedRunnerFactory(runner, nil)},
			)

			if exitCode != exitSuccess {
				t.Fatalf("exit code = %d, want success\n%s", exitCode, output.String())
			}
			if runner.closeCalls != 1 {
				t.Fatalf("Close calls = %d, want 1", runner.closeCalls)
			}
			if got := decodeMigrationLog(t, output.String())["result"]; got != string(state) {
				t.Fatalf("result = %#v, want %q", got, state)
			}
		})
	}
}

func TestRunStatusReportsCleanAndNoMigrationStates(t *testing.T) {
	for _, status := range []dbmigration.Status{
		{State: dbmigration.StatusNoMigrations},
		{State: dbmigration.StatusUninitialized, Latest: 4},
		{State: dbmigration.StatusPending, Version: 3, Latest: 4},
		{State: dbmigration.StatusClean, Version: 4, Latest: 4},
	} {
		t.Run(string(status.State), func(t *testing.T) {
			runner := &fakeMigrationRunner{statusResult: status}
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				[]string{"status"},
				migrationLookup(nil),
				&output,
				runtimeDependencies{NewRunner: fixedRunnerFactory(runner, nil)},
			)

			if exitCode != exitSuccess {
				t.Fatalf("exit code = %d, want success\n%s", exitCode, output.String())
			}
			if runner.upCalls != 0 || runner.statusCalls != 1 || runner.closeCalls != 1 {
				t.Fatalf("runner calls: up=%d status=%d close=%d", runner.upCalls, runner.statusCalls, runner.closeCalls)
			}
			record := decodeMigrationLog(t, output.String())
			assertMigrationLogFields(t, record, map[string]any{
				"msg":                      "migration status completed",
				"operation":                "status",
				"result":                   string(status.State),
				"version":                  version,
				"migration_version":        float64(status.Version),
				"latest_migration_version": float64(status.Latest),
			})
		})
	}
}

func TestRunDirtyStatusFailsAndClosesExactlyOnce(t *testing.T) {
	runner := &fakeMigrationRunner{statusErr: dbmigration.ErrDirty}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"status"},
		migrationLookup(nil),
		&output,
		runtimeDependencies{NewRunner: fixedRunnerFactory(runner, nil)},
	)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want %d", exitCode, exitFailure)
	}
	if runner.statusCalls != 1 || runner.closeCalls != 1 {
		t.Fatalf("runner calls: status=%d close=%d", runner.statusCalls, runner.closeCalls)
	}
	record := decodeMigrationLog(t, output.String())
	if record["msg"] != "migration status is dirty" || record["operation"] != "status" {
		t.Fatalf("dirty status log = %#v", record)
	}
}

func TestRunNeverLogsFactoryOperationOrCloseCauses(t *testing.T) {
	tests := []struct {
		name       string
		factoryErr error
		upErr      error
		closeErr   error
	}{
		{name: "factory", factoryErr: errors.New("dsn=migrator:factory-secret@tcp(private-host)")},
		{name: "operation", upErr: errors.New("ALTER TABLE private_secret operation-secret")},
		{name: "close", closeErr: errors.New("driver close password=close-secret")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeMigrationRunner{
				upResult: dbmigration.Result{State: dbmigration.ResultNoChange},
				upErr:    test.upErr,
				closeErr: test.closeErr,
			}
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				[]string{"up"},
				migrationLookup(nil),
				&output,
				runtimeDependencies{NewRunner: fixedRunnerFactory(runner, test.factoryErr)},
			)

			if exitCode != exitFailure {
				t.Fatalf("exit code = %d, want failure", exitCode)
			}
			for _, secret := range []string{
				"factory-secret",
				"private-host",
				"operation-secret",
				"private_secret",
				"close-secret",
			} {
				if strings.Contains(output.String(), secret) {
					t.Fatalf("safe log leaked %q: %s", secret, output.String())
				}
			}
			if test.factoryErr != nil {
				if runner.closeCalls != 1 {
					t.Fatalf("runner returned with factory error was closed %d times, want 1", runner.closeCalls)
				}
			} else if runner.closeCalls != 1 {
				t.Fatalf("Close calls = %d, want exactly 1", runner.closeCalls)
			}
		})
	}
}

func TestRunPassesCancellationContextAndStillCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeMigrationRunner{upErr: context.Canceled}
	var factoryContext context.Context
	var output bytes.Buffer
	exitCode := runWithDependencies(
		ctx,
		[]string{"up"},
		migrationLookup(nil),
		&output,
		runtimeDependencies{NewRunner: func(got context.Context, _ appconfig.MigrationMySQLConfig) (migrationRunner, error) {
			factoryContext = got
			return runner, nil
		}},
	)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want failure", exitCode)
	}
	if factoryContext != ctx || runner.upContext != ctx {
		t.Fatal("signal-derived context was not passed through factory and operation")
	}
	if runner.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", runner.closeCalls)
	}
	if strings.Contains(output.String(), context.Canceled.Error()) {
		t.Fatalf("cancellation cause was expanded into log: %s", output.String())
	}
}

func TestRunDefendsAgainstNilDependencies(t *testing.T) {
	tests := []struct {
		name         string
		dependencies runtimeDependencies
	}{
		{name: "nil factory"},
		{name: "nil runner", dependencies: runtimeDependencies{NewRunner: fixedRunnerFactory(nil, nil)}},
		{name: "typed nil runner", dependencies: runtimeDependencies{NewRunner: func(context.Context, appconfig.MigrationMySQLConfig) (migrationRunner, error) {
			var runner *fakeMigrationRunner
			return runner, nil
		}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if exitCode := runWithDependencies(
				context.Background(),
				[]string{"up"},
				migrationLookup(nil),
				&output,
				test.dependencies,
			); exitCode != exitFailure {
				t.Fatalf("exit code = %d, want failure", exitCode)
			}
			if !strings.Contains(output.String(), `"component":"migration"`) {
				t.Fatalf("missing stable dependency log: %s", output.String())
			}
		})
	}
}

func TestRunRejectsNilAndTypedNilOutputBeforeConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		output io.Writer
	}{
		{name: "nil"},
		{name: "typed nil", output: (*bytes.Buffer)(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookupCalls := 0
			exitCode := runWithDependencies(
				context.Background(),
				[]string{"up"},
				func(string) (string, bool) {
					lookupCalls++
					return "", false
				},
				test.output,
				runtimeDependencies{},
			)
			if exitCode != exitFailure {
				t.Fatalf("exit code = %d, want failure", exitCode)
			}
			if lookupCalls != 0 {
				t.Fatalf("nil output reached configuration lookup %d times", lookupCalls)
			}
		})
	}
}

func TestRunConfigurationFailureDoesNotEchoRejectedValue(t *testing.T) {
	const secret = "secret-invalid-log-format"
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"status"},
		migrationLookup(map[string]string{
			"GROWTHOS_LOG_FORMAT":               secret,
			"GROWTHOS_MYSQL_MIGRATION_PASSWORD": "valid-migration-password",
		}),
		&output,
		runtimeDependencies{},
	)

	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want failure", exitCode)
	}
	if strings.Contains(output.String(), secret) {
		t.Fatalf("configuration log leaked rejected value: %s", output.String())
	}
	record := decodeMigrationLog(t, output.String())
	if record["component"] != "configuration" || record["operation"] != "load" {
		t.Fatalf("configuration log = %#v", record)
	}
	if errorText, ok := record["error"].(string); !ok || !strings.Contains(errorText, "GROWTHOS_LOG_FORMAT") {
		t.Fatalf("configuration log did not identify the rejected variable: %#v", record)
	}
}

func TestRunReplacesNilContextWithUsableContext(t *testing.T) {
	runner := &fakeMigrationRunner{upResult: dbmigration.Result{State: dbmigration.ResultNoChange}}
	var factoryContext context.Context
	var output bytes.Buffer
	exitCode := runWithDependencies(
		nil,
		[]string{"up"},
		migrationLookup(nil),
		&output,
		runtimeDependencies{NewRunner: func(ctx context.Context, _ appconfig.MigrationMySQLConfig) (migrationRunner, error) {
			factoryContext = ctx
			return runner, nil
		}},
	)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want success\n%s", exitCode, output.String())
	}
	if factoryContext == nil || runner.upContext == nil {
		t.Fatal("nil input context was not replaced")
	}
}

type fakeMigrationRunner struct {
	upResult     dbmigration.Result
	statusResult dbmigration.Status
	upErr        error
	statusErr    error
	closeErr     error
	upCalls      int
	statusCalls  int
	closeCalls   int
	upContext    context.Context
}

func (runner *fakeMigrationRunner) Up(ctx context.Context) (dbmigration.Result, error) {
	runner.upCalls++
	runner.upContext = ctx
	return runner.upResult, runner.upErr
}

func (runner *fakeMigrationRunner) Status(context.Context) (dbmigration.Status, error) {
	runner.statusCalls++
	return runner.statusResult, runner.statusErr
}

func (runner *fakeMigrationRunner) Close() error {
	runner.closeCalls++
	return runner.closeErr
}

func fixedRunnerFactory(runner migrationRunner, err error) runnerFactory {
	return func(context.Context, appconfig.MigrationMySQLConfig) (migrationRunner, error) {
		return runner, err
	}
}

func migrationLookup(overrides map[string]string) appconfig.LookupFunc {
	values := map[string]string{
		"GROWTHOS_ENVIRONMENT":              "test",
		"GROWTHOS_LOG_LEVEL":                "info",
		"GROWTHOS_LOG_FORMAT":               "json",
		"GROWTHOS_MYSQL_MIGRATION_PASSWORD": "migration-password",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func decodeMigrationLog(t *testing.T, output string) map[string]any {
	t.Helper()
	lines := nonemptyMigrationLogLines(output)
	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1\n%s", len(lines), output)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode JSON log: %v\n%s", err, output)
	}
	return record
}

func nonemptyMigrationLogLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func assertMigrationLogFields(t *testing.T, record, want map[string]any) {
	t.Helper()
	for key, expected := range want {
		if got := record[key]; got != expected {
			t.Fatalf("log field %q = %#v, want %#v\n%#v", key, got, expected, record)
		}
	}
}
