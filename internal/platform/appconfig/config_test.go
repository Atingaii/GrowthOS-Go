package appconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultUsesOnlyPublicNonSecretValues(t *testing.T) {
	config := Default()
	want := Config{
		Environment: EnvironmentDevelopment,
		HTTP: HTTPConfig{
			Address:           ":8080",
			ShutdownTimeout:   5 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		Log: LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
		MySQL: MySQLConfig{
			MySQLConnectionConfig: MySQLConnectionConfig{
				Address:        "127.0.0.1:3306",
				Database:       "growthos",
				TLSMode:        MySQLTLSDisabled,
				ConnectTimeout: 3 * time.Second,
				ReadTimeout:    5 * time.Second,
				WriteTimeout:   5 * time.Second,
			},
			User:                  "growthos_app",
			PingTimeout:           3 * time.Second,
			MaxOpenConnections:    10,
			MaxIdleConnections:    10,
			ConnectionMaxLifetime: 3 * time.Minute,
			ConnectionMaxIdleTime: time.Minute,
		},
	}
	if !reflect.DeepEqual(config, want) {
		t.Fatal("Default() did not return the documented public defaults")
	}
	if config.MySQL.Password != "" {
		t.Fatal("Default() unexpectedly contains an API password")
	}
}

func TestLoadRequiresAPIPassword(t *testing.T) {
	config, err := Load(mapLookup(nil))
	if err == nil {
		t.Fatal("Load() error = nil, want missing-password failure")
	}
	if config != (Config{}) {
		t.Fatal("Load() returned a nonzero config on failure")
	}
	if !strings.Contains(err.Error(), mysqlPasswordVariable) {
		t.Fatalf("Load() error = %q, want password variable name", err)
	}
}

func TestLoadAppliesCompleteOverride(t *testing.T) {
	variables := map[string]string{
		environmentVariable:           "production",
		httpAddressVariable:           "127.0.0.1:9090",
		httpShutdownTimeoutVariable:   "10s",
		httpReadHeaderTimeoutVariable: "3s",
		httpReadTimeoutVariable:       "20s",
		httpWriteTimeoutVariable:      "45s",
		httpIdleTimeoutVariable:       "2m",
		logLevelVariable:              "warn",
		logFormatVariable:             "text",
		mysqlAddressVariable:          "db.internal.example:4406",
		mysqlDatabaseVariable:         "growthos_prod",
		mysqlTLSModeVariable:          "verify_identity",
		mysqlTLSCAFileVariable:        "/run/secrets/mysql-ca.pem",
		mysqlConnectTimeoutVariable:   "7s",
		mysqlReadTimeoutVariable:      "25s",
		mysqlWriteTimeoutVariable:     "35s",
		mysqlUserVariable:             "api-user@private",
		mysqlPasswordVariable:         " arbitrary API password \x00 ",
		mysqlPingTimeoutVariable:      "8s",
		mysqlMaxOpenConnsVariable:     "42",
		mysqlMaxIdleConnsVariable:     "17",
		mysqlConnMaxLifetimeVariable:  "12m",
		mysqlConnMaxIdleTimeVariable:  "7m",
	}

	config, err := Load(mapLookup(variables))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		Environment: EnvironmentProduction,
		HTTP: HTTPConfig{
			Address:           "127.0.0.1:9090",
			ShutdownTimeout:   10 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			ReadTimeout:       20 * time.Second,
			WriteTimeout:      45 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
		Log: LogConfig{
			Level:  LogLevelWarn,
			Format: LogFormatText,
		},
		MySQL: MySQLConfig{
			MySQLConnectionConfig: MySQLConnectionConfig{
				Address:        "db.internal.example:4406",
				Database:       "growthos_prod",
				TLSMode:        MySQLTLSVerifyIdentity,
				TLSCAFile:      "/run/secrets/mysql-ca.pem",
				ConnectTimeout: 7 * time.Second,
				ReadTimeout:    25 * time.Second,
				WriteTimeout:   35 * time.Second,
			},
			User:                  "api-user@private",
			Password:              " arbitrary API password \x00 ",
			PingTimeout:           8 * time.Second,
			MaxOpenConnections:    42,
			MaxIdleConnections:    17,
			ConnectionMaxLifetime: 12 * time.Minute,
			ConnectionMaxIdleTime: 7 * time.Minute,
		},
	}
	if !reflect.DeepEqual(config, want) {
		t.Fatal("Load() did not return the complete validated override")
	}
}

func TestLoadAcceptsEverySupportedEnumValue(t *testing.T) {
	for _, environment := range []Environment{
		EnvironmentDevelopment,
		EnvironmentTest,
		EnvironmentStaging,
		EnvironmentProduction,
	} {
		t.Run("environment_"+string(environment), func(t *testing.T) {
			variables := apiVariables(map[string]string{environmentVariable: string(environment)})
			if environment == EnvironmentStaging || environment == EnvironmentProduction {
				variables[mysqlTLSModeVariable] = string(MySQLTLSVerifyIdentity)
			}
			config, err := Load(mapLookup(variables))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Environment != environment {
				t.Fatalf("environment = %q, want %q", config.Environment, environment)
			}
		})
	}

	for _, level := range []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError} {
		t.Run("log_level_"+string(level), func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{logLevelVariable: string(level)})))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Log.Level != level {
				t.Fatalf("log level = %q, want %q", config.Log.Level, level)
			}
		})
	}

	for _, format := range []LogFormat{LogFormatJSON, LogFormatText} {
		t.Run("log_format_"+string(format), func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{logFormatVariable: string(format)})))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Log.Format != format {
				t.Fatalf("log format = %q, want %q", config.Log.Format, format)
			}
		})
	}
}

