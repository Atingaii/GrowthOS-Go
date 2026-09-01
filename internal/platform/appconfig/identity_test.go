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

func TestLoadIdentityUsesRequiredSecretsAndDocumentedDefaults(t *testing.T) {
	config, err := Load(mapLookup(apiVariables(nil)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Identity.PublicOrigin != "http://127.0.0.1:8080" ||
		config.Identity.CookieMode != IdentityCookieModeDevelopment ||
		config.Identity.PasswordHash != (IdentityPasswordHashConfig{
			MaxConcurrent:  defaultIdentityArgon2MaxConcurrent,
			AcquireTimeout: defaultIdentityArgon2AcquireTimeout,
		}) || config.Identity.HandlerTimeout != defaultIdentityHTTPHandlerTimeout {
		t.Fatalf("Identity defaults = %#v", config.Identity)
	}
	if config.Identity.ThrottleHMACKey != mustSecret32(t, testIdentityThrottleKey) ||
		config.Identity.CSRF.ActiveKeyID != "active" ||
		config.Identity.CSRF.ActiveKey != mustSecret32(t, testIdentityCSRFActiveKey) ||
		config.Identity.CSRF.HasPrevious ||
		config.Identity.CSRF.Previous != (IdentityPreviousCSRFKeyConfig{}) {
		t.Fatal("Load() did not preserve the independent active key configuration")
	}
}

func TestLoadIdentityRequiresEveryActiveBoundary(t *testing.T) {
	config, err := Load(mapLookup(map[string]string{
		mysqlPasswordVariable:         "business password",
		identityMySQLPasswordVariable: "identity password",
	}))
	if err == nil || config != (Config{}) {
		t.Fatal("Load() did not fail closed for missing Identity security configuration")
	}
	for _, variable := range []string{
		identityPublicOriginVariable,
		identityThrottleHMACKeyVariable,
		identityThrottleHMACKeyFileVariable,
		identityCSRFActiveKeyIDVariable,
		identityCSRFActiveKeyVariable,
		identityCSRFActiveKeyFileVariable,
	} {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("Load() error = %q, want %s", err, variable)
		}
	}
}

func TestLoadIdentityDerivesCookieModeAndOriginPolicyFromEnvironment(t *testing.T) {
	for _, test := range []struct {
		environment Environment
		origin      string
		mode        IdentityCookieMode
	}{
		{environment: EnvironmentDevelopment, origin: "http://127.0.0.1:8088", mode: IdentityCookieModeDevelopment},
		{environment: EnvironmentTest, origin: "http://[::1]:8088", mode: IdentityCookieModeDevelopment},
		{environment: EnvironmentStaging, origin: "https://staging.growth.example:8443", mode: IdentityCookieModeProduction},
		{environment: EnvironmentProduction, origin: "https://growth.example", mode: IdentityCookieModeProduction},
	} {
		t.Run(string(test.environment), func(t *testing.T) {
			variables := apiVariables(map[string]string{
				environmentVariable:          string(test.environment),
				identityPublicOriginVariable: test.origin,
			})
			if test.mode == IdentityCookieModeProduction {
				variables[mysqlTLSModeVariable] = string(MySQLTLSVerifyIdentity)
			}
			// apiVariables chooses a safe environment default; this test then
			// deliberately verifies the exact caller-supplied origin.
			variables[identityPublicOriginVariable] = test.origin
			config, err := Load(mapLookup(variables))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Identity.PublicOrigin != test.origin || config.Identity.CookieMode != test.mode {
				t.Fatalf("Identity origin/mode = %q/%q, want %q/%q", config.Identity.PublicOrigin, config.Identity.CookieMode, test.origin, test.mode)
			}
		})
	}
}

func TestLoadIdentityRejectsOriginsOutsideTheEnvironmentBoundary(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment Environment
		origin      string
	}{
		{name: "development https", environment: EnvironmentDevelopment, origin: "https://127.0.0.1:8088"},
		{name: "development hostname", environment: EnvironmentDevelopment, origin: "http://localhost:8088"},
		{name: "development non-loopback", environment: EnvironmentDevelopment, origin: "http://192.0.2.10:8088"},
		{name: "development path", environment: EnvironmentDevelopment, origin: "http://127.0.0.1:8088/"},
		{name: "development query", environment: EnvironmentDevelopment, origin: "http://127.0.0.1:8088?x=1"},
		{name: "development credentials", environment: EnvironmentDevelopment, origin: "http://user@127.0.0.1:8088"},
		{name: "development noncanonical port", environment: EnvironmentDevelopment, origin: "http://127.0.0.1:08080"},
		{name: "development default port", environment: EnvironmentDevelopment, origin: "http://127.0.0.1:80"},
		{name: "development noncanonical ipv6", environment: EnvironmentDevelopment, origin: "http://[0:0:0:0:0:0:0:1]:8088"},
		{name: "production http", environment: EnvironmentProduction, origin: "http://growth.example"},
		{name: "production path", environment: EnvironmentProduction, origin: "https://growth.example/"},
		{name: "production fragment", environment: EnvironmentProduction, origin: "https://growth.example#private"},
		{name: "production whitespace", environment: EnvironmentProduction, origin: " https://growth.example"},
		{name: "production uppercase host", environment: EnvironmentProduction, origin: "https://Growth.example"},
		{name: "production default port", environment: EnvironmentProduction, origin: "https://growth.example:443"},
	} {
		t.Run(test.name, func(t *testing.T) {
			variables := apiVariables(map[string]string{environmentVariable: string(test.environment)})
			if test.environment == EnvironmentStaging || test.environment == EnvironmentProduction {
				variables[mysqlTLSModeVariable] = string(MySQLTLSVerifyIdentity)
			}
			variables[identityPublicOriginVariable] = test.origin
			config, err := Load(mapLookup(variables))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an origin outside the environment Cookie boundary")
			}
			if !strings.Contains(err.Error(), identityPublicOriginVariable) || strings.Contains(err.Error(), test.origin) {
				t.Fatalf("Load() returned an unsafe origin error: %q", err)
			}
		})
	}
}

