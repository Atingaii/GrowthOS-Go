package appconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestDefaultIdentityMaintenanceContainsOnlyPublicDefaults(t *testing.T) {
	got := DefaultIdentityMaintenance()
	want := IdentityMaintenanceConfig{
		Environment: EnvironmentDevelopment,
		Log: LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
		MySQL: IdentityMaintenanceMySQLConfig{
			MySQLConnectionConfig: MySQLConnectionConfig{
				Address:        defaultMySQLAddress,
				Database:       defaultMySQLDatabase,
				TLSMode:        MySQLTLSDisabled,
				ConnectTimeout: defaultMySQLConnectTimeout,
				ReadTimeout:    defaultIdentityMaintenanceMySQLReadTimeout,
				WriteTimeout:   defaultMySQLWriteTimeout,
			},
			User:        defaultIdentityMySQLUser,
			PingTimeout: defaultIdentityMaintenanceMySQLPingTimeout,
		},
		OperationTimeout: defaultIdentityMaintenanceOperationTimeout,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultIdentityMaintenance() = %#v, want documented public defaults", got)
	}
	if got.MySQL.Password != "" {
		t.Fatal("DefaultIdentityMaintenance() supplied a secret default")
	}
}

func TestLoadIdentityMaintenanceUsesRuntimeIdentityAndDedicatedDefaults(t *testing.T) {
	config, err := LoadIdentityMaintenance(mapLookup(map[string]string{
		identityMySQLPasswordVariable: "runtime identity secret",
	}))
	if err != nil {
		t.Fatalf("LoadIdentityMaintenance() error = %v", err)
	}
	if config.MySQL.User != defaultIdentityMySQLUser ||
		config.MySQL.Password != "runtime identity secret" ||
		config.MySQL.ReadTimeout != defaultIdentityMaintenanceMySQLReadTimeout ||
		config.MySQL.PingTimeout != defaultIdentityMaintenanceMySQLPingTimeout ||
		config.OperationTimeout != defaultIdentityMaintenanceOperationTimeout {
		t.Fatal("LoadIdentityMaintenance() did not preserve its runtime identity and maintenance defaults")
	}
}

func TestLoadIdentityMaintenanceAppliesCompleteOverride(t *testing.T) {
	variables := map[string]string{
		environmentVariable:                         "production",
		logLevelVariable:                            "error",
		logFormatVariable:                           "text",
		mysqlAddressVariable:                        "mysql.internal.example:4406",
		mysqlDatabaseVariable:                       "growthos_identity",
		mysqlTLSModeVariable:                        "verify_identity",
		mysqlTLSCAFileVariable:                      "/run/secrets/mysql-ca.pem",
		mysqlConnectTimeoutVariable:                 "7s",
		mysqlWriteTimeoutVariable:                   "13s",
		identityMySQLUserVariable:                   "growthos-identity@private",
		identityMySQLPasswordVariable:               "runtime identity password",
		identityMaintenanceMySQLReadTimeoutVariable: "12s",
		identityMaintenanceMySQLPingTimeoutVariable: "2s",
		identityMaintenanceOperationTimeoutVariable: "9s",
	}

	config, err := LoadIdentityMaintenance(mapLookup(variables))
	if err != nil {
		t.Fatalf("LoadIdentityMaintenance() error = %v", err)
	}
	wantConnection := MySQLConnectionConfig{
		Address:        "mysql.internal.example:4406",
		Database:       "growthos_identity",
		TLSMode:        MySQLTLSVerifyIdentity,
		TLSCAFile:      "/run/secrets/mysql-ca.pem",
		ConnectTimeout: 7 * time.Second,
		ReadTimeout:    12 * time.Second,
		WriteTimeout:   13 * time.Second,
	}
	if config.Environment != EnvironmentProduction ||
		config.Log != (LogConfig{Level: LogLevelError, Format: LogFormatText}) ||
		config.MySQL.MySQLConnectionConfig != wantConnection ||
		config.MySQL.User != "growthos-identity@private" ||
		config.MySQL.Password != "runtime identity password" ||
		config.MySQL.PingTimeout != 2*time.Second ||
		config.OperationTimeout != 9*time.Second {
		t.Fatal("LoadIdentityMaintenance() did not apply the complete override")
	}
}

