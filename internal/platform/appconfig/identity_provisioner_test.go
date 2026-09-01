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

func TestDefaultIdentityProvisionerContainsOnlyPublicDefaults(t *testing.T) {
	got := DefaultIdentityProvisioner()
	want := IdentityProvisionerConfig{
		Environment: EnvironmentDevelopment,
		Log: LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
		MySQL: IdentityProvisionerMySQLConfig{
			MySQLConnectionConfig: MySQLConnectionConfig{
				Address:        defaultMySQLAddress,
				Database:       defaultMySQLDatabase,
				TLSMode:        MySQLTLSDisabled,
				ConnectTimeout: defaultMySQLConnectTimeout,
				ReadTimeout:    defaultIdentityProvisionerMySQLReadTimeout,
				WriteTimeout:   defaultMySQLWriteTimeout,
			},
			User:        defaultIdentityProvisionerMySQLUser,
			PingTimeout: defaultIdentityProvisionerMySQLPingTimeout,
		},
		OperationTimeout: defaultIdentityProvisionerOperationTimeout,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultIdentityProvisioner() = %#v, want documented public defaults", got)
	}
	if got.MySQL.Password != "" {
		t.Fatal("DefaultIdentityProvisioner() supplied a secret default")
	}
}

func TestLoadIdentityProvisionerUsesDedicatedDefaultsAndSecret(t *testing.T) {
	config, err := LoadIdentityProvisioner(mapLookup(map[string]string{
		identityProvisionerMySQLPasswordVariable: "provisioner secret",
	}))
	if err != nil {
		t.Fatalf("LoadIdentityProvisioner() error = %v", err)
	}
	if config.MySQL.User != defaultIdentityProvisionerMySQLUser ||
		config.MySQL.Password != "provisioner secret" ||
		config.OperationTimeout != defaultIdentityProvisionerOperationTimeout {
		t.Fatal("LoadIdentityProvisioner() did not preserve its dedicated defaults and secret")
	}
}

func TestLoadIdentityProvisionerAppliesCompleteOverride(t *testing.T) {
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
		identityProvisionerMySQLUserVariable:        "identity-provisioner@private",
		identityProvisionerMySQLPasswordVariable:    "dedicated provisioner password",
		identityProvisionerMySQLReadTimeoutVariable: "12s",
		identityProvisionerMySQLPingTimeoutVariable: "2s",
		identityProvisionerOperationTimeoutVariable: "9s",
	}

	config, err := LoadIdentityProvisioner(mapLookup(variables))
	if err != nil {
		t.Fatalf("LoadIdentityProvisioner() error = %v", err)
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
		config.MySQL.User != "identity-provisioner@private" ||
		config.MySQL.Password != "dedicated provisioner password" ||
		config.MySQL.PingTimeout != 2*time.Second ||
		config.OperationTimeout != 9*time.Second {
		t.Fatal("LoadIdentityProvisioner() did not apply the complete override")
	}
}

func TestLoadIdentityProvisionerReadsOnlyItsDeclaredVariables(t *testing.T) {
	requested := make(map[string]struct{})
	values := map[string]string{
		identityProvisionerMySQLPasswordVariable: "provisioner secret",
	}
	lookup := func(key string) (string, bool) {
		requested[key] = struct{}{}
		value, found := values[key]
		return value, found
	}

	if _, err := LoadIdentityProvisioner(lookup); err != nil {
		t.Fatalf("LoadIdentityProvisioner() error = %v", err)
	}
	got := make([]string, 0, len(requested))
	for variable := range requested {
		got = append(got, variable)
	}
	sort.Strings(got)
	want := []string{
		environmentVariable,
		identityProvisionerMySQLPasswordFileVariable,
		identityProvisionerMySQLPasswordVariable,
		identityProvisionerMySQLPingTimeoutVariable,
		identityProvisionerMySQLReadTimeoutVariable,
		identityProvisionerMySQLUserVariable,
		identityProvisionerOperationTimeoutVariable,
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
		t.Fatalf("LoadIdentityProvisioner() requested variables\n got: %v\nwant: %v", got, want)
	}
}