func TestLoadIdentityReadsExactSecretsFromFiles(t *testing.T) {
	testDirectory := t.TempDir()
	throttlePath := filepath.Join(testDirectory, "throttle-key")
	activePath := filepath.Join(testDirectory, "csrf-key")
	if err := os.WriteFile(throttlePath, []byte(testIdentityThrottleKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activePath, []byte(testIdentityCSRFActiveKey+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	variables := apiVariables(map[string]string{
		identityThrottleHMACKeyFileVariable: throttlePath,
		identityCSRFActiveKeyFileVariable:   activePath,
	})
	delete(variables, identityThrottleHMACKeyVariable)
	delete(variables, identityCSRFActiveKeyVariable)

	config, err := Load(mapLookup(variables))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Identity.ThrottleHMACKey != mustSecret32(t, testIdentityThrottleKey) ||
		config.Identity.CSRF.ActiveKey != mustSecret32(t, testIdentityCSRFActiveKey) {
		t.Fatal("Load() did not remove exactly one conventional secret-file line ending")
	}
}

func TestLoadIdentityPreservesRawSecretEndingInLineEndingBytes(t *testing.T) {
	for _, test := range []struct {
		name   string
		suffix []byte
	}{
		{name: "raw lf", suffix: []byte{'\n'}},
		{name: "raw crlf", suffix: []byte{'\r', '\n'}},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := bytes.Repeat([]byte{0xa5}, identitySecretBytes)
			defer clear(key)
			copy(key[len(key)-len(test.suffix):], test.suffix)
			path := filepath.Join(t.TempDir(), "raw-key")
			if err := os.WriteFile(path, key, 0o600); err != nil {
				t.Fatal(err)
			}
			variables := apiVariables(map[string]string{
				identityThrottleHMACKeyFileVariable: path,
			})
			delete(variables, identityThrottleHMACKeyVariable)

			config, err := Load(mapLookup(variables))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			loaded := config.Identity.ThrottleHMACKey.Bytes()
			defer clear(loaded)
			if !bytes.Equal(loaded, key) {
				t.Fatal("Load() stripped bytes from an exact raw 32-byte secret")
			}
		})
	}
}

func TestLoadIdentityRejectsInvalidSecretSourcesWithoutDisclosure(t *testing.T) {
	testDirectory := t.TempDir()
	missingPath := filepath.Join(testDirectory, "MISSING_PRIVATE_IDENTITY_KEY")
	oversizedSecret := "OVERSIZED_PRIVATE_IDENTITY_KEY_" + strings.Repeat("x", 64)
	oversizedPath := filepath.Join(testDirectory, "OVERSIZED_PRIVATE_IDENTITY_KEY")
	if err := os.WriteFile(oversizedPath, []byte(oversizedSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	regularPath := filepath.Join(testDirectory, "REGULAR_PRIVATE_IDENTITY_KEY")
	if err := os.WriteFile(regularPath, []byte(testIdentityThrottleKey), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(testDirectory, "SYMLINK_PRIVATE_IDENTITY_KEY")
	if err := os.Symlink(regularPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		overrides map[string]string
		sensitive []string
	}{
		{
			name: "direct too short",
			overrides: map[string]string{
				identityThrottleHMACKeyVariable: "PRIVATE_KEY_TOO_SHORT",
			},
			sensitive: []string{"PRIVATE_KEY_TOO_SHORT"},
		},
		{
			name: "direct all zero",
			overrides: map[string]string{
				identityThrottleHMACKeyVariable: string(make([]byte, identitySecretBytes)),
			},
			sensitive: []string{string(make([]byte, identitySecretBytes))},
		},
		{
			name: "mutually exclusive",
			overrides: map[string]string{
				identityThrottleHMACKeyVariable:     testIdentityThrottleKey,
				identityThrottleHMACKeyFileVariable: missingPath,
			},
			sensitive: []string{testIdentityThrottleKey, missingPath},
		},
		{
			name: "missing file",
			overrides: map[string]string{
				identityThrottleHMACKeyFileVariable: missingPath,
			},
			sensitive: []string{missingPath},
		},
		{
			name: "oversized file",
			overrides: map[string]string{
				identityThrottleHMACKeyFileVariable: oversizedPath,
			},
			sensitive: []string{oversizedPath, oversizedSecret},
		},
		{
			name: "directory is not a secret file",
			overrides: map[string]string{
				identityThrottleHMACKeyFileVariable: testDirectory,
			},
			sensitive: []string{testDirectory},
		},
		{
			name: "symlink is not an owned secret file",
			overrides: map[string]string{
				identityThrottleHMACKeyFileVariable: symlinkPath,
			},
			sensitive: []string{symlinkPath, regularPath},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			variables := apiVariables(test.overrides)
			if _, fileOverride := test.overrides[identityThrottleHMACKeyFileVariable]; fileOverride {
				if _, directOverride := test.overrides[identityThrottleHMACKeyVariable]; !directOverride {
					delete(variables, identityThrottleHMACKeyVariable)
				}
			}
			config, err := Load(mapLookup(variables))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an invalid Identity secret source")
			}
			if !strings.Contains(err.Error(), identityThrottleHMACKeyVariable) &&
				!strings.Contains(err.Error(), identityThrottleHMACKeyFileVariable) {
				t.Fatalf("Load() error omitted the stable source name: %q", err)
			}
			for _, sensitive := range test.sensitive {
				if sensitive != "" && strings.Contains(err.Error(), sensitive) {
					t.Fatalf("Load() error disclosed Identity secret material or path: %q", err)
				}
			}
		})
	}
}

func TestLoadIdentityPreviousCSRFKeyIsAnAtomicAbsoluteTuple(t *testing.T) {
	acceptUntil := time.Now().UTC().Truncate(time.Second).Add(time.Hour)
	variables := apiVariables(map[string]string{
		identityCSRFPreviousKeyIDVariable:       "retired_2026",
		identityCSRFPreviousKeyVariable:         testIdentityCSRFPreviousKey,
		identityCSRFPreviousAcceptUntilVariable: acceptUntil.Format(time.RFC3339),
	})
	config, err := Load(mapLookup(variables))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !config.Identity.CSRF.HasPrevious ||
		config.Identity.CSRF.Previous.KeyID != "retired_2026" ||
		config.Identity.CSRF.Previous.Key != mustSecret32(t, testIdentityCSRFPreviousKey) ||
		!config.Identity.CSRF.Previous.AcceptUntil.Equal(acceptUntil) {
		t.Fatal("Load() did not preserve the complete previous-key tuple")
	}

	for _, test := range []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{
			name:      "id only",
			overrides: map[string]string{identityCSRFPreviousKeyIDVariable: "retired"},
			want:      []string{identityCSRFPreviousKeyVariable, identityCSRFPreviousKeyFileVariable, identityCSRFPreviousAcceptUntilVariable},
		},
		{
			name:      "key only",
			overrides: map[string]string{identityCSRFPreviousKeyVariable: testIdentityCSRFPreviousKey},
			want:      []string{identityCSRFPreviousKeyIDVariable, identityCSRFPreviousAcceptUntilVariable},
		},
		{
			name:      "timestamp only",
			overrides: map[string]string{identityCSRFPreviousAcceptUntilVariable: acceptUntil.Format(time.RFC3339)},
			want:      []string{identityCSRFPreviousKeyIDVariable, identityCSRFPreviousKeyVariable, identityCSRFPreviousKeyFileVariable},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(test.overrides)))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted a partial previous CSRF tuple")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Errorf("Load() error = %q, want %s", err, variable)
				}
			}
		})
	}
}