func TestLoadIdentityMaintenanceReadsOnlyItsDeclaredVariables(t *testing.T) {
	requested := make(map[string]struct{})
	values := map[string]string{identityMySQLPasswordVariable: "runtime identity secret"}
	lookup := func(key string) (string, bool) {
		requested[key] = struct{}{}
		value, found := values[key]
		return value, found
	}

	if _, err := LoadIdentityMaintenance(lookup); err != nil {
		t.Fatalf("LoadIdentityMaintenance() error = %v", err)
	}
	got := make([]string, 0, len(requested))
	for variable := range requested {
		got = append(got, variable)
	}
	sort.Strings(got)
	want := []string{
		environmentVariable,
		identityMaintenanceMySQLPingTimeoutVariable,
		identityMaintenanceMySQLReadTimeoutVariable,
		identityMaintenanceOperationTimeoutVariable,
		identityMySQLPasswordFileVariable,
		identityMySQLPasswordVariable,
		identityMySQLUserVariable,
		logFormatVariable,
		logLevelVariable,
		mysqlAddressVariable,
		mysqlConnectTimeoutVariable,
		mysqlDatabaseVariable,
		mysqlTLSCAFileVariable,
		mysqlTLSModeVariable,
		mysqlWriteTimeoutVariable,
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadIdentityMaintenance() requested variables\n got: %v\nwant: %v", got, want)
	}
}

func TestLoadIdentityMaintenanceIgnoresOtherProcessVariables(t *testing.T) {
	variables := map[string]string{
		identityMySQLPasswordVariable:                "runtime identity secret",
		httpAddressVariable:                          "not-an-address",
		httpWriteTimeoutVariable:                     "not-a-duration",
		lotterySelectionTimeoutVariable:              "not-a-duration",
		lotteryEphemeralSelectionVariable:            "not-a-boolean",
		mysqlUserVariable:                            "",
		mysqlPasswordVariable:                        "",
		mysqlPasswordFileVariable:                    "/DO_NOT_READ_API_PASSWORD",
		mysqlReadTimeoutVariable:                     "not-a-duration",
		mysqlPingTimeoutVariable:                     "not-a-duration",
		mysqlMaxOpenConnsVariable:                    "not-an-integer",
		identityMySQLReadTimeoutVariable:             "not-a-duration",
		identityMySQLPingTimeoutVariable:             "not-a-duration",
		identityMySQLMaxOpenConnsVariable:            "not-an-integer",
		identityMySQLMaxIdleConnsVariable:            "not-an-integer",
		identityMySQLConnMaxLifetimeVariable:         "not-a-duration",
		identityMySQLConnMaxIdleTimeVariable:         "not-a-duration",
		identityPublicOriginVariable:                 "not-an-origin",
		identityThrottleHMACKeyFileVariable:          "/DO_NOT_READ_IDENTITY_KEY",
		identityCSRFActiveKeyFileVariable:            "/DO_NOT_READ_CSRF_KEY",
		identityArgon2MaxConcurrentVariable:          "not-an-integer",
		identityArgon2AcquireTimeoutVariable:         "not-a-duration",
		identityHTTPHandlerTimeoutVariable:           "not-a-duration",
		identityProvisionerMySQLUserVariable:         "",
		identityProvisionerMySQLPasswordFileVariable: "/DO_NOT_READ_PROVISIONER_PASSWORD",
		identityProvisionerMySQLReadTimeoutVariable:  "not-a-duration",
		identityProvisionerMySQLPingTimeoutVariable:  "not-a-duration",
		identityProvisionerOperationTimeoutVariable:  "not-a-duration",
		migrationUserVariable:                        "",
		migrationPasswordVariable:                    "",
		migrationPasswordFileVariable:                "/DO_NOT_READ_MIGRATION_PASSWORD",
		redisAddressVariable:                         "not-an-address",
		redisPasswordFileVariable:                    "/DO_NOT_READ_REDIS_PASSWORD",
	}
	config, err := LoadIdentityMaintenance(mapLookup(variables))
	if err != nil {
		t.Fatalf("LoadIdentityMaintenance() error = %v, want unrelated variables ignored", err)
	}
	if config.MySQL.Password != "runtime identity secret" {
		t.Fatal("LoadIdentityMaintenance() lost its runtime Identity secret")
	}
}