func TestLoadRejectsPresentEmptyVariables(t *testing.T) {
	variables := []string{
		environmentVariable,
		httpAddressVariable,
		httpShutdownTimeoutVariable,
		httpReadHeaderTimeoutVariable,
		httpReadTimeoutVariable,
		httpWriteTimeoutVariable,
		httpIdleTimeoutVariable,
		logLevelVariable,
		logFormatVariable,
		mysqlAddressVariable,
		mysqlDatabaseVariable,
		mysqlTLSModeVariable,
		mysqlTLSCAFileVariable,
		mysqlConnectTimeoutVariable,
		mysqlReadTimeoutVariable,
		mysqlWriteTimeoutVariable,
		mysqlUserVariable,
		mysqlPingTimeoutVariable,
		mysqlMaxOpenConnsVariable,
		mysqlMaxIdleConnsVariable,
		mysqlConnMaxLifetimeVariable,
		mysqlConnMaxIdleTimeVariable,
	}

	for _, variable := range variables {
		t.Run(variable, func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{variable: "   "})))
			if err == nil {
				t.Fatal("Load() error = nil, want empty-value failure")
			}
			if !strings.Contains(err.Error(), variable+" must not be empty") {
				t.Fatalf("Load() error = %q, want variable and empty-value constraint", err)
			}
		})
	}
}

func TestLoadRejectsInvalidValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "environment", variable: environmentVariable, value: "TOP_SECRET_ENV"},
		{name: "address without port", variable: httpAddressVariable, value: "TOP_SECRET_HOST"},
		{name: "address with zero port", variable: httpAddressVariable, value: "localhost:0"},
		{name: "shutdown syntax", variable: httpShutdownTimeoutVariable, value: "TOP_SECRET_DURATION"},
		{name: "read header zero", variable: httpReadHeaderTimeoutVariable, value: "00h"},
		{name: "read negative", variable: httpReadTimeoutVariable, value: "-1s"},
		{name: "write over maximum", variable: httpWriteTimeoutVariable, value: "11m"},
		{name: "idle over maximum", variable: httpIdleTimeoutVariable, value: "11m"},
		{name: "log level", variable: logLevelVariable, value: "TOP_SECRET_LEVEL"},
		{name: "log format", variable: logFormatVariable, value: "TOP_SECRET_FORMAT"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{test.variable: test.value})))
			if err == nil {
				t.Fatal("Load() error = nil, want validation failure")
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %q, want variable %s", err, test.variable)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("Load() error echoed supplied value: %q", err)
			}
		})
	}
}

