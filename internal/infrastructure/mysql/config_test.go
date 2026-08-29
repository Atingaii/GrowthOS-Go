package mysqlstore

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
)

func TestDriverConfigAppliesSafeAPIDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := driverConfig(validConnection(), false)
	if err != nil {
		t.Fatalf("driverConfig() error = %v", err)
	}

	if cfg.Net != "tcp" || cfg.Addr != "db.example.test:3306" || cfg.DBName != "growthos" {
		t.Fatalf("unexpected connection fields: network=%q address=%q database=%q", cfg.Net, cfg.Addr, cfg.DBName)
	}
	if !cfg.ParseTime || cfg.Loc != time.UTC || cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("time invariants not applied: ParseTime=%v Loc=%v Params=%v", cfg.ParseTime, cfg.Loc, cfg.Params)
	}
	if dsn := cfg.FormatDSN(); !strings.Contains(dsn, "charset=utf8mb4") {
		t.Fatal("driver configuration did not use the dedicated utf8mb4 charset option")
	}
	if cfg.MultiStatements {
		t.Fatal("API connections must never enable multi-statements")
	}
	if cfg.AllowAllFiles || cfg.AllowCleartextPasswords || cfg.AllowFallbackToPlaintext || cfg.AllowNativePasswords || cfg.AllowOldPasswords || cfg.InterpolateParams {
		t.Fatal("a dangerous driver option was enabled")
	}
	if _, ok := cfg.Logger.(*drivermysql.NopLogger); !ok {
		t.Fatalf("driver logger = %T, want a per-connection NopLogger", cfg.Logger)
	}
	if cfg.TLS != nil {
		t.Fatal("disabled TLS mode unexpectedly installed TLS configuration")
	}
}

func TestDriverConfigEnablesMultiStatementsOnlyForMigration(t *testing.T) {
	t.Parallel()

	cfg, err := driverConfig(validConnection(), true)
	if err != nil {
		t.Fatalf("driverConfig() error = %v", err)
	}
	if !cfg.MultiStatements {
		t.Fatal("migration connection must enable multi-statements")
	}
}

func TestMigrationDriverConfigEnforcesStatementReadLockBudgets(t *testing.T) {
	t.Parallel()

	connection := validConnection()
	connection.ReadTimeout = 0
	cfg, err := migrationDriverConfig(MigrationConfig{ConnectionConfig: connection})
	if err != nil {
		t.Fatalf("migrationDriverConfig(defaults) error = %v", err)
	}
	if cfg.ReadTimeout != 35*time.Second {
		t.Fatalf("migration read timeout = %s, want 35s", cfg.ReadTimeout)
	}

	tests := []struct {
		name      string
		statement time.Duration
		read      time.Duration
		lock      time.Duration
		wantError bool
	}{
		{name: "ordered", statement: time.Second, read: 6 * time.Second, lock: 11 * time.Second},
		{name: "insufficient statement margin", statement: time.Second + time.Nanosecond, read: 6 * time.Second, lock: 11 * time.Second, wantError: true},
		{name: "insufficient lock margin", statement: 2 * time.Second, read: 7 * time.Second, lock: 11 * time.Second, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validConnection()
			input.ReadTimeout = test.read
			_, err := migrationDriverConfig(MigrationConfig{
				ConnectionConfig: input,
				StatementTimeout: test.statement,
				LockTimeout:      test.lock,
			})
			if test.wantError && err == nil {
				t.Fatal("migrationDriverConfig() error = nil, want timeout-order failure")
			}
			if !test.wantError && err != nil {
				t.Fatalf("migrationDriverConfig() error = %v", err)
			}
		})
	}
}

func TestSecretBearingConfigsRedactCommonFormattingBoundaries(t *testing.T) {
	t.Parallel()

	const secret = "SENTINEL_MYSQL_DRIVER_PASSWORD"
	connection := validConnection()
	connection.Password = secret
	values := []any{
		connection,
		Config{ConnectionConfig: connection},
		MigrationConfig{ConnectionConfig: connection},
	}
	for _, value := range values {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		logger.Info("config", slog.Any("config", value))
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		rendered := strings.Join([]string{
			fmt.Sprint(value),
			fmt.Sprintf("%+v", value),
			fmt.Sprintf("%#v", value),
			string(encoded),
			output.String(),
		}, "\n")
		if strings.Contains(rendered, secret) {
			t.Fatalf("common formatting boundary leaked the password: %s", rendered)
		}
		if !strings.Contains(rendered, "redacted") {
			t.Fatalf("common formatting boundary omitted the redaction marker: %s", rendered)
		}
	}
}

