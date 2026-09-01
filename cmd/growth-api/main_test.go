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
		mapLookup(runtimeVariables(map[string]string{
			"GROWTHOS_LOG_LEVEL": secretValue,
		})),
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
	variables := runtimeVariables(map[string]string{
		"GROWTHOS_ENVIRONMENT":  "test",
		"GROWTHOS_HTTP_ADDRESS": "127.0.0.1:9090",
		"GROWTHOS_LOG_LEVEL":    "info",
		"GROWTHOS_LOG_FORMAT":   "json",
	})
	database := &stubDatabase{}
	identityDatabase := &stubDatabase{}
	identity := &stubIdentityRuntime{}

	if exitCode := runWithDependencies(
		ctx,
		mapLookup(variables),
		&output,
		runtimeDependencies{OpenRuntime: func(
			context.Context,
			runtimeConfiguration,
			strategycache.Observer,
		) (runtimeComponents, error) {
			return stubRuntimeWith(database, identityDatabase, identity), nil
		}},
	); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
	if database.closeCalls != 1 || identityDatabase.closeCalls != 1 || identity.registerCalls != 1 {
		t.Fatalf(
			"business/identity close calls and route calls = %d/%d/%d, want 1/1/1",
			database.closeCalls,
			identityDatabase.closeCalls,
			identity.registerCalls,
		)
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
	variables := runtimeVariables(map[string]string{
		"GROWTHOS_LOG_LEVEL":  "error",
		"GROWTHOS_LOG_FORMAT": "json",
	})

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
	variables := runtimeVariables(map[string]string{
		"GROWTHOS_ENVIRONMENT":                    "test",
		"GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED": "true",
		"GROWTHOS_REDIS_PASSWORD":                 "redis-unit-test-password",
	})
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
	variables := runtimeVariables(map[string]string{
		"GROWTHOS_ENVIRONMENT":                    "test",
		"GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED": "true",
		"GROWTHOS_REDIS_PASSWORD":                 "redis-unit-test-password",
	})
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
	variables := runtimeVariables(nil)

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

func TestRunClosesUnexpectedDatabaseOwnersReturnedWithStartupError(t *testing.T) {
	const secret = "startup-error-password=must-not-leak"
	var closeOrder []string
	database := &stubDatabase{
		closeErr:   errors.New("close-error-password=must-not-leak"),
		closeName:  "business",
		closeOrder: &closeOrder,
	}
	identityDatabase := &stubDatabase{
		closeErr:   errors.New("identity-close-password=must-not-leak"),
		closeName:  "identity",
		closeOrder: &closeOrder,
	}
	var output bytes.Buffer

	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimeVariables(nil)),
		&output,
		runtimeDependencies{OpenRuntime: func(
			context.Context,
			runtimeConfiguration,
			strategycache.Observer,
		) (runtimeComponents, error) {
			return stubRuntimeWith(database, identityDatabase, &stubIdentityRuntime{}), errors.New(secret)
		}},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 || identityDatabase.closeCalls != 1 {
		t.Fatalf(
			"database close calls business/identity = %d/%d, want exactly 1/1",
			database.closeCalls,
			identityDatabase.closeCalls,
		)
	}
	if got, want := strings.Join(closeOrder, ","), "identity,business"; got != want {
		t.Fatalf("startup cleanup order = %q, want %q", got, want)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "close-error-password") ||
		strings.Contains(output.String(), "identity-close-password") {
		t.Fatalf("database startup log leaked an adapter error: %s", output.String())
	}
}

func TestRunRejectsTypedNilDatabase(t *testing.T) {
	var database *stubDatabase
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimeVariables(nil)),
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
	identityDatabase := &stubDatabase{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimeVariables(nil)),
		&output,
		runtimeDependencies{OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			components := stubRuntimeWith(database, identityDatabase, &stubIdentityRuntime{})
			components.selection = nil
			return components, nil
		}},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 || identityDatabase.closeCalls != 1 {
		t.Fatalf(
			"database close calls business/identity = %d/%d, want exactly 1/1 after partial composition",
			database.closeCalls,
			identityDatabase.closeCalls,
		)
	}
	if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
		t.Fatalf("missing selection service was not logged safely: %s", output.String())
	}
}