func TestLoadIdentityRejectsInvalidCSRFKeyIDsAndPreviousWindows(t *testing.T) {
	for _, keyID := range []string{"", strings.Repeat("a", 17), "contains.dot", "密钥"} {
		t.Run("active_id_"+keyID, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(map[string]string{
				identityCSRFActiveKeyIDVariable: keyID,
			})))
			if err == nil || config != (Config{}) || !strings.Contains(err.Error(), identityCSRFActiveKeyIDVariable) {
				t.Fatalf("Load() did not reject invalid active key ID: %v", err)
			}
			if keyID != "" && strings.Contains(err.Error(), keyID) {
				t.Fatalf("Load() echoed invalid key ID: %q", err)
			}
		})
	}

	now := time.Now().UTC().Truncate(time.Second)
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "duration not absolute", value: "1h"},
		{name: "past", value: now.Add(-time.Second).Format(time.RFC3339)},
		{name: "over eight hours", value: now.Add(8*time.Hour + time.Minute).Format(time.RFC3339)},
		{name: "precision incompatible with keyring", value: now.Add(time.Hour + time.Nanosecond).Format(time.RFC3339Nano)},
	} {
		t.Run(test.name, func(t *testing.T) {
			variables := apiVariables(map[string]string{
				identityCSRFPreviousKeyIDVariable:       "retired",
				identityCSRFPreviousKeyVariable:         testIdentityCSRFPreviousKey,
				identityCSRFPreviousAcceptUntilVariable: test.value,
			})
			config, err := Load(mapLookup(variables))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an invalid previous-key window")
			}
			if !strings.Contains(err.Error(), identityCSRFPreviousAcceptUntilVariable) || strings.Contains(err.Error(), test.value) {
				t.Fatalf("Load() returned an unsafe previous-window error: %q", err)
			}
		})
	}
}

