package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAccessLogUsesRoutePatternAndSafeFixedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	startedAt := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)
	clock := &sequenceClock{times: []time.Time{
		startedAt,
		startedAt.Add(1750 * time.Millisecond),
	}}
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	router := NewRouter(RouterOptions{
		Clock:              clock,
		Logger:             logger,
		RequestIDGenerator: func() string { return "access-request-id" },
	})
	router.GET("/api/v1/items/:id", func(ginContext *gin.Context) {
		ginContext.Status(http.StatusCreated)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/items/sensitive-item-42?token=query-secret",
		nil,
	)
	request.Header.Set("Authorization", "Bearer header-secret")
	router.ServeHTTP(recorder, request)

	lines := nonemptyLines(logOutput.String())
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1\n%s", len(lines), logOutput.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("decode access log: %v\n%s", err, lines[0])
	}
	wantFields := map[string]any{
		"msg":         "http_request",
		"request_id":  "access-request-id",
		"method":      http.MethodGet,
		"route":       "/api/v1/items/:id",
		"status":      float64(http.StatusCreated),
		"duration_ms": float64(1750),
	}
	for key, want := range wantFields {
		if got := record[key]; got != want {
			t.Fatalf("access log %s = %#v, want %#v\n%s", key, got, want, lines[0])
		}
	}
	for _, unsafe := range []string{
		"sensitive-item-42",
		"token",
		"query-secret",
		"Authorization",
		"header-secret",
	} {
		if strings.Contains(logOutput.String(), unsafe) {
			t.Fatalf("access log leaked %q: %s", unsafe, logOutput.String())
		}
	}
}

func TestAccessLogDoesNotUseUnmatchedRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checkedAt := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	router := NewRouter(RouterOptions{
		Clock:              ClockFunc(func() time.Time { return checkedAt }),
		Logger:             logger,
		RequestIDGenerator: func() string { return "unmatched-request-id" },
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/missing/customer-secret?token=query-secret", nil),
	)

	var record map[string]any
	if err := json.Unmarshal([]byte(nonemptyLines(logOutput.String())[0]), &record); err != nil {
		t.Fatalf("decode access log: %v\n%s", err, logOutput.String())
	}
	if got := record["route"]; got != unmatchedRoute {
		t.Fatalf("route = %#v, want %q", got, unmatchedRoute)
	}
	for _, unsafe := range []string{"customer-secret", "query-secret", "token"} {
		if strings.Contains(logOutput.String(), unsafe) {
			t.Fatalf("unmatched access log leaked %q: %s", unsafe, logOutput.String())
		}
	}
}

func TestAccessLogLevelFollowsResponseStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checkedAt := time.Date(2026, time.August, 29, 9, 0, 0, 0, time.UTC)
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	router := NewRouter(RouterOptions{
		Clock:              ClockFunc(func() time.Time { return checkedAt }),
		Logger:             logger,
		RequestIDGenerator: func() string { return "leveled-request-id" },
	})
	router.GET("/_test/success", func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})
	router.GET("/_test/internal", func(ginContext *gin.Context) {
		AbortWithError(ginContext, errors.New("hidden internal cause"))
	})

	for _, path := range []string{"/_test/success", "/missing", "/_test/internal"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	}

	lines := nonemptyLines(logOutput.String())
	if len(lines) != 2 {
		t.Fatalf("warn-threshold log lines = %d, want 2 (4xx and 5xx)\n%s", len(lines), logOutput.String())
	}
	want := []struct {
		level  string
		status float64
	}{
		{level: "WARN", status: float64(http.StatusNotFound)},
		{level: "ERROR", status: float64(http.StatusInternalServerError)},
	}
	for index, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode log line %d: %v\n%s", index, err, line)
		}
		if got := record["level"]; got != want[index].level {
			t.Fatalf("line %d level = %#v, want %q", index, got, want[index].level)
		}
		if got := record["status"]; got != want[index].status {
			t.Fatalf("line %d status = %#v, want %.0f", index, got, want[index].status)
		}
	}
}

func TestAccessLogClampsBackwardClockToZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	startedAt := time.Date(2026, time.August, 29, 9, 0, 1, 0, time.UTC)
	clock := &sequenceClock{times: []time.Time{startedAt, startedAt.Add(-time.Second)}}
	var logOutput bytes.Buffer
	router := NewRouter(RouterOptions{
		Clock:              clock,
		Logger:             slog.New(slog.NewJSONHandler(&logOutput, nil)),
		RequestIDGenerator: func() string { return "backward-clock-id" },
	})
	router.GET("/_test/backward-clock", func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/_test/backward-clock", nil))

	var record map[string]any
	if err := json.Unmarshal([]byte(nonemptyLines(logOutput.String())[0]), &record); err != nil {
		t.Fatalf("decode access log: %v\n%s", err, logOutput.String())
	}
	if got := record["duration_ms"]; got != float64(0) {
		t.Fatalf("duration_ms = %#v, want 0", got)
	}
}

type sequenceClock struct {
	mu    sync.Mutex
	times []time.Time
	next  int
}

func (clock *sequenceClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if clock.next >= len(clock.times) {
		panic("sequenceClock exhausted")
	}
	value := clock.times[clock.next]
	clock.next++
	return value
}

func nonemptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