func TestRunRejectsUnconfiguredSelectionServiceAndClosesDatabase(t *testing.T) {
	database := &stubDatabase{}
	identityDatabase := &stubDatabase{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimeVariables(nil)),
		&output,
		runtimeDependencies{OpenRuntime: func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
			components := stubRuntimeWith(database, identityDatabase, &stubIdentityRuntime{})
			components.selection = &application.EphemeralSelectionService{}
			return components, nil
		}},
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 || identityDatabase.closeCalls != 1 {
		t.Fatalf(
			"database close calls business/identity = %d/%d, want exactly 1/1 after invalid composition",
			database.closeCalls,
			identityDatabase.closeCalls,
		)
	}
	if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
		t.Fatalf("unconfigured selection service was not logged safely: %s", output.String())
	}
}

func TestRunRejectsInvalidReadinessAndIdentityContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(runtimeComponents, *stubDatabase, *stubDatabase, *stubIdentityRuntime) runtimeComponents
	}{
		{
			name: "missing readiness",
			mutate: func(
				components runtimeComponents,
				_, _ *stubDatabase,
				_ *stubIdentityRuntime,
			) runtimeComponents {
				components.readiness = nil
				return components
			},
		},
		{
			name: "invalid readiness",
			mutate: func(
				components runtimeComponents,
				business, identityDatabase *stubDatabase,
				_ *stubIdentityRuntime,
			) runtimeComponents {
				components.readiness = &dualMySQLReadiness{
					business:        business,
					identity:        identityDatabase,
					businessTimeout: 0,
					identityTimeout: time.Second,
				}
				return components
			},
		},
		{
			name: "invalid identity runtime",
			mutate: func(
				components runtimeComponents,
				_, _ *stubDatabase,
				identity *stubIdentityRuntime,
			) runtimeComponents {
				identity.validateErr = errors.New("identity-validation-secret")
				return components
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			business := &stubDatabase{}
			identityDatabase := &stubDatabase{}
			identity := &stubIdentityRuntime{}
			components := stubRuntimeWith(business, identityDatabase, identity)
			components = test.mutate(components, business, identityDatabase, identity)
			var output bytes.Buffer

			exitCode := runWithDependencies(
				context.Background(),
				mapLookup(runtimeVariables(nil)),
				&output,
				runtimeDependencies{OpenRuntime: func(
					context.Context,
					runtimeConfiguration,
					strategycache.Observer,
				) (runtimeComponents, error) {
					return components, nil
				}},
			)

			if exitCode != 1 {
				t.Fatalf("run() exit code = %d, want 1", exitCode)
			}
			if business.closeCalls != 1 || identityDatabase.closeCalls != 1 {
				t.Fatalf(
					"database close calls business/identity = %d/%d, want 1/1",
					business.closeCalls,
					identityDatabase.closeCalls,
				)
			}
			if identity.registerCalls != 0 {
				t.Fatalf("identity route registration calls = %d, want 0", identity.registerCalls)
			}
			if strings.Contains(output.String(), "identity-validation-secret") ||
				!strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
				t.Fatalf("runtime contract rejection was unsafe or missing: %s", output.String())
			}
		})
	}
}

func TestRunRejectsMissingRuntimeOpener(t *testing.T) {
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimeVariables(nil)),
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
		mapLookup(runtimeVariables(nil)),
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

func TestRunClosesEveryOwnerInReverseOrderWhenIdentityRouteRegistrationFails(t *testing.T) {
	const secret = "identity-route-secret=must-not-leak"
	var closeOrder []string
	business := &stubDatabase{closeName: "business", closeOrder: &closeOrder}
	identityDatabase := &stubDatabase{closeName: "identity", closeOrder: &closeOrder}
	cache := newStubCacheRuntime()
	cache.closeName = "cache"
	cache.closeOrder = &closeOrder
	identity := &stubIdentityRuntime{registerErr: errors.New(secret)}
	var output bytes.Buffer
	variables := runtimeVariables(map[string]string{
		"GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED": "true",
		"GROWTHOS_REDIS_PASSWORD":                 "redis-unit-test-password",
	})

	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(variables),
		&output,
		runtimeDependencies{OpenRuntime: func(
			context.Context,
			runtimeConfiguration,
			strategycache.Observer,
		) (runtimeComponents, error) {
			components := stubRuntimeWith(business, identityDatabase, identity)
			components.cache = cache
			return components, nil
		}},
	)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if identity.registerCalls != 1 {
		t.Fatalf("identity route registration calls = %d, want 1", identity.registerCalls)
	}
	if got, want := strings.Join(closeOrder, ","), "cache,identity,business"; got != want {
		t.Fatalf("close order = %q, want %q", got, want)
	}
	if cache.closeCalls != 1 || identityDatabase.closeCalls != 1 || business.closeCalls != 1 {
		t.Fatalf(
			"close calls cache/identity/business = %d/%d/%d, want 1/1/1",
			cache.closeCalls,
			identityDatabase.closeCalls,
			business.closeCalls,
		)
	}
	if strings.Contains(output.String(), secret) ||
		!strings.Contains(output.String(), `"msg":"identity HTTP adapter startup failed"`) {
		t.Fatalf("identity route startup log was unsafe or missing: %s", output.String())
	}
}