func TestLoadIdentityProvisionerIgnoresOtherProcessVariables(t *testing.T) {
	variables := map[string]string{
		identityProvisionerMySQLPasswordVariable: "provisioner secret",
		httpAddressVariable:                      "not-an-address",
		httpWriteTimeoutVariable:                 "not-a-duration",
		lotterySelectionTimeoutVariable:          "not-a-duration",
		lotteryEphemeralSelectionVariable:        "not-a-boolean",
		mysqlUserVariable:                        "",
		mysqlPasswordVariable:                    "",
		mysqlPasswordFileVariable:                "/DO_NOT_READ_API_PASSWORD",
		mysqlReadTimeoutVariable:                 "not-a-duration",
		mysqlPingTimeoutVariable:                 "not-a-duration",
		mysqlMaxOpenConnsVariable:                "not-an-integer",
		identityMySQLUserVariable:                "",
		identityMySQLPasswordVariable:            "",
		identityMySQLPasswordFileVariable:        "/DO_NOT_READ_IDENTITY_PASSWORD",
		identityPublicOriginVariable:             "not-an-origin",
		identityThrottleHMACKeyFileVariable:      "/DO_NOT_READ_IDENTITY_KEY",
		identityArgon2MaxConcurrentVariable:      "not-an-integer",
		identityArgon2AcquireTimeoutVariable:     "not-a-duration",
		migrationUserVariable:                    "",
		migrationPasswordVariable:                "",
		migrationPasswordFileVariable:            "/DO_NOT_READ_MIGRATION_PASSWORD",
		redisAddressVariable:                     "not-an-address",
		redisPasswordFileVariable:                "/DO_NOT_READ_REDIS_PASSWORD",
	}
	config, err := LoadIdentityProvisioner(mapLookup(variables))
	if err != nil {
		t.Fatalf("LoadIdentityProvisioner() error = %v, want unrelated variables ignored", err)
	}
	if config.MySQL.Password != "provisioner secret" {
		t.Fatal("LoadIdentityProvisioner() lost its own secret while ignoring unrelated variables")
	}
}

func TestLoadIdentityProvisionerPasswordFileAndSourceBoundary(t *testing.T) {
	secret := " file-backed provisioner password \t"
	path := filepath.Join(t.TempDir(), "provisioner-password")
	if err := os.WriteFile(path, []byte(secret+"\r\n"), 0o600); err != nil {
		t.Fatalf("write password fixture: %v", err)
	}
	config, err := LoadIdentityProvisioner(mapLookup(map[string]string{
		identityProvisionerMySQLPasswordFileVariable: path,
	}))
	if err != nil {
		t.Fatalf("LoadIdentityProvisioner() file error = %v", err)
	}
	if config.MySQL.Password != secret {
		t.Fatal("LoadIdentityProvisioner() did not preserve the file password exactly")
	}

	privateDirect := "CONFLICTING_PROVISIONER_PASSWORD_DO_NOT_ECHO"
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
				identityProvisionerMySQLPasswordVariable,
				identityProvisionerMySQLPasswordFileVariable,
			},
			want: "exactly one",
		},
		{
			name: "both",
			variables: map[string]string{
				identityProvisionerMySQLPasswordVariable:     privateDirect,
				identityProvisionerMySQLPasswordFileVariable: privatePath,
			},
			wantVariables: []string{
				identityProvisionerMySQLPasswordVariable,
				identityProvisionerMySQLPasswordFileVariable,
			},
			want:    "mutually exclusive",
			private: []string{privateDirect, privatePath},
		},
		{
			name: "missing file",
			variables: map[string]string{
				identityProvisionerMySQLPasswordFileVariable: privatePath,
			},
			wantVariables: []string{identityProvisionerMySQLPasswordFileVariable},
			want:          "could not be read",
			private:       []string{privatePath},
		},
		{
			name: "oversized direct",
			variables: map[string]string{
				identityProvisionerMySQLPasswordVariable: strings.Repeat("private", 200),
			},
			wantVariables: []string{identityProvisionerMySQLPasswordVariable},
			want:          "no more than 1024 bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, loadErr := LoadIdentityProvisioner(mapLookup(test.variables))
			if loadErr == nil || got != (IdentityProvisionerConfig{}) {
				t.Fatal("LoadIdentityProvisioner() accepted an invalid password source")
			}
			for _, variable := range test.wantVariables {
				if !strings.Contains(loadErr.Error(), variable) {
					t.Fatalf("LoadIdentityProvisioner() error = %q, want %s", loadErr, variable)
				}
			}
			if !strings.Contains(loadErr.Error(), test.want) {
				t.Fatalf("LoadIdentityProvisioner() error = %q, want %q", loadErr, test.want)
			}
			for _, private := range test.private {
				if strings.Contains(loadErr.Error(), private) {
					t.Fatalf("LoadIdentityProvisioner() error leaked a secret or path: %q", loadErr)
				}
			}
		})
	}
}

