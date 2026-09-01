package sessioncookie

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var cookieTestNow = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

func TestDevelopmentAndProductionCookieShapes(t *testing.T) {
	development, err := NewDevelopment("http://127.0.0.1:8088")
	if err != nil {
		t.Fatal(err)
	}
	production, err := NewProduction("https://growth.example.com")
	if err != nil {
		t.Fatal(err)
	}
	rawToken := bytes.Repeat([]byte{0x41}, SessionTokenBytes)
	for _, test := range []struct {
		name       string
		policy     *Policy
		cookieName string
		secure     bool
	}{
		{name: "development", policy: development, cookieName: DevelopmentCookieName},
		{name: "production", policy: production, cookieName: ProductionCookieName, secure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cookie, err := test.policy.Build(rawToken, cookieTestNow, cookieTestNow.Add(8*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if cookie.Name != test.cookieName || cookie.Path != CookiePath || cookie.Domain != "" ||
				cookie.Secure != test.secure || !cookie.HttpOnly ||
				cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 8*60*60 ||
				cookie.Expires != cookieTestNow.Add(8*time.Hour) {
				t.Fatalf("Build() cookie = %#v", cookie)
			}
			rendered := cookie.String()
			if !strings.Contains(rendered, "Path=/") ||
				!strings.Contains(rendered, "HttpOnly") ||
				!strings.Contains(rendered, "SameSite=Strict") ||
				(test.secure && !strings.Contains(rendered, "Secure")) ||
				(!test.secure && strings.Contains(rendered, "Secure")) ||
				strings.Contains(rendered, "Domain=") {
				t.Fatalf("unsafe Set-Cookie = %q", rendered)
			}
		})
	}
}

func TestBuildBoundsAndCallerOwnership(t *testing.T) {
	policy, _ := NewDevelopment("http://[::1]:8088")
	rawToken := bytes.Repeat([]byte{0x52}, SessionTokenBytes)
	callerCopy := bytes.Clone(rawToken)
	if _, err := policy.Build(rawToken, cookieTestNow, cookieTestNow.Add(8*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rawToken, callerCopy) {
		t.Fatal("Build() modified caller token")
	}
	for _, test := range []struct {
		name    string
		token   []byte
		now     time.Time
		expires time.Time
	}{
		{name: "short token", token: rawToken[:31], now: cookieTestNow, expires: cookieTestNow.Add(time.Hour)},
		{name: "zero token", token: make([]byte, SessionTokenBytes), now: cookieTestNow, expires: cookieTestNow.Add(time.Hour)},
		{name: "expired", token: rawToken, now: cookieTestNow, expires: cookieTestNow},
		{name: "over absolute", token: rawToken, now: cookieTestNow, expires: cookieTestNow.Add(8*time.Hour + time.Second)},
		{name: "subsecond", token: rawToken, now: cookieTestNow, expires: cookieTestNow.Add(time.Second - time.Microsecond)},
		{name: "noncanonical now", token: rawToken, now: cookieTestNow.Add(time.Nanosecond), expires: cookieTestNow.Add(time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := policy.Build(test.token, test.now, test.expires); !errors.Is(err, ErrCookieInvalid) {
				t.Fatalf("Build() error = %v", err)
			}
		})
	}
}

func TestReadIsExactAndReturnsOwnedToken(t *testing.T) {
	policy, _ := NewDevelopment("http://127.0.0.1:8088")
	rawToken := bytes.Repeat([]byte{0x63}, SessionTokenBytes)
	encoded := base64.RawURLEncoding.EncodeToString(rawToken)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8088/api/v1/session", nil)
	request.Header.Add("Cookie", DevelopmentCookieName+"="+encoded)
	got, err := policy.Read(request)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !bytes.Equal(got, rawToken) {
		t.Fatal("Read() token mismatch")
	}
	got[0] = 0
	if rawToken[0] == 0 {
		t.Fatal("Read() did not return caller-owned bytes")
	}
}

func TestReadRejectsMissingDuplicateAlternateAndMalformed(t *testing.T) {
	policy, _ := NewDevelopment("http://127.0.0.1:8088")
	valid := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x74}, SessionTokenBytes))
	cases := []struct {
		name       string
		cookieLine string
		want       error
	}{
		{name: "missing", cookieLine: "other=value", want: ErrCookieMissing},
		{name: "duplicate", cookieLine: DevelopmentCookieName + "=" + valid + "; " + DevelopmentCookieName + "=" + valid, want: ErrCookieInvalid},
		{name: "alternate", cookieLine: ProductionCookieName + "=" + valid, want: ErrCookieInvalid},
		{name: "both modes", cookieLine: DevelopmentCookieName + "=" + valid + "; " + ProductionCookieName + "=" + valid, want: ErrCookieInvalid},
		{name: "padded", cookieLine: DevelopmentCookieName + "=" + valid + "=", want: ErrCookieInvalid},
		{name: "short", cookieLine: DevelopmentCookieName + "=YWJj", want: ErrCookieInvalid},
		{name: "zero", cookieLine: DevelopmentCookieName + "=" + base64.RawURLEncoding.EncodeToString(make([]byte, SessionTokenBytes)), want: ErrCookieInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8088/", nil)
			request.Header.Set("Cookie", test.cookieLine)
			if _, err := policy.Read(request); !errors.Is(err, test.want) {
				t.Fatalf("Read() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadOptionalDistinguishesUnrelatedCookiesFromBrokenSessionCookies(t *testing.T) {
	policy := mustDevelopmentPolicy(t)
	valid := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x75}, SessionTokenBytes))

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/v1/session", nil)
	request.Header.Add("Cookie", "analytics=opaque; locale=zh-CN")
	rawToken, present, err := policy.ReadOptional(request)
	if err != nil || present || rawToken != nil {
		t.Fatalf("unrelated cookies = token:%x present:%v error:%v", rawToken, present, err)
	}

	for _, cookieLine := range []string{
		DevelopmentCookieName,
		DevelopmentCookieName + "=short",
		DevelopmentCookieName + "=" + valid + "; " + DevelopmentCookieName + "=" + valid,
		ProductionCookieName + "=" + valid,
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/v1/session", nil)
		request.Header.Add("Cookie", "analytics=opaque; "+cookieLine)
		if rawToken, present, err := policy.ReadOptional(request); !present || !errors.Is(err, ErrCookieInvalid) || rawToken != nil {
			t.Fatalf("broken session cookie %q = token:%x present:%v error:%v", cookieLine, rawToken, present, err)
		}
	}

	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8088/api/v1/session", nil)
	request.Header.Add("Cookie", "analytics=opaque; "+DevelopmentCookieName+"="+valid)
	rawToken, present, err = policy.ReadOptional(request)
	if err != nil || !present || !bytes.Equal(rawToken, bytes.Repeat([]byte{0x75}, SessionTokenBytes)) {
		t.Fatalf("valid optional session = token:%x present:%v error:%v", rawToken, present, err)
	}
}

