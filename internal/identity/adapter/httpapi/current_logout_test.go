package httpapi

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/sessioncookie"
	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
)

func TestCurrentSessionSuccessIsMinimalAndCSRFBound(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x71}, identityapp.SessionTokenBytes)
	dependencies := validStubDependencies(t)
	dependencies.Resolve = &stubResolve{result: mustVerified(t, rawToken)}
	csrf := &stubCSRF{issueToken: "v1.active.current-token"}
	dependencies.CSRF = csrf
	router := mustRouter(t, dependencies, Options{})
	request := httptest.NewRequest(http.MethodGet, SessionPath, nil)
	request.Header.Set("Cookie", sessioncookie.DevelopmentCookieName+"="+
		base64.RawURLEncoding.EncodeToString(rawToken))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if len(csrf.issued) != 1 {
		t.Fatalf("csrf issue calls = %d", len(csrf.issued))
	}
	if len(recorder.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("current session unexpectedly replaced the Cookie")
	}
	assertNoAuthorizationVocabulary(t, recorder.Body.String())
	assertSafetyHeaders(t, recorder)
}

func TestCurrentSessionRejectsInvalidFramingAndCredentialSourcesBeforeResolve(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x72}, identityapp.SessionTokenBytes)
	cases := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "query", mutate: func(request *http.Request) { request.URL.RawQuery = "x=1" }},
		{name: "body", mutate: func(request *http.Request) { request.Body = ioNopCloser("junk"); request.ContentLength = 4 }},
		{name: "unframed body junk", mutate: func(request *http.Request) { request.Body = ioNopCloser("junk"); request.ContentLength = 0 }},
		{name: "transfer encoding", mutate: func(request *http.Request) { request.TransferEncoding = []string{"chunked"} }},
		{name: "authorization", mutate: func(request *http.Request) { request.Header.Set("Authorization", "Bearer attacker") }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			resolver := &stubResolve{result: mustVerified(t, rawToken)}
			dependencies.Resolve = resolver
			router := mustRouter(t, dependencies, Options{})
			request := httptest.NewRequest(http.MethodGet, SessionPath, nil)
			request.Header.Set("Cookie", sessioncookie.DevelopmentCookieName+"="+
				base64.RawURLEncoding.EncodeToString(rawToken))
			testCase.mutate(request)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", recorder.Code)
			}
			if resolver.calls != 0 {
				t.Fatalf("resolve calls = %d", resolver.calls)
			}
		})
	}
}

func TestCurrentSessionCookieAndDependencyFailures(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x73}, identityapp.SessionTokenBytes)
	validCookie := sessioncookie.DevelopmentCookieName + "=" +
		base64.RawURLEncoding.EncodeToString(rawToken)
	cases := []struct {
		name          string
		cookieHeaders []string
		resolveErr    error
		wantStatus    int
		wantCode      string
		wantClear     bool
	}{
		{name: "missing", wantStatus: 401, wantCode: "unauthenticated", wantClear: true},
		{name: "random", cookieHeaders: []string{sessioncookie.DevelopmentCookieName + "=random"}, wantStatus: 401, wantCode: "unauthenticated", wantClear: true},
		{name: "duplicate", cookieHeaders: []string{validCookie, validCookie}, wantStatus: 401, wantCode: "unauthenticated", wantClear: true},
		{name: "inactive", cookieHeaders: []string{validCookie}, resolveErr: identityapp.ErrUnauthenticated, wantStatus: 401, wantCode: "unauthenticated", wantClear: true},
		{name: "mysql", cookieHeaders: []string{validCookie}, resolveErr: identityapp.ErrAuthenticationUnavailable, wantStatus: 503, wantCode: "authentication_unavailable"},
		{name: "unknown", cookieHeaders: []string{validCookie}, resolveErr: errors.New("private mysql detail"), wantStatus: 500, wantCode: "internal_error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			dependencies.Resolve = &stubResolve{err: testCase.resolveErr}
			router := mustRouter(t, dependencies, Options{})
			request := httptest.NewRequest(http.MethodGet, SessionPath, nil)
			for _, value := range testCase.cookieHeaders {
				request.Header.Add("Cookie", value)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
			}
			if code := decodeErrorBody(t, recorder).Code; code != testCase.wantCode {
				t.Fatalf("code = %q, want %q", code, testCase.wantCode)
			}
			cleared := len(recorder.Header().Values("Set-Cookie")) == 1
			if cleared != testCase.wantClear {
				t.Fatalf("clear Cookie = %v, want %v; headers=%v", cleared, testCase.wantClear, recorder.Header())
			}
			assertSafetyHeaders(t, recorder)
		})
	}
}

