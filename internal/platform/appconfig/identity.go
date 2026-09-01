package appconfig

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/platform/weborigin"
)

const (
	identityPublicOriginVariable            = "GROWTHOS_IDENTITY_PUBLIC_ORIGIN"
	identityThrottleHMACKeyVariable         = "GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY"
	identityThrottleHMACKeyFileVariable     = "GROWTHOS_IDENTITY_THROTTLE_HMAC_KEY_FILE"
	identityCSRFActiveKeyIDVariable         = "GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_ID"
	identityCSRFActiveKeyVariable           = "GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY"
	identityCSRFActiveKeyFileVariable       = "GROWTHOS_IDENTITY_CSRF_ACTIVE_KEY_FILE"
	identityCSRFPreviousKeyIDVariable       = "GROWTHOS_IDENTITY_CSRF_PREVIOUS_KEY_ID"
	identityCSRFPreviousKeyVariable         = "GROWTHOS_IDENTITY_CSRF_PREVIOUS_KEY"
	identityCSRFPreviousKeyFileVariable     = "GROWTHOS_IDENTITY_CSRF_PREVIOUS_KEY_FILE"
	identityCSRFPreviousAcceptUntilVariable = "GROWTHOS_IDENTITY_CSRF_PREVIOUS_ACCEPT_UNTIL"
	identityArgon2MaxConcurrentVariable     = "GROWTHOS_IDENTITY_ARGON2_MAX_CONCURRENT"
	identityArgon2AcquireTimeoutVariable    = "GROWTHOS_IDENTITY_ARGON2_ACQUIRE_TIMEOUT"
	identityHTTPHandlerTimeoutVariable      = "GROWTHOS_IDENTITY_HTTP_HANDLER_TIMEOUT"

	identitySecretBytes                   = 32
	maximumIdentitySecretFileBytes        = identitySecretBytes + 2
	minimumIdentityArgon2MaxConcurrent    = 1
	maximumIdentityArgon2MaxConcurrent    = 4
	defaultIdentityArgon2MaxConcurrent    = 2
	minimumIdentityArgon2AcquireTimeout   = time.Millisecond
	maximumIdentityArgon2AcquireTimeout   = time.Second
	defaultIdentityArgon2AcquireTimeout   = 250 * time.Millisecond
	maximumIdentityHTTPHandlerTimeout     = 30 * time.Second
	defaultIdentityHTTPHandlerTimeout     = 3 * time.Second
	minimumIdentityExecutionBudget        = time.Second
	identityHTTPResponseBudget            = time.Second
	maximumIdentityPreviousCSRFVerifyTime = 8 * time.Hour
	maximumIdentityCSRFKeyIDBytes         = 16
	redactedIdentityConfig                = "identity configuration (redacted)"
)

// IdentityCookieMode is derived exclusively from Environment. It is not an
// independently configurable switch: development/test use the loopback HTTP
// policy, while staging/production use the Secure __Host- policy.
type IdentityCookieMode string

const (
	IdentityCookieModeDevelopment IdentityCookieMode = "development"
	IdentityCookieModeProduction  IdentityCookieMode = "production"
)

// Secret32 owns one exact, nonzero 256-bit secret. The value remains
// comparable for whole-Config zero checks while all ordinary formatting and
// JSON/logging boundaries are redacted.
type Secret32 [identitySecretBytes]byte

// Bytes returns an owned copy for an adapter constructor. Callers should clear
// the returned slice after the constructor has copied it.
func (secret Secret32) Bytes() []byte {
	value := make([]byte, len(secret))
	copy(value, secret[:])
	return value
}

func (Secret32) String() string   { return "[REDACTED]" }
func (Secret32) GoString() string { return "[REDACTED]" }
func (Secret32) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED]")
}
func (Secret32) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
func (Secret32) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// IdentityConfig controls the security boundary of the Identity HTTP surface.
// Secrets have no defaults and Load fails closed until every active key is
// supplied from exactly one direct or file-backed source.
type IdentityConfig struct {
	PublicOrigin    string
	CookieMode      IdentityCookieMode
	ThrottleHMACKey Secret32
	CSRF            IdentityCSRFConfig
	PasswordHash    IdentityPasswordHashConfig
	HandlerTimeout  time.Duration
}

type IdentityPasswordHashConfig struct {
	MaxConcurrent  int
	AcquireTimeout time.Duration
}

