package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader is the request/response header used for correlation.
	RequestIDHeader = "X-Request-ID"
	// MaxRequestIDLength bounds user-controlled identifiers before they reach
	// logs or downstream application code.
	MaxRequestIDLength = 64
)

// RequestIDGenerator supplies a request ID when a caller did not provide a
// safe one. Tests can inject a deterministic implementation.
type RequestIDGenerator func() string

type requestIDContextKey struct{}

const requestIDGinKey = "growthos.request_id"

var fallbackRequestIDSequence atomic.Uint64

func requestIDMiddleware(generator RequestIDGenerator) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestID := incomingRequestID(ginContext)
		if requestID == "" {
			requestID = generatedRequestID(generator)
		}

		ginContext.Set(requestIDGinKey, requestID)
		ginContext.Header(RequestIDHeader, requestID)
		ginContext.Request.Header.Set(RequestIDHeader, requestID)
		ginContext.Request = ginContext.Request.WithContext(
			context.WithValue(ginContext.Request.Context(), requestIDContextKey{}, requestID),
		)

		ginContext.Next()
	}
}

// RequestID returns the validated correlation ID assigned to a Gin request.
func RequestID(ginContext *gin.Context) string {
	if ginContext == nil {
		return ""
	}
	value, exists := ginContext.Get(requestIDGinKey)
	if !exists {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}

// RequestIDFromContext returns the correlation ID for application code that
// depends only on context.Context rather than Gin.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func incomingRequestID(ginContext *gin.Context) string {
	values := ginContext.Request.Header.Values(RequestIDHeader)
	if len(values) != 1 || !validRequestID(values[0]) {
		return ""
	}
	return values[0]
}

func generatedRequestID(generator RequestIDGenerator) string {
	if generator != nil {
		if requestID := generator(); validRequestID(requestID) {
			return requestID
		}
	}
	return defaultRequestID()
}

func defaultRequestID() string {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err == nil {
		return hex.EncodeToString(entropy[:])
	}

	// crypto/rand failure is exceptional, but correlation must still work. Both
	// components contain only characters accepted by validRequestID.
	return fmt.Sprintf("fallback-%x-%x", time.Now().UnixNano(), fallbackRequestIDSequence.Add(1))
}

func validRequestID(requestID string) bool {
	if len(requestID) == 0 || len(requestID) > MaxRequestIDLength {
		return false
	}
	for index := 0; index < len(requestID); index++ {
		character := requestID[index]
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '-' &&
			character != '_' &&
			character != '.' &&
			character != ':' {
			return false
		}
	}
	return true
}