func TestLoadIdentityProvisionerEnforcesTLSByEnvironment(t *testing.T) {
	for _, environment := range []Environment{EnvironmentStaging, EnvironmentProduction} {
		t.Run(string(environment)+"_rejects_disabled", func(t *testing.T) {
			config, err := LoadIdentityProvisioner(mapLookup(map[string]string{
				environmentVariable:                      string(environment),
				identityProvisionerMySQLPasswordVariable: "provisioner secret",
			}))
			if err == nil || config != (IdentityProvisionerConfig{}) ||
				!strings.Contains(err.Error(), mysqlTLSModeVariable) {
				t.Fatalf("LoadIdentityProvisioner() error = %v, want TLS failure", err)
			}
		})
		t.Run(string(environment)+"_accepts_verify_identity", func(t *testing.T) {
			_, err := LoadIdentityProvisioner(mapLookup(map[string]string{
				environmentVariable:                      string(environment),
				mysqlTLSModeVariable:                     string(MySQLTLSVerifyIdentity),
				identityProvisionerMySQLPasswordVariable: "provisioner secret",
			}))
			if err != nil {
				t.Fatalf("LoadIdentityProvisioner() error = %v", err)
			}
		})
	}
}

func TestLoadIdentityProvisionerValidatesDedicatedRangesWithoutEchoingValues(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{variable: identityProvisionerMySQLReadTimeoutVariable, value: "5m1ns"},
		{variable: identityProvisionerMySQLPingTimeoutVariable, value: "30s1ns"},
		{variable: identityProvisionerOperationTimeoutVariable, value: "999ms"},
		{variable: identityProvisionerOperationTimeoutVariable, value: "30s1ns"},
		{variable: identityProvisionerMySQLUserVariable, value: " PRIVATE_USER"},
	}
	for _, test := range tests {
		t.Run(test.variable+"/"+test.value, func(t *testing.T) {
			config, err := LoadIdentityProvisioner(mapLookup(map[string]string{
				identityProvisionerMySQLPasswordVariable: "provisioner secret",
				test.variable:                            test.value,
			}))
			if err == nil || config != (IdentityProvisionerConfig{}) {
				t.Fatal("LoadIdentityProvisioner() accepted an invalid dedicated value")
			}
			if !strings.Contains(err.Error(), test.variable) || strings.Contains(err.Error(), test.value) {
				t.Fatalf("LoadIdentityProvisioner() error = %q, want redacted %s failure", err, test.variable)
			}
		})
	}
}

