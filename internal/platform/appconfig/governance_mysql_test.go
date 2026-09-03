package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresGovernancePasswordIndependently(t *testing.T) {
	variables := apiVariables(nil)
	delete(variables, governanceMySQLPasswordVariable)
	delete(variables, governanceMySQLPasswordFileVariable)

	config, err := Load(mapLookup(variables))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() accepted an API configuration without a Governance password")
	}
	for _, variable := range []string{governanceMySQLPasswordVariable, governanceMySQLPasswordFileVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadBuildsIndependentGovernancePoolOnSharedDeploymentEndpoint(t *testing.T) {
	config, err := Load(mapLookup(apiVariables(map[string]string{
		mysqlAddressVariable:                   "mysql.internal:4406",
		mysqlDatabaseVariable:                  "growthos_governance_test",
		mysqlTLSModeVariable:                   string(MySQLTLSVerifyIdentity),
		mysqlTLSCAFileVariable:                 "/run/mysql/ca.pem",
		mysqlConnectTimeoutVariable:            "9s",
		mysqlWriteTimeoutVariable:              "17s",
		mysqlReadTimeoutVariable:               "31s",
		mysqlMaxOpenConnsVariable:              "27",
		mysqlMaxIdleConnsVariable:              "13",
		governanceMySQLReadTimeoutVariable:     "23s",
		governanceMySQLMaxOpenConnsVariable:    "8",
		governanceMySQLMaxIdleConnsVariable:    "3",
		governanceMySQLConnMaxLifetimeVariable: "14m",
		governanceMySQLConnMaxIdleTimeVariable: "4m",
	})))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for name, got := range map[string]string{
		"address":  config.GovernanceMySQL.Address,
		"database": config.GovernanceMySQL.Database,
		"CA file":  config.GovernanceMySQL.TLSCAFile,
	} {
		want := map[string]string{
			"address":  config.MySQL.Address,
			"database": config.MySQL.Database,
			"CA file":  config.MySQL.TLSCAFile,
		}[name]
		if got != want {
			t.Fatalf("Governance %s = %q, want shared deployment value %q", name, got, want)
		}
	}
	if config.GovernanceMySQL.TLSMode != config.MySQL.TLSMode ||
		config.GovernanceMySQL.ConnectTimeout != config.MySQL.ConnectTimeout ||
		config.GovernanceMySQL.WriteTimeout != config.MySQL.WriteTimeout {
		t.Fatal("Governance did not inherit the shared endpoint/TLS connection policy")
	}
	if config.GovernanceMySQL.ReadTimeout != 23*time.Second || config.MySQL.ReadTimeout != 31*time.Second {
		t.Fatal("Governance and business read budgets were not independent")
	}
	if config.GovernanceMySQL.MaxOpenConnections != 8 || config.MySQL.MaxOpenConnections != 27 {
		t.Fatal("Governance and business pools were not independent")
	}
	if config.GovernanceMySQL.User != defaultGovernanceMySQLUser ||
		config.GovernanceMySQL.Password != "test-governance-password" {
		t.Fatal("Governance runtime credentials were not loaded independently")
	}
}

func TestLoadRejectsAliasedGovernanceRuntimeAccounts(t *testing.T) {
	const privateUser = "SAME_PRIVATE_RUNTIME_USER"
	tests := []struct {
		name      string
		variables []string
	}{
		{name: "business and governance", variables: []string{mysqlUserVariable, governanceMySQLUserVariable}},
		{name: "identity and governance", variables: []string{identityMySQLUserVariable, governanceMySQLUserVariable}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			overrides := map[string]string{}
			for _, variable := range test.variables {
				overrides[variable] = privateUser
			}
			config, err := Load(mapLookup(apiVariables(overrides)))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted one MySQL identity for two authority boundaries")
			}
			for _, variable := range test.variables {
				if !strings.Contains(err.Error(), variable) {
					t.Fatalf("Load() error = %q, want %s", err, variable)
				}
			}
			if strings.Contains(err.Error(), privateUser) {
				t.Fatalf("Load() disclosed the aliased account name: %q", err)
			}
		})
	}
}

func TestLoadRejectsInvalidGovernanceMySQLValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{governanceMySQLUserVariable, " SECRET_USER"},
		{governanceMySQLReadTimeoutVariable, "SECRET_READ"},
		{governanceMySQLPingTimeoutVariable, "SECRET_PING"},
		{governanceMySQLMaxOpenConnsVariable, "SECRET_OPEN"},
		{governanceMySQLMaxIdleConnsVariable, "SECRET_IDLE"},
		{governanceMySQLConnMaxLifetimeVariable, "SECRET_LIFETIME"},
		{governanceMySQLConnMaxIdleTimeVariable, "SECRET_IDLE_TIME"},
	}

	for _, test := range tests {
		t.Run(test.variable, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{test.variable: test.value})))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an invalid Governance MySQL value")
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

