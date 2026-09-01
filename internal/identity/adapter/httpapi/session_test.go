package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/sessioncookie"
	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesRejectsNilTypedNilAndInvalidDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := validStubDependencies(t)
	if err := RegisterRoutes(nil, valid, Options{}); !errors.Is(err, ErrRouterRequired) {
		t.Fatalf("nil router error = %v", err)
	}

	type dependencyCase struct {
		name string
		want error
		set  func(*Dependencies)
	}
	cases := []dependencyCase{
		{name: "login nil", want: ErrLoginRequired, set: func(deps *Dependencies) { deps.Login = nil }},
		{name: "login typed nil", want: ErrLoginRequired, set: func(deps *Dependencies) { var value *stubLogin; deps.Login = value }},
		{name: "login invalid", want: ErrLoginRequired, set: func(deps *Dependencies) { deps.Login = &stubLogin{validateErr: errors.New("bad")} }},
		{name: "resolve nil", want: ErrResolveRequired, set: func(deps *Dependencies) { deps.Resolve = nil }},
		{name: "resolve typed nil", want: ErrResolveRequired, set: func(deps *Dependencies) { var value *stubResolve; deps.Resolve = value }},
		{name: "revoke nil", want: ErrRevokeRequired, set: func(deps *Dependencies) { deps.Revoke = nil }},
		{name: "revoke typed nil", want: ErrRevokeRequired, set: func(deps *Dependencies) { var value *stubRevoke; deps.Revoke = value }},
		{name: "cookie nil", want: ErrCookiePolicyRequired, set: func(deps *Dependencies) { deps.Cookies = nil }},
		{name: "cookie typed nil", want: ErrCookiePolicyRequired, set: func(deps *Dependencies) { var value *stubCookiePolicy; deps.Cookies = value }},
		{name: "csrf nil", want: ErrCSRFRequired, set: func(deps *Dependencies) { deps.CSRF = nil }},
		{name: "csrf typed nil", want: ErrCSRFRequired, set: func(deps *Dependencies) { var value *stubCSRF; deps.CSRF = value }},
		{name: "guard nil", want: ErrRequestGuardRequired, set: func(deps *Dependencies) { deps.Guard = nil }},
		{name: "guard typed nil", want: ErrRequestGuardRequired, set: func(deps *Dependencies) { var value *stubGuard; deps.Guard = value }},
		{name: "digester nil", want: ErrDigesterRequired, set: func(deps *Dependencies) { deps.Digester = nil }},
		{name: "digester typed nil", want: ErrDigesterRequired, set: func(deps *Dependencies) { var value *stubDigester; deps.Digester = value }},
		{name: "clock nil", want: ErrClockRequired, set: func(deps *Dependencies) { deps.Clock = nil }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := valid
			testCase.set(&dependencies)
			if err := RegisterRoutes(gin.New(), dependencies, Options{}); !errors.Is(err, testCase.want) {
				t.Fatalf("RegisterRoutes() error = %v, want %v", err, testCase.want)
			}
		})
	}

	mismatch := valid
	mismatch.Guard = &stubGuard{origin: "http://127.0.0.1:8081"}
	if err := RegisterRoutes(gin.New(), mismatch, Options{}); !errors.Is(err, ErrOriginMismatch) {
		t.Fatalf("origin mismatch error = %v", err)
	}
	for _, timeout := range []time.Duration{-time.Second, MaximumHandlerTimeout + time.Microsecond} {
		if err := RegisterRoutes(gin.New(), valid, Options{Timeout: timeout}); !errors.Is(err, ErrTimeoutInvalid) {
			t.Fatalf("timeout %v error = %v", timeout, err)
		}
	}
}