func TestLoadIdentityMaintenancePasswordFileAndSourceBoundary(t *testing.T) {
	secret := " file-backed identity password \t"
	path := filepath.Join(t.TempDir(), "identity-password")
	if err := os.WriteFile(path, []byte(secret+"\r\n"), 0o600); err != nil {
		t.Fatalf("write password fixture: %v", err)
	}
	config, err := LoadIdentityMaintenance(mapLookup(map[string]string{
		identityMySQLPasswordFileVariable: path,
	}))
	if err != nil {
		t.Fatalf("LoadIdentityMaintenance() file error = %v", err)
	}
	if config.MySQL.Password != secret {
		t.Fatal("LoadIdentityMaintenance() did not preserve the file password exactly")
	}

	privateDirect := "CONFLICTING_IDENTITY_PASSWORD_DO_NOT_ECHO"
	privatePath := filepath.Join(t.TempDir(), "PRIVATE_PATH_DO_NOT_ECHO")
	tests := []struct {
		name          string
		variables     map[string]string
		wantVariables []string
		want          string
		private       []string
	}{
		{
			name: "neither",
			wantVariables: []string{
				identityMySQLPasswordVariable,
				identityMySQLPasswordFileVariable,
			},
			want: "exactly one",
		},
		{
			name: "both",
			variables: map[string]string{
				identityMySQLPasswordVariable:     privateDirect,
				identityMySQLPasswordFileVariable: privatePath,
			},
			wantVariables: []string{
				identityMySQLPasswordVariable,
				identityMySQLPasswordFileVariable,
			},
			want:    "mutually exclusive",
			private: []string{privateDirect, privatePath},
		},
		{
			name: "missing file",
			variables: map[string]string{
				identityMySQLPasswordFileVariable: privatePath,
			},
			wantVariables: []string{identityMySQLPasswordFileVariable},
			want:          "could not be read",
			private:       []string{privatePath},
		},
		{
			name: "oversized direct",
			variables: map[string]string{
				identityMySQLPasswordVariable: strings.Repeat("private", 200),
			},
			wantVariables: []string{identityMySQLPasswordVariable},
			want:          "no more than 1024 bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, loadErr := LoadIdentityMaintenance(mapLookup(test.variables))
			if loadErr == nil || got != (IdentityMaintenanceConfig{}) {
				t.Fatal("LoadIdentityMaintenance() accepted an invalid password source")
			}
			for _, variable := range test.wantVariables {
				if !strings.Contains(loadErr.Error(), variable) {
					t.Fatalf("LoadIdentityMaintenance() error = %q, want %s", loadErr, variable)
				}
			}
			if !strings.Contains(loadErr.Error(), test.want) {
				t.Fatalf("LoadIdentityMaintenance() error = %q, want %q", loadErr, test.want)
			}
			for _, private := range test.private {
				if strings.Contains(loadErr.Error(), private) {
					t.Fatalf("LoadIdentityMaintenance() error leaked a secret or path: %q", loadErr)
				}
			}
		})
	}
}

func TestLoadIdentityMaintenanceEnforcesTLSByEnvironment(t *testing.T) {
	for _, environment := range []Environment{EnvironmentStaging, EnvironmentProduction} {
		t.Run(string(environment)+"_rejects_disabled", func(t *testing.T) {
			config, err := LoadIdentityMaintenance(mapLookup(map[string]string{
				environmentVariable:           string(environment),
				identityMySQLPasswordVariable: "runtime identity secret",
			}))
			if err == nil || config != (IdentityMaintenanceConfig{}) ||
				!strings.Contains(err.Error(), mysqlTLSModeVariable) {
				t.Fatalf("LoadIdentityMaintenance() error = %v, want TLS failure", err)
			}
		})
		t.Run(string(environment)+"_accepts_verify_identity", func(t *testing.T) {
			_, err := LoadIdentityMaintenance(mapLookup(map[string]string{
				environmentVariable:           string(environment),
				mysqlTLSModeVariable:          string(MySQLTLSVerifyIdentity),
				identityMySQLPasswordVariable: "runtime identity secret",
			}))
			if err != nil {
				t.Fatalf("LoadIdentityMaintenance() error = %v", err)
			}
		})
	}
}

func TestLoadIdentityMaintenanceValidatesDedicatedRangesWithoutEchoingValues(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{variable: identityMaintenanceMySQLReadTimeoutVariable, value: "5m1ns"},
		{variable: identityMaintenanceMySQLPingTimeoutVariable, value: "30s1ns"},
		{variable: identityMaintenanceOperationTimeoutVariable, value: "999ms"},
		{variable: identityMaintenanceOperationTimeoutVariable, value: "30s1ns"},
		{variable: identityMySQLUserVariable, value: " PRIVATE_USER"},
	}
	for _, test := range tests {
		t.Run(test.variable+"/"+test.value, func(t *testing.T) {
			config, err := LoadIdentityMaintenance(mapLookup(map[string]string{
				identityMySQLPasswordVariable: "runtime identity secret",
				test.variable:                 test.value,
			}))
			if err == nil || config != (IdentityMaintenanceConfig{}) {
				t.Fatal("LoadIdentityMaintenance() accepted an invalid dedicated value")
			}
			if !strings.Contains(err.Error(), test.variable) || strings.Contains(err.Error(), test.value) {
				t.Fatalf("LoadIdentityMaintenance() error = %q, want redacted %s failure", err, test.variable)
			}
		})
	}
}

