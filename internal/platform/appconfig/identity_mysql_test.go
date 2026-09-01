package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresIdentityPasswordIndependently(t *testing.T) {
	config, err := Load(mapLookup(map[string]string{
		mysqlPasswordVariable: "business password",
	}))
	if err == nil {
		t.Fatal("Load() error = nil, want missing Identity password failure")
	}
	if config != (Config{}) {
		t.Fatal("Load() returned a nonzero config on failure")
	}
	for _, variable := range []string{identityMySQLPasswordVariable, identityMySQLPasswordFileVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadBuildsIndependentIdentityPoolOnSharedDeploymentEndpoint(t *testing.T) {
	config, err := Load(mapLookup(apiVariables(map[string]string{
		mysqlAddressVariable:                 "mysql.internal:4406",
		mysqlDatabaseVariable:                "growthos_auth_test",
		mysqlTLSModeVariable:                 string(MySQLTLSVerifyIdentity),
		mysqlTLSCAFileVariable:               "/run/mysql/ca.pem",
		mysqlConnectTimeoutVariable:          "9s",
		mysqlWriteTimeoutVariable:            "17s",
		mysqlReadTimeoutVariable:             "31s",
		mysqlMaxOpenConnsVariable:            "27",
		mysqlMaxIdleConnsVariable:            "13",
		identityMySQLReadTimeoutVariable:     "23s",
		identityMySQLMaxOpenConnsVariable:    "8",
		identityMySQLMaxIdleConnsVariable:    "3",
		identityMySQLConnMaxLifetimeVariable: "14m",
		identityMySQLConnMaxIdleTimeVariable: "4m",
	})))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for name, got := range map[string]string{
		"address":  config.IdentityMySQL.Address,
		"database": config.IdentityMySQL.Database,
		"CA file":  config.IdentityMySQL.TLSCAFile,
	} {
		want := map[string]string{
			"address":  config.MySQL.Address,
			"database": config.MySQL.Database,
			"CA file":  config.MySQL.TLSCAFile,
		}[name]
		if got != want {
			t.Fatalf("Identity %s = %q, want shared deployment value %q", name, got, want)
		}
	}
	if config.IdentityMySQL.TLSMode != config.MySQL.TLSMode ||
		config.IdentityMySQL.ConnectTimeout != config.MySQL.ConnectTimeout ||
		config.IdentityMySQL.WriteTimeout != config.MySQL.WriteTimeout {
		t.Fatal("Identity did not inherit the shared endpoint/TLS connection policy")
	}
	if config.IdentityMySQL.ReadTimeout != 23*time.Second || config.MySQL.ReadTimeout != 31*time.Second {
		t.Fatal("Identity and business read budgets were not independent")
	}
	if config.IdentityMySQL.MaxOpenConnections != 8 || config.MySQL.MaxOpenConnections != 27 {
		t.Fatal("Identity and business pools were not independent")
	}
	if config.IdentityMySQL.User != defaultIdentityMySQLUser || config.IdentityMySQL.Password != "test-identity-password" {
		t.Fatal("Identity runtime credentials were not loaded independently")
	}
}

func TestLoadRejectsAliasedBusinessAndIdentityRuntimeAccounts(t *testing.T) {
	const privateUser = "SAME_PRIVATE_RUNTIME_USER"
	config, err := Load(mapLookup(apiVariables(map[string]string{
		mysqlUserVariable:         privateUser,
		identityMySQLUserVariable: privateUser,
	})))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() accepted one MySQL identity for both authority boundaries")
	}
	for _, variable := range []string{mysqlUserVariable, identityMySQLUserVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
	if strings.Contains(err.Error(), privateUser) {
		t.Fatalf("Load() disclosed the aliased account name: %q", err)
	}
}

func TestLoadRejectsInvalidIdentityMySQLValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{identityMySQLUserVariable, " SECRET_USER"},
		{identityMySQLReadTimeoutVariable, "SECRET_READ"},
		{identityMySQLPingTimeoutVariable, "SECRET_PING"},
		{identityMySQLMaxOpenConnsVariable, "SECRET_OPEN"},
		{identityMySQLMaxIdleConnsVariable, "SECRET_IDLE"},
		{identityMySQLConnMaxLifetimeVariable, "SECRET_LIFETIME"},
		{identityMySQLConnMaxIdleTimeVariable, "SECRET_IDLE_TIME"},
	}

	for _, test := range tests {
		t.Run(test.variable, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{test.variable: test.value})))
			if err == nil {
				t.Fatal("Load() error = nil, want Identity validation failure")
			}
			if config != (Config{}) {
				t.Fatal("Load() returned a nonzero config on failure")
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %q, want %s", err, test.variable)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("Load() error echoed supplied value: %q", err)
			}
		})
	}
}