func TestLoadReportsMultipleIndependentProblems(t *testing.T) {
	variables := map[string]string{
		environmentVariable:         "local",
		httpAddressVariable:         "localhost",
		httpReadTimeoutVariable:     "00h",
		httpWriteTimeoutVariable:    "invalid-duration",
		logLevelVariable:            "trace",
		logFormatVariable:           "yaml",
		httpIdleTimeoutVariable:     "10m1ns",
		httpShutdownTimeoutVariable: "2m1ns",
	}

	config, err := Load(mapLookup(apiVariables(variables)))
	if err == nil {
		t.Fatal("Load() error = nil, want aggregated validation failure")
	}
	if config != (Config{}) {
		t.Fatal("Load() returned a nonzero config on failure")
	}
	for variable, value := range variables {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("Load() error = %q, want variable %s", err, variable)
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("Load() error echoed supplied value %q", value)
		}
	}
}

func TestLoadRequiresLookupFunction(t *testing.T) {
	_, err := Load(nil)
	if err == nil {
		t.Fatal("Load(nil) error = nil, want failure")
	}
}

func TestLoadAcceptsMySQLBoundaries(t *testing.T) {
	variables := apiVariables(map[string]string{
		httpWriteTimeoutVariable:     "31s",
		mysqlDatabaseVariable:        "a" + strings.Repeat("0", 63),
		mysqlTLSModeVariable:         string(MySQLTLSVerifyIdentity),
		mysqlConnectTimeoutVariable:  "30s",
		mysqlReadTimeoutVariable:     "5m",
		mysqlWriteTimeoutVariable:    "5m",
		mysqlPingTimeoutVariable:     "30s",
		mysqlMaxOpenConnsVariable:    "100",
		mysqlMaxIdleConnsVariable:    "100",
		mysqlConnMaxLifetimeVariable: "1h",
		mysqlConnMaxIdleTimeVariable: "30m",
	})

	config, err := Load(mapLookup(variables))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.MySQL.Database != variables[mysqlDatabaseVariable] {
		t.Fatalf("database = %q, want boundary identifier", config.MySQL.Database)
	}
	if config.MySQL.MaxOpenConnections != 100 || config.MySQL.MaxIdleConnections != 100 {
		t.Fatalf("pool limits = %d/%d, want 100/100", config.MySQL.MaxOpenConnections, config.MySQL.MaxIdleConnections)
	}

	config, err = Load(mapLookup(apiVariables(map[string]string{mysqlMaxIdleConnsVariable: "0"})))
	if err != nil {
		t.Fatalf("Load() idle=0 error = %v", err)
	}
	if config.MySQL.MaxIdleConnections != 0 {
		t.Fatalf("max idle connections = %d, want 0", config.MySQL.MaxIdleConnections)
	}
}

func TestLoadRejectsInvalidMySQLValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "address", variable: mysqlAddressVariable, value: "SENSITIVE_BAD_ADDRESS"},
		{name: "address empty host", variable: mysqlAddressVariable, value: ":3306"},
		{name: "database uppercase", variable: mysqlDatabaseVariable, value: "SENSITIVE_DATABASE"},
		{name: "database too long", variable: mysqlDatabaseVariable, value: "a" + strings.Repeat("sensitive", 9)},
		{name: "TLS mode", variable: mysqlTLSModeVariable, value: "SENSITIVE_TLS_MODE"},
		{name: "connect timeout", variable: mysqlConnectTimeoutVariable, value: "SENSITIVE_CONNECT_TIMEOUT"},
		{name: "read timeout", variable: mysqlReadTimeoutVariable, value: "SENSITIVE_READ_TIMEOUT"},
		{name: "write timeout", variable: mysqlWriteTimeoutVariable, value: "SENSITIVE_WRITE_TIMEOUT"},
		{name: "ping timeout", variable: mysqlPingTimeoutVariable, value: "SENSITIVE_PING_TIMEOUT"},
		{name: "max open", variable: mysqlMaxOpenConnsVariable, value: "SENSITIVE_MAX_OPEN"},
		{name: "max idle", variable: mysqlMaxIdleConnsVariable, value: "SENSITIVE_MAX_IDLE"},
		{name: "connection lifetime", variable: mysqlConnMaxLifetimeVariable, value: "SENSITIVE_LIFETIME"},
		{name: "connection idle time", variable: mysqlConnMaxIdleTimeVariable, value: "SENSITIVE_IDLE_TIME"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{test.variable: test.value})))
			if err == nil {
				t.Fatal("Load() error = nil, want validation failure")
			}
			if config != (Config{}) {
				t.Fatal("Load() returned a nonzero config on failure")
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %q, want variable %s", err, test.variable)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("Load() error echoed supplied value: %q", err)
			}
		})
	}
}