func TestLoadIdentityMaintenanceOrdersOperationBeforeReadAndWriteDeadlines(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{
			name: "operation exceeds read budget",
			overrides: map[string]string{
				identityMaintenanceOperationTimeoutVariable: "4s1ns",
				identityMaintenanceMySQLReadTimeoutVariable: "5s",
				mysqlWriteTimeoutVariable:                   "6s",
			},
			want: []string{identityMaintenanceOperationTimeoutVariable, identityMaintenanceMySQLReadTimeoutVariable},
		},
		{
			name: "operation exceeds write budget",
			overrides: map[string]string{
				identityMaintenanceOperationTimeoutVariable: "4s1ns",
				identityMaintenanceMySQLReadTimeoutVariable: "6s",
				mysqlWriteTimeoutVariable:                   "5s",
			},
			want: []string{identityMaintenanceOperationTimeoutVariable, mysqlWriteTimeoutVariable},
		},
		{
			name: "read has no cleanup budget",
			overrides: map[string]string{
				identityMaintenanceOperationTimeoutVariable: "1s",
				identityMaintenanceMySQLReadTimeoutVariable: "1s",
				mysqlWriteTimeoutVariable:                   "5s",
			},
			want: []string{identityMaintenanceOperationTimeoutVariable, identityMaintenanceMySQLReadTimeoutVariable},
		},
		{
			name: "write has no cleanup budget",
			overrides: map[string]string{
				identityMaintenanceOperationTimeoutVariable: "1s",
				identityMaintenanceMySQLReadTimeoutVariable: "5s",
				mysqlWriteTimeoutVariable:                   "1s",
			},
			want: []string{identityMaintenanceOperationTimeoutVariable, mysqlWriteTimeoutVariable},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.overrides[identityMySQLPasswordVariable] = "runtime identity secret"
			config, err := LoadIdentityMaintenance(mapLookup(test.overrides))
			if err == nil || config != (IdentityMaintenanceConfig{}) {
				t.Fatal("LoadIdentityMaintenance() accepted an invalid operation budget")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Fatalf("LoadIdentityMaintenance() error = %q, want %s", err, variable)
				}
			}
		})
	}

	for _, boundary := range []struct {
		name      string
		operation string
		network   string
	}{
		{name: "exact_budget", operation: "4s", network: "5s"},
		{name: "minimum", operation: "1s", network: "2s"},
		{name: "maximum", operation: "30s", network: "31s"},
	} {
		t.Run("accept_"+boundary.name, func(t *testing.T) {
			config, err := LoadIdentityMaintenance(mapLookup(map[string]string{
				identityMySQLPasswordVariable:               "runtime identity secret",
				identityMaintenanceOperationTimeoutVariable: boundary.operation,
				identityMaintenanceMySQLReadTimeoutVariable: boundary.network,
				mysqlWriteTimeoutVariable:                   boundary.network,
			}))
			if err != nil {
				t.Fatalf("LoadIdentityMaintenance() %s boundary error = %v", boundary.name, err)
			}
			if config.OperationTimeout.String() != boundary.operation {
				t.Fatalf("operation timeout = %s, want %s", config.OperationTimeout, boundary.operation)
			}
		})
	}
}

func TestLoadIdentityMaintenanceDoesNotDeriveBudgetFromInvalidWriteTimeout(t *testing.T) {
	config, err := LoadIdentityMaintenance(mapLookup(map[string]string{
		identityMySQLPasswordVariable:               "runtime identity secret",
		identityMaintenanceOperationTimeoutVariable: "30s",
		identityMaintenanceMySQLReadTimeoutVariable: "31s",
		mysqlWriteTimeoutVariable:                   "PRIVATE_INVALID_WRITE_TIMEOUT",
	}))
	if err == nil || config != (IdentityMaintenanceConfig{}) {
		t.Fatal("LoadIdentityMaintenance() accepted an invalid write timeout")
	}
	if !strings.Contains(err.Error(), mysqlWriteTimeoutVariable) ||
		strings.Contains(err.Error(), identityMaintenanceNetworkBudget.String()) ||
		strings.Contains(err.Error(), "PRIVATE_INVALID_WRITE_TIMEOUT") {
		t.Fatalf("LoadIdentityMaintenance() produced a leaked or default-derived write error: %q", err)
	}
}