func TestLoadIdentityProvisionerOrdersOperationBeforeReadAndWriteDeadlines(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{
			name: "operation exceeds read budget",
			overrides: map[string]string{
				identityProvisionerOperationTimeoutVariable: "4s1ns",
				identityProvisionerMySQLReadTimeoutVariable: "5s",
				mysqlWriteTimeoutVariable:                   "6s",
			},
			want: []string{identityProvisionerOperationTimeoutVariable, identityProvisionerMySQLReadTimeoutVariable},
		},
		{
			name: "operation exceeds write budget",
			overrides: map[string]string{
				identityProvisionerOperationTimeoutVariable: "4s1ns",
				identityProvisionerMySQLReadTimeoutVariable: "6s",
				mysqlWriteTimeoutVariable:                   "5s",
			},
			want: []string{identityProvisionerOperationTimeoutVariable, mysqlWriteTimeoutVariable},
		},
		{
			name: "read has no response budget",
			overrides: map[string]string{
				identityProvisionerOperationTimeoutVariable: "1s",
				identityProvisionerMySQLReadTimeoutVariable: "1s",
				mysqlWriteTimeoutVariable:                   "5s",
			},
			want: []string{identityProvisionerOperationTimeoutVariable, identityProvisionerMySQLReadTimeoutVariable},
		},
		{
			name: "write has no response budget",
			overrides: map[string]string{
				identityProvisionerOperationTimeoutVariable: "1s",
				identityProvisionerMySQLReadTimeoutVariable: "5s",
				mysqlWriteTimeoutVariable:                   "1s",
			},
			want: []string{identityProvisionerOperationTimeoutVariable, mysqlWriteTimeoutVariable},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.overrides[identityProvisionerMySQLPasswordVariable] = "provisioner secret"
			config, err := LoadIdentityProvisioner(mapLookup(test.overrides))
			if err == nil || config != (IdentityProvisionerConfig{}) {
				t.Fatal("LoadIdentityProvisioner() accepted an invalid operation budget")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Fatalf("LoadIdentityProvisioner() error = %q, want %s", err, variable)
				}
			}
		})
	}

	config, err := LoadIdentityProvisioner(mapLookup(map[string]string{
		identityProvisionerMySQLPasswordVariable:    "provisioner secret",
		identityProvisionerOperationTimeoutVariable: "4s",
		identityProvisionerMySQLReadTimeoutVariable: "5s",
		mysqlWriteTimeoutVariable:                   "5s",
	}))
	if err != nil {
		t.Fatalf("LoadIdentityProvisioner() exact budget boundary error = %v", err)
	}
	if config.OperationTimeout != 4*time.Second {
		t.Fatalf("operation timeout = %s, want 4s", config.OperationTimeout)
	}

	for _, boundary := range []struct {
		name      string
		operation string
		network   string
	}{
		{name: "minimum", operation: "1s", network: "2s"},
		{name: "maximum", operation: "30s", network: "31s"},
	} {
		t.Run("accept_"+boundary.name, func(t *testing.T) {
			config, boundaryErr := LoadIdentityProvisioner(mapLookup(map[string]string{
				identityProvisionerMySQLPasswordVariable:    "provisioner secret",
				identityProvisionerOperationTimeoutVariable: boundary.operation,
				identityProvisionerMySQLReadTimeoutVariable: boundary.network,
				mysqlWriteTimeoutVariable:                   boundary.network,
			}))
			if boundaryErr != nil {
				t.Fatalf("LoadIdentityProvisioner() %s boundary error = %v", boundary.name, boundaryErr)
			}
			if config.OperationTimeout.String() != boundary.operation {
				t.Fatalf("operation timeout = %s, want %s", config.OperationTimeout, boundary.operation)
			}
		})
	}
}

func TestLoadIdentityProvisionerDoesNotDeriveBudgetFromInvalidWriteTimeout(t *testing.T) {
	config, err := LoadIdentityProvisioner(mapLookup(map[string]string{
		identityProvisionerMySQLPasswordVariable:    "provisioner secret",
		identityProvisionerOperationTimeoutVariable: "30s",
		identityProvisionerMySQLReadTimeoutVariable: "31s",
		mysqlWriteTimeoutVariable:                   "PRIVATE_INVALID_WRITE_TIMEOUT",
	}))
	if err == nil || config != (IdentityProvisionerConfig{}) {
		t.Fatal("LoadIdentityProvisioner() accepted an invalid write timeout")
	}
	if !strings.Contains(err.Error(), mysqlWriteTimeoutVariable) ||
		strings.Contains(err.Error(), identityProvisionerNetworkBudget.String()) ||
		strings.Contains(err.Error(), "PRIVATE_INVALID_WRITE_TIMEOUT") {
		t.Fatalf("LoadIdentityProvisioner() produced a leaked or default-derived write error: %q", err)
	}
}

