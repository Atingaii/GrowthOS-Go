package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDPreservesSafeIncomingValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	generated := false
	router := NewRouter(RouterOptions{
		RequestIDGenerator: func() string {
			generated = true
			return "generated-id"
		},
	})
	router.GET("/_test/request-id", func(ginContext *gin.Context) {
		ginContext.JSON(http.StatusOK, gin.H{
			"gin":     RequestID(ginContext),
			"context": RequestIDFromContext(ginContext.Request.Context()),
			"header":  ginContext.GetHeader(RequestIDHeader),
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/_test/request-id", nil)
	request.Header.Set(RequestIDHeader, "client.Request_ID:42")
	router.ServeHTTP(recorder, request)

	if generated {
		t.Fatal("generator was called for a safe incoming request ID")
	}
	if got := recorder.Header().Get(RequestIDHeader); got != "client.Request_ID:42" {
		t.Fatalf("response request ID = %q", got)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for source, got := range body {
		if got != "client.Request_ID:42" {
			t.Fatalf("%s request ID = %q", source, got)
		}
	}
}

func TestRequestIDReplacesMissingOrUnsafeValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "whitespace", values: []string{"unsafe id"}},
		{name: "unicode", values: []string{"请求-42"}},
		{name: "separator", values: []string{"unsafe/value"}},
		{name: "comma", values: []string{"first,second"}},
		{name: "too long", values: []string{strings.Repeat("a", MaxRequestIDLength+1)}},
		{name: "multiple fields", values: []string{"first", "second"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter(RouterOptions{
				RequestIDGenerator: func() string { return "replacement-id" },
			})
			router.GET("/_test/request-id", func(ginContext *gin.Context) {
				ginContext.String(http.StatusOK, RequestID(ginContext))
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/_test/request-id", nil)
			for _, value := range test.values {
				request.Header.Add(RequestIDHeader, value)
			}
			router.ServeHTTP(recorder, request)

			if got := recorder.Header().Get(RequestIDHeader); got != "replacement-id" {
				t.Fatalf("response request ID = %q, want replacement", got)
			}
			if got := recorder.Body.String(); got != "replacement-id" {
				t.Fatalf("handler request ID = %q, want replacement", got)
			}
		})
	}
}

func TestRequestIDRejectsUnsafeInjectedGeneratorValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewRouter(RouterOptions{
		RequestIDGenerator: func() string { return "invalid generated value" },
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, HealthPath, nil))

	requestID := recorder.Header().Get(RequestIDHeader)
	if requestID == "invalid generated value" || !validRequestID(requestID) {
		t.Fatalf("response request ID = %q, want safe fallback", requestID)
	}
}

func TestRequestIDLookupHandlesAbsentContext(t *testing.T) {
	if got := RequestID(nil); got != "" {
		t.Fatalf("RequestID(nil) = %q", got)
	}
	if got := RequestIDFromContext(nil); got != "" {
		t.Fatalf("RequestIDFromContext(nil) = %q", got)
	}
}