func TestRunAttemptsEveryReverseOrderCloseAfterServing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var closeOrder []string
	business := &stubDatabase{
		closeErr:   errors.New("business-close-secret"),
		closeName:  "business",
		closeOrder: &closeOrder,
	}
	identityDatabase := &stubDatabase{
		closeErr:   errors.New("identity-close-secret"),
		closeName:  "identity",
		closeOrder: &closeOrder,
	}
	cache := newStubCacheRuntime()
	cache.closeErr = errors.New("cache-close-secret")
	cache.closeName = "cache"
	cache.closeOrder = &closeOrder
	var output bytes.Buffer
	variables := runtimeVariables(map[string]string{
		"GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED": "true",
		"GROWTHOS_REDIS_PASSWORD":                 "redis-unit-test-password",
	})

	exitCode := runWithDependencies(
		ctx,
		mapLookup(variables),
		&output,
		runtimeDependencies{OpenRuntime: func(
			context.Context,
			runtimeConfiguration,
			strategycache.Observer,
		) (runtimeComponents, error) {
			components := stubRuntimeWith(business, identityDatabase, &stubIdentityRuntime{})
			components.cache = cache
			return components, nil
		}},
	)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if got, want := strings.Join(closeOrder, ","), "cache,identity,business"; got != want {
		t.Fatalf("close order = %q, want %q", got, want)
	}
	for _, secret := range []string{"cache-close-secret", "identity-close-secret", "business-close-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("shutdown log leaked %q: %s", secret, output.String())
		}
	}
	for _, message := range []string{
		`"msg":"cache shutdown failed"`,
		`"msg":"identity database shutdown failed"`,
		`"msg":"database shutdown failed"`,
	} {
		if !strings.Contains(output.String(), message) {
			t.Fatalf("shutdown log is missing %s: %s", message, output.String())
		}
	}
}

func TestRunRejectsTypedNilIdentityOwnersAndCleansAcquiredPools(t *testing.T) {
	for _, test := range []struct {
		name              string
		components        func(*stubDatabase, *stubDatabase) runtimeComponents
		wantIdentityClose int
	}{
		{
			name: "identity database",
			components: func(business, _ *stubDatabase) runtimeComponents {
				var identityDatabase *stubDatabase
				return runtimeComponents{
					database:         business,
					identityDatabase: identityDatabase,
					identity:         &stubIdentityRuntime{},
					selection:        stubSelection(),
				}
			},
		},
		{
			name: "identity runtime",
			components: func(business, identityDatabase *stubDatabase) runtimeComponents {
				var identity *stubIdentityRuntime
				return stubRuntimeWith(business, identityDatabase, identity)
			},
			wantIdentityClose: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			business := &stubDatabase{}
			identityDatabase := &stubDatabase{}
			var output bytes.Buffer
			exitCode := runWithDependencies(
				context.Background(),
				mapLookup(runtimeVariables(nil)),
				&output,
				runtimeDependencies{OpenRuntime: func(
					context.Context,
					runtimeConfiguration,
					strategycache.Observer,
				) (runtimeComponents, error) {
					return test.components(business, identityDatabase), nil
				}},
			)
			if exitCode != 1 {
				t.Fatalf("run() exit code = %d, want 1", exitCode)
			}
			if business.closeCalls != 1 || identityDatabase.closeCalls != test.wantIdentityClose {
				t.Fatalf(
					"close calls business/identity = %d/%d, want 1/%d",
					business.closeCalls,
					identityDatabase.closeCalls,
					test.wantIdentityClose,
				)
			}
			if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
				t.Fatalf("typed-nil rejection was not logged safely: %s", output.String())
			}
		})
	}
}