func TestCurrentSessionPostResolveDependencyAndContractFailures(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x76}, identityapp.SessionTokenBytes)
	validCookie := sessioncookie.DevelopmentCookieName + "=" +
		base64.RawURLEncoding.EncodeToString(rawToken)
	cases := []struct {
		name   string
		mutate func(*Dependencies)
	}{
		{name: "invalid resolve output", mutate: func(dependencies *Dependencies) {
			dependencies.Resolve = &stubResolve{}
		}},
		{name: "csrf issue error", mutate: func(dependencies *Dependencies) {
			dependencies.Resolve = &stubResolve{result: mustVerified(t, rawToken)}
			dependencies.CSRF = &stubCSRF{issueErr: errors.New("entropy")}
		}},
		{name: "empty csrf contract", mutate: func(dependencies *Dependencies) {
			dependencies.Resolve = &stubResolve{result: mustVerified(t, rawToken)}
			dependencies.CSRF = &stubCSRF{}
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			testCase.mutate(&dependencies)
			router := mustRouter(t, dependencies, Options{})
			request := httptest.NewRequest(http.MethodGet, SessionPath, nil)
			request.Header.Set("Cookie", validCookie)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable && recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
			}
			if len(recorder.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("post-resolve failure changed the valid Cookie")
			}
		})
	}
}

func TestRequiredCookieClearFailureDoesNotClaimTheRequestedOutcome(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x77}, identityapp.SessionTokenBytes)
	cases := []struct {
		name   string
		method string
	}{
		{name: "current unauthenticated", method: http.MethodGet},
		{name: "confirmed logout", method: http.MethodDelete},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			dependencies.Cookies = &stubCookiePolicy{
				origin:   testOrigin,
				read:     rawToken,
				clearErr: errors.New("clear failed"),
			}
			if testCase.method == http.MethodGet {
				dependencies.Resolve = &stubResolve{err: identityapp.ErrUnauthenticated}
			} else {
				dependencies.Resolve = &stubResolve{result: mustVerified(t, rawToken)}
				dependencies.CSRF = &stubCSRF{}
			}
			router := mustRouter(t, dependencies, Options{})
			request := httptest.NewRequest(testCase.method, SessionPath, nil)
			request.Header.Set("Cookie", "stub_session=opaque")
			if testCase.method == http.MethodDelete {
				request.Header.Set("Origin", testOrigin)
				request.Header.Set(CSRFHeader, "valid")
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
			}
			if code := decodeErrorBody(t, recorder).Code; code != "authentication_unavailable" {
				t.Fatalf("code = %q", code)
			}
			if len(recorder.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("failed clear emitted a Cookie")
			}
		})
	}
}

func TestLogoutOrchestrationRejectsBeforeRevokeAndClearsOnlyRequiredFailures(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x74}, identityapp.SessionTokenBytes)
	validCookie := sessioncookie.DevelopmentCookieName + "=" +
		base64.RawURLEncoding.EncodeToString(rawToken)
	cases := []struct {
		name        string
		origin      string
		csrfHeaders []string
		csrfErr     error
		revokeErr   error
		wantStatus  int
		wantResolve int
		wantVerify  int
		wantRevoke  int
		wantClear   bool
		wantCode    string
	}{
		{name: "wrong origin", origin: "http://127.0.0.1:8081", csrfHeaders: []string{"valid"}, wantStatus: 403, wantCode: "request_origin_rejected"},
		{name: "missing csrf", origin: testOrigin, wantStatus: 403, wantCode: "request_origin_rejected"},
		{name: "duplicate csrf", origin: testOrigin, csrfHeaders: []string{"valid", "valid"}, wantStatus: 403, wantCode: "request_origin_rejected"},
		{name: "wrong csrf", origin: testOrigin, csrfHeaders: []string{"wrong"}, csrfErr: errors.New("invalid"), wantStatus: 403, wantVerify: 1, wantCode: "request_origin_rejected"},
		{name: "confirmed", origin: testOrigin, csrfHeaders: []string{"valid"}, wantStatus: 204, wantVerify: 1, wantRevoke: 1, wantClear: true},
		{name: "revoke inactive", origin: testOrigin, csrfHeaders: []string{"valid"}, revokeErr: identityapp.ErrUnauthenticated, wantStatus: 401, wantVerify: 1, wantRevoke: 1, wantClear: true, wantCode: "unauthenticated"},
		{name: "commit unknown", origin: testOrigin, csrfHeaders: []string{"valid"}, revokeErr: identityapp.ErrRevocationIndeterminate, wantStatus: 503, wantVerify: 1, wantRevoke: 1, wantClear: true, wantCode: "session_revocation_indeterminate"},
		{name: "dependency unavailable", origin: testOrigin, csrfHeaders: []string{"valid"}, revokeErr: identityapp.ErrAuthenticationUnavailable, wantStatus: 503, wantVerify: 1, wantRevoke: 1, wantCode: "authentication_unavailable"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			resolver := &stubResolve{result: mustVerified(t, rawToken)}
			revoker := &stubRevoke{err: testCase.revokeErr}
			csrf := &stubCSRF{verifyErr: testCase.csrfErr}
			dependencies.Resolve = resolver
			dependencies.Revoke = revoker
			dependencies.CSRF = csrf
			router := mustRouter(t, dependencies, Options{})
			request := httptest.NewRequest(http.MethodDelete, SessionPath, nil)
			request.Header.Set("Cookie", validCookie)
			if testCase.origin != "" {
				request.Header.Set("Origin", testCase.origin)
			}
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			for _, value := range testCase.csrfHeaders {
				request.Header.Add(CSRFHeader, value)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			if resolver.calls != testCase.wantResolve || len(csrf.verified) != testCase.wantVerify || revoker.calls != testCase.wantRevoke {
				t.Fatalf("calls resolve/verify/revoke = %d/%d/%d, want %d/%d/%d", resolver.calls, len(csrf.verified), revoker.calls, testCase.wantResolve, testCase.wantVerify, testCase.wantRevoke)
			}
			cleared := len(recorder.Header().Values("Set-Cookie")) == 1
			if cleared != testCase.wantClear {
				t.Fatalf("clear Cookie = %v, want %v", cleared, testCase.wantClear)
			}
			if testCase.wantCode != "" {
				if code := decodeErrorBody(t, recorder).Code; code != testCase.wantCode {
					t.Fatalf("code = %q, want %q", code, testCase.wantCode)
				}
			} else if recorder.Body.Len() != 0 {
				t.Fatalf("204 body = %q", recorder.Body.String())
			}
			assertSafetyHeaders(t, recorder)
		})
	}
}