func TestRegisterRoutesMountsOnlyExactSessionMethods(t *testing.T) {
	router := gin.New()
	if err := RegisterRoutes(router, validStubDependencies(t), Options{}); err != nil {
		t.Fatal(err)
	}
	routes := router.Routes()
	if len(routes) != 3 {
		t.Fatalf("route count = %d, want 3", len(routes))
	}
	want := map[string]bool{
		http.MethodPost + " " + SessionPath:   true,
		http.MethodGet + " " + SessionPath:    true,
		http.MethodDelete + " " + SessionPath: true,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if !want[key] {
			t.Errorf("unexpected route %s", key)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing routes: %v", want)
	}
}

func TestLoginStrictRequestVocabularyHasNoApplicationSideEffects(t *testing.T) {
	validBody := `{"login_name":"operator-1","password":"correct horse battery staple"}`
	oversized := `{"login_name":"operator-1","password":"` + strings.Repeat("x", MaximumLoginBodyBytes) + `"}`
	cases := []struct {
		name       string
		body       string
		mutate     func(*http.Request)
		wantStatus int
	}{
		{name: "missing content type", body: validBody, mutate: func(request *http.Request) { request.Header.Del("Content-Type") }, wantStatus: http.StatusUnsupportedMediaType},
		{name: "content type parameters", body: validBody, mutate: func(request *http.Request) { request.Header.Set("Content-Type", "application/json; charset=utf-8") }, wantStatus: http.StatusUnsupportedMediaType},
		{name: "duplicate content type", body: validBody, mutate: func(request *http.Request) { request.Header.Add("Content-Type", "application/json") }, wantStatus: http.StatusUnsupportedMediaType},
		{name: "query", body: validBody, mutate: func(request *http.Request) { request.URL.RawQuery = "login_name=operator-1" }, wantStatus: http.StatusBadRequest},
		{name: "authorization", body: validBody, mutate: func(request *http.Request) { request.Header.Set("Authorization", "Basic secret") }, wantStatus: http.StatusBadRequest},
		{name: "principal header", body: validBody, mutate: func(request *http.Request) { request.Header.Set("X-Principal-ID", "attacker") }, wantStatus: http.StatusBadRequest},
		{name: "duplicate session cookie", body: validBody, mutate: func(request *http.Request) {
			value := sessioncookie.DevelopmentCookieName + "=" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, identityapp.SessionTokenBytes))
			request.Header.Add("Cookie", value)
			request.Header.Add("Cookie", value)
		}, wantStatus: http.StatusBadRequest},
		{name: "alternate environment cookie", body: validBody, mutate: func(request *http.Request) {
			request.Header.Set("Cookie", sessioncookie.ProductionCookieName+"="+base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, identityapp.SessionTokenBytes)))
		}, wantStatus: http.StatusBadRequest},
		{name: "wrong origin", body: validBody, mutate: func(request *http.Request) { request.Header.Set("Origin", "http://127.0.0.1:8081") }, wantStatus: http.StatusForbidden},
		{name: "duplicate origin", body: validBody, mutate: func(request *http.Request) { request.Header.Add("Origin", testOrigin) }, wantStatus: http.StatusForbidden},
		{name: "same site fetch", body: validBody, mutate: func(request *http.Request) { request.Header.Set("Sec-Fetch-Site", "same-site") }, wantStatus: http.StatusForbidden},
		{name: "duplicate fetch", body: validBody, mutate: func(request *http.Request) { request.Header.Add("Sec-Fetch-Site", "same-origin") }, wantStatus: http.StatusForbidden},
		{name: "invalid source", body: validBody, mutate: func(request *http.Request) { request.RemoteAddr = "attacker" }, wantStatus: http.StatusBadRequest},
		{name: "empty body", body: "", wantStatus: http.StatusBadRequest},
		{name: "zero content length", body: validBody, mutate: func(request *http.Request) { request.ContentLength = 0 }, wantStatus: http.StatusBadRequest},
		{name: "unknown content length", body: validBody, mutate: func(request *http.Request) { request.ContentLength = -1 }, wantStatus: http.StatusBadRequest},
		{name: "chunked transfer", body: validBody, mutate: func(request *http.Request) { request.TransferEncoding = []string{"chunked"} }, wantStatus: http.StatusBadRequest},
		{name: "request trailer", body: validBody, mutate: func(request *http.Request) { request.Trailer = http.Header{"X-Trace": []string{"late"}} }, wantStatus: http.StatusBadRequest},
		{name: "body length mismatch", body: validBody, mutate: func(request *http.Request) { request.ContentLength-- }, wantStatus: http.StatusBadRequest},
		{name: "elapsed body budget", body: validBody, mutate: func(request *http.Request) {
			ctx, cancel := context.WithCancel(request.Context())
			cancel()
			*request = *request.WithContext(ctx)
		}, wantStatus: http.StatusServiceUnavailable},
		{name: "array", body: `[]`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"login_name":"operator-1","password":"secret","role":"admin"}`, wantStatus: http.StatusBadRequest},
		{name: "duplicate login", body: `{"login_name":"operator-1","login_name":"operator-2","password":"secret"}`, wantStatus: http.StatusBadRequest},
		{name: "duplicate password", body: `{"login_name":"operator-1","password":"one","password":"two"}`, wantStatus: http.StatusBadRequest},
		{name: "missing password", body: `{"login_name":"operator-1"}`, wantStatus: http.StatusBadRequest},
		{name: "wrong field type", body: `{"login_name":"operator-1","password":12}`, wantStatus: http.StatusBadRequest},
		{name: "trailing object", body: validBody + `{}`, wantStatus: http.StatusBadRequest},
		{name: "trailing junk", body: validBody + `junk`, wantStatus: http.StatusBadRequest},
		{name: "invalid utf8", body: string(append([]byte(`{"login_name":"operator-1","password":"`), 0xff, '"', '}')), wantStatus: http.StatusBadRequest},
		{name: "unpaired high surrogate", body: `{"login_name":"operator-1","password":"\ud800"}`, wantStatus: http.StatusBadRequest},
		{name: "unpaired low surrogate", body: `{"login_name":"operator-1","password":"\udc00"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid login grammar", body: `{"login_name":"Operator-1","password":"secret"}`, wantStatus: http.StatusBadRequest},
		{name: "empty password", body: `{"login_name":"operator-1","password":""}`, wantStatus: http.StatusBadRequest},
		{name: "too many password runes", body: `{"login_name":"operator-1","password":"` + strings.Repeat("界", 129) + `"}`, wantStatus: http.StatusBadRequest},
		{name: "oversized", body: oversized, wantStatus: http.StatusBadRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			login := dependencies.Login.(*stubLogin)
			router := mustRouter(t, dependencies, Options{})
			request := validLoginRequest(strings.NewReader(testCase.body))
			if testCase.mutate != nil {
				testCase.mutate(request)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			if login.calls != 0 {
				t.Fatalf("login calls = %d, want zero", login.calls)
			}
			assertSafetyHeaders(t, recorder)
			assertErrorEnvelope(t, recorder)
		})
	}
}

func TestLoginAllowsUnrelatedBrowserCookiesWithoutCreatingARevokeHint(t *testing.T) {
	issuer := &issuerPort{}
	dependencies := validStubDependencies(t)
	dependencies.Login = mustLoginService(t, issuer)
	dependencies.CSRF = &stubCSRF{issueToken: "v1.active.public-csrf"}
	router := mustRouter(t, dependencies, Options{})
	request := validLoginRequest(strings.NewReader(
		`{"login_name":"operator-1","password":"correct horse battery staple"}`,
	))
	request.Header.Set("Cookie", "analytics=opaque; locale=zh-CN")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if len(issuer.attempts) != 1 {
		t.Fatalf("issue attempts = %d", len(issuer.attempts))
	}
	if _, present := issuer.attempts[0].PreviousTokenDigest(); present {
		t.Fatal("unrelated browser cookies became a session revoke hint")
	}
}

func TestLoginSuccessRotatesCookieAndReturnsMinimalDTO(t *testing.T) {
	issuer := &issuerPort{}
	dependencies := validStubDependencies(t)
	dependencies.Login = mustLoginService(t, issuer)
	csrf := &stubCSRF{issueToken: "v1.active.public-csrf"}
	dependencies.CSRF = csrf
	router := mustRouter(t, dependencies, Options{})
	incoming := bytes.Repeat([]byte{0xa5}, identityapp.SessionTokenBytes)
	request := validLoginRequest(strings.NewReader(
		`{"login_name":"operator-1","password":"correct horse battery staple"}`,
	))
	request.Header.Set("Cookie", sessioncookie.DevelopmentCookieName+"="+
		base64.RawURLEncoding.EncodeToString(incoming))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if len(issuer.attempts) != 1 {
		t.Fatalf("issue attempts = %d, want 1", len(issuer.attempts))
	}
	previousDigest, ok := issuer.attempts[0].PreviousTokenDigest()
	if !ok || previousDigest.Validate() != nil {
		t.Fatal("valid incoming token was not carried only as a revoke hint")
	}
	response := recorder.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("response cookies = %d, want 1", len(cookies))
	}
	issuedRaw, err := base64.RawURLEncoding.DecodeString(cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(issuedRaw, incoming) {
		t.Fatal("incoming bearer token was reused")
	}
	if cookies[0].Name != sessioncookie.DevelopmentCookieName || !cookies[0].HttpOnly ||
		cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode ||
		cookies[0].Path != "/" || cookies[0].Domain != "" {
		t.Fatalf("unsafe Cookie: %#v", cookies[0])
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	data := payload["data"].(map[string]any)
	if data["authenticated"] != true || data["csrf_token"] != csrf.issueToken {
		t.Fatalf("unexpected data: %#v", data)
	}
	principal := data["principal"].(map[string]any)
	if principal["kind"] != "human" || principal["id"] != "operator-1" {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	assertNoAuthorizationVocabulary(t, recorder.Body.String())
	assertSafetyHeaders(t, recorder)
}

func TestLoginPostCommitDependencyFailuresNeverSetCookie(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Dependencies)
	}{
		{name: "csrf entropy", mutate: func(dependencies *Dependencies) {
			dependencies.CSRF = &stubCSRF{issueErr: errors.New("entropy failed")}
		}},
		{name: "cookie build", mutate: func(dependencies *Dependencies) {
			dependencies.Cookies = &stubCookiePolicy{origin: testOrigin, buildErr: errors.New("cookie failed")}
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			issuer := &issuerPort{}
			dependencies := validStubDependencies(t)
			dependencies.Login = mustLoginService(t, issuer)
			testCase.mutate(&dependencies)
			router := mustRouter(t, dependencies, Options{})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, validLoginRequest(strings.NewReader(
				`{"login_name":"operator-1","password":"secret"}`,
			)))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
			}
			if len(recorder.Header().Values("Set-Cookie")) != 0 {
				t.Fatalf("unexpected Set-Cookie: %v", recorder.Header().Values("Set-Cookie"))
			}
			if len(issuer.attempts) != 1 {
				t.Fatalf("issue attempts = %d, want committed issue", len(issuer.attempts))
			}
		})
	}
}

func TestApplicationFailureMappingIsStableAndLowDisclosure(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "credential", err: identityapp.ErrAuthenticationFailed, wantStatus: 401, wantCode: "authentication_failed"},
		{name: "throttled", err: identityapp.ErrAuthenticationThrottled, wantStatus: 429, wantCode: "authentication_throttled"},
		{name: "dependency", err: identityapp.ErrAuthenticationUnavailable, wantStatus: 503, wantCode: "authentication_unavailable"},
		{name: "issue commit unknown", err: identityapp.ErrCommitOutcomeUnknown, wantStatus: 503, wantCode: "authentication_unavailable"},
		{name: "canceled", err: identityapp.ErrOperationCanceled, wantStatus: 503, wantCode: "authentication_unavailable"},
		{name: "raw dependency deadline", err: context.DeadlineExceeded, wantStatus: 503, wantCode: "authentication_unavailable"},
		{name: "unknown", err: errors.New("password=sentinel token=sentinel"), wantStatus: 500, wantCode: "internal_error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dependencies := validStubDependencies(t)
			dependencies.Login = &stubLogin{err: testCase.err}
			var logs bytes.Buffer
			router := mustRouter(t, dependencies, Options{Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, validLoginRequest(strings.NewReader(
				`{"login_name":"operator-1","password":"secret-sentinel"}`,
			)))
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
			body := decodeErrorBody(t, recorder)
			if body.Code != testCase.wantCode {
				t.Fatalf("code = %q, want %q", body.Code, testCase.wantCode)
			}
			combined := recorder.Body.String() + logs.String()
			for _, secret := range []string{"secret-sentinel", "password=sentinel", "token=sentinel"} {
				if strings.Contains(combined, secret) {
					t.Fatalf("secret %q leaked: %s", secret, combined)
				}
			}
			if len(recorder.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("failure set a Cookie")
			}
		})
	}
}

func mustRouter(t *testing.T, dependencies Dependencies, options Options) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := RegisterRoutes(router, dependencies, options); err != nil {
		t.Fatal(err)
	}
	return router
}

func assertSafetyHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	want := map[string]string{
		"Cache-Control":                "no-store",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Permissions-Policy":           "camera=(), geolocation=(), microphone=()",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
	}
	for name, value := range want {
		if got := recorder.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func assertErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	_ = decodeErrorBody(t, recorder)
}

func decodeErrorBody(t *testing.T, recorder *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var envelope errorEnvelope
	decoder := json.NewDecoder(bytes.NewReader(recorder.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v; body=%s", err, recorder.Body.String())
	}
	if envelope.Error.Code == "" || envelope.Error.Message == "" {
		t.Fatalf("incomplete error envelope: %#v", envelope)
	}
	return envelope.Error
}

func assertNoAuthorizationVocabulary(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, forbidden := range []string{"role", "permission", "scope", "capability", "policy", "account_id", "session"} {
		if strings.Contains(lower, `"`+forbidden+`"`) {
			t.Fatalf("forbidden authorization/internal vocabulary %q in %s", forbidden, body)
		}
	}
}

func FuzzDecodeLoginRequestStrict(f *testing.F) {
	f.Add([]byte(`{"login_name":"operator-1","password":"secret"}`))
	f.Add([]byte(`{"login_name":"operator-1","password":"one","password":"two"}`))
	f.Add(bytes.Repeat([]byte{'x'}, MaximumLoginBodyBytes+1))
	f.Fuzz(func(t *testing.T, body []byte) {
		request, err := http.NewRequest(http.MethodPost, SessionPath, bytes.NewReader(body))
		if err != nil {
			t.Skip()
		}
		decoded, err := decodeLoginRequest(request)
		if err == nil {
			if decoded.loginName == "" || len(decoded.password) == 0 || len(body) > MaximumLoginBodyBytes {
				t.Fatalf("accepted incomplete or oversized body")
			}
			clear(decoded.password)
		}
	})
}
