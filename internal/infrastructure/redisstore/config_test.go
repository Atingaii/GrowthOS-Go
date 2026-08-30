package redisstore

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
)

func TestRedisOptionsApplyExplicitBoundedClientPolicy(t *testing.T) {
	t.Parallel()

	input := validConfig()
	options, err := redisOptions(input)
	if err != nil {
		t.Fatalf("redisOptions() error = %v", err)
	}

	if options.Network != "tcp" || options.Addr != input.Address {
		t.Fatalf("endpoint = %q/%q, want tcp/%q", options.Network, options.Addr, input.Address)
	}
	if options.Protocol != 2 {
		t.Fatalf("Protocol = %d, want RESP2", options.Protocol)
	}
	if options.Username != input.Username || options.Password != input.Password || options.DB != input.Database {
		t.Fatal("redis identity or database was not preserved")
	}
	if options.MaxRetries != -1 || options.DialerRetries != 1 {
		t.Fatalf("retry policy = max:%d dial:%d, want -1/1", options.MaxRetries, options.DialerRetries)
	}
	if !options.ContextTimeoutEnabled || !options.DisableIdentity {
		t.Fatalf("context/identity policy = %v/%v, want true/true", options.ContextTimeoutEnabled, options.DisableIdentity)
	}
	if options.DialTimeout != input.DialTimeout ||
		options.ReadTimeout != input.ReadTimeout ||
		options.WriteTimeout != input.WriteTimeout ||
		options.PoolTimeout != input.PoolTimeout {
		t.Fatal("one or more bounded timeouts were not preserved")
	}
	if options.PoolSize != input.PoolSize ||
		options.MinIdleConns != input.MinIdleConnections ||
		options.MaxIdleConns != input.PoolSize ||
		options.MaxActiveConns != input.PoolSize {
		t.Fatalf(
			"pool limits = size:%d min:%d max-idle:%d max-active:%d",
			options.PoolSize,
			options.MinIdleConns,
			options.MaxIdleConns,
			options.MaxActiveConns,
		)
	}
	if options.ConnMaxLifetime != input.ConnectionMaxLifetime ||
		options.ConnMaxIdleTime != input.ConnectionMaxIdleTime {
		t.Fatal("connection lifecycle limits were not preserved")
	}
	if options.TLSConfig != nil {
		t.Fatal("disabled TLS mode unexpectedly installed TLS configuration")
	}
}

func TestRedisOptionsApplyDocumentedDefaults(t *testing.T) {
	t.Parallel()

	options, err := redisOptions(Config{
		Username: "growthos_cache",
		Password: "not-logged-password",
	})
	if err != nil {
		t.Fatalf("redisOptions(defaults) error = %v", err)
	}

	if options.Addr != defaultAddress || options.DB != 0 || options.TLSConfig != nil {
		t.Fatalf("unexpected connection defaults: address=%q db=%d tls=%v", options.Addr, options.DB, options.TLSConfig != nil)
	}
	if options.DialTimeout != defaultDialTimeout ||
		options.ReadTimeout != defaultReadTimeout ||
		options.WriteTimeout != defaultWriteTimeout ||
		options.PoolTimeout != defaultPoolTimeout {
		t.Fatal("unexpected timeout defaults")
	}
	if options.PoolSize != defaultPoolSize || options.MinIdleConns != 0 ||
		options.ConnMaxLifetime != defaultConnectionMaxLifetime ||
		options.ConnMaxIdleTime != defaultConnectionMaxIdleTime {
		t.Fatal("unexpected pool defaults")
	}
}

func TestRedisConfigRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Config){
		"address without port":      func(c *Config) { c.Address = "cache.example.test" },
		"address with empty host":   func(c *Config) { c.Address = ":6379" },
		"port out of range":         func(c *Config) { c.Address = "cache.example.test:65536" },
		"empty username":            func(c *Config) { c.Username = "" },
		"trimmed username":          func(c *Config) { c.Username = " cache" },
		"control in username":       func(c *Config) { c.Username = "cache\u0000" },
		"oversized username":        func(c *Config) { c.Username = strings.Repeat("用", maximumUsernameRunes+1) },
		"empty password":            func(c *Config) { c.Password = "" },
		"oversized password":        func(c *Config) { c.Password = strings.Repeat("p", maximumPasswordBytes+1) },
		"negative database":         func(c *Config) { c.Database = -1 },
		"nonzero database":          func(c *Config) { c.Database = 1 },
		"unknown tls mode":          func(c *Config) { c.TLSMode = "preferred" },
		"ca while tls disabled":     func(c *Config) { c.TLSCAFile = "private-ca.pem" },
		"negative dial timeout":     func(c *Config) { c.DialTimeout = -time.Second },
		"oversized read timeout":    func(c *Config) { c.ReadTimeout = time.Minute + time.Nanosecond },
		"negative write timeout":    func(c *Config) { c.WriteTimeout = -time.Second },
		"negative pool timeout":     func(c *Config) { c.PoolTimeout = -time.Second },
		"negative pool size":        func(c *Config) { c.PoolSize = -1 },
		"oversized pool size":       func(c *Config) { c.PoolSize = maximumPoolSize + 1 },
		"negative minimum idle":     func(c *Config) { c.MinIdleConnections = -1 },
		"minimum idle exceeds pool": func(c *Config) { c.MinIdleConnections = c.PoolSize + 1 },
		"negative max lifetime":     func(c *Config) { c.ConnectionMaxLifetime = -time.Second },
		"negative max idle time":    func(c *Config) { c.ConnectionMaxIdleTime = -time.Second },
	}

	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := validConfig()
			mutate(&input)
			_, err := redisOptions(input)
			var safeErr *Error
			if !errors.As(err, &safeErr) || safeErr.Stage() != StageConfigInvalid {
				t.Fatalf("error = %v, want safe config error", err)
			}
		})
	}
}

