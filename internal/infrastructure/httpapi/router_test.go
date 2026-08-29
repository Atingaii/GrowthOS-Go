package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRouterRegistersHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checkedAt := time.Date(2026, time.August, 29, 8, 30, 45, 0, time.UTC)
	router := NewRouter(RouterOptions{
		Version: "test-build",
		Clock: ClockFunc(func() time.Time {
			return checkedAt
		}),
		RequestIDGenerator: func() string { return "health-request-id" },
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, HealthPath, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get(RequestIDHeader); got != "health-request-id" {
		t.Fatalf("%s = %q, want %q", RequestIDHeader, got, "health-request-id")
	}
	var response healthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "ok" || response.Version != "test-build" || response.Timestamp != checkedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestRouterKeepsProcessHealthOutsideVersionedBusinessAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestRouterRejectsUnsupportedHealthMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, HealthPath, nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestRouterDoesNotTrustForwardedClientIPByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{})
	router.GET("/_test/client-ip", func(context *gin.Context) {
		context.String(http.StatusOK, context.ClientIP())
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/_test/client-ip", nil)
	request.RemoteAddr = "203.0.113.10:43210"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != "203.0.113.10" {
		t.Fatalf("ClientIP = %q, want direct peer and not spoofed forwarding header", got)
	}
}
