package sessioncookie

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/platform/weborigin"
)

const (
	DevelopmentCookieName = "growthos_dev_session"
	ProductionCookieName  = "__Host-growthos_session"
	SessionTokenBytes     = 32
	SessionAbsoluteLimit  = 8 * time.Hour
	CookiePath            = "/"
)

var (
	ErrInvalidConfiguration = errors.New("identity session cookie configuration is invalid")
	ErrCookieMissing        = errors.New("identity session cookie is missing")
	ErrCookieInvalid        = errors.New("identity session cookie is invalid")
)

type Mode string

const (
	ModeDevelopment Mode = "development"
	ModeProduction  Mode = "production"
)

// Policy is immutable and contains no bearer material.
type Policy struct {
	mode         Mode
	name         string
	secure       bool
	publicOrigin string
}

// NewDevelopment allows an insecure cookie only for an exact HTTP loopback
// origin. This constructor is not valid for staging or production composition.
func NewDevelopment(publicOrigin string) (*Policy, error) {
	if !validPublicOrigin(publicOrigin, "http", true) {
		return nil, ErrInvalidConfiguration
	}
	return &Policy{
		mode:         ModeDevelopment,
		name:         DevelopmentCookieName,
		secure:       false,
		publicOrigin: publicOrigin,
	}, nil
}

// NewProduction requires an exact HTTPS origin and fixes the __Host- name.
// Staging and production both use this constructor.
func NewProduction(publicOrigin string) (*Policy, error) {
	if !validPublicOrigin(publicOrigin, "https", false) {
		return nil, ErrInvalidConfiguration
	}
	return &Policy{
		mode:         ModeProduction,
		name:         ProductionCookieName,
		secure:       true,
		publicOrigin: publicOrigin,
	}, nil
}

func (policy *Policy) Validate() error {
	if policy == nil {
		return ErrInvalidConfiguration
	}
	switch policy.mode {
	case ModeDevelopment:
		if policy.name != DevelopmentCookieName || policy.secure ||
			!validPublicOrigin(policy.publicOrigin, "http", true) {
			return ErrInvalidConfiguration
		}
	case ModeProduction:
		if policy.name != ProductionCookieName || !policy.secure ||
			!validPublicOrigin(policy.publicOrigin, "https", false) {
			return ErrInvalidConfiguration
		}
	default:
		return ErrInvalidConfiguration
	}
	return nil
}

func (policy *Policy) Name() string {
	if policy == nil {
		return ""
	}
	return policy.name
}

func (policy *Policy) PublicOrigin() string {
	if policy == nil {
		return ""
	}
	return policy.publicOrigin
}

func (policy *Policy) Secure() bool { return policy != nil && policy.secure }

func (*Policy) String() string   { return "sessioncookie.Policy{[REDACTED]}" }
func (*Policy) GoString() string { return "sessioncookie.Policy{[REDACTED]}" }
func (*Policy) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
func (*Policy) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// Build creates the only session Set-Cookie shape. The caller retains
// ownership of rawToken and should clear it after this call.
func (policy *Policy) Build(
	rawToken []byte,
	now time.Time,
	absoluteExpiresAt time.Time,
) (*http.Cookie, error) {
	if policy.Validate() != nil || !canonicalNonZero(now) ||
		!canonicalNonZero(absoluteExpiresAt) || len(rawToken) != SessionTokenBytes ||
		allZero(rawToken) || !now.Before(absoluteExpiresAt) {
		return nil, ErrCookieInvalid
	}
	lifetime := absoluteExpiresAt.Sub(now)
	if lifetime > SessionAbsoluteLimit || lifetime < time.Second {
		return nil, ErrCookieInvalid
	}
	maxAge := int(lifetime / time.Second)
	if maxAge <= 0 {
		return nil, ErrCookieInvalid
	}
	return &http.Cookie{
		Name:     policy.name,
		Value:    base64.RawURLEncoding.EncodeToString(rawToken),
		Path:     CookiePath,
		Domain:   "",
		Expires:  absoluteExpiresAt,
		MaxAge:   maxAge,
		Secure:   policy.secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

// Clear returns the same scope/security tuple with an expired value.
func (policy *Policy) Clear() (*http.Cookie, error) {
	if policy.Validate() != nil {
		return nil, ErrInvalidConfiguration
	}
	return &http.Cookie{
		Name:     policy.name,
		Value:    "",
		Path:     CookiePath,
		Domain:   "",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		Secure:   policy.secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

// Read accepts exactly one configured cookie and rejects the alternate
// environment name, duplicates, noncanonical Base64URL, and wrong lengths.
func (policy *Policy) Read(request *http.Request) ([]byte, error) {
	rawToken, present, err := policy.ReadOptional(request)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, ErrCookieMissing
	}
	return rawToken, nil
}

// ReadOptional distinguishes an absent session credential from a malformed
// one. This lets login requests keep unrelated browser cookies without
// weakening duplicate, alternate-mode, or malformed session-cookie rejection.
func (policy *Policy) ReadOptional(request *http.Request) ([]byte, bool, error) {
	if policy.Validate() != nil || request == nil {
		return nil, false, ErrCookieInvalid
	}
	if !hasRelatedCookieHeader(request, policy.name, alternateCookieName(policy.name)) {
		return nil, false, nil
	}
	var selected *http.Cookie
	for _, cookie := range request.Cookies() {
		if cookie.Name == alternateCookieName(policy.name) {
			return nil, true, ErrCookieInvalid
		}
		if cookie.Name != policy.name {
			continue
		}
		if selected != nil {
			return nil, true, ErrCookieInvalid
		}
		selected = cookie
	}
	if selected == nil {
		// The raw header named this credential, but net/http could not restore
		// it as one valid cookie-pair. Treat that as malformed, not absent.
		return nil, true, ErrCookieInvalid
	}
	if len(selected.Value) != base64.RawURLEncoding.EncodedLen(SessionTokenBytes) {
		return nil, true, ErrCookieInvalid
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(selected.Value)
	if err != nil || len(rawToken) != SessionTokenBytes || allZero(rawToken) ||
		base64.RawURLEncoding.EncodeToString(rawToken) != selected.Value {
		clear(rawToken)
		return nil, true, ErrCookieInvalid
	}
	return rawToken, true, nil
}

func hasRelatedCookieHeader(request *http.Request, names ...string) bool {
	for _, line := range request.Header.Values("Cookie") {
		for _, part := range strings.Split(line, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			name = strings.TrimSpace(name)
			for _, expected := range names {
				if name == expected {
					return true
				}
			}
		}
	}
	return false
}

func alternateCookieName(configured string) string {
	if configured == DevelopmentCookieName {
		return ProductionCookieName
	}
	return DevelopmentCookieName
}

func validPublicOrigin(value, scheme string, loopbackOnly bool) bool {
	origin, err := weborigin.ParseExact(value)
	return err == nil && origin.Scheme() == scheme && (!loopbackOnly || origin.IsLoopback())
}

func canonicalNonZero(value time.Time) bool {
	return !value.IsZero() && value == value.UTC().Truncate(time.Microsecond)
}

func allZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