func TestRedisConfigRedactsCommonFormattingBoundaries(t *testing.T) {
	t.Parallel()

	input := validConfig()
	input.Password = "SENTINEL_REDIS_PASSWORD"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logger.Info("config", slog.Any("config", input))
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	rendered := strings.Join([]string{
		fmt.Sprint(input),
		fmt.Sprintf("%+v", input),
		fmt.Sprintf("%#v", input),
		string(encoded),
		output.String(),
	}, "\n")
	if strings.Contains(rendered, input.Password) {
		t.Fatalf("common formatting boundary leaked Redis password: %s", rendered)
	}
	if !strings.Contains(rendered, "redacted") {
		t.Fatalf("redaction marker missing: %s", rendered)
	}
}

func TestRedisOptionsBuildVerifiedTLS(t *testing.T) {
	t.Parallel()

	caFile := filepath.Join(t.TempDir(), "test-ca.pem")
	if err := os.WriteFile(caFile, testCAPEM(t), 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}
	input := validConfig()
	input.TLSMode = TLSVerifyIdentity
	input.TLSCAFile = caFile

	options, err := redisOptions(input)
	if err != nil {
		t.Fatalf("redisOptions() error = %v", err)
	}
	tlsConfig := options.TLSConfig
	if tlsConfig == nil {
		t.Fatal("verified TLS configuration is nil")
	}
	if tlsConfig.MinVersion != 0x0303 || tlsConfig.ServerName != "cache.example.test" {
		t.Fatalf("TLS identity = version:%#x server:%q", tlsConfig.MinVersion, tlsConfig.ServerName)
	}
	if tlsConfig.InsecureSkipVerify || tlsConfig.RootCAs == nil {
		t.Fatal("verified TLS must use certificate verification and roots")
	}
}

func TestRedisTLSCAFailureDoesNotRenderPathOrCredential(t *testing.T) {
	t.Parallel()

	secretPath := filepath.Join(t.TempDir(), "private-customer-name-ca.pem")
	input := validConfig()
	input.Password = "credential-that-must-not-appear"
	input.TLSMode = TLSVerifyIdentity
	input.TLSCAFile = secretPath

	_, err := redisOptions(input)
	if err == nil {
		t.Fatal("redisOptions() error = nil")
	}
	if err.Error() != string(StageTLSCA) {
		t.Fatalf("error = %q, want safe TLS CA stage", err.Error())
	}
	for _, forbidden := range []string{secretPath, input.Password, "private-customer-name"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error leaked %q: %q", forbidden, err)
		}
	}
}

func TestRedisErrorPreservesCauseWithoutRenderingIt(t *testing.T) {
	t.Parallel()

	cause := errors.New("sensitive redis topology detail")
	err := newError(StageGetRange, cause)
	if err.Error() != string(StageGetRange) {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapped cause is unavailable through errors.Is")
	}
}

func TestZeroRedisErrorFailsClosedToConfigStage(t *testing.T) {
	t.Parallel()

	var err Error
	if err.Error() != string(StageConfigInvalid) || err.Stage() != StageConfigInvalid {
		t.Fatalf("zero Error = %q/%q, want config stage", err.Error(), err.Stage())
	}
}

func validConfig() Config {
	return Config{
		Address:               "cache.example.test:6379",
		Username:              "growthos_cache",
		Password:              "not-logged-password",
		Database:              0,
		TLSMode:               TLSDisabled,
		DialTimeout:           time.Second,
		ReadTimeout:           2 * time.Second,
		WriteTimeout:          2 * time.Second,
		PoolTimeout:           3 * time.Second,
		PoolSize:              12,
		MinIdleConnections:    2,
		ConnectionMaxLifetime: 20 * time.Minute,
		ConnectionMaxIdleTime: 4 * time.Minute,
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
		Subject:               pkix.Name{CommonName: "GrowthOS Redis test CA"},
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