func TestLoadRejectsOutOfRangeMySQLNumbersAndDurations(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{variable: mysqlConnectTimeoutVariable, value: "30s1ns"},
		{variable: mysqlReadTimeoutVariable, value: "5m1ns"},
		{variable: mysqlWriteTimeoutVariable, value: "5m1ns"},
		{variable: mysqlPingTimeoutVariable, value: "30s1ns"},
		{variable: mysqlMaxOpenConnsVariable, value: "101"},
		{variable: mysqlMaxOpenConnsVariable, value: "0"},
		{variable: mysqlMaxIdleConnsVariable, value: "101"},
		{variable: mysqlMaxIdleConnsVariable, value: "-1"},
		{variable: mysqlConnMaxLifetimeVariable, value: "1h1ns"},
		{variable: mysqlConnMaxIdleTimeVariable, value: "30m1ns"},
	}
	for _, test := range tests {
		t.Run(test.variable, func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{test.variable: test.value})))
			if err == nil || !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %v, want range failure for %s", err, test.variable)
			}
		})
	}
}

func TestLoadValidatesAPIAndMigrationUserNames(t *testing.T) {
	validUsers := []string{
		"growthos_app",
		"增长 用户",
		strings.Repeat("界", 32),
	}
	for _, user := range validUsers {
		t.Run("api_valid", func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{mysqlUserVariable: user})))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.MySQL.User != user {
				t.Fatal("Load() did not preserve the valid API user")
			}
		})
		t.Run("migration_valid", func(t *testing.T) {
			config, err := LoadMigration(mapLookup(map[string]string{
				migrationUserVariable:     user,
				migrationPasswordVariable: "migration secret",
			}))
			if err != nil {
				t.Fatalf("LoadMigration() error = %v", err)
			}
			if config.MySQL.User != user {
				t.Fatal("LoadMigration() did not preserve the valid migration user")
			}
		})
	}

	invalidUsers := []string{
		"",
		" SECRET_LEADING",
		"SECRET_TRAILING ",
		"SECRET\nCONTROL",
		strings.Repeat("界", 33),
		string([]byte{'a', 0xff}),
	}
	for index, user := range invalidUsers {
		t.Run(fmt.Sprintf("api_invalid_%d", index), func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{mysqlUserVariable: user})))
			if err == nil {
				t.Fatal("Load() error = nil, want API user failure")
			}
			if config != (Config{}) {
				t.Fatal("Load() returned a nonzero config on failure")
			}
			if !strings.Contains(err.Error(), mysqlUserVariable) {
				t.Fatalf("Load() error = %q, want API user variable", err)
			}
			if user != "" && strings.Contains(err.Error(), user) {
				t.Fatalf("Load() error echoed API user: %q", err)
			}
		})
		t.Run(fmt.Sprintf("migration_invalid_%d", index), func(t *testing.T) {
			config, err := LoadMigration(mapLookup(map[string]string{
				migrationUserVariable:     user,
				migrationPasswordVariable: "migration secret",
			}))
			if err == nil {
				t.Fatal("LoadMigration() error = nil, want migration user failure")
			}
			if config != (MigrationConfig{}) {
				t.Fatal("LoadMigration() returned a nonzero config on failure")
			}
			if !strings.Contains(err.Error(), migrationUserVariable) {
				t.Fatalf("LoadMigration() error = %q, want migration user variable", err)
			}
			if user != "" && strings.Contains(err.Error(), user) {
				t.Fatalf("LoadMigration() error echoed migration user: %q", err)
			}
		})
	}
}