type IdentityCSRFConfig struct {
	ActiveKeyID string
	ActiveKey   Secret32
	Previous    IdentityPreviousCSRFKeyConfig
	HasPrevious bool
}

type IdentityPreviousCSRFKeyConfig struct {
	KeyID       string
	Key         Secret32
	AcceptUntil time.Time
}

func (IdentityConfig) String() string   { return redactedIdentityConfig }
func (IdentityConfig) GoString() string { return redactedIdentityConfig }
func (IdentityConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityConfig)
}
func (IdentityConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityConfig)
}
func (IdentityConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityConfig)
}

func (IdentityCSRFConfig) String() string   { return redactedIdentityConfig }
func (IdentityCSRFConfig) GoString() string { return redactedIdentityConfig }
func (IdentityCSRFConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityConfig)
}
func (IdentityCSRFConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityConfig)
}
func (IdentityCSRFConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityConfig)
}

func (IdentityPreviousCSRFKeyConfig) String() string   { return redactedIdentityConfig }
func (IdentityPreviousCSRFKeyConfig) GoString() string { return redactedIdentityConfig }
func (IdentityPreviousCSRFKeyConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityConfig)
}
func (IdentityPreviousCSRFKeyConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityConfig)
}
func (IdentityPreviousCSRFKeyConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityConfig)
}

func defaultIdentityConfig() IdentityConfig {
	return IdentityConfig{
		CookieMode: IdentityCookieModeDevelopment,
		PasswordHash: IdentityPasswordHashConfig{
			MaxConcurrent:  defaultIdentityArgon2MaxConcurrent,
			AcquireTimeout: defaultIdentityArgon2AcquireTimeout,
		},
		HandlerTimeout: defaultIdentityHTTPHandlerTimeout,
	}
}