func TestLoadIdentityRejectsEveryKeyReuseCombination(t *testing.T) {
	acceptUntil := time.Now().UTC().Truncate(time.Second).Add(time.Hour).Format(time.RFC3339)
	for _, test := range []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{
			name: "throttle equals active",
			overrides: map[string]string{
				identityCSRFActiveKeyVariable: testIdentityThrottleKey,
			},
			want: []string{identityThrottleHMACKeyVariable, identityCSRFActiveKeyVariable},
		},
		{
			name: "previous equals active",
			overrides: map[string]string{
				identityCSRFPreviousKeyIDVariable:       "retired",
				identityCSRFPreviousKeyVariable:         testIdentityCSRFActiveKey,
				identityCSRFPreviousAcceptUntilVariable: acceptUntil,
			},
			want: []string{identityCSRFActiveKeyVariable, identityCSRFPreviousKeyVariable},
		},
		{
			name: "previous equals throttle",
			overrides: map[string]string{
				identityCSRFPreviousKeyIDVariable:       "retired",
				identityCSRFPreviousKeyVariable:         testIdentityThrottleKey,
				identityCSRFPreviousAcceptUntilVariable: acceptUntil,
			},
			want: []string{identityThrottleHMACKeyVariable, identityCSRFPreviousKeyVariable},
		},
		{
			name: "previous id equals active",
			overrides: map[string]string{
				identityCSRFPreviousKeyIDVariable:       "active",
				identityCSRFPreviousKeyVariable:         testIdentityCSRFPreviousKey,
				identityCSRFPreviousAcceptUntilVariable: acceptUntil,
			},
			want: []string{identityCSRFActiveKeyIDVariable, identityCSRFPreviousKeyIDVariable},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(test.overrides)))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted reused Identity key material or ID")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Errorf("Load() error = %q, want %s", err, variable)
				}
			}
			for _, secret := range []string{testIdentityThrottleKey, testIdentityCSRFActiveKey, testIdentityCSRFPreviousKey} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("Load() disclosed reused key material: %q", err)
				}
			}
		})
	}
}