func TestLoadRejectsInvalidPoolRelationship(t *testing.T) {
	config, err := Load(mapLookup(apiVariables(map[string]string{
		mysqlMaxOpenConnsVariable: "4",
		mysqlMaxIdleConnsVariable: "5",
	})))
	if err == nil {
		t.Fatal("Load() error = nil, want pool relationship failure")
	}
	if config != (Config{}) {
		t.Fatal("Load() returned a nonzero config on failure")
	}
	for _, variable := range []string{mysqlMaxOpenConnsVariable, mysqlMaxIdleConnsVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadRequiresReadinessBudgetBeforeHTTPWriteDeadline(t *testing.T) {
	for _, pingTimeout := range []string{"3s", "2.000000001s"} {
		config, err := Load(mapLookup(apiVariables(map[string]string{
			httpWriteTimeoutVariable: "3s",
			mysqlPingTimeoutVariable: pingTimeout,
		})))
		if err == nil {
			t.Fatal("Load() error = nil, want readiness/write timeout relationship failure")
		}
		if config != (Config{}) {
			t.Fatal("Load() returned a nonzero config on failure")
		}
		for _, variable := range []string{mysqlPingTimeoutVariable, httpWriteTimeoutVariable} {
			if !strings.Contains(err.Error(), variable) {
				t.Fatalf("Load() error = %q, want %s", err, variable)
			}
		}
	}

	config, err := Load(mapLookup(apiVariables(map[string]string{
		httpWriteTimeoutVariable: "3s",
		mysqlPingTimeoutVariable: "2s",
	})))
	if err != nil {
		t.Fatalf("Load() ordered timeout error = %v", err)
	}
	if config.MySQL.PingTimeout != 2*time.Second {
		t.Fatalf("ping timeout = %s, want 2s", config.MySQL.PingTimeout)
	}
}

func TestLoadIgnoresMigrationAccountVariables(t *testing.T) {
	config, err := Load(mapLookup(apiVariables(map[string]string{
		migrationUserVariable:        "",
		migrationPasswordVariable:    "",
		migrationReadTimeoutVariable: "not-a-duration",
		migrationLockTimeoutVariable: "not-a-duration",
		migrationStatementVariable:   "not-a-duration",
	})))
	if err != nil {
		t.Fatalf("Load() error = %v, want migration-only variables ignored", err)
	}
	if config.MySQL.User != defaultMySQLUser {
		t.Fatalf("Load() API user = %q, want %q", config.MySQL.User, defaultMySQLUser)
	}
}

func TestLoadEnforcesDeploymentTLS(t *testing.T) {
	for _, environment := range []Environment{EnvironmentStaging, EnvironmentProduction} {
		t.Run(string(environment)+"_rejects_disabled", func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{
				environmentVariable:  string(environment),
				mysqlTLSModeVariable: string(MySQLTLSDisabled),
			})))
			if err == nil || !strings.Contains(err.Error(), mysqlTLSModeVariable) {
				t.Fatalf("Load() error = %v, want deployment TLS failure", err)
			}
		})
		t.Run(string(environment)+"_accepts_verify_identity", func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{
				environmentVariable:  string(environment),
				mysqlTLSModeVariable: string(MySQLTLSVerifyIdentity),
			})))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadRejectsCAFileWhenTLSIsDisabled(t *testing.T) {
	_, err := Load(mapLookup(apiVariables(map[string]string{
		mysqlTLSModeVariable:   string(MySQLTLSDisabled),
		mysqlTLSCAFileVariable: "/private/ca.pem",
	})))
	if err == nil {
		t.Fatal("Load() error = nil, want TLS/CA relationship failure")
	}
	for _, variable := range []string{mysqlTLSModeVariable, mysqlTLSCAFileVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadAcceptsArbitraryBoundedPassword(t *testing.T) {
	password := strings.Repeat("密", 341) + "x"
	if len(password) != maximumPasswordBytes {
		t.Fatalf("test password length = %d, want %d", len(password), maximumPasswordBytes)
	}
	config, err := Load(mapLookup(map[string]string{mysqlPasswordVariable: password}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.MySQL.Password != password {
		t.Fatal("Load() did not preserve the API password exactly")
	}

	config, err = Load(mapLookup(map[string]string{mysqlPasswordVariable: "   "}))
	if err != nil {
		t.Fatalf("Load() whitespace password error = %v", err)
	}
	if config.MySQL.Password != "   " {
		t.Fatal("Load() did not preserve whitespace password exactly")
	}
}

func TestLoadRejectsInvalidPasswordWithoutEchoingIt(t *testing.T) {
	for _, variables := range []map[string]string{
		{mysqlPasswordVariable: ""},
		{mysqlPasswordVariable: strings.Repeat("private-value", 100)},
	} {
		config, err := Load(mapLookup(variables))
		if err == nil {
			t.Fatal("Load() error = nil, want password validation failure")
		}
		if config != (Config{}) {
			t.Fatal("Load() returned a nonzero config on failure")
		}
		if !strings.Contains(err.Error(), mysqlPasswordVariable) {
			t.Fatalf("Load() error = %q, want password variable", err)
		}
		if value := variables[mysqlPasswordVariable]; value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("Load() error echoed password: %q", err)
		}
	}
}

func TestLoadMigrationUsesSeparateDefaultsAndSecret(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable: "migration secret",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v", err)
	}
	want := MigrationConfig{
		Environment: EnvironmentDevelopment,
		Log: LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
		MySQL: MigrationMySQLConfig{
			MySQLConnectionConfig: MySQLConnectionConfig{
				Address:        defaultMySQLAddress,
				Database:       defaultMySQLDatabase,
				TLSMode:        MySQLTLSDisabled,
				ConnectTimeout: defaultMySQLConnectTimeout,
				ReadTimeout:    35 * time.Second,
				WriteTimeout:   defaultMySQLWriteTimeout,
			},
			User:             "growthos_migrator",
			Password:         "migration secret",
			LockTimeout:      40 * time.Second,
			StatementTimeout: 30 * time.Second,
		},
	}
	if !reflect.DeepEqual(config, want) {
		t.Fatal("LoadMigration() did not return the documented validated defaults")
	}
}

func TestLoadMigrationAppliesCompleteOverride(t *testing.T) {
	variables := map[string]string{
		environmentVariable:          "production",
		logLevelVariable:             "error",
		logFormatVariable:            "text",
		mysqlAddressVariable:         "mysql.internal:3307",
		mysqlDatabaseVariable:        "growthos_migration",
		mysqlTLSModeVariable:         "verify_identity",
		mysqlTLSCAFileVariable:       "/run/secrets/mysql-ca.pem",
		mysqlConnectTimeoutVariable:  "9s",
		migrationReadTimeoutVariable: "4m30s",
		mysqlWriteTimeoutVariable:    "55s",
		migrationUserVariable:        "migration-admin@private",
		migrationPasswordVariable:    "migration-password",
		migrationLockTimeoutVariable: "5m",
		migrationStatementVariable:   "4m",
	}
	config, err := LoadMigration(mapLookup(variables))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v", err)
	}
	if config.Environment != EnvironmentProduction || config.Log != (LogConfig{Level: LogLevelError, Format: LogFormatText}) {
		t.Fatal("LoadMigration() did not apply the common environment and log overrides")
	}
	if config.MySQL.Address != "mysql.internal:3307" || config.MySQL.Database != "growthos_migration" ||
		config.MySQL.User != "migration-admin@private" || config.MySQL.Password != "migration-password" ||
		config.MySQL.ReadTimeout != 4*time.Minute+30*time.Second ||
		config.MySQL.LockTimeout != 5*time.Minute || config.MySQL.StatementTimeout != 4*time.Minute {
		t.Fatal("LoadMigration() did not apply the MySQL migration overrides")
	}
}

func TestLoadMigrationEnforcesLockTimeoutAdapterBoundary(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:    "migration secret",
		migrationReadTimeoutVariable: "6s",
		migrationLockTimeoutVariable: "10s",
		migrationStatementVariable:   "1s",
	}))
	if err == nil {
		t.Fatal("LoadMigration() error = nil, want lock timeout lower-bound failure")
	}
	if config != (MigrationConfig{}) {
		t.Fatal("LoadMigration() returned a nonzero config on failure")
	}
	if !strings.Contains(err.Error(), migrationLockTimeoutVariable) ||
		!strings.Contains(err.Error(), "11s") ||
		!strings.Contains(err.Error(), "11m0s") {
		t.Fatalf("LoadMigration() error = %q, want variable and supported range", err)
	}
	if strings.Contains(err.Error(), "10s") {
		t.Fatalf("LoadMigration() error echoed rejected value: %q", err)
	}

	config, err = LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:    "migration secret",
		migrationReadTimeoutVariable: "6s",
		migrationLockTimeoutVariable: "11s",
		migrationStatementVariable:   "1s",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() 11s error = %v", err)
	}
	if config.MySQL.LockTimeout != 11*time.Second {
		t.Fatalf("lock timeout = %s, want 11s", config.MySQL.LockTimeout)
	}
}

