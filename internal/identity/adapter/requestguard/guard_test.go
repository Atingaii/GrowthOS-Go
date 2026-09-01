package requestguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

const testOrigin = "http://127.0.0.1:8088"

func TestValidateUnsafeRequiresExactOriginAndSameOriginFetchMetadata(t *testing.T) {
	guard := mustGuard(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		request := httptest.NewRequest(method, testOrigin+"/api/v1/session", nil)
		request.Header.Set(OriginHeader, testOrigin)
		request.Header.Set(FetchSiteHeader, FetchSiteSameOrigin)
		if err := guard.ValidateUnsafe(request); err != nil {
			t.Fatalf("ValidateUnsafe(%s) error = %v", method, err)
		}
		request.Header.Del(FetchSiteHeader)
		if err := guard.ValidateUnsafe(request); err != nil {
			t.Fatalf("ValidateUnsafe(%s without Fetch Metadata) error = %v", method, err)
		}
	}
}

func TestValidateUnsafeRejectsAmbiguousOrCrossSiteRequests(t *testing.T) {
	guard := mustGuard(t)
	cases := []struct {
		name  string
		build func(*http.Request)
	}{
		{name: "missing origin"},
		{name: "wrong origin", build: func(request *http.Request) { request.Header.Set(OriginHeader, "http://localhost:8088") }},
		{name: "null origin", build: func(request *http.Request) { request.Header.Set(OriginHeader, "null") }},
		{name: "duplicate origin", build: func(request *http.Request) {
			request.Header.Add(OriginHeader, testOrigin)
			request.Header.Add(OriginHeader, testOrigin)
		}},
		{name: "cross site", build: func(request *http.Request) {
			request.Header.Set(OriginHeader, testOrigin)
			request.Header.Set(FetchSiteHeader, "cross-site")
		}},
		{name: "same site sibling", build: func(request *http.Request) {
			request.Header.Set(OriginHeader, testOrigin)
			request.Header.Set(FetchSiteHeader, "same-site")
		}},
		{name: "duplicate fetch", build: func(request *http.Request) {
			request.Header.Set(OriginHeader, testOrigin)
			request.Header.Add(FetchSiteHeader, FetchSiteSameOrigin)
			request.Header.Add(FetchSiteHeader, FetchSiteSameOrigin)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testOrigin+"/api/v1/session", nil)
			if test.build != nil {
				test.build(request)
			}
			if err := guard.ValidateUnsafe(request); !errors.Is(err, ErrOriginRejected) {
				t.Fatalf("ValidateUnsafe() error = %v", err)
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, testOrigin+"/api/v1/session", nil)
	request.Header.Set(OriginHeader, testOrigin)
	if err := guard.ValidateUnsafe(request); !errors.Is(err, ErrOriginRejected) {
		t.Fatalf("ValidateUnsafe(safe method) error = %v", err)
	}
}

func TestTrustedSourceUsesOnlyConnectedSocket(t *testing.T) {
	guard := mustGuard(t)
	request := httptest.NewRequest(http.MethodPost, testOrigin+"/api/v1/session", nil)
	request.RemoteAddr = "[::ffff:192.0.2.10]:54321"
	request.Header.Set("Forwarded", "for=198.51.100.10")
	request.Header.Set("X-Forwarded-For", "203.0.113.20")
	request.Header.Set("X-Real-IP", "203.0.113.21")
	got, err := guard.TrustedSource(request)
	if err != nil {
		t.Fatal(err)
	}
	if got != netip.MustParseAddr("192.0.2.10") {
		t.Fatalf("TrustedSource() = %v", got)
	}
}

func TestTrustedSourceRejectsMalformedPeer(t *testing.T) {
	guard := mustGuard(t)
	for _, remoteAddress := range []string{
		"", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:08080",
		"127.0.0.1:not-a-port", "[fe80::1%en0]:8080", "host.example:8080",
	} {
		request := httptest.NewRequest(http.MethodPost, testOrigin+"/", nil)
		request.RemoteAddr = remoteAddress
		if _, err := guard.TrustedSource(request); !errors.Is(err, ErrSourceRejected) {
			t.Fatalf("TrustedSource(%q) error = %v", remoteAddress, err)
		}
	}
}

func TestGuardConfigurationAndFormattingFailClosed(t *testing.T) {
	for _, origin := range []string{
		"", " http://127.0.0.1:8088", "ftp://127.0.0.1:8088",
		"http://127.0.0.1:8088/", "http://user@127.0.0.1:8088",
		"http://127.0.0.1:08088", "http://127.0.0.1:not-a-port",
		"http://127.0.0.1:80", "https://Growth.example",
		"http://[0:0:0:0:0:0:0:1]:8088",
	} {
		if _, err := New(origin); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("New(%q) error = %v", origin, err)
		}
	}
	guard := mustGuard(t)
	encoded, err := json.Marshal(guard)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		fmt.Sprint(guard), fmt.Sprintf("%#v", guard), guard.LogValue().String(),
		slog.AnyValue(guard).Resolve().String(), string(encoded),
	} {
		if !strings.Contains(rendered, "REDACTED") {
			t.Fatalf("unsafe Guard rendering %q", rendered)
		}
	}
	var nilGuard *Guard
	if nilGuard.Validate() == nil || nilGuard.PublicOrigin() != "" {
		t.Fatal("nil Guard did not fail closed")
	}
}

func TestUnsafeMethodVocabularyIsClosed(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !UnsafeMethod(method) {
			t.Fatalf("UnsafeMethod(%s) = false", method)
		}
	}
	for _, method := range []string{"", http.MethodGet, http.MethodHead, http.MethodOptions, "post"} {
		if UnsafeMethod(method) {
			t.Fatalf("UnsafeMethod(%q) = true", method)
		}
	}
}

func mustGuard(t *testing.T) *Guard {
	t.Helper()
	guard, err := New(testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}