func TestClearMatchesScopeAndExpiresImmediately(t *testing.T) {
	for _, policy := range []*Policy{
		mustDevelopmentPolicy(t), mustProductionPolicy(t),
	} {
		cookie, err := policy.Clear()
		if err != nil {
			t.Fatal(err)
		}
		if cookie.Name != policy.Name() || cookie.Value != "" || cookie.Path != CookiePath ||
			cookie.Domain != "" || cookie.Secure != policy.Secure() || !cookie.HttpOnly ||
			cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge >= 0 ||
			!cookie.Expires.Before(time.Now()) {
			t.Fatalf("Clear() cookie = %#v", cookie)
		}
	}
}

func TestPolicyConfigurationFailsClosedAndFormattingIsRedacted(t *testing.T) {
	for _, origin := range []string{
		"", " http://127.0.0.1:8088", "https://127.0.0.1:8088",
		"http://example.com", "http://127.0.0.1:8088/", "http://127.0.0.1:8088?x=1",
		"http://user@127.0.0.1:8088", "http://127.0.0.1:not-a-port",
		"http://127.0.0.1:80", "http://[0:0:0:0:0:0:0:1]:8088",
	} {
		if _, err := NewDevelopment(origin); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("NewDevelopment(%q) error = %v", origin, err)
		}
	}
	for _, origin := range []string{
		"", "http://growth.example.com", "https://growth.example.com/",
		"https://user@growth.example.com", "https://growth.example.com#fragment",
		"https://Growth.example.com", "https://growth.example.com:443",
	} {
		if _, err := NewProduction(origin); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("NewProduction(%q) error = %v", origin, err)
		}
	}
	policy := mustProductionPolicy(t)
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		fmt.Sprint(policy), fmt.Sprintf("%#v", policy), policy.LogValue().String(),
		slog.AnyValue(policy).Resolve().String(), string(encoded),
	} {
		if !strings.Contains(rendered, "REDACTED") {
			t.Fatalf("unsafe policy rendering %q", rendered)
		}
	}
	var nilPolicy *Policy
	if nilPolicy.Validate() == nil || nilPolicy.Name() != "" || nilPolicy.PublicOrigin() != "" || nilPolicy.Secure() {
		t.Fatal("nil Policy did not fail closed")
	}
}

func mustDevelopmentPolicy(t *testing.T) *Policy {
	t.Helper()
	policy, err := NewDevelopment("http://127.0.0.1:8088")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func mustProductionPolicy(t *testing.T) *Policy {
	t.Helper()
	policy, err := NewProduction("https://growth.example.com")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