func TestRunRejectsAliasedDatabaseOwnersAndClosesOnce(t *testing.T) {
	database := &stubDatabase{}
	var output bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		mapLookup(runtimeVariables(nil)),
		&output,
		runtimeDependencies{OpenRuntime: func(
			context.Context,
			runtimeConfiguration,
			strategycache.Observer,
		) (runtimeComponents, error) {
			return stubRuntimeWith(database, database, &stubIdentityRuntime{}), nil
		}},
	)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if database.closeCalls != 1 {
		t.Fatalf("aliased database close calls = %d, want exactly 1", database.closeCalls)
	}
	if !strings.Contains(output.String(), `"msg":"runtime startup failed"`) {
		t.Fatalf("aliased database rejection was not logged safely: %s", output.String())
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

func TestRuntimeConfigPreservesBothMySQLAuthoritiesAndIdentityPolicy(t *testing.T) {
	config, err := appconfig.Load(mapLookup(runtimeVariables(map[string]string{
		"GROWTHOS_ENVIRONMENT":                 "test",
		"GROWTHOS_MYSQL_USER":                  "growthos_business_test",
		"GROWTHOS_IDENTITY_MYSQL_USER":         "growthos_identity_test",
		"GROWTHOS_MYSQL_PING_TIMEOUT":          "2s",
		"GROWTHOS_IDENTITY_MYSQL_PING_TIMEOUT": "3s",
	})))
	if err != nil {
		t.Fatalf("load validated runtime config: %v", err)
	}

	got := runtimeConfig(config)
	if got.Environment != config.Environment ||
		got.MySQL != config.MySQL ||
		got.IdentityMySQL != config.IdentityMySQL ||
		got.Identity != config.Identity ||
		got.Redis != config.Redis ||
		got.StrategyCache != config.Lottery.StrategyCache {
		t.Fatal("runtimeConfig() did not preserve every module authority and policy")
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
	closeName  string
	closeOrder *[]string
}

func (database *stubDatabase) PingContext(context.Context) error {
	return database.pingErr
}

func (database *stubDatabase) Close() error {
	database.closeCalls++
	if database.closeOrder != nil {
		*database.closeOrder = append(*database.closeOrder, database.closeName)
	}
	return database.closeErr
}

type stubIdentityRuntime struct {
	validateErr   error
	registerErr   error
	registerCalls int
}

func (runtime *stubIdentityRuntime) Validate() error {
	if runtime == nil {
		return errIdentityHTTPRuntime
	}
	return runtime.validateErr
}

func (runtime *stubIdentityRuntime) RegisterRoutes(
	*gin.Engine,
	*slog.Logger,
	time.Duration,
) error {
	if runtime == nil {
		return errIdentityHTTPRuntime
	}
	runtime.registerCalls++
	return runtime.registerErr
}

func stubRuntimeOpener(database databaseRuntime, err error) func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
	return func(context.Context, runtimeConfiguration, strategycache.Observer) (runtimeComponents, error) {
		return stubRuntime(database), err
	}
}

func stubRuntime(database databaseRuntime) runtimeComponents {
	if nilDatabaseRuntime(database) {
		return runtimeComponents{database: database}
	}
	identityDatabase := &stubDatabase{}
	return stubRuntimeWith(database, identityDatabase, &stubIdentityRuntime{})
}

func stubRuntimeWith(
	database databaseRuntime,
	identityDatabase databaseRuntime,
	identity identitySessionRuntime,
) runtimeComponents {
	readiness, err := newDualMySQLReadiness(database, identityDatabase, time.Second, time.Second)
	if err != nil {
		panic(err)
	}
	return runtimeComponents{
		database:         database,
		identityDatabase: identityDatabase,
		readiness:        readiness,
		identity:         identity,
		selection:        stubSelection(),
	}
}

func stubSelection() *application.EphemeralSelectionService {
	selection, err := application.NewEphemeralSelectionService(stubStrategyReader{}, stubAwardSelector{})
	if err != nil {
		panic(err)
	}
	return selection
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

func runtimeVariables(overrides map[string]string) map[string]string {
	values := map[string]string{
		"GROWTHOS_MYSQL_PASSWORD":              "unit-test-password",
		"GROWTHOS_IDENTITY_MYSQL_PASSWORD":     "unit-test-identity-password",
		"GROWTHOS_IDENTITY_PUBLIC_ORIGIN":      "http://127.0.0.1:8080",
		"GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY":  "abcdefghijklmnopqrstuvwxyz123456",
		"GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID": "active",
		"GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY":    "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return values
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