func TestLoadIdentityBoundsArgonGateAndHandlerBudgets(t *testing.T) {
	for _, test := range []struct {
		name      string
		overrides map[string]string
	}{
		{name: "lower", overrides: map[string]string{
			identityArgon2MaxConcurrentVariable:  "1",
			identityArgon2AcquireTimeoutVariable: "1ms",
			identityHTTPHandlerTimeoutVariable:   "1.001s",
		}},
		{name: "upper", overrides: map[string]string{
			identityArgon2MaxConcurrentVariable:  "4",
			identityArgon2AcquireTimeoutVariable: "1s",
			identityHTTPHandlerTimeoutVariable:   "30s",
			httpWriteTimeoutVariable:             "31s",
		}},
	} {
		t.Run("accept_"+test.name, func(t *testing.T) {
			if _, err := Load(mapLookup(apiVariables(test.overrides))); err != nil {
				t.Fatalf("Load() boundary error = %v", err)
			}
		})
	}

	for _, test := range []struct {
		name      string
		overrides map[string]string
		want      []string
	}{
		{name: "concurrency zero", overrides: map[string]string{identityArgon2MaxConcurrentVariable: "0"}, want: []string{identityArgon2MaxConcurrentVariable}},
		{name: "concurrency five", overrides: map[string]string{identityArgon2MaxConcurrentVariable: "5"}, want: []string{identityArgon2MaxConcurrentVariable}},
		{name: "acquire too short", overrides: map[string]string{identityArgon2AcquireTimeoutVariable: "999us"}, want: []string{identityArgon2AcquireTimeoutVariable}},
		{name: "acquire too long", overrides: map[string]string{identityArgon2AcquireTimeoutVariable: "1.000001s"}, want: []string{identityArgon2AcquireTimeoutVariable}},
		{name: "handler zero", overrides: map[string]string{identityHTTPHandlerTimeoutVariable: "0s"}, want: []string{identityHTTPHandlerTimeoutVariable}},
		{name: "handler too long", overrides: map[string]string{identityHTTPHandlerTimeoutVariable: "30.000001s"}, want: []string{identityHTTPHandlerTimeoutVariable}},
		{name: "acquire equals handler", overrides: map[string]string{
			identityArgon2AcquireTimeoutVariable: "250ms",
			identityHTTPHandlerTimeoutVariable:   "250ms",
		}, want: []string{identityArgon2AcquireTimeoutVariable, identityHTTPHandlerTimeoutVariable}},
		{name: "insufficient execution budget", overrides: map[string]string{
			identityArgon2AcquireTimeoutVariable: "250ms",
			identityHTTPHandlerTimeoutVariable:   "1.249999s",
		}, want: []string{identityArgon2AcquireTimeoutVariable, identityHTTPHandlerTimeoutVariable}},
		{name: "handler exceeds transport write budget", overrides: map[string]string{
			identityHTTPHandlerTimeoutVariable: "3s",
			httpWriteTimeoutVariable:           "3.999999s",
		}, want: []string{identityHTTPHandlerTimeoutVariable, httpWriteTimeoutVariable}},
	} {
		t.Run("reject_"+test.name, func(t *testing.T) {
			config, err := Load(mapLookup(apiVariables(test.overrides)))
			if err == nil || config != (Config{}) {
				t.Fatal("Load() accepted an invalid Identity resource budget")
			}
			for _, variable := range test.want {
				if !strings.Contains(err.Error(), variable) {
					t.Errorf("Load() error = %q, want %s", err, variable)
				}
			}
		})
	}
}

