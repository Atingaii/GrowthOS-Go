package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/strategycache"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/gin-gonic/gin"
)

func TestRunRejectsInvalidConfigurationWithoutEchoingValue(t *testing.T) {
	var output bytes.Buffer
	secretValue := "secret-invalid-level"
	exitCode := run(
		context.Background(),
		mapLookup(map[string]string{
			"GROWTHOS_LOG_LEVEL":               secretValue,
			"GROWTHOS_MYSQL_PASSWORD":          "unit-test-password",
			"GROWTHOS_IDENTITY_MYSQL_PASSWORD": "unit-test-identity-password",
		}),
		&output,
	)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if strings.Contains(output.String(), secretValue) {
		t.Fatalf("configuration log echoed rejected value: %s", output.String())
	}

	entry := decodeJSONLog(t, output.String())
	if entry["level"] != "ERROR" || entry["msg"] != "configuration rejected" {
		t.Fatalf("unexpected bootstrap log: %#v", entry)
	}
	if entry["service"] != serviceName || entry["version"] != version {
		t.Fatalf("bootstrap identity fields = %#v", entry)
	}
}

func TestRunLogsLifecycleWithValidatedConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	variables := map[string]string{
		"GROWTHOS_ENVIRONMENT":             "test",
		"GROWTHOS_HTTP_ADDRESS":            "127.0.0.1:9090",
		"GROWTHOS_LOG_LEVEL":               "info",
		"GROWTHOS_LOG_FORMAT":              "json",
		"GROWTHOS_MYSQL_PASSWORD":          "unit-test-password",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD": "unit-test-identity-password",
	}
	database := &stubDatabase{}

	if exitCode := runWithDependencies(
		ctx,
		mapLookup(variables),
		&output,
		runtimeDependencies{OpenRuntime: stubRuntimeOpener(database, nil)},
	); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if database.closeCalls != 1 {
		t.Fatalf("database close calls = %d, want exactly 1", database.closeCalls)
	}
	if got := gin.Mode(); got != gin.ReleaseMode {
		t.Fatalf("Gin mode = %q, want %q so stdout remains structured", got, gin.ReleaseMode)
	}

	lines := nonEmptyLines(output.String())
	if len(lines) != 2 {
		t.Fatalf("log line count = %d, want 2; output = %q", len(lines), output.String())
	}
	started := decodeJSONLog(t, lines[0])
	stopped := decodeJSONLog(t, lines[1])
	if started["msg"] != "service starting" || stopped["msg"] != "service stopped" {
		t.Fatalf("lifecycle messages = %q, %q", started["msg"], stopped["msg"])
	}
	if started["service"] != serviceName || started["environment"] != "test" || started["version"] != version {
		t.Fatalf("service identity fields = %#v", started)
	}
	if started["http_address"] != "127.0.0.1:9090" || started["shutdown_timeout_ms"] != float64(5000) {
		t.Fatalf("service configuration fields = %#v", started)
	}
}

func TestRunHonorsErrorLogLevel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	variables := map[string]string{
		"GROWTHOS_LOG_LEVEL":               "error",
		"GROWTHOS_LOG_FORMAT":              "json",
		"GROWTHOS_MYSQL_PASSWORD":          "unit-test-password",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD": "unit-test-identity-password",
	}

	if exitCode := runWithDependencies(
		ctx,
		mapLookup(variables),
		&output,
		runtimeDependencies{OpenRuntime: stubRuntimeOpener(&stubDatabase{}, nil)},
	); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if output.Len() != 0 {
		t.Fatalf("error-level logger emitted info lifecycle logs: %q", output.String())
	}
}

func TestRunOwnsAndClosesEnabledCacheRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache := newStubCacheRuntime()
	database := &stubDatabase{}
	var output bytes.Buffer
	variables := map[string]string{
		"GROWTHOS_ENVIRONMENT":                    "test",
		"GROWTHOS_MYSQL_PASSWORD":                 "unit-test-password",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD":        "unit-test-identity-password",
		"GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED": "true",
		"GROWTHOS_REDIS_PASSWORD":                 "redis-unit-test-password",
	}
	dependencies := runtimeDependencies{
		OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			components := stubRuntime(database)
			components.cache = cache
			return components, nil
		},
	}

	if exitCode := runWithDependencies(ctx, mapLookup(variables), &output, dependencies); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; output = %s", exitCode, output.String())
	}
	if cache.closeCalls != 1 || database.closeCalls != 1 {
		t.Fatalf("close calls cache/database = %d/%d, want 1/1", cache.closeCalls, database.closeCalls)
	}
}

func TestRunRedactsEnabledCacheShutdownFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	const secret = "redis-close-password=must-not-leak"
	cache := newStubCacheRuntime()
	cache.closeErr = errors.New(secret)
	database := &stubDatabase{}
	var output bytes.Buffer
	variables := map[string]string{
		"GROWTHOS_ENVIRONMENT":                    "test",
		"GROWTHOS_MYSQL_PASSWORD":                 "unit-test-password",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD":        "unit-test-identity-password",
		"GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED": "true",
		"GROWTHOS_REDIS_PASSWORD":                 "redis-unit-test-password",
	}
	dependencies := runtimeDependencies{
		OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			components := stubRuntime(database)
			components.cache = cache
			return components, nil
		},
	}

	if exitCode := runWithDependencies(ctx, mapLookup(variables), &output, dependencies); exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if cache.closeCalls != 1 || database.closeCalls != 1 {
		t.Fatalf("close calls cache/database = %d/%d, want 1/1", cache.closeCalls, database.closeCalls)
	}
	if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), `"msg":"cache shutdown failed"`) {
		t.Fatalf("cache close log was unsafe or missing: %s", output.String())
	}
}

func TestRunRejectsMissingDatabaseSecretBeforeOpening(t *testing.T) {
	var output bytes.Buffer
	openCalls := 0
	dependencies := runtimeDependencies{
		OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			openCalls++
			return stubRuntime(&stubDatabase{}), nil
		},
	}

	exitCode := runWithDependencies(context.Background(), mapLookup(nil), &output, dependencies)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if openCalls != 0 {
		t.Fatalf("database open calls = %d, want 0 after configuration rejection", openCalls)
	}
	if !strings.Contains(output.String(), "GROWTHOS_MYSQL_PASSWORD") {
		t.Fatalf("configuration log does not identify the missing secret variable: %s", output.String())
	}
}

func TestRunRedactsDatabaseStartupFailure(t *testing.T) {
	const secret = "mysql://root:must-not-leak@private-host/growthos"
	var output bytes.Buffer
	variables := runtimePasswordVariables()

	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(variables),
		&output,
		runtimeDependencies{OpenRuntime: stubRuntimeOpener(nil, errors.New(secret))},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "private-host") {
		t.Fatalf("database startup log leaked adapter error: %s", output.String())
	}

	entry := decodeJSONLog(t, output.String())
	if entry["level"] != "ERROR" || entry["msg"] != "runtime startup failed" || entry["component"] != "application" {
		t.Fatalf("unexpected runtime startup log: %#v", entry)
	}
}

func TestRunClosesUnexpectedDatabaseReturnedWithStartupError(t *testing.T) {
	const secret = "startup-error-password=must-not-leak"
	database := &stubDatabase{closeErr: errors.New("close-error-password=must-not-leak")}
	var output bytes.Buffer

	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimePasswordVariables()),
		&output,
		runtimeDependencies{OpenRuntime: stubRuntimeOpener(database, errors.New(secret))},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 {
		t.Fatalf("database close calls = %d, want exactly 1", database.closeCalls)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "close-error-password") {
		t.Fatalf("database startup log leaked an adapter error: %s", output.String())
	}
}

func TestRunRejectsTypedNilDatabase(t *testing.T) {
	var database *stubDatabase
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimePasswordVariables()),
		&output,
		runtimeDependencies{OpenRuntime: stubRuntimeOpener(database, nil)},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
		t.Fatalf("typed-nil database rejection was not logged safely: %s", output.String())
	}
}

func TestRunRejectsMissingSelectionServiceAndClosesDatabase(t *testing.T) {
	database := &stubDatabase{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimePasswordVariables()),
		&output,
		runtimeDependencies{OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			return runtimeComponents{database: database}, nil
		}},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 {
		t.Fatalf("database close calls = %d, want exactly 1 after partial composition", database.closeCalls)
	}
	if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
		t.Fatalf("missing selection service was not logged safely: %s", output.String())
	}
}

