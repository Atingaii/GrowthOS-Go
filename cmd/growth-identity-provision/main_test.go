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

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlprovisioner"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/logging"
)

func TestRunParsesCreateBeforeOutputConfigurationFileAndRuntime(t *testing.T) {
	tests := [][]string{
		nil,
		{"create"},
		{"delete", "--account-id", "account.alex", "--login-name", "alex.rivera", "--principal-id", "principal.alex", "--password-file", "/private/path"},
		{"create", "--account-id", "account.alex", "--login-name", "alex.rivera", "--principal-id", "principal.alex", "--password", "PRIVATE_RAW_PASSWORD"},
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
			runtimeDependencies{NewRuntime: func(context.Context, appconfig.IdentityProvisionerConfig) (provisionRuntime, error) {
				factoryCalls++
				return nil, nil
			}},
		)
		if exitCode != exitUsage || lookupCalls != 0 || factoryCalls != 0 || output.Len() != 0 {
			t.Fatalf("invalid grammar crossed a dependency boundary: exit=%d lookup=%d factory=%d output=%q", exitCode, lookupCalls, factoryCalls, output.String())
		}
	}
}

func TestRunCreatesOnceWithDedicatedConfigAndClearsPassword(t *testing.T) {
	const (
		enrollmentPassword = "GrowthOS-enrollment-32"
		databasePassword   = "PRIVATE_PROVISIONER_DATABASE_PASSWORD"
	)
	passwordPath := writePasswordFixture(t, []byte(enrollmentPassword+"\n"))
	args := validProvisionArgs(passwordPath)
	runtime := &fakeProvisionRuntime{}
	var gotConfig appconfig.IdentityProvisionerConfig
	var factoryContext context.Context
	var output bytes.Buffer

	exitCode := runWithDependencies(
		context.Background(),
		args,
		provisionLookup(map[string]string{
			"GROWTHOS_IDENTITY_PROVISIONER_MYSQL_USER":     "identity_provisioner_test",
			"GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD": databasePassword,
		}),
		&output,
		runtimeDependencies{NewRuntime: func(ctx context.Context, config appconfig.IdentityProvisionerConfig) (provisionRuntime, error) {
			factoryContext = ctx
			gotConfig = config
			return runtime, nil
		}},
	)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want success\n%s", exitCode, output.String())
	}
	if factoryContext == nil || gotConfig.MySQL.User != "identity_provisioner_test" || gotConfig.MySQL.Password != databasePassword {
		t.Fatal("factory did not receive the dedicated validated provisioner configuration")
	}
	if runtime.createCalls != 1 || runtime.closeCalls != 1 || runtime.gotCommand != mustProvisionCommand(t, args) {
		t.Fatalf("runtime calls create=%d close=%d command=%#v", runtime.createCalls, runtime.closeCalls, runtime.gotCommand)
	}
	if string(runtime.passwordCopy) != enrollmentPassword {
		t.Fatal("runtime did not receive the exact enrollment password payload")
	}
	if runtime.createContext == nil {
		t.Fatal("runtime received a nil operation context")
	}
	if runtime.deadlineRemaining <= 0 || runtime.deadlineRemaining > 4*time.Second {
		t.Fatalf("operation budget at invocation = %s, want bounded default", runtime.deadlineRemaining)
	}
	for _, value := range runtime.passwordReference {
		if value != 0 {
			t.Fatal("caller-owned enrollment password was not cleared after the command")
		}
	}

	record := decodeProvisionLog(t, output.String())
	assertProvisionLogFields(t, record, map[string]any{
		"level":       "INFO",
		"msg":         "identity account provision completed",
		"service":     identityProvisionServiceName,
		"environment": "test",
		"version":     version,
		"component":   "identity_provisioning",
		"operation":   provisionCreateCommand,
		"result":      "created",
	})
	for _, private := range []string{
		enrollmentPassword,
		databasePassword,
		passwordPath,
		"account.alex",
		"alex.rivera",
		"principal.alex",
	} {
		if strings.Contains(output.String(), private) {
			t.Fatalf("success log leaked private input %q: %s", private, output.String())
		}
	}
}