func loadIdentity(
	lookup LookupFunc,
	environment Environment,
	environmentValid bool,
	httpWriteTimeout time.Duration,
	httpWriteTimeoutValid bool,
	configuredAt time.Time,
	destination *IdentityConfig,
	problems *[]error,
) {
	loadIdentityPublicOrigin(lookup, environment, environmentValid, destination, problems)

	throttleValid := loadRequiredIdentitySecret(
		lookup,
		identityThrottleHMACKeyVariable,
		identityThrottleHMACKeyFileVariable,
		&destination.ThrottleHMACKey,
		problems,
	)
	activeIDValid := loadRequiredIdentityKeyID(
		lookup,
		identityCSRFActiveKeyIDVariable,
		&destination.CSRF.ActiveKeyID,
		problems,
	)
	activeKeyValid := loadRequiredIdentitySecret(
		lookup,
		identityCSRFActiveKeyVariable,
		identityCSRFActiveKeyFileVariable,
		&destination.CSRF.ActiveKey,
		problems,
	)

	if throttleValid && activeKeyValid && identitySecretsEqual(destination.ThrottleHMACKey, destination.CSRF.ActiveKey) {
		*problems = append(*problems, fmt.Errorf(
			"%s/%s and %s/%s must use distinct key material",
			identityThrottleHMACKeyVariable,
			identityThrottleHMACKeyFileVariable,
			identityCSRFActiveKeyVariable,
			identityCSRFActiveKeyFileVariable,
		))
	}

	if identityPreviousCSRFRequested(lookup) {
		previous := IdentityPreviousCSRFKeyConfig{}
		previousIDValid := loadRequiredIdentityKeyID(
			lookup,
			identityCSRFPreviousKeyIDVariable,
			&previous.KeyID,
			problems,
		)
		previousKeyValid := loadRequiredIdentitySecret(
			lookup,
			identityCSRFPreviousKeyVariable,
			identityCSRFPreviousKeyFileVariable,
			&previous.Key,
			problems,
		)
		previousUntilValid := loadIdentityPreviousAcceptUntil(
			lookup,
			configuredAt,
			&previous.AcceptUntil,
			problems,
		)
		if activeIDValid && previousIDValid && destination.CSRF.ActiveKeyID == previous.KeyID {
			*problems = append(*problems, fmt.Errorf(
				"%s and %s must be different",
				identityCSRFActiveKeyIDVariable,
				identityCSRFPreviousKeyIDVariable,
			))
		}
		if activeKeyValid && previousKeyValid && identitySecretsEqual(destination.CSRF.ActiveKey, previous.Key) {
			*problems = append(*problems, fmt.Errorf(
				"%s/%s and %s/%s must use distinct key material",
				identityCSRFActiveKeyVariable,
				identityCSRFActiveKeyFileVariable,
				identityCSRFPreviousKeyVariable,
				identityCSRFPreviousKeyFileVariable,
			))
		}
		if throttleValid && previousKeyValid && identitySecretsEqual(destination.ThrottleHMACKey, previous.Key) {
			*problems = append(*problems, fmt.Errorf(
				"%s/%s and %s/%s must use distinct key material",
				identityThrottleHMACKeyVariable,
				identityThrottleHMACKeyFileVariable,
				identityCSRFPreviousKeyVariable,
				identityCSRFPreviousKeyFileVariable,
			))
		}
		if previousIDValid && previousKeyValid && previousUntilValid {
			destination.CSRF.Previous = previous
			destination.CSRF.HasPrevious = true
		}
	}

	loadInteger(
		lookup,
		identityArgon2MaxConcurrentVariable,
		minimumIdentityArgon2MaxConcurrent,
		maximumIdentityArgon2MaxConcurrent,
		&destination.PasswordHash.MaxConcurrent,
		problems,
	)
	acquireValid := loadDurationRange(
		lookup,
		identityArgon2AcquireTimeoutVariable,
		minimumIdentityArgon2AcquireTimeout,
		maximumIdentityArgon2AcquireTimeout,
		&destination.PasswordHash.AcquireTimeout,
		problems,
	)
	handlerValid := loadDuration(
		lookup,
		identityHTTPHandlerTimeoutVariable,
		maximumIdentityHTTPHandlerTimeout,
		&destination.HandlerTimeout,
		problems,
	)
	if acquireValid && handlerValid &&
		destination.PasswordHash.AcquireTimeout+minimumIdentityExecutionBudget > destination.HandlerTimeout {
		*problems = append(*problems, fmt.Errorf(
			"%s plus a %s execution budget must be no greater than %s",
			identityArgon2AcquireTimeoutVariable,
			minimumIdentityExecutionBudget,
			identityHTTPHandlerTimeoutVariable,
		))
	}
	if handlerValid && httpWriteTimeoutValid &&
		(httpWriteTimeout <= identityHTTPResponseBudget ||
			destination.HandlerTimeout > httpWriteTimeout-identityHTTPResponseBudget) {
		*problems = append(*problems, fmt.Errorf(
			"%s plus a %s response budget must be no greater than %s",
			identityHTTPHandlerTimeoutVariable,
			identityHTTPResponseBudget,
			httpWriteTimeoutVariable,
		))
	}
}

func loadIdentityPublicOrigin(
	lookup LookupFunc,
	environment Environment,
	environmentValid bool,
	destination *IdentityConfig,
	problems *[]error,
) {
	value, present := lookup(identityPublicOriginVariable)
	if !present {
		*problems = append(*problems, fmt.Errorf("%s is required", identityPublicOriginVariable))
		return
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", identityPublicOriginVariable))
		return
	}
	if !environmentValid {
		return
	}

	scheme := "http"
	loopbackOnly := true
	switch environment {
	case EnvironmentDevelopment, EnvironmentTest:
		destination.CookieMode = IdentityCookieModeDevelopment
	case EnvironmentStaging, EnvironmentProduction:
		destination.CookieMode = IdentityCookieModeProduction
		scheme = "https"
		loopbackOnly = false
	default:
		return
	}
	if !validIdentityPublicOrigin(value, scheme, loopbackOnly) {
		constraint := "an exact HTTP loopback origin without credentials, path, query, or fragment"
		if !loopbackOnly {
			constraint = "an exact HTTPS origin without credentials, path, query, or fragment"
		}
		*problems = append(*problems, fmt.Errorf("%s must be %s", identityPublicOriginVariable, constraint))
		return
	}
	destination.PublicOrigin = value
}

func loadRequiredIdentityKeyID(
	lookup LookupFunc,
	variable string,
	destination *string,
	problems *[]error,
) bool {
	value, present := lookup(variable)
	if !present {
		*problems = append(*problems, fmt.Errorf("%s is required", variable))
		return false
	}
	if !validIdentityCSRFKeyID(value) {
		*problems = append(*problems, fmt.Errorf("%s must match [A-Za-z0-9_-]{1,16}", variable))
		return false
	}
	*destination = value
	return true
}

