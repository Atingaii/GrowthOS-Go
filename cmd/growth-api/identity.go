package main

import (
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"time"

	identitycsrf "github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/csrf"
	identityhttp "github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/httpapi"
	identitymysql "github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlrepo"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/passwordhash"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/requestguard"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/sessioncookie"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/throttledigest"
	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

var errIdentityHTTPRuntime = errors.New("identity HTTP runtime is not configured")

// identitySessionRuntime is the narrow route-registration capability retained
// by the process after composition. It exposes neither the Identity pool nor
// any credential/session persistence port to unrelated modules.
type identitySessionRuntime interface {
	Validate() error
	RegisterRoutes(*gin.Engine, *slog.Logger, time.Duration) error
}

type identityHTTPRuntime struct {
	login    *identityapp.LoginService
	resolve  *identityapp.ResolveService
	revoke   *identityapp.RevokeCurrentService
	cookies  *sessioncookie.Policy
	csrf     *identitycsrf.Keyring
	guard    *requestguard.Guard
	digester *throttledigest.Digester
	clock    identityapp.ClockFunc
}

type identityCompositionDependencies struct {
	entropy io.Reader
	now     func() time.Time
}

func composeIdentityHTTP(
	database *sqlx.DB,
	config appconfig.IdentityConfig,
) (*identityHTTPRuntime, error) {
	return composeIdentityHTTPWith(database, config, identityCompositionDependencies{
		entropy: rand.Reader,
		now:     time.Now,
	})
}

func composeIdentityHTTPWith(
	database *sqlx.DB,
	config appconfig.IdentityConfig,
	dependencies identityCompositionDependencies,
) (*identityHTTPRuntime, error) {
	if database == nil || database.DB == nil || nilIdentityEntropy(dependencies.entropy) ||
		dependencies.now == nil || !validIdentityKeySeparation(config) {
		return nil, errIdentityHTTPRuntime
	}
	clock := identityapp.ClockFunc(func() time.Time {
		return canonicalRuntimeInstant(dependencies.now())
	})
	configuredAt := clock.Now()
	if configuredAt.IsZero() {
		return nil, errIdentityHTTPRuntime
	}

	repository, err := identitymysql.New(database)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	hasher, err := passwordhash.New(passwordhash.Config{
		MaxConcurrent:  config.PasswordHash.MaxConcurrent,
		AcquireTimeout: config.PasswordHash.AcquireTimeout,
		Entropy:        dependencies.entropy,
	})
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	verifier, err := passwordhash.NewApplicationVerifier(hasher)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}

	throttleMaterial := config.ThrottleHMACKey.Bytes()
	digester, err := throttledigest.New(throttleMaterial)
	clear(throttleMaterial)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	activeMaterial := config.CSRF.ActiveKey.Bytes()
	activeKey, err := identitycsrf.NewKey(config.CSRF.ActiveKeyID, activeMaterial)
	clear(activeMaterial)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	var previousKey *identitycsrf.PreviousKey
	if config.CSRF.HasPrevious {
		previousMaterial := config.CSRF.Previous.Key.Bytes()
		key, keyErr := identitycsrf.NewKey(config.CSRF.Previous.KeyID, previousMaterial)
		clear(previousMaterial)
		if keyErr != nil {
			return nil, errIdentityHTTPRuntime
		}
		previous, previousErr := identitycsrf.NewPreviousKey(key, config.CSRF.Previous.AcceptUntil)
		if previousErr != nil {
			return nil, errIdentityHTTPRuntime
		}
		previousKey = &previous
	}
	csrfKeyring, err := identitycsrf.NewKeyring(
		activeKey,
		previousKey,
		dependencies.entropy,
		configuredAt,
	)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}

	var cookies *sessioncookie.Policy
	switch config.CookieMode {
	case appconfig.IdentityCookieModeDevelopment:
		cookies, err = sessioncookie.NewDevelopment(config.PublicOrigin)
	case appconfig.IdentityCookieModeProduction:
		cookies, err = sessioncookie.NewProduction(config.PublicOrigin)
	default:
		err = errIdentityHTTPRuntime
	}
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	guard, err := requestguard.New(config.PublicOrigin)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	login, err := identityapp.NewLoginService(identityapp.LoginDependencies{
		Clock:       clock,
		Credentials: repository,
		Passwords:   verifier,
		Admissions:  repository,
		Entropy:     dependencies.entropy,
		Issuer:      repository,
	})
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	resolve, err := identityapp.NewResolveService(clock, repository)
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	revoke, err := identityapp.NewRevokeCurrentService(identityapp.RevokeCurrentDependencies{
		Clock:   clock,
		Reader:  repository,
		Revoker: repository,
		Entropy: dependencies.entropy,
	})
	if err != nil {
		return nil, errIdentityHTTPRuntime
	}
	runtime := &identityHTTPRuntime{
		login:    login,
		resolve:  resolve,
		revoke:   revoke,
		cookies:  cookies,
		csrf:     csrfKeyring,
		guard:    guard,
		digester: digester,
		clock:    clock,
	}
	if runtime.Validate() != nil {
		return nil, errIdentityHTTPRuntime
	}
	return runtime, nil
}

func (runtime *identityHTTPRuntime) Validate() error {
	if runtime == nil || runtime.login == nil || runtime.login.Validate() != nil ||
		runtime.resolve == nil || runtime.resolve.Validate() != nil ||
		runtime.revoke == nil || runtime.revoke.Validate() != nil ||
		runtime.cookies == nil || runtime.cookies.Validate() != nil ||
		runtime.csrf == nil || runtime.guard == nil || runtime.guard.Validate() != nil ||
		runtime.digester == nil || runtime.clock == nil {
		return errIdentityHTTPRuntime
	}
	return nil
}

func (runtime *identityHTTPRuntime) RegisterRoutes(
	router *gin.Engine,
	logger *slog.Logger,
	timeout time.Duration,
) error {
	if runtime.Validate() != nil {
		return errIdentityHTTPRuntime
	}
	return identityhttp.RegisterRoutes(router, identityhttp.Dependencies{
		Login:    runtime.login,
		Resolve:  runtime.resolve,
		Revoke:   runtime.revoke,
		Cookies:  runtime.cookies,
		CSRF:     runtime.csrf,
		Guard:    runtime.guard,
		Digester: runtime.digester,
		Clock:    runtime.clock,
	}, identityhttp.Options{Logger: logger, Timeout: timeout})
}

func validIdentityKeySeparation(config appconfig.IdentityConfig) bool {
	zeroPrevious := appconfig.IdentityPreviousCSRFKeyConfig{}
	if config.ThrottleHMACKey == config.CSRF.ActiveKey ||
		(!config.CSRF.HasPrevious && config.CSRF.Previous != zeroPrevious) {
		return false
	}
	if config.CSRF.HasPrevious &&
		(config.CSRF.Previous.Key == config.CSRF.ActiveKey ||
			config.CSRF.Previous.Key == config.ThrottleHMACKey) {
		return false
	}
	return true
}

func nilIdentitySessionRuntime(runtime identitySessionRuntime) bool {
	if runtime == nil {
		return true
	}
	value := reflect.ValueOf(runtime)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func nilIdentityEntropy(entropy io.Reader) bool {
	if entropy == nil {
		return true
	}
	value := reflect.ValueOf(entropy)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func canonicalRuntimeInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.Round(0).UTC().Truncate(time.Microsecond)
}
