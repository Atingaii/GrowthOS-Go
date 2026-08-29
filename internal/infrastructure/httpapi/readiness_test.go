package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestReadinessReturnsStrictReadyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checkedAt := time.Date(2026, time.August, 29, 17, 45, 12, 987654321, time.FixedZone("CST", 8*60*60))
	checker := readinessCheckerFunc(func(context.Context) error { return nil })
	router := NewRouter(RouterOptions{
		Version:            "v0.13.0-test",
		Clock:              ClockFunc(func() time.Time { return checkedAt }),
		ReadinessChecker:   checker,
		ReadinessTimeout:   time.Second,
		RequestIDGenerator: func() string { return "generated-ready-id" },
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, ReadyPath, nil)
	request.Header.Set(RequestIDHeader, "ready-client-id")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := recorder.Header().Get(RequestIDHeader); got != "ready-client-id" {
		t.Fatalf("%s = %q, want preserved client ID", RequestIDHeader, got)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v\n%s", err, recorder.Body.String())
	}
	if len(raw) != 3 || raw["status"] == nil || raw["version"] == nil || raw["timestamp"] == nil {
		t.Fatalf("response fields = %v, want exactly status, version, timestamp", raw)
	}
	var response readinessResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode typed response: %v", err)
	}
	want := readinessResponse{
		Status:    "ready",
		Version:   "v0.13.0-test",
		Timestamp: checkedAt.UTC().Format(time.RFC3339Nano),
	}
	if response != want {
		t.Fatalf("response = %#v, want %#v", response, want)
	}
}

func TestReadinessFailsClosedWithoutChecker(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{
		RequestIDGenerator: func() string { return "nil-checker-id" },
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, ReadyPath, nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "dependency_unavailable",
		Message:   "service unavailable",
		RequestID: "nil-checker-id",
	})
}

func TestReadinessFailsClosedForTypedNilChecker(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var checker *typedNilReadinessChecker
	router := NewRouter(RouterOptions{
		ReadinessChecker:   checker,
		RequestIDGenerator: func() string { return "typed-nil-checker-id" },
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, ReadyPath, nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "dependency_unavailable",
		Message:   "service unavailable",
		RequestID: "typed-nil-checker-id",
	})
}

func TestReadinessFailureIsSafeAndAccessLogRecords503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const secret = "mysql-password=readiness-secret"
	var logOutput bytes.Buffer
	router := NewRouter(RouterOptions{
		Logger: slog.New(slog.NewJSONHandler(&logOutput, nil)),
		Clock: ClockFunc(func() time.Time {
			return time.Date(2026, time.August, 29, 18, 0, 0, 0, time.UTC)
		}),
		ReadinessChecker: readinessCheckerFunc(func(context.Context) error {
			return errors.New(secret)
		}),
		RequestIDGenerator: func() string { return "unused-generated-id" },
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, ReadyPath, nil)
	request.Header.Set(RequestIDHeader, "ready-failure-id")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "dependency_unavailable",
		Message:   "service unavailable",
		RequestID: "ready-failure-id",
	})
	if strings.Contains(recorder.Body.String(), secret) || strings.Contains(logOutput.String(), secret) {
		t.Fatalf("readiness response or log leaked checker error\nresponse=%s\nlog=%s", recorder.Body.String(), logOutput.String())
	}

	lines := nonemptyLines(logOutput.String())
	if len(lines) != 1 {
		t.Fatalf("access log lines = %d, want 1\n%s", len(lines), logOutput.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode access log: %v\n%s", err, lines[0])
	}
	if got := record["level"]; got != "ERROR" {
		t.Fatalf("access log level = %#v, want ERROR", got)
	}
	if got := record["route"]; got != ReadyPath {
		t.Fatalf("access log route = %#v, want %q", got, ReadyPath)
	}
	if got := record["status"]; got != float64(http.StatusServiceUnavailable) {
		t.Fatalf("access log status = %#v, want %d", got, http.StatusServiceUnavailable)
	}
	if got := record["request_id"]; got != "ready-failure-id" {
		t.Fatalf("access log request_id = %#v, want ready-failure-id", got)
	}
}

func TestReadinessTimeoutUsesRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const timeout = 20 * time.Millisecond
	deadlineObserved := make(chan time.Duration, 1)
	checker := readinessCheckerFunc(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("missing readiness deadline")
		}
		deadlineObserved <- time.Until(deadline)
		<-ctx.Done()
		return ctx.Err()
	})
	router := NewRouter(RouterOptions{
		ReadinessChecker:   checker,
		ReadinessTimeout:   timeout,
		RequestIDGenerator: func() string { return "ready-timeout-id" },
	})

	recorder := httptest.NewRecorder()
	startedAt := time.Now()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, ReadyPath, nil))
	elapsed := time.Since(startedAt)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "dependency_unavailable",
		Message:   "service unavailable",
		RequestID: "ready-timeout-id",
	})
	if elapsed >= time.Second {
		t.Fatalf("readiness probe elapsed %s, want bounded by configured timeout", elapsed)
	}
	observed := <-deadlineObserved
	if observed <= 0 || observed > timeout {
		t.Fatalf("checker deadline remaining = %s, want within (0, %s]", observed, timeout)
	}
}

func TestReadinessUsesDefaultTimeoutForNonPositiveValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name    string
		timeout time.Duration
	}{
		{name: "zero", timeout: 0},
		{name: "negative", timeout: -time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			deadlineObserved := make(chan time.Duration, 1)
			checker := readinessCheckerFunc(func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok {
					return errors.New("missing readiness deadline")
				}
				deadlineObserved <- time.Until(deadline)
				return nil
			})
			router := NewRouter(RouterOptions{
				ReadinessChecker: checker,
				ReadinessTimeout: test.timeout,
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, ReadyPath, nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			observed := <-deadlineObserved
			if observed <= DefaultReadinessTimeout-time.Second || observed > DefaultReadinessTimeout {
				t.Fatalf("checker deadline remaining = %s, want approximately %s", observed, DefaultReadinessTimeout)
			}
		})
	}
}

func TestRouterRejectsUnsupportedReadinessMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{
		RequestIDGenerator: func() string { return "ready-method-id" },
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, ReadyPath, nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if got := recorder.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "method_not_allowed",
		Message:   "method not allowed",
		RequestID: "ready-method-id",
	})
}

type readinessCheckerFunc func(context.Context) error

func (checker readinessCheckerFunc) PingContext(ctx context.Context) error {
	return checker(ctx)
}

type typedNilReadinessChecker struct{}

func (*typedNilReadinessChecker) PingContext(context.Context) error {
	panic("typed nil readiness checker was invoked")
}