func TestRunRejectsPasswordFileBeforeOpeningRuntime(t *testing.T) {
	const privatePath = "/PRIVATE/MISSING/ENROLLMENT/PASSWORD"
	factoryCalls := 0
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		validProvisionArgs(privatePath),
		provisionLookup(nil),
		&output,
		runtimeDependencies{NewRuntime: func(context.Context, appconfig.IdentityProvisionerConfig) (provisionRuntime, error) {
			factoryCalls++
			return nil, nil
		}},
	)
	if exitCode != exitFailure || factoryCalls != 0 {
		t.Fatalf("password file failure exit=%d factory=%d", exitCode, factoryCalls)
	}
	if strings.Contains(output.String(), privatePath) || !strings.Contains(output.String(), "credential_input") {
		t.Fatalf("password file log leaked its path or omitted the stable component: %s", output.String())
	}
}

func TestRunRejectsMissingFactoryBeforeReadingEnrollmentPassword(t *testing.T) {
	const privateMissingPath = "/PRIVATE/PATH/MUST/NOT/BE/OPENED"
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		validProvisionArgs(privateMissingPath),
		provisionLookup(nil),
		&output,
		runtimeDependencies{},
	)
	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want failure", exitCode)
	}
	if !strings.Contains(output.String(), "identity_provisioning") ||
		strings.Contains(output.String(), "credential_input") ||
		strings.Contains(output.String(), privateMissingPath) {
		t.Fatalf("missing-factory boundary log = %s", output.String())
	}
}

func TestRunConfigurationFailurePrecedesPasswordAndRedactsRejectedValues(t *testing.T) {
	const (
		privatePath  = "/PRIVATE/PATH/THAT/MUST/NOT/BE/READ"
		privateValue = "PRIVATE_INVALID_LOG_FORMAT"
	)
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		validProvisionArgs(privatePath),
		provisionLookup(map[string]string{"GROWTHOS_LOG_FORMAT": privateValue}),
		&output,
		runtimeDependencies{},
	)
	if exitCode != exitFailure {
		t.Fatalf("exit code = %d, want failure", exitCode)
	}
	if strings.Contains(output.String(), privatePath) || strings.Contains(output.String(), privateValue) {
		t.Fatalf("configuration failure leaked a rejected value or later file path: %s", output.String())
	}
	record := decodeProvisionLog(t, output.String())
	if record["component"] != "configuration" || record["operation"] != "load" {
		t.Fatalf("configuration record = %#v", record)
	}
}

func TestRunClosesUnexpectedRuntimeFromFactoryFailureWithoutLoggingCause(t *testing.T) {
	const privateCause = "PRIVATE_DSN_FACTORY_CAUSE"
	runtime := &fakeProvisionRuntime{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		validProvisionArgs(writePasswordFixture(t, []byte("GrowthOS-enrollment-32"))),
		provisionLookup(nil),
		&output,
		runtimeDependencies{NewRuntime: fixedProvisionRuntimeFactory(runtime, errors.New(privateCause))},
	)
	if exitCode != exitFailure || runtime.closeCalls != 1 || runtime.createCalls != 0 {
		t.Fatalf("factory failure exit=%d create=%d close=%d", exitCode, runtime.createCalls, runtime.closeCalls)
	}
	if strings.Contains(output.String(), privateCause) {
		t.Fatalf("factory log leaked cause: %s", output.String())
	}
}