func loadRequiredIdentitySecret(
	lookup LookupFunc,
	variable string,
	fileVariable string,
	destination *Secret32,
	problems *[]error,
) bool {
	value, valuePresent := lookup(variable)
	filePath, filePresent := lookup(fileVariable)
	if valuePresent && filePresent {
		*problems = append(*problems, fmt.Errorf("%s and %s are mutually exclusive", variable, fileVariable))
		return false
	}
	if !valuePresent && !filePresent {
		*problems = append(*problems, fmt.Errorf("exactly one of %s or %s is required", variable, fileVariable))
		return false
	}

	if valuePresent {
		if len(value) != identitySecretBytes || identityBytesAllZero([]byte(value)) {
			*problems = append(*problems, fmt.Errorf("%s must contain exactly %d non-zero secret bytes", variable, identitySecretBytes))
			return false
		}
		copy(destination[:], value)
		return true
	}

	if strings.TrimSpace(filePath) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", fileVariable))
		return false
	}
	secret, ok := readIdentitySecretFile(filePath)
	if !ok {
		*problems = append(*problems, fmt.Errorf("%s must be a readable file containing exactly %d non-zero secret bytes", fileVariable, identitySecretBytes))
		return false
	}
	*destination = secret
	return true
}

func readIdentitySecretFile(path string) (Secret32, bool) {
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() {
		return Secret32{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return Secret32{}, false
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		_ = file.Close()
		return Secret32{}, false
	}
	contents, readErr := io.ReadAll(io.LimitReader(file, maximumIdentitySecretFileBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		clear(contents)
		return Secret32{}, false
	}
	defer clear(contents)
	if len(contents) > maximumIdentitySecretFileBytes {
		return Secret32{}, false
	}
	if len(contents) >= 2 && contents[len(contents)-2] == '\r' && contents[len(contents)-1] == '\n' {
		contents = contents[:len(contents)-2]
	} else if len(contents) >= 1 && contents[len(contents)-1] == '\n' {
		contents = contents[:len(contents)-1]
	}
	if len(contents) != identitySecretBytes || identityBytesAllZero(contents) {
		return Secret32{}, false
	}
	secret := Secret32{}
	copy(secret[:], contents)
	return secret, true
}

func identityPreviousCSRFRequested(lookup LookupFunc) bool {
	for _, variable := range []string{
		identityCSRFPreviousKeyIDVariable,
		identityCSRFPreviousKeyVariable,
		identityCSRFPreviousKeyFileVariable,
		identityCSRFPreviousAcceptUntilVariable,
	} {
		if _, present := lookup(variable); present {
			return true
		}
	}
	return false
}

func loadIdentityPreviousAcceptUntil(
	lookup LookupFunc,
	configuredAt time.Time,
	destination *time.Time,
	problems *[]error,
) bool {
	value, present := lookup(identityCSRFPreviousAcceptUntilVariable)
	if !present {
		*problems = append(*problems, fmt.Errorf("%s is required when a previous CSRF key is configured", identityCSRFPreviousAcceptUntilVariable))
		return false
	}
	instant, err := time.Parse(time.RFC3339Nano, value)
	canonicalConfiguredAt := configuredAt.UTC().Truncate(time.Microsecond)
	canonicalInstant := instant.UTC().Truncate(time.Microsecond)
	if err != nil || instant.Nanosecond()%1_000 != 0 || configuredAt != canonicalConfiguredAt || configuredAt.IsZero() ||
		!configuredAt.Before(canonicalInstant) || canonicalInstant.After(configuredAt.Add(maximumIdentityPreviousCSRFVerifyTime)) {
		*problems = append(*problems, fmt.Errorf(
			"%s must be an absolute RFC3339 timestamp in the next %s",
			identityCSRFPreviousAcceptUntilVariable,
			maximumIdentityPreviousCSRFVerifyTime,
		))
		return false
	}
	*destination = canonicalInstant
	return true
}

func validIdentityCSRFKeyID(value string) bool {
	if len(value) == 0 || len(value) > maximumIdentityCSRFKeyIDBytes {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validIdentityPublicOrigin(value string, scheme string, loopbackOnly bool) bool {
	origin, err := weborigin.ParseExact(value)
	return err == nil && origin.Scheme() == scheme && (!loopbackOnly || origin.IsLoopback())
}

func identitySecretsEqual(left Secret32, right Secret32) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func identityBytesAllZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