func TestRunRejectsUnconfiguredSelectionServiceAndClosesDatabase(t *testing.T) {
	database := &stubDatabase{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimePasswordVariables()),
		&output,
		runtimeDependencies{OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			return runtimeComponents{
				database:  database,
				selection: &application.EphemeralSelectionService{},
			}, nil
		}},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 {
		t.Fatalf("database close calls = %d, want exactly 1 after invalid composition", database.closeCalls)
	}
	if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
		t.Fatalf("unconfigured selection service was not logged safely: %s", output.String())
	}
}

func TestRunRejectsMissingRuntimeOpener(t *testing.T) {
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimePasswordVariables()),
		&output,
		runtimeDependencies{},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(output.String(), `"msg":"runtime dependency is unavailable"`) {
		t.Fatalf("missing runtime opener was not logged safely: %s", output.String())
	}
}

func TestRunClosesDatabaseAfterHTTPServerAndRedactsCloseFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	const secret = "close-error-password=must-not-leak"
	database := &stubDatabase{closeErr: errors.New(secret)}
	var output bytes.Buffer

	exitCode := runWithDependencies(
		ctx,
		mapLookup(runtimePasswordVariables()),
		&output,
		runtimeDependencies{OpenRuntime: stubRuntimeOpener(database, nil)},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 {
		t.Fatalf("database close calls = %d, want exactly 1", database.closeCalls)
	}
	if strings.Contains(output.String(), secret) {
		t.Fatalf("database shutdown log leaked adapter error: %s", output.String())
	}
	if !strings.Contains(output.String(), `"msg":"database shutdown failed"`) {
		t.Fatalf("database shutdown failure was not logged: %s", output.String())
	}
}

func TestRunRejectsNilOutput(t *testing.T) {
	if exitCode := run(context.Background(), mapLookup(nil), nil); exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}

	var typedNil *bytes.Buffer
	if exitCode := run(context.Background(), mapLookup(nil), typedNil); exitCode != 1 {
		t.Fatalf("run() typed-nil exit code = %d, want 1", exitCode)
	}
}

func TestHTTPServerConfigMapsEveryValidatedSetting(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	input := appconfig.HTTPConfig{
		Address:           "127.0.0.1:18080",
		ShutdownTimeout:   1 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      4 * time.Second,
		IdleTimeout:       5 * time.Second,
	}

	got := httpServerConfig(input, logger)
	if got.Address != input.Address ||
		got.ShutdownTimeout != input.ShutdownTimeout ||
		got.ReadHeaderTimeout != input.ReadHeaderTimeout ||
		got.ReadTimeout != input.ReadTimeout ||
		got.WriteTimeout != input.WriteTimeout ||
		got.IdleTimeout != input.IdleTimeout ||
		got.ErrorLogger != logger {
		t.Fatalf("httpServerConfig() = %#v, want all validated values and logger", got)
	}
}

func TestMySQLRuntimeConfigMapsEveryValidatedSetting(t *testing.T) {
	input := appconfig.MySQLConfig{
		MySQLConnectionConfig: appconfig.MySQLConnectionConfig{
			Address:        "db.internal.example:3307",
			Database:       "growthos_test",
			TLSMode:        appconfig.MySQLTLSVerifyIdentity,
			TLSCAFile:      "/runtime/ca.pem",
			ConnectTimeout: 2 * time.Second,
			ReadTimeout:    3 * time.Second,
			WriteTimeout:   4 * time.Second,
		},
		User:                  "growthos_app_test",
		Password:              "not-logged-test-password",
		PingTimeout:           5 * time.Second,
		MaxOpenConnections:    6,
		MaxIdleConnections:    4,
		ConnectionMaxLifetime: 7 * time.Minute,
		ConnectionMaxIdleTime: 8 * time.Minute,
	}

	got := mysqlRuntimeConfig(input)
	if got.Address != input.Address ||
		got.Database != input.Database ||
		got.User != input.User ||
		got.Password != input.Password ||
		string(got.TLSMode) != string(input.TLSMode) ||
		got.TLSCAFile != input.TLSCAFile ||
		got.ConnectTimeout != input.ConnectTimeout ||
		got.ReadTimeout != input.ReadTimeout ||
		got.WriteTimeout != input.WriteTimeout ||
		got.PingTimeout != input.PingTimeout ||
		got.MaxOpenConnections != input.MaxOpenConnections ||
		got.MaxIdleConnections != input.MaxIdleConnections ||
		got.ConnectionMaxLifetime != input.ConnectionMaxLifetime ||
		got.ConnectionMaxIdleTime != input.ConnectionMaxIdleTime {
		t.Fatal("mysqlRuntimeConfig() did not map every validated field")
	}
}

