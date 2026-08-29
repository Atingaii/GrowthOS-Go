package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHealthHandlerUsesInjectedProcessMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checkedAt := time.Date(2026, time.August, 29, 16, 30, 45, 123456789, time.FixedZone("CST", 8*60*60))
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, HealthPath, nil)

	newHealthHandler("v0.11.0-test", ClockFunc(func() time.Time {
		return checkedAt
	}))(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}

	var response healthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := healthResponse{
		Status:    "ok",
		Version:   "v0.11.0-test",
		Timestamp: checkedAt.UTC().Format(time.RFC3339Nano),
	}
	if response != want {
		t.Fatalf("response = %#v, want %#v", response, want)
	}
}