func TestLogoutRejectsBodyQueryAndDuplicateOriginBeforeResolve(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x75}, identityapp.SessionTokenBytes)
	cases := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
	}{
		{name: "body", mutate: func(request *http.Request) { request.Body = ioNopCloser("junk"); request.ContentLength = 4 }, wantStatus: 400},
		{name: "unframed body junk", mutate: func(request *http.Request) { request.Body = ioNopCloser("junk"); request.ContentLength = 0 }, wantStatus: 400},
		{name: "query", mutate: func(request *http.Request) { request.URL.RawQuery = "session_id=attacker" }, wantStatus: 400},
		{name: "duplicate origin", mutate: func(request *http.Request) { request.Header.Add("Origin", testOrigin) }, wantStatus: 403},
		{name: "same-site", mutate: func(request *http.Request) { request.Header.Set("Sec-Fetch-Site", "same-site") }, wantStatus: 403},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			resolver := &stubResolve{result: mustVerified(t, rawToken)}
			dependencies.Resolve = resolver
			router := mustRouter(t, dependencies, Options{})
			request := httptest.NewRequest(http.MethodDelete, SessionPath, nil)
			request.Header.Set("Cookie", sessioncookie.DevelopmentCookieName+"="+
				base64.RawURLEncoding.EncodeToString(rawToken))
			request.Header.Set("Origin", testOrigin)
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			request.Header.Set(CSRFHeader, "valid")
			testCase.mutate(request)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d", recorder.Code)
			}
			if resolver.calls != 0 {
				t.Fatalf("resolve calls = %d", resolver.calls)
			}
		})
	}
}

func TestCurrentAndLogoutDoNotWriteSuccessAfterHandlerDeadline(t *testing.T) {
	rawToken := bytes.Repeat([]byte{0x79}, identityapp.SessionTokenBytes)
	validCookie := sessioncookie.DevelopmentCookieName + "=" +
		base64.RawURLEncoding.EncodeToString(rawToken)

	t.Run("current", func(t *testing.T) {
		dependencies := validStubDependencies(t)
		dependencies.Resolve = &deadlineResolve{result: mustVerified(t, rawToken)}
		router := mustRouter(t, dependencies, Options{Timeout: time.Millisecond})
		request := httptest.NewRequest(http.MethodGet, SessionPath, nil)
		request.Header.Set("Cookie", validCookie)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("logout", func(t *testing.T) {
		dependencies := validStubDependencies(t)
		dependencies.CSRF = &stubCSRF{}
		dependencies.Revoke = &deadlineRevoke{}
		router := mustRouter(t, dependencies, Options{Timeout: time.Millisecond})
		request := httptest.NewRequest(http.MethodDelete, SessionPath, nil)
		request.Header.Set("Cookie", validCookie)
		request.Header.Set("Origin", testOrigin)
		request.Header.Set(CSRFHeader, "valid")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
		}
		if len(recorder.Header().Values("Set-Cookie")) != 1 {
			t.Fatal("confirmed revoke at the deadline did not clear the browser Cookie")
		}
	})
}

func ioNopCloser(value string) *readCloser {
	return &readCloser{Reader: strings.NewReader(value)}
}

type readCloser struct{ *strings.Reader }

func (closer *readCloser) Close() error { return nil }