func TestLoadMigrationEnforcesStatementReadLockTimeoutOrder(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{
			name: "network read equals statement",
			overrides: map[string]string{
				migrationStatementVariable:   "30s",
				migrationReadTimeoutVariable: "30s",
				migrationLockTimeoutVariable: "40s",
			},
			want: []string{migrationStatementVariable, migrationReadTimeoutVariable},
		},
		{
			name: "lock equals network read",
			overrides: map[string]string{
				migrationStatementVariable:   "30s",
				migrationReadTimeoutVariable: "35s",
				migrationLockTimeoutVariable: "35s",
			},
			want: []string{migrationReadTimeoutVariable, migrationLockTimeoutVariable},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.overrides[migrationPasswordVariable] = "migration secret"
			config, err := LoadMigration(mapLookup(test.overrides))
			if err == nil {
				t.Fatal("LoadMigration() error = nil, want timeout ordering failure")
			}
			if config != (MigrationConfig{}) {
				t.Fatal("LoadMigration() returned a nonzero config on failure")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Fatalf("LoadMigration() error = %q, want %s", err, variable)
				}
			}
		})
	}

	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:    "migration secret",
		migrationStatementVariable:   "10m",
		migrationReadTimeoutVariable: "10m5s",
		migrationLockTimeoutVariable: "10m10s",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() ordered upper-bound error = %v", err)
	}
	if !(config.MySQL.StatementTimeout < config.MySQL.ReadTimeout && config.MySQL.ReadTimeout < config.MySQL.LockTimeout) {
		t.Fatal("LoadMigration() did not preserve the strict statement < read < lock order")
	}
}

