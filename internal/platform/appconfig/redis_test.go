package appconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresRedisCredentialOnlyWhenStrategyCacheIsEnabled(t *testing.T) {
	disabled, err := Load(mapLookup(apiVariables(nil)))
	if err != nil {
		t.Fatalf("Load(disabled) error = %v", err)
	}
	if disabled.Lottery.StrategyCache.Enabled || disabled.Redis.Password != "" {
		t.Fatal("disabled cache unexpectedly required or retained a Redis credential")
	}

	_, err = Load(mapLookup(apiVariables(map[string]string{
		lotteryStrategyCacheEnabledVariable: "true",
	})))
	if err == nil || !strings.Contains(err.Error(), redisPasswordVariable) ||
		!strings.Contains(err.Error(), redisPasswordFileVariable) {
		t.Fatalf("Load(enabled without credential) error = %v, want both credential variable names", err)
	}

	const password = "redis-cache-test-password"
	enabled, err := Load(mapLookup(apiVariables(map[string]string{
		lotteryStrategyCacheEnabledVariable: "true",
		redisPasswordVariable:               password,
	})))
	if err != nil {
		t.Fatalf("Load(enabled) error = %v", err)
	}
	if !enabled.Lottery.StrategyCache.Enabled || enabled.Redis.Password != password {
		t.Fatal("enabled cache did not preserve the explicitly supplied Redis credential")
	}
}

func TestLoadReadsRedisCredentialFromBoundedSecretFile(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "redis-password")
	if err := os.WriteFile(secretFile, []byte("redis-file-secret\r\n"), 0o600); err != nil {
		t.Fatalf("write Redis secret fixture: %v", err)
	}

	config, err := Load(mapLookup(apiVariables(map[string]string{
		lotteryStrategyCacheEnabledVariable: "true",
		redisPasswordFileVariable:           secretFile,
	})))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Redis.Password != "redis-file-secret" {
		t.Fatal("Load() did not remove only the secret file line ending")
	}
}

func TestLoadRejectsAmbiguousRedisCredential(t *testing.T) {
	const secret = "REDIS_SECRET_MUST_NOT_BE_ECHOED"
	_, err := Load(mapLookup(apiVariables(map[string]string{
		lotteryStrategyCacheEnabledVariable: "true",
		redisPasswordVariable:               secret,
		redisPasswordFileVariable:           "/run/secrets/redis_password",
	})))
	if err == nil {
		t.Fatal("Load() error = nil, want mutually-exclusive credential failure")
	}
	if !strings.Contains(err.Error(), redisPasswordVariable) ||
		!strings.Contains(err.Error(), redisPasswordFileVariable) ||
		strings.Contains(err.Error(), secret) {
		t.Fatalf("Load() returned an unsafe or incomplete error: %v", err)
	}
}

func TestLoadEnforcesVerifiedRedisTLSOnlyWhenEnabledInDeployment(t *testing.T) {
	base := map[string]string{
		environmentVariable:                 string(EnvironmentProduction),
		mysqlTLSModeVariable:                string(MySQLTLSVerifyIdentity),
		lotteryStrategyCacheEnabledVariable: "true",
		redisPasswordVariable:               "redis-production-test-password",
	}

	_, err := Load(mapLookup(apiVariables(base)))
	if err == nil || !strings.Contains(err.Error(), redisTLSModeVariable) {
		t.Fatalf("Load(disabled Redis TLS) error = %v, want deployment TLS failure", err)
	}

	base[redisTLSModeVariable] = string(RedisTLSVerifyIdentity)
	config, err := Load(mapLookup(apiVariables(base)))
	if err != nil {
		t.Fatalf("Load(verified Redis TLS) error = %v", err)
	}
	if config.Redis.TLSMode != RedisTLSVerifyIdentity {
		t.Fatalf("Redis TLS mode = %q, want verify_identity", config.Redis.TLSMode)
	}
}

func TestLoadRejectsRedisCAWhenTLSIsDisabled(t *testing.T) {
	_, err := Load(mapLookup(apiVariables(map[string]string{
		redisTLSModeVariable:   string(RedisTLSDisabled),
		redisTLSCAFileVariable: "/private/redis-ca.pem",
	})))
	if err == nil {
		t.Fatal("Load() error = nil, want TLS/CA relationship failure")
	}
	for _, variable := range []string{redisTLSModeVariable, redisTLSCAFileVariable} {
		if !strings.Contains(err.Error(), variable) {
			t.Fatalf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadRejectsRedisPoolAndCacheBudgetRelationships(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		variables []string
	}{
		{
			name: "idle exceeds pool",
			overrides: map[string]string{
				redisPoolSizeVariable:     "2",
				redisMinIdleConnsVariable: "3",
			},
			variables: []string{redisPoolSizeVariable, redisMinIdleConnsVariable},
		},
		{
			name: "cache work exceeds selection deadline",
			overrides: map[string]string{
				lotteryStrategyCacheEnabledVariable: "true",
				redisPasswordVariable:               "test-password",
				lotterySelectionTimeoutVariable:     "2s",
				lotteryStrategyCacheLookupVariable:  "100ms",
				lotteryStrategyCacheFillVariable:    "2s",
				lotteryStrategyCacheWriteVariable:   "100ms",
			},
			variables: []string{
				lotteryStrategyCacheLookupVariable,
				lotteryStrategyCacheFillVariable,
				lotteryStrategyCacheWriteVariable,
				lotterySelectionTimeoutVariable,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(test.overrides)))
			if err == nil {
				t.Fatal("Load() error = nil, want relationship failure")
			}
			for _, variable := range test.variables {
				if !strings.Contains(err.Error(), variable) {
					t.Fatalf("Load() error = %q, want %s", err, variable)
				}
			}
		})
	}
}

func TestLoadRejectsInvalidRedisValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		variable string
		value    string
	}{
		{lotteryStrategyCacheEnabledVariable, "SENSITIVE_BOOLEAN"},
		{redisAddressVariable, "SENSITIVE_BAD_ADDRESS"},
		{redisUsernameVariable, "SENSITIVE USER"},
		{redisDatabaseVariable, "SENSITIVE_DATABASE"},
		{redisTLSModeVariable, "SENSITIVE_TLS_MODE"},
		{redisDialTimeoutVariable, "SENSITIVE_DIAL_TIMEOUT"},
		{redisReadTimeoutVariable, "SENSITIVE_READ_TIMEOUT"},
		{redisWriteTimeoutVariable, "SENSITIVE_WRITE_TIMEOUT"},
		{redisPoolTimeoutVariable, "SENSITIVE_POOL_TIMEOUT"},
		{redisPoolSizeVariable, "SENSITIVE_POOL_SIZE"},
		{redisMinIdleConnsVariable, "SENSITIVE_MIN_IDLE"},
		{redisConnMaxLifetimeVariable, "SENSITIVE_LIFETIME"},
		{redisConnMaxIdleTimeVariable, "SENSITIVE_IDLE_TIME"},
		{lotteryStrategyCacheTTLVariable, "SENSITIVE_TTL"},
		{lotteryStrategyCacheLookupVariable, "SENSITIVE_LOOKUP"},
		{lotteryStrategyCacheWriteVariable, "SENSITIVE_WRITE"},
		{lotteryStrategyCacheFillVariable, "SENSITIVE_FILL"},
	}

	for _, test := range tests {
		t.Run(test.variable, func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{test.variable: test.value})))
			if err == nil || !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %v, want %s validation failure", err, test.variable)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("Load() error echoed supplied value: %v", err)
			}
		})
	}
}

func TestLoadCapsStrategyCacheTTLAtFiveMinutes(t *testing.T) {
	_, err := Load(mapLookup(apiVariables(map[string]string{
		lotteryStrategyCacheTTLVariable: "5m1ns",
	})))
	if err == nil || !strings.Contains(err.Error(), lotteryStrategyCacheTTLVariable) {
		t.Fatalf("Load() error = %v, want five-minute TTL ceiling", err)
	}

	config, err := Load(mapLookup(apiVariables(map[string]string{
		lotteryStrategyCacheTTLVariable: "5m",
	})))
	if err != nil {
		t.Fatalf("Load(5m) error = %v", err)
	}
	if config.Lottery.StrategyCache.TTL != 5*time.Minute {
		t.Fatalf("strategy cache TTL = %s, want 5m", config.Lottery.StrategyCache.TTL)
	}
}

func TestLoadRejectsSubMillisecondCacheBudgetsAndSubsecondTTL(t *testing.T) {
	tests := map[string]string{
		lotteryStrategyCacheTTLVariable:    "999ms",
		lotteryStrategyCacheLookupVariable: "999us",
		lotteryStrategyCacheWriteVariable:  "999us",
		lotteryStrategyCacheFillVariable:   "999us",
	}
	for variable, value := range tests {
		t.Run(variable, func(t *testing.T) {
			_, err := Load(mapLookup(apiVariables(map[string]string{variable: value})))
			if err == nil || !strings.Contains(err.Error(), variable) {
				t.Fatalf("Load() error = %v, want lower-bound failure for %s", err, variable)
			}
		})
	}
}

func TestRedisConfigRedactsCommonFormattingBoundaries(t *testing.T) {
	const secret = "SENTINEL_REDIS_PASSWORD"
	config := defaultRedisConfig()
	config.Password = secret

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logger.Info("config", slog.Any("redis", config))
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	rendered := strings.Join([]string{
		fmt.Sprint(config),
		fmt.Sprintf("%+v", config),
		fmt.Sprintf("%#v", config),
		string(encoded),
		output.String(),
	}, "\n")
	if strings.Contains(rendered, secret) || !strings.Contains(rendered, "redacted") {
		t.Fatalf("Redis formatting boundary was unsafe: %s", rendered)
	}
}

func TestLoadMigrationIgnoresRedisOnlyVariables(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:           "migration-password",
		lotteryStrategyCacheEnabledVariable: "not-a-boolean",
		redisPasswordFileVariable:           "/DO_NOT_READ_REDIS_SECRET",
		redisAddressVariable:                "not-an-address",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v, want API cache variables ignored", err)
	}
	if config.MySQL.User != defaultMigrationUser {
		t.Fatalf("migration user = %q, want %q", config.MySQL.User, defaultMigrationUser)
	}
}

func TestStrategyCacheDefaultsFitSelectionBudget(t *testing.T) {
	config := Default()
	cache := config.Lottery.StrategyCache
	if cache.LookupTimeout+cache.FillTimeout+cache.WriteTimeout > config.Lottery.SelectionTimeout {
		t.Fatalf("default cache budget %s exceeds selection budget %s",
			cache.LookupTimeout+cache.FillTimeout+cache.WriteTimeout,
			config.Lottery.SelectionTimeout,
		)
	}
}