func TestRunDefendsAgainstMissingAndTypedNilRuntime(t *testing.T) {
	tests := []struct {
		name         string
		dependencies runtimeDependencies
	}{
		{name: "nil factory"},
		{name: "nil runtime", dependencies: runtimeDependencies{NewRuntime: fixedProvisionRuntimeFactory(nil, nil)}},
		{name: "typed nil runtime", dependencies: runtimeDependencies{NewRuntime: func(context.Context, appconfig.IdentityProvisionerConfig) (provisionRuntime, error) {
			var runtime *fakeProvisionRuntime
			return runtime, nil
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				validProvisionArgs(writePasswordFixture(t, []byte("GrowthOS-enrollment-32"))),
				provisionLookup(nil),
				&output,
				test.dependencies,
			)
			if exitCode != exitFailure || !strings.Contains(output.String(), "identity_provisioning") {
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
		{name: "already exists", err: mysqlprovisioner.ErrAlreadyExists, message: "identity account already exists"},
		{name: "unknown commit", err: mysqlprovisioner.ErrCommitOutcomeUnknown, message: "identity account provision outcome is unknown"},
		{name: "wrapped unknown commit", err: fmt.Errorf("PRIVATE_COMMIT_CAUSE: %w", mysqlprovisioner.ErrCommitOutcomeUnknown), message: "identity account provision outcome is unknown"},
		{name: "unknown dominates duplicate", err: errors.Join(mysqlprovisioner.ErrAlreadyExists, mysqlprovisioner.ErrCommitOutcomeUnknown), message: "identity account provision outcome is unknown"},
		{name: "unknown dominates cancel", err: errors.Join(context.Canceled, mysqlprovisioner.ErrCommitOutcomeUnknown), message: "identity account provision outcome is unknown"},
		{name: "canceled", err: context.Canceled, message: "identity account provision canceled"},
		{name: "wrapped stable cancel", err: fmt.Errorf("PRIVATE_CANCEL_CAUSE: %w", mysqlprovisioner.ErrOperationCanceled), message: "identity account provision canceled"},
		{name: "dependency", err: errors.New("PRIVATE_PASSWORD_DSN_ENVELOPE"), message: "identity account provision failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeProvisionRuntime{createErr: test.err}
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				validProvisionArgs(writePasswordFixture(t, []byte("GrowthOS-enrollment-32"))),
				provisionLookup(nil),
				&output,
				runtimeDependencies{NewRuntime: fixedProvisionRuntimeFactory(runtime, nil)},
			)
			if exitCode != exitFailure || runtime.createCalls != 1 || runtime.closeCalls != 1 {
				t.Fatalf("operation failure exit=%d create=%d close=%d", exitCode, runtime.createCalls, runtime.closeCalls)
			}
			if !strings.Contains(output.String(), test.message) || strings.Contains(output.String(), "PRIVATE_") {
				t.Fatalf("operation log = %s", output.String())
			}
			for _, value := range runtime.passwordReference {
				if value != 0 {
					t.Fatal("operation failure retained the enrollment password")
				}
			}
		})
	}
}

func TestExecuteProvisionCloseFailureDowngradesSuccess(t *testing.T) {
	runtime := &fakeProvisionRuntime{closeErr: errors.New("PRIVATE_CLOSE_DSN")}
	var output bytes.Buffer
	logger := newProvisionTestLogger(t, &output)
	command := mustProvisionCommand(t, validProvisionArgs("ignored"))

	exitCode := executeProvisionCommand(
		context.Background(),
		time.Second,
		command,
		[]byte("GrowthOS-enrollment-32"),
		runtime,
		logger,
	)
	if exitCode != exitFailure || runtime.createCalls != 1 || runtime.closeCalls != 1 {
		t.Fatalf("close failure exit=%d create=%d close=%d", exitCode, runtime.createCalls, runtime.closeCalls)
	}
	if strings.Contains(output.String(), "PRIVATE_CLOSE_DSN") ||
		!strings.Contains(output.String(), "runtime close failed") ||
		strings.Contains(output.String(), "provision completed") {
		t.Fatalf("close log = %s", output.String())
	}
}

func TestExecuteProvisionEnforcesOperationDeadlineAndStillCloses(t *testing.T) {
	runtime := &fakeProvisionRuntime{waitForContext: true}
	var output bytes.Buffer
	start := time.Now()
	exitCode := executeProvisionCommand(
		context.Background(),
		5*time.Millisecond,
		mustProvisionCommand(t, validProvisionArgs("ignored")),
		[]byte("GrowthOS-enrollment-32"),
		runtime,
		newProvisionTestLogger(t, &output),
	)
	if exitCode != exitFailure || runtime.closeCalls != 1 || !errors.Is(runtime.observedContextError, context.DeadlineExceeded) {
		t.Fatalf("deadline exit=%d close=%d error=%v", exitCode, runtime.closeCalls, runtime.observedContextError)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("operation deadline took %s", elapsed)
	}
}

func TestExecuteProvisionRejectsInvalidBudgetWithoutCallingCreate(t *testing.T) {
	runtime := &fakeProvisionRuntime{}
	var output bytes.Buffer
	password := []byte("GrowthOS-enrollment-32")
	exitCode := executeProvisionCommand(
		context.Background(),
		0,
		mustProvisionCommand(t, validProvisionArgs("ignored")),
		password,
		runtime,
		newProvisionTestLogger(t, &output),
	)
	if exitCode != exitFailure || runtime.createCalls != 0 || runtime.closeCalls != 1 {
		t.Fatalf("invalid budget exit=%d create=%d close=%d", exitCode, runtime.createCalls, runtime.closeCalls)
	}
	for _, value := range password {
		if value != 0 {
			t.Fatal("invalid operation budget retained the enrollment password")
		}
	}
}

func TestRunRejectsNilAndTypedNilOutputBeforeConfiguration(t *testing.T) {
	for _, output := range []io.Writer{nil, (*bytes.Buffer)(nil)} {
		lookupCalls := 0
		exitCode := runWithDependencies(
			context.Background(),
			validProvisionArgs("ignored"),
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
	runtime := &fakeProvisionRuntime{}
	var factoryContext context.Context
	var output bytes.Buffer
	exitCode := runWithDependencies(
		nil,
		validProvisionArgs(writePasswordFixture(t, []byte("GrowthOS-enrollment-32"))),
		provisionLookup(nil),
		&output,
		runtimeDependencies{NewRuntime: func(ctx context.Context, _ appconfig.IdentityProvisionerConfig) (provisionRuntime, error) {
			factoryContext = ctx
			return runtime, nil
		}},
	)
	if exitCode != exitSuccess || factoryContext == nil || runtime.createContext == nil {
		t.Fatalf("nil context exit=%d factory=%v create=%v", exitCode, factoryContext, runtime.createContext)
	}
}

type fakeProvisionRuntime struct {
	createErr            error
	closeErr             error
	createCalls          int
	closeCalls           int
	gotCommand           provisionCommand
	passwordCopy         []byte
	passwordReference    []byte
	createContext        context.Context
	deadlineRemaining    time.Duration
	waitForContext       bool
	observedContextError error
}

func (runtime *fakeProvisionRuntime) Create(ctx context.Context, command provisionCommand, password []byte) error {
	runtime.createCalls++
	runtime.createContext = ctx
	if deadline, ok := ctx.Deadline(); ok {
		runtime.deadlineRemaining = time.Until(deadline)
	}
	runtime.gotCommand = command
	runtime.passwordCopy = append([]byte(nil), password...)
	runtime.passwordReference = password
	if runtime.waitForContext {
		<-ctx.Done()
		runtime.observedContextError = ctx.Err()
		return ctx.Err()
	}
	return runtime.createErr
}

func (runtime *fakeProvisionRuntime) Close() error {
	runtime.closeCalls++
	return runtime.closeErr
}

func fixedProvisionRuntimeFactory(runtime provisionRuntime, err error) runtimeFactory {
	return func(context.Context, appconfig.IdentityProvisionerConfig) (provisionRuntime, error) {
		return runtime, err
	}
}

func validProvisionArgs(passwordPath string) []string {
	return []string{
		"create",
		"--account-id", "account.alex",
		"--login-name", "alex.rivera",
		"--principal-id", "principal.alex",
		"--password-file", passwordPath,
	}
}

func mustProvisionCommand(t *testing.T, args []string) provisionCommand {
	t.Helper()
	command, ok := parseProvisionCommand(args)
	if !ok {
		t.Fatalf("invalid test command: %#v", args)
	}
	return command
}

func provisionLookup(overrides map[string]string) appconfig.LookupFunc {
	values := map[string]string{
		"GROWTHOS_ENVIRONMENT":                         "test",
		"GROWTHOS_LOG_LEVEL":                           "info",
		"GROWTHOS_LOG_FORMAT":                          "json",
		"GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD": "provisioner-database-password",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func decodeProvisionLog(t *testing.T, output string) map[string]any {
	t.Helper()
	lines := nonemptyProvisionLogLines(output)
	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1\n%s", len(lines), output)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode JSON log: %v\n%s", err, output)
	}
	return record
}

func nonemptyProvisionLogLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func assertProvisionLogFields(t *testing.T, record, want map[string]any) {
	t.Helper()
	for key, expected := range want {
		if got := record[key]; got != expected {
			t.Fatalf("log field %q = %#v, want %#v\n%#v", key, got, expected, record)
		}
	}
}

func newProvisionTestLogger(t *testing.T, output io.Writer) *slog.Logger {
	t.Helper()
	logger, err := logging.New(output, "info", "json")
	if err != nil {
		t.Fatal(err)
	}
	return logger
}