func TestSecretBearingConfigsRedactCommonFormattingBoundaries(t *testing.T) {
	const secret = "SENTINEL_DATABASE_PASSWORD"
	values := []struct {
		name  string
		value any
	}{
		{name: "api root", value: Config{MySQL: MySQLConfig{Password: secret}}},
		{name: "api mysql", value: MySQLConfig{Password: secret}},
		{name: "migration root", value: MigrationConfig{MySQL: MigrationMySQLConfig{Password: secret}}},
		{name: "migration mysql", value: MigrationMySQLConfig{Password: secret}},
	}

	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			logger.Info("config", slog.Any("config", test.value))
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			rendered := strings.Join([]string{
				fmt.Sprint(test.value),
				fmt.Sprintf("%+v", test.value),
				fmt.Sprintf("%#v", test.value),
				string(encoded),
				output.String(),
			}, "\n")
			if strings.Contains(rendered, secret) {
				t.Fatalf("common formatting boundary leaked the password: %s", rendered)
			}
			if !strings.Contains(rendered, "redacted") {
				t.Fatalf("common formatting boundary omitted the redaction marker: %s", rendered)
			}
		})
	}
}

func TestLoadMigrationIgnoresHTTPAndAPIVariables(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:    "migration secret",
		httpAddressVariable:          "not-an-address",
		httpShutdownTimeoutVariable:  "not-a-duration",
		mysqlUserVariable:            "",
		mysqlPasswordVariable:        "",
		mysqlReadTimeoutVariable:     "not-a-duration",
		mysqlPingTimeoutVariable:     "not-a-duration",
		mysqlMaxOpenConnsVariable:    "not-an-integer",
		mysqlConnMaxLifetimeVariable: "not-a-duration",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v, want unrelated variables ignored", err)
	}
	if config.MySQL.Password != "migration secret" {
		t.Fatal("LoadMigration() did not load the migration password")
	}
}