func TestLoadRejectsGovernancePoolAndDeadlineViolations(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{
			name: "pool",
			overrides: map[string]string{
				governanceMySQLMaxOpenConnsVariable: "4",
				governanceMySQLMaxIdleConnsVariable: "5",
			},
			want: []string{governanceMySQLMaxOpenConnsVariable, governanceMySQLMaxIdleConnsVariable},
		},
		{
			name: "readiness",
			overrides: map[string]string{
				httpWriteTimeoutVariable:           "3s",
				mysqlPingTimeoutVariable:           "2s",
				identityMySQLPingTimeoutVariable:   "2s",
				governanceMySQLPingTimeoutVariable: "2.000000001s",
				lotterySelectionTimeoutVariable:    "2s",
				identityHTTPHandlerTimeoutVariable: "2s",
			},
			want: []string{governanceMySQLPingTimeoutVariable, httpWriteTimeoutVariable},
		},
		{
			name: "policy read before selection deadline",
			overrides: map[string]string{
				lotterySelectionTimeoutVariable:    "4.000000001s",
				mysqlReadTimeoutVariable:           "10s",
				mysqlWriteTimeoutVariable:          "10s",
				governanceMySQLReadTimeoutVariable: "5s",
			},
			want: []string{lotterySelectionTimeoutVariable, governanceMySQLReadTimeoutVariable},
		},
		{
			name: "audit commit before selection deadline",
			overrides: map[string]string{
				lotterySelectionTimeoutVariable:    "4.000000001s",
				mysqlReadTimeoutVariable:           "10s",
				governanceMySQLReadTimeoutVariable: "10s",
				mysqlWriteTimeoutVariable:          "5s",
			},
			want: []string{lotterySelectionTimeoutVariable, mysqlWriteTimeoutVariable},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(test.overrides)))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an invalid Governance pool or deadline relationship")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Fatalf("Load() error = %q, want %s", err, variable)
				}
			}
		})
	}
}

func TestLoadGovernancePasswordSourcesAreMutuallyExclusiveAndRedacted(t *testing.T) {
	const secret = "GOVERNANCE_PASSWORD_MUST_NOT_LEAK"
	config, err := Load(mapLookup(apiVariables(map[string]string{
		governanceMySQLPasswordVariable:     secret,
		governanceMySQLPasswordFileVariable: "/SECRET/GOVERNANCE/PATH",
	})))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() accepted conflicting Governance password sources")
	}
	for _, sensitive := range []string{secret, "/SECRET/GOVERNANCE/PATH"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("Load() error leaked Governance secret material: %q", err)
		}
	}
	for _, variable := range []string{governanceMySQLPasswordVariable, governanceMySQLPasswordFileVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadRejectsInvalidGovernancePasswordFilesWithoutDisclosure(t *testing.T) {
	testDirectory := t.TempDir()
	missingPath := filepath.Join(testDirectory, "GOVERNANCE_MISSING_PATH_MUST_NOT_LEAK")
	oversizedSecret := "GOVERNANCE_OVERSIZED_SECRET_" + strings.Repeat("x", maximumPasswordBytes)
	oversizedPath := filepath.Join(testDirectory, "GOVERNANCE_OVERSIZED_PATH_MUST_NOT_LEAK")
	if err := os.WriteFile(oversizedPath, []byte(oversizedSecret), 0o600); err != nil {
		t.Fatalf("write oversized Governance password file: %v", err)
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
			variables := apiVariables(map[string]string{governanceMySQLPasswordFileVariable: test.path})
			delete(variables, governanceMySQLPasswordVariable)
			config, err := Load(mapLookup(variables))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an invalid Governance password file")
			}
			if !strings.Contains(err.Error(), governanceMySQLPasswordFileVariable) ||
				!strings.Contains(err.Error(), test.wantReason) {
				t.Fatalf("Load() error = %q, want stable Governance file failure", err)
			}
			for _, sensitive := range test.sensitive {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("Load() error leaked Governance password material: %q", err)
				}
			}
		})
	}
}

func TestLoadMigrationIgnoresGovernanceRuntimeVariables(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:              "migration password",
		governanceMySQLUserVariable:            "",
		governanceMySQLPasswordVariable:        "",
		governanceMySQLPasswordFileVariable:    "/DO_NOT_READ_GOVERNANCE_PASSWORD",
		governanceMySQLReadTimeoutVariable:     "not-a-duration",
		governanceMySQLPingTimeoutVariable:     "not-a-duration",
		governanceMySQLMaxOpenConnsVariable:    "not-an-integer",
		governanceMySQLMaxIdleConnsVariable:    "not-an-integer",
		governanceMySQLConnMaxLifetimeVariable: "not-a-duration",
		governanceMySQLConnMaxIdleTimeVariable: "not-a-duration",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v, want Governance runtime variables ignored", err)
	}
	if config.MySQL.Password != "migration password" {
		t.Fatal("LoadMigration() did not load the migration password")
	}
}
