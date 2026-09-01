package main

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"
	"time"

	identitycsrf "github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/csrf"
	identityhttp "github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/httpapi"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/sessioncookie"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type typedNilEntropyReader struct{}

func (*typedNilEntropyReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestComposeIdentityHTTPBuildsValidatedDevelopmentAndProductionRuntimes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	for _, mode := range []appconfig.IdentityCookieMode{
		appconfig.IdentityCookieModeDevelopment,
		appconfig.IdentityCookieModeProduction,
	} {
		t.Run(string(mode), func(t *testing.T) {
			database := newIdentityCompositionDatabase(t)
			config := testIdentityConfig(mode)
			runtime, err := composeIdentityHTTP(database, config)
			if err != nil || runtime == nil || runtime.Validate() != nil {
				t.Fatalf("runtime=%#v err=%v", runtime, err)
			}
			if runtime.cookies.PublicOrigin() != config.PublicOrigin {
				t.Fatal("Cookie and configured origins diverged")
			}
			wantName := sessioncookie.DevelopmentCookieName
			wantSecure := false
			if mode == appconfig.IdentityCookieModeProduction {
				wantName = sessioncookie.ProductionCookieName
				wantSecure = true
			}
			if runtime.cookies.Name() != wantName || runtime.cookies.Secure() != wantSecure {
				t.Fatalf("cookie name/secure = %q/%v", runtime.cookies.Name(), runtime.cookies.Secure())
			}

			router := gin.New()
			if err := runtime.RegisterRoutes(router, nil, config.HandlerTimeout); err != nil {
				t.Fatal(err)
			}
			methods := map[string]int{}
			for _, route := range router.Routes() {
				if route.Path == identityhttp.SessionPath {
					methods[route.Method]++
				}
			}
			for _, method := range []string{"POST", "GET", "DELETE"} {
				if methods[method] != 1 {
					t.Fatalf("session route %s count = %d", method, methods[method])
				}
			}
			if len(methods) != 3 {
				t.Fatalf("session methods = %#v", methods)
			}
		})
	}
}

func TestComposeIdentityHTTPCopiesKeysAtEveryConstructorBoundary(t *testing.T) {
	database := newIdentityCompositionDatabase(t)
	config := testIdentityConfig(appconfig.IdentityCookieModeDevelopment)
	activeMaterial := config.CSRF.ActiveKey.Bytes()
	defer clear(activeMaterial)
	runtime, err := composeIdentityHTTP(database, config)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginName("alice")
	if err != nil {
		t.Fatal(err)
	}
	before, err := runtime.digester.DigestLogin(login)
	if err != nil {
		t.Fatal(err)
	}
	config.ThrottleHMACKey[0] ^= 0xff
	config.CSRF.ActiveKey[0] ^= 0xff
	after, err := runtime.digester.DigestLogin(login)
	if err != nil || before != after {
		t.Fatal("composed throttle digester retained mutable Config key storage")
	}
	digestBytes := bytes.Repeat([]byte{0x71}, 32)
	digest, err := identity.NewTokenDigest(digestBytes)
	clear(digestBytes)
	if err != nil {
		t.Fatal(err)
	}
	token, err := runtime.csrf.Issue(digest)
	if err != nil {
		t.Fatal(err)
	}
	active, err := identitycsrf.NewKey("active", activeMaterial)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := identitycsrf.NewKeyring(
		active,
		nil,
		rand.Reader,
		canonicalRuntimeInstant(time.Now()),
	)
	if err != nil || verifier.Verify(token, digest, canonicalRuntimeInstant(time.Now())) != nil {
		t.Fatal("composed CSRF keyring retained mutable Config key storage")
	}
}