func TestOtherLoadersIgnoreIdentityProvisionerVariables(t *testing.T) {
	privatePath := "/DO_NOT_READ_IDENTITY_PROVISIONER_PASSWORD"
	provisionerVariables := map[string]string{
		identityProvisionerMySQLUserVariable:         "",
		identityProvisionerMySQLPasswordVariable:     "",
		identityProvisionerMySQLPasswordFileVariable: privatePath,
		identityProvisionerMySQLReadTimeoutVariable:  "not-a-duration",
		identityProvisionerMySQLPingTimeoutVariable:  "not-a-duration",
		identityProvisionerOperationTimeoutVariable:  "not-a-duration",
	}

	apiConfig, err := Load(mapLookup(apiVariables(provisionerVariables)))
	if err != nil {
		t.Fatalf("Load() error = %v, want provisioner variables ignored", err)
	}
	if apiConfig.MySQL.User != defaultMySQLUser || apiConfig.IdentityMySQL.User != defaultIdentityMySQLUser {
		t.Fatal("Load() consumed the provisioner database identity")
	}

	migrationVariables := map[string]string{migrationPasswordVariable: "migration secret"}
	for variable, value := range provisionerVariables {
		migrationVariables[variable] = value
	}
	migrationConfig, err := LoadMigration(mapLookup(migrationVariables))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v, want provisioner variables ignored", err)
	}
	if migrationConfig.MySQL.User != defaultMigrationUser {
		t.Fatal("LoadMigration() consumed the provisioner database identity")
	}
}

func TestLoadIdentityProvisionerAggregatesAndRedactsIndependentProblems(t *testing.T) {
	variables := map[string]string{
		environmentVariable:                         "PRIVATE_ENVIRONMENT",
		logLevelVariable:                            "PRIVATE_LOG_LEVEL",
		mysqlAddressVariable:                        "PRIVATE_ADDRESS",
		mysqlDatabaseVariable:                       "PRIVATE_DATABASE",
		mysqlTLSModeVariable:                        "PRIVATE_TLS_MODE",
		mysqlConnectTimeoutVariable:                 "PRIVATE_CONNECT_TIMEOUT",
		mysqlWriteTimeoutVariable:                   "PRIVATE_WRITE_TIMEOUT",
		identityProvisionerMySQLUserVariable:        " PRIVATE_USER",
		identityProvisionerMySQLPasswordVariable:    strings.Repeat("private", 200),
		identityProvisionerMySQLReadTimeoutVariable: "PRIVATE_READ_TIMEOUT",
		identityProvisionerMySQLPingTimeoutVariable: "PRIVATE_PING_TIMEOUT",
		identityProvisionerOperationTimeoutVariable: "PRIVATE_OPERATION_TIMEOUT",
	}
	config, err := LoadIdentityProvisioner(mapLookup(variables))
	if err == nil || config != (IdentityProvisionerConfig{}) {
		t.Fatal("LoadIdentityProvisioner() accepted invalid independent values")
	}
	for variable, value := range variables {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("LoadIdentityProvisioner() error = %q, want %s", err, variable)
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("LoadIdentityProvisioner() error echoed value for %s", variable)
		}
	}
}

func TestIdentityProvisionerConfigsRedactEveryFormattingBoundary(t *testing.T) {
	const secret = "SENTINEL_IDENTITY_PROVISIONER_PASSWORD"
	mysqlConfig := IdentityProvisionerMySQLConfig{Password: secret}
	values := []any{
		mysqlConfig,
		IdentityProvisionerConfig{MySQL: mysqlConfig},
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
			t.Fatalf("Identity provisioner formatting leaked the password: %s", rendered)
		}
		if !strings.Contains(strings.ToLower(rendered), "redacted") {
			t.Fatalf("Identity provisioner formatting omitted the redaction marker: %s", rendered)
		}
	}
}

func TestLoadIdentityProvisionerRequiresLookup(t *testing.T) {
	config, err := LoadIdentityProvisioner(nil)
	if err == nil || config != (IdentityProvisionerConfig{}) {
		t.Fatal("LoadIdentityProvisioner(nil) did not fail closed")
	}
}