func TestLoadMigrationRequiresOnlyMigrationPassword(t *testing.T) {
	for _, variables := range []map[string]string{
		nil,
		{migrationPasswordVariable: ""},
		{migrationPasswordVariable: strings.Repeat("private-value", 100)},
	} {
		config, err := LoadMigration(mapLookup(variables))
		if err == nil {
			t.Fatal("LoadMigration() error = nil, want password failure")
		}
		if config != (MigrationConfig{}) {
			t.Fatal("LoadMigration() returned a nonzero config on failure")
		}
		if !strings.Contains(err.Error(), migrationPasswordVariable) {
			t.Fatalf("LoadMigration() error = %q, want migration password variable", err)
		}
		if variables != nil {
			if value := variables[migrationPasswordVariable]; value != "" && strings.Contains(err.Error(), value) {
				t.Fatalf("LoadMigration() error echoed password: %q", err)
			}
		}
	}
}

func TestLoadMigrationAggregatesIndependentProblems(t *testing.T) {
	variables := map[string]string{
		environmentVariable:          "invalid-environment",
		logLevelVariable:             "trace",
		mysqlAddressVariable:         "invalid-address",
		mysqlDatabaseVariable:        "InvalidDatabase",
		mysqlTLSModeVariable:         "required",
		mysqlConnectTimeoutVariable:  "31s",
		migrationReadTimeoutVariable: "10m31s",
		mysqlWriteTimeoutVariable:    "6m",
		migrationPasswordVariable:    strings.Repeat("secret", 200),
		migrationLockTimeoutVariable: "11m1s",
		migrationStatementVariable:   "10m1s",
	}
	config, err := LoadMigration(mapLookup(variables))
	if err == nil {
		t.Fatal("LoadMigration() error = nil, want aggregated failure")
	}
	if config != (MigrationConfig{}) {
		t.Fatal("LoadMigration() returned a nonzero config on failure")
	}
	for variable, value := range variables {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("LoadMigration() error = %q, want %s", err, variable)
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("LoadMigration() error echoed value for %s", variable)
		}
	}
}

func TestLoadMigrationRequiresLookupFunction(t *testing.T) {
	config, err := LoadMigration(nil)
	if err == nil {
		t.Fatal("LoadMigration(nil) error = nil, want failure")
	}
	if config != (MigrationConfig{}) {
		t.Fatal("LoadMigration(nil) returned a nonzero config")
	}
}

func TestValidAddressAcceptsSupportedListenerForms(t *testing.T) {
	addresses := []string{
		":8080",
		"localhost:8080",
		"api.internal.example:443",
		"127.0.0.1:8080",
		"[::1]:8080",
	}
	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			if !validAddress(address) {
				t.Fatalf("validAddress(%q) = false, want true", address)
			}
		})
	}
}

func mapLookup(values map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}

func apiVariables(overrides map[string]string) map[string]string {
	values := map[string]string{mysqlPasswordVariable: "test-api-password"}
	for key, value := range overrides {
		values[key] = value
	}
	return values
}