func TestLoadRejectsIdentityPoolAndReadinessBudgetViolations(t *testing.T) {
	config, err := Load(mapLookup(apiVariables(map[string]string{
		identityMySQLMaxOpenConnsVariable: "4",
		identityMySQLMaxIdleConnsVariable: "5",
	})))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() did not fail closed for an invalid Identity pool")
	}
	for _, variable := range []string{identityMySQLMaxOpenConnsVariable, identityMySQLMaxIdleConnsVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}

	config, err = Load(mapLookup(apiVariables(map[string]string{
		httpWriteTimeoutVariable:         "3s",
		mysqlPingTimeoutVariable:         "2s",
		lotterySelectionTimeoutVariable:  "2s",
		identityMySQLPingTimeoutVariable: "2.000000001s",
	})))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() did not fail closed for an invalid Identity readiness budget")
	}
	for _, variable := range []string{identityMySQLPingTimeoutVariable, httpWriteTimeoutVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadIdentityPasswordSourcesAreMutuallyExclusiveAndRedacted(t *testing.T) {
	const secret = "IDENTITY_PASSWORD_MUST_NOT_LEAK"
	config, err := Load(mapLookup(apiVariables(map[string]string{
		identityMySQLPasswordVariable:     secret,
		identityMySQLPasswordFileVariable: "/SECRET/IDENTITY/PATH",
	})))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() did not fail closed for conflicting Identity password sources")
	}
	for _, sensitive := range []string{secret, "/SECRET/IDENTITY/PATH"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("Load() error leaked Identity secret material: %q", err)
		}
	}
	for _, variable := range []string{identityMySQLPasswordVariable, identityMySQLPasswordFileVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadRejectsInvalidIdentityPasswordFilesWithoutDisclosure(t *testing.T) {
	testDirectory := t.TempDir()
	missingPath := filepath.Join(testDirectory, "IDENTITY_MISSING_PATH_MUST_NOT_LEAK")
	oversizedSecret := "IDENTITY_OVERSIZED_SECRET_" + strings.Repeat("x", maximumPasswordBytes)
	oversizedPath := filepath.Join(testDirectory, "IDENTITY_OVERSIZED_PATH_MUST_NOT_LEAK")
	if err := os.WriteFile(oversizedPath, []byte(oversizedSecret), 0o600); err != nil {
		t.Fatalf("write oversized Identity password file: %v", err)
	}

	for _, test := range []struct {
		name       string
		path       string
		wantReason string
		sensitive  []string
	}{
		{name: "empty path", path: "   ", wantReason: "must not be empty", sensitive: []string{"   "}},
		{name: "missing", path: missingPath, wantReason: "could not be read", sensitive: []string{missingPath}},
		{name: "oversized", path: oversizedPath, wantReason: "no more than 1024 password bytes", sensitive: []string{oversizedPath, oversizedSecret}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := Load(mapLookup(map[string]string{
				mysqlPasswordVariable:             "business password",
				identityMySQLPasswordFileVariable: test.path,
			}))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() did not fail closed for an invalid Identity password file")
			}
			if !strings.Contains(err.Error(), identityMySQLPasswordFileVariable) || !strings.Contains(err.Error(), test.wantReason) {
				t.Fatalf("Load() error = %q, want stable Identity file failure", err)
			}
			for _, sensitive := range test.sensitive {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("Load() error leaked Identity password material: %q", err)
				}
			}
		})
	}
}

func TestLoadMigrationIgnoresIdentityRuntimeVariables(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:            "migration password",
		identityMySQLUserVariable:            "",
		identityMySQLPasswordVariable:        "",
		identityMySQLPasswordFileVariable:    "/DO_NOT_READ_IDENTITY_PASSWORD",
		identityMySQLReadTimeoutVariable:     "not-a-duration",
		identityMySQLPingTimeoutVariable:     "not-a-duration",
		identityMySQLMaxOpenConnsVariable:    "not-an-integer",
		identityMySQLMaxIdleConnsVariable:    "not-an-integer",
		identityMySQLConnMaxLifetimeVariable: "not-a-duration",
		identityMySQLConnMaxIdleTimeVariable: "not-a-duration",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v, want Identity runtime variables ignored", err)
	}
	if config.MySQL.Password != "migration password" {
		t.Fatal("LoadMigration() did not load the migration password")
	}
}
