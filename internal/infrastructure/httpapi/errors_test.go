package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/platform/fault"
	"github.com/gin-gonic/gin"
)

func TestAbortWithErrorMapsEveryFaultKind(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		kind   fault.Kind
		status int
	}{
		{kind: fault.KindInvalid, status: http.StatusBadRequest},
		{kind: fault.KindUnauthenticated, status: http.StatusUnauthorized},
		{kind: fault.KindForbidden, status: http.StatusForbidden},
		{kind: fault.KindNotFound, status: http.StatusNotFound},
		{kind: fault.KindConflict, status: http.StatusConflict},
		{kind: fault.KindRateLimited, status: http.StatusTooManyRequests},
		{kind: fault.KindInternal, status: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			cause := errors.New("database password=never-return-this")
			publicFault, err := fault.New(test.kind, "stable_test_code", "safe public message", cause)
			if err != nil {
				t.Fatalf("create fault: %v", err)
			}
			router := NewRouter(RouterOptions{
				RequestIDGenerator: func() string { return "mapping-request-id" },
			})
			router.GET("/_test/failure", func(ginContext *gin.Context) {
				AbortWithError(ginContext, fmt.Errorf("use case: %w", publicFault))
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_test/failure", nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			assertErrorEnvelope(t, recorder, errorBody{
				Code:      "stable_test_code",
				Message:   "safe public message",
				RequestID: "mapping-request-id",
			})
			if strings.Contains(recorder.Body.String(), "password") {
				t.Fatalf("response leaked cause: %s", recorder.Body.String())
			}
		})
	}
}

func TestAbortWithErrorHidesUnknownErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{
		RequestIDGenerator: func() string { return "unknown-error-id" },
	})
	router.GET("/_test/failure", func(ginContext *gin.Context) {
		AbortWithError(ginContext, errors.New("dsn=user:secret@database"))
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_test/failure", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "internal_error",
		Message:   "internal server error",
		RequestID: "unknown-error-id",
	})
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("response leaked unknown error: %s", recorder.Body.String())
	}
}

func TestRouterUsesUnifiedErrorsForNoRouteAndNoMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		method  string
		path    string
		status  int
		code    string
		message string
	}{
		{
			name:    "route",
			method:  http.MethodGet,
			path:    "/not-registered",
			status:  http.StatusNotFound,
			code:    "route_not_found",
			message: "resource not found",
		},
		{
			name:    "method",
			method:  http.MethodPost,
			path:    HealthPath,
			status:  http.StatusMethodNotAllowed,
			code:    "method_not_allowed",
			message: "method not allowed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter(RouterOptions{
				RequestIDGenerator: func() string { return "router-error-id" },
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			assertErrorEnvelope(t, recorder, errorBody{
				Code:      test.code,
				Message:   test.message,
				RequestID: "router-error-id",
			})
		})
	}
}

func TestRecoveryReturnsSafeUnifiedInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	router := NewRouter(RouterOptions{
		Logger:             logger,
		RequestIDGenerator: func() string { return "panic-request-id" },
	})
	router.GET("/_test/panic", func(*gin.Context) {
		panic("credential=panic-secret")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/_test/panic?token=query-secret", nil)
	request.Header.Set("Authorization", "Bearer header-secret")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	assertErrorEnvelope(t, recorder, errorBody{
		Code:      "internal_error",
		Message:   "internal server error",
		RequestID: "panic-request-id",
	})
	for _, secret := range []string{"panic-secret", "query-secret", "header-secret", "Authorization"} {
		if strings.Contains(recorder.Body.String(), secret) {
			t.Fatalf("response leaked %q: %s", secret, recorder.Body.String())
		}
		if strings.Contains(logOutput.String(), secret) {
			t.Fatalf("log leaked %q: %s", secret, logOutput.String())
		}
	}
	if !strings.Contains(logOutput.String(), `"msg":"panic recovered"`) {
		t.Fatalf("recovery log missing safe event: %s", logOutput.String())
	}
}

func TestRecoveryDoesNotAppendEnvelopeAfterResponseWasCommitted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	router := NewRouter(RouterOptions{
		Logger:             logger,
		RequestIDGenerator: func() string { return "committed-panic-id" },
	})
	router.GET("/_test/committed-panic", func(ginContext *gin.Context) {
		ginContext.String(http.StatusAccepted, "response already committed")
		panic("committed-panic-secret")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_test/committed-panic", nil))

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want original committed status %d", recorder.Code, http.StatusAccepted)
	}
	if got := recorder.Body.String(); got != "response already committed" {
		t.Fatalf("body = %q, recovery appended or replaced committed response", got)
	}
	if strings.Contains(logOutput.String(), "committed-panic-secret") {
		t.Fatalf("recovery log leaked panic: %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), `"msg":"panic recovered"`) {
		t.Fatalf("recovery log missing safe event: %s", logOutput.String())
	}
}

func assertErrorEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, want errorBody) {
	t.Helper()
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	if got := recorder.Header().Get(RequestIDHeader); got != want.RequestID {
		t.Fatalf("response request ID = %q, want %q", got, want.RequestID)
	}
	var response errorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v\n%s", err, recorder.Body.String())
	}
	if response.Error != want {
		t.Fatalf("error response = %#v, want %#v", response.Error, want)
	}
}