func TestComposeIdentityHTTPAcceptsOneBoundedPreviousCSRFKey(t *testing.T) {
	database := newIdentityCompositionDatabase(t)
	configuredAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	config := testIdentityConfig(appconfig.IdentityCookieModeDevelopment)
	config.CSRF.HasPrevious = true
	config.CSRF.Previous = appconfig.IdentityPreviousCSRFKeyConfig{
		KeyID:       "retired",
		Key:         repeatedSecret32(0x33),
		AcceptUntil: configuredAt.Add(time.Hour),
	}
	runtime, err := composeIdentityHTTPWith(database, config, identityCompositionDependencies{
		entropy: rand.Reader,
		now:     func() time.Time { return configuredAt },
	})
	if err != nil {
		t.Fatal(err)
	}
	previousMaterial := config.CSRF.Previous.Key.Bytes()
	defer clear(previousMaterial)
	previousActive, err := identitycsrf.NewKey(config.CSRF.Previous.KeyID, previousMaterial)
	if err != nil {
		t.Fatal(err)
	}
	oldIssuer, err := identitycsrf.NewKeyring(previousActive, nil, rand.Reader, configuredAt)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes := bytes.Repeat([]byte{0x62}, 32)
	digest, err := identity.NewTokenDigest(digestBytes)
	clear(digestBytes)
	if err != nil {
		t.Fatal(err)
	}
	token, err := oldIssuer.Issue(digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.csrf.Verify(token, digest, configuredAt); err != nil {
		t.Fatalf("previous token rejected inside window: %v", err)
	}
	if err := runtime.csrf.Verify(token, digest, config.CSRF.Previous.AcceptUntil); !errors.Is(err, identitycsrf.ErrTokenInvalid) {
		t.Fatalf("previous token accepted at closed expiry boundary: %v", err)
	}
}

func TestComposeIdentityHTTPFailsClosedForForgedConfiguration(t *testing.T) {
	validDatabase := newIdentityCompositionDatabase(t)
	valid := testIdentityConfig(appconfig.IdentityCookieModeDevelopment)
	var typedNilEntropy *typedNilEntropyReader
	for _, test := range []struct {
		name         string
		database     *sqlx.DB
		config       appconfig.IdentityConfig
		dependencies identityCompositionDependencies
	}{
		{
			name: "nil database", config: valid,
			dependencies: identityCompositionDependencies{entropy: rand.Reader, now: time.Now},
		},
		{
			name: "typed nil entropy", database: validDatabase, config: valid,
			dependencies: identityCompositionDependencies{entropy: typedNilEntropy, now: time.Now},
		},
		{
			name: "zero clock", database: validDatabase, config: valid,
			dependencies: identityCompositionDependencies{entropy: rand.Reader, now: func() time.Time { return time.Time{} }},
		},
		{
			name: "reused active key", database: validDatabase,
			config: func() appconfig.IdentityConfig {
				forged := valid
				forged.CSRF.ActiveKey = forged.ThrottleHMACKey
				return forged
			}(),
			dependencies: identityCompositionDependencies{entropy: rand.Reader, now: time.Now},
		},
		{
			name: "hidden previous tuple", database: validDatabase,
			config: func() appconfig.IdentityConfig {
				forged := valid
				forged.CSRF.Previous.KeyID = "hidden"
				return forged
			}(),
			dependencies: identityCompositionDependencies{entropy: rand.Reader, now: time.Now},
		},
		{
			name: "invalid cookie mode", database: validDatabase,
			config: func() appconfig.IdentityConfig {
				forged := valid
				forged.CookieMode = "forged"
				return forged
			}(),
			dependencies: identityCompositionDependencies{entropy: rand.Reader, now: time.Now},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, err := composeIdentityHTTPWith(test.database, test.config, test.dependencies)
			if runtime != nil || !errors.Is(err, errIdentityHTTPRuntime) ||
				err.Error() != errIdentityHTTPRuntime.Error() {
				t.Fatalf("runtime=%#v err=%v", runtime, err)
			}
		})
	}
}

func TestNilIdentitySessionRuntimeRejectsTypedNil(t *testing.T) {
	var runtime *identityHTTPRuntime
	if !nilIdentitySessionRuntime(runtime) || !nilIdentitySessionRuntime(nil) {
		t.Fatal("typed-nil Identity runtime was accepted")
	}
}

func newIdentityCompositionDatabase(t *testing.T) *sqlx.DB {
	t.Helper()
	database, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return sqlx.NewDb(database, "mysql")
}

func testIdentityConfig(mode appconfig.IdentityCookieMode) appconfig.IdentityConfig {
	origin := "http://127.0.0.1:8080"
	if mode == appconfig.IdentityCookieModeProduction {
		origin = "https://growth.example"
	}
	return appconfig.IdentityConfig{
		PublicOrigin:    origin,
		CookieMode:      mode,
		ThrottleHMACKey: repeatedSecret32(0x11),
		CSRF: appconfig.IdentityCSRFConfig{
			ActiveKeyID: "active",
			ActiveKey:   repeatedSecret32(0x22),
		},
		PasswordHash: appconfig.IdentityPasswordHashConfig{
			MaxConcurrent:  2,
			AcquireTimeout: 250 * time.Millisecond,
		},
		HandlerTimeout: 3 * time.Second,
	}
}

func repeatedSecret32(value byte) appconfig.Secret32 {
	secret := appconfig.Secret32{}
	for index := range secret {
		secret[index] = value
	}
	return secret
}