func TestIdentitySecurityConfigRedactsEveryFormattingBoundary(t *testing.T) {
	previous := IdentityPreviousCSRFKeyConfig{
		KeyID:       "retired",
		Key:         mustSecret32(t, testIdentityCSRFPreviousKey),
		AcceptUntil: time.Now().UTC().Truncate(time.Second).Add(time.Hour),
	}
	identityConfig := IdentityConfig{
		PublicOrigin:    "https://growth.example",
		CookieMode:      IdentityCookieModeProduction,
		ThrottleHMACKey: mustSecret32(t, testIdentityThrottleKey),
		CSRF: IdentityCSRFConfig{
			ActiveKeyID: "active",
			ActiveKey:   mustSecret32(t, testIdentityCSRFActiveKey),
			Previous:    previous,
			HasPrevious: true,
		},
	}
	values := []any{
		identityConfig,
		identityConfig.CSRF,
		previous,
		identityConfig.ThrottleHMACKey,
		Config{Identity: identityConfig},
	}
	for _, value := range values {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		logger.Info("config", slog.Any("value", value))
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		rendered := strings.Join([]string{
			fmt.Sprint(value),
			fmt.Sprintf("%+v", value),
			fmt.Sprintf("%#v", value),
			fmt.Sprintf("%d", value),
			fmt.Sprintf("%x", value),
			fmt.Sprintf("%s", value),
			string(encoded),
			output.String(),
		}, "\n")
		for _, secret := range []string{testIdentityThrottleKey, testIdentityCSRFActiveKey, testIdentityCSRFPreviousKey} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("formatting boundary disclosed Identity key material: %s", rendered)
			}
		}
		if !strings.Contains(strings.ToLower(rendered), "redacted") {
			t.Fatalf("formatting boundary omitted redaction marker: %s", rendered)
		}
	}

	owned := identityConfig.ThrottleHMACKey.Bytes()
	owned[0] ^= 0xff
	if identityConfig.ThrottleHMACKey != mustSecret32(t, testIdentityThrottleKey) {
		t.Fatal("Secret32.Bytes() exposed mutable configuration storage")
	}
	clear(owned)

	copyOfConfig := identityConfig
	copyOfConfig.CSRF.Previous.KeyID = "changed"
	copyOfConfig.CSRF.Previous.Key[0] ^= 0xff
	copyOfConfig.CSRF.Previous.AcceptUntil = copyOfConfig.CSRF.Previous.AcceptUntil.Add(time.Hour)
	if identityConfig.CSRF.Previous != previous {
		t.Fatal("copying IdentityConfig shared mutable previous-key state")
	}
}

func TestLoadMigrationIgnoresIdentitySecurityVariables(t *testing.T) {
	config, err := LoadMigration(mapLookup(map[string]string{
		migrationPasswordVariable:               "migration secret",
		identityPublicOriginVariable:            "not an origin",
		identityThrottleHMACKeyVariable:         "private invalid throttle material",
		identityThrottleHMACKeyFileVariable:     "/DO_NOT_READ_THROTTLE_KEY",
		identityCSRFActiveKeyIDVariable:         "invalid.id",
		identityCSRFActiveKeyVariable:           "private invalid active material",
		identityCSRFActiveKeyFileVariable:       "/DO_NOT_READ_ACTIVE_KEY",
		identityCSRFPreviousKeyIDVariable:       "invalid.previous",
		identityCSRFPreviousKeyVariable:         "private invalid previous material",
		identityCSRFPreviousKeyFileVariable:     "/DO_NOT_READ_PREVIOUS_KEY",
		identityCSRFPreviousAcceptUntilVariable: "not-rfc3339",
		identityArgon2MaxConcurrentVariable:     "999",
		identityArgon2AcquireTimeoutVariable:    "not-a-duration",
		identityHTTPHandlerTimeoutVariable:      "not-a-duration",
	}))
	if err != nil {
		t.Fatalf("LoadMigration() error = %v, want API-only Identity variables ignored", err)
	}
	if config.MySQL.Password != "migration secret" {
		t.Fatal("LoadMigration() lost its own secret while ignoring Identity variables")
	}
}

const testIdentityCSRFPreviousKey = "0123456789abcdef0123456789ABCDEF"