func TestOtherLoadersIgnoreIdentityMaintenanceVariables(t *testing.T) {
	maintenanceVariables := map[string]string{
		identityMaintenanceMySQLReadTimeoutVariable: "not-a-duration",
		identityMaintenanceMySQLPingTimeoutVariable: "not-a-duration",
		identityMaintenanceOperationTimeoutVariable: "not-a-duration",
	}

	apiConfig, err := Load(mapLookup(apiVariables(maintenanceVariables)))
	if err != nil {
		t.Fatalf("Load() error = %v, want maintenance variables ignored", err)
	}
	if apiConfig.IdentityMySQL.ReadTimeout != defaultIdentityMySQLReadTimeout {
		t.Fatal("Load() consumed the maintenance read timeout")
	}

	migrationVariables := map[string]string{migrationPasswordVariable: "migration secret"}
	for variable, value := range maintenanceVariables {
		migrationVariables[variable] = value
	}
	if _, err := LoadMigration(mapLookup(migrationVariables)); err != nil {
		t.Fatalf("LoadMigration() error = %v, want maintenance variables ignored", err)
	}

	provisionerVariables := map[string]string{
		identityProvisionerMySQLPasswordVariable: "provisioner secret",
	}
	for variable, value := range maintenanceVariables {
		provisionerVariables[variable] = value
	}
	if _, err := LoadIdentityProvisioner(mapLookup(provisionerVariables)); err != nil {
		t.Fatalf("LoadIdentityProvisioner() error = %v, want maintenance variables ignored", err)
	}
}

func TestLoadIdentityMaintenanceAggregatesAndRedactsIndependentProblems(t *testing.T) {
	variables := map[string]string{
		environmentVariable:                         "PRIVATE_ENVIRONMENT",
		logLevelVariable:                            "PRIVATE_LOG_LEVEL",
		mysqlAddressVariable:                        "PRIVATE_ADDRESS",
		mysqlDatabaseVariable:                       "PRIVATE_DATABASE",
		mysqlTLSModeVariable:                        "PRIVATE_TLS_MODE",
		mysqlConnectTimeoutVariable:                 "PRIVATE_CONNECT_TIMEOUT",
		mysqlWriteTimeoutVariable:                   "PRIVATE_WRITE_TIMEOUT",
		identityMySQLUserVariable:                   " PRIVATE_USER",
		identityMySQLPasswordVariable:               strings.Repeat("private", 200),
		identityMaintenanceMySQLReadTimeoutVariable: "PRIVATE_READ_TIMEOUT",
		identityMaintenanceMySQLPingTimeoutVariable: "PRIVATE_PING_TIMEOUT",
		identityMaintenanceOperationTimeoutVariable: "PRIVATE_OPERATION_TIMEOUT",
	}
	config, err := LoadIdentityMaintenance(mapLookup(variables))
	if err == nil || config != (IdentityMaintenanceConfig{}) {
		t.Fatal("LoadIdentityMaintenance() accepted invalid independent values")
	}
	for variable, value := range variables {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("LoadIdentityMaintenance() error = %q, want %s", err, variable)
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("LoadIdentityMaintenance() error echoed value for %s", variable)
		}
	}
}

func TestIdentityMaintenanceConfigsRedactEveryFormattingBoundary(t *testing.T) {
	const secret = "SENTINEL_IDENTITY_MAINTENANCE_PASSWORD"
	mysqlConfig := IdentityMaintenanceMySQLConfig{Password: secret}
	values := []any{
		mysqlConfig,
		&mysqlConfig,
		IdentityMaintenanceConfig{MySQL: mysqlConfig},
		&IdentityMaintenanceConfig{MySQL: mysqlConfig},
	}
	for _, value := range values {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		logger.Info("config", slog.Any("value", value))
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		rendered := strings.Join([]string{
			fmt.Sprint(value),
			fmt.Sprintf("%+v", value),
			fmt.Sprintf("%#v", value),
			fmt.Sprintf("%s", value),
			fmt.Sprintf("%q", value),
			fmt.Sprintf("%x", value),
			fmt.Sprintf("%d", value),
			string(encoded),
			output.String(),
		}, "\n")
		if strings.Contains(rendered, secret) {
			t.Fatalf("Identity maintenance formatting leaked the password: %s", rendered)
		}
		if !strings.Contains(strings.ToLower(rendered), "redacted") {
			t.Fatalf("Identity maintenance formatting omitted the redaction marker: %s", rendered)
		}
	}
}

func TestLoadIdentityMaintenanceRequiresLookup(t *testing.T) {
	config, err := LoadIdentityMaintenance(nil)
	if err == nil || config != (IdentityMaintenanceConfig{}) {
		t.Fatal("LoadIdentityMaintenance(nil) did not fail closed")
	}
}