func TestRedisRuntimeConfigMapsEveryValidatedSetting(t *testing.T) {
	input := appconfig.RedisConfig{
		Address:               "cache.internal.example:6380",
		Username:              "growthos_api_test",
		Password:              "not-logged-redis-password",
		Database:              0,
		TLSMode:               appconfig.RedisTLSVerifyIdentity,
		TLSCAFile:             "/runtime/redis-ca.pem",
		DialTimeout:           2 * time.Second,
		ReadTimeout:           3 * time.Second,
		WriteTimeout:          4 * time.Second,
		PoolTimeout:           5 * time.Second,
		PoolSize:              6,
		MinIdleConnections:    2,
		ConnectionMaxLifetime: 7 * time.Minute,
		ConnectionMaxIdleTime: 8 * time.Minute,
	}

	got := redisRuntimeConfig(input)
	if got.Address != input.Address || got.Username != input.Username || got.Password != input.Password ||
		got.Database != input.Database || string(got.TLSMode) != string(input.TLSMode) ||
		got.TLSCAFile != input.TLSCAFile || got.DialTimeout != input.DialTimeout ||
		got.ReadTimeout != input.ReadTimeout || got.WriteTimeout != input.WriteTimeout ||
		got.PoolTimeout != input.PoolTimeout || got.PoolSize != input.PoolSize ||
		got.MinIdleConnections != input.MinIdleConnections ||
		got.ConnectionMaxLifetime != input.ConnectionMaxLifetime ||
		got.ConnectionMaxIdleTime != input.ConnectionMaxIdleTime {
		t.Fatal("redisRuntimeConfig() did not map every validated field")
	}
}

func TestStrategyCacheOptionsMapEnvironmentPolicyAndLifecycle(t *testing.T) {
	lifecycle, cancel := context.WithCancel(context.Background())
	defer cancel()
	observer := &stubStrategyCacheObserver{}
	config := runtimeConfiguration{
		Environment: appconfig.EnvironmentStaging,
		StrategyCache: appconfig.StrategyCacheConfig{
			Enabled:       true,
			TTL:           4 * time.Minute,
			LookupTimeout: 100 * time.Millisecond,
			WriteTimeout:  120 * time.Millisecond,
			FillTimeout:   2 * time.Second,
		},
	}

	got := strategyCacheOptions(lifecycle, config, observer)
	if got.Namespace != "growthos:staging" || got.TTL != config.StrategyCache.TTL ||
		got.LookupTimeout != config.StrategyCache.LookupTimeout ||
		got.WriteTimeout != config.StrategyCache.WriteTimeout ||
		got.FillTimeout != config.StrategyCache.FillTimeout ||
		got.Lifecycle != lifecycle || got.Observer != observer {
		t.Fatal("strategyCacheOptions() did not map the validated cache policy")
	}
}

type stubDatabase struct {
	pingErr    error
	closeErr   error
	closeCalls int
}

func (database *stubDatabase) PingContext(context.Context) error {
	return database.pingErr
}

func (database *stubDatabase) Close() error {
	database.closeCalls++
	return database.closeErr
}

func stubRuntimeOpener(database databaseRuntime, err error) func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
	return func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
		return stubRuntime(database), err
	}
}

func stubRuntime(database databaseRuntime) runtimeComponents {
	selection, err := application.NewEphemeralSelectionService(stubStrategyReader{}, stubAwardSelector{})
	if err != nil {
		panic(err)
	}
	return runtimeComponents{database: database, selection: selection}
}

type stubStrategyReader struct{}

func (stubStrategyReader) FindByID(context.Context, domain.StrategyID) (domain.Strategy, error) {
	return domain.Strategy{}, application.ErrStrategyNotFound
}

type stubAwardSelector struct{}

func (stubAwardSelector) Select(domain.Strategy) (domain.Award, error) {
	return domain.Award{}, domain.ErrSelectionStrategyInvalid
}

type stubStrategyCacheObserver struct{}

func (*stubStrategyCacheObserver) Observe(context.Context, strategycache.Observation) {}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func runtimePasswordVariables() map[string]string {
	return map[string]string{
		"GROWTHOS_MYSQL_PASSWORD":          "unit-test-password",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD": "unit-test-identity-password",
	}
}

func decodeJSONLog(t *testing.T, line string) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &entry); err != nil {
		t.Fatalf("decode log %q: %v", line, err)
	}
	return entry
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