func TestDriverConfigBuildsVerifiedTLS(t *testing.T) {
	t.Parallel()

	caFile := filepath.Join(t.TempDir(), "test-ca.pem")
	if err := os.WriteFile(caFile, testCAPEM(t), 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	input := validConnection()
	input.TLSMode = TLSVerifyIdentity
	input.TLSCAFile = caFile

	cfg, err := driverConfig(input, false)
	if err != nil {
		t.Fatalf("driverConfig() error = %v", err)
	}
	if cfg.TLS == nil {
		t.Fatal("verified TLS configuration is nil")
	}
	if cfg.TLS.MinVersion != 0x0303 {
		t.Fatalf("MinVersion = %#x, want TLS 1.2", cfg.TLS.MinVersion)
	}
	if cfg.TLS.ServerName != "db.example.test" {
		t.Fatalf("ServerName = %q", cfg.TLS.ServerName)
	}
	if cfg.TLS.InsecureSkipVerify {
		t.Fatal("verified TLS must not skip certificate verification")
	}
	if cfg.TLS.RootCAs == nil {
		t.Fatal("verified TLS requires a root pool")
	}
}

func TestConfigRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*ConnectionConfig){
		"address without port": func(c *ConnectionConfig) { c.Address = "localhost" },
		"empty user":           func(c *ConnectionConfig) { c.User = "" },
		"trimmed user":         func(c *ConnectionConfig) { c.User = " admin" },
		"non printable user":   func(c *ConnectionConfig) { c.User = "admin\u0000" },
		"long user":            func(c *ConnectionConfig) { c.User = strings.Repeat("用", 33) },
		"empty password":       func(c *ConnectionConfig) { c.Password = "" },
		"long password":        func(c *ConnectionConfig) { c.Password = strings.Repeat("p", 1025) },
		"empty database":       func(c *ConnectionConfig) { c.Database = "" },
		"unknown TLS":          func(c *ConnectionConfig) { c.TLSMode = "preferred" },
		"CA without TLS":       func(c *ConnectionConfig) { c.TLSCAFile = "ca.pem" },
		"negative timeout":     func(c *ConnectionConfig) { c.ReadTimeout = -time.Second },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validConnection()
			mutate(&input)
			_, err := driverConfig(input, false)
			var safeErr *Error
			if !errors.As(err, &safeErr) || safeErr.Stage() != StageConfigInvalid {
				t.Fatalf("error = %v, want safe config error", err)
			}
		})
	}
}

func TestTLSCAFailureDoesNotRenderPathOrPassword(t *testing.T) {
	t.Parallel()

	secretPath := filepath.Join(t.TempDir(), "private-customer-name-ca.pem")
	input := validConnection()
	input.Password = "credential-that-must-not-appear"
	input.TLSMode = TLSVerifyIdentity
	input.TLSCAFile = secretPath

	_, err := driverConfig(input, false)
	if err == nil {
		t.Fatal("driverConfig() error = nil")
	}
	rendered := err.Error()
	if rendered != string(StageTLSCA) {
		t.Fatalf("error = %q, want stable stage", rendered)
	}
	for _, forbidden := range []string{secretPath, input.Password, "private-customer-name"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("error leaked %q: %q", forbidden, rendered)
		}
	}
}

func TestErrorPreservesCauseWithoutRenderingIt(t *testing.T) {
	t.Parallel()

	cause := errors.New("sensitive driver detail")
	err := newError(StagePing, cause)
	if err.Error() != string(StagePing) {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is not available through errors.Is")
	}
}

func TestNormalizePoolDefaultsAndLimits(t *testing.T) {
	t.Parallel()

	pool, err := normalizePool(Config{})
	if err != nil {
		t.Fatalf("normalizePool() error = %v", err)
	}
	if pool.maxOpen != defaultMaxOpen || pool.maxIdle != 0 || pool.pingTimeout != defaultPingTimeout {
		t.Fatalf("unexpected defaults: %+v", pool)
	}
	if pool.maxLifetime != 3*time.Minute || pool.maxIdleTime != time.Minute {
		t.Fatalf("unexpected lifetime defaults: %+v", pool)
	}

	_, err = normalizePool(Config{MaxOpenConnections: 2, MaxIdleConnections: 3})
	if !errors.Is(err, errConfigValue) {
		t.Fatalf("invalid pool error = %v", err)
	}
}

func validConnection() ConnectionConfig {
	return ConnectionConfig{
		Address:        "db.example.test:3306",
		Database:       "growthos",
		User:           "growth_api",
		Password:       "not-logged-password",
		TLSMode:        TLSDisabled,
		ConnectTimeout: time.Second,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
	}
}

func testCAPEM(t *testing.T) []byte {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GrowthOS test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
