package httpapi

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

const unmatchedRoute = "unmatched"

func accessLogMiddleware(logger *slog.Logger, clock Clock) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		startedAt := clock.Now()
		ginContext.Next()

		durationMillis := clock.Now().Sub(startedAt).Milliseconds()
		if durationMillis < 0 {
			durationMillis = 0
		}
		route := ginContext.FullPath()
		if route == "" {
			route = unmatchedRoute
		}
		logger.LogAttrs(
			ginContext.Request.Context(),
			accessLogLevel(ginContext.Writer.Status()),
			"http_request",
			slog.String("request_id", RequestID(ginContext)),
			slog.String("method", ginContext.Request.Method),
			slog.String("route", route),
			slog.Int("status", ginContext.Writer.Status()),
			slog.Int64("duration_ms", durationMillis),
		)
	}
}

func recoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}

			// Never log the panic value or stack here: either can contain request
			// payloads, credentials, or internal implementation details.
			logger.ErrorContext(
				ginContext.Request.Context(),
				"panic recovered",
				slog.String("request_id", RequestID(ginContext)),
			)
			if !ginContext.Writer.Written() {
				abortWithFaultStatus(ginContext, http.StatusInternalServerError, internalServerFault)
				return
			}
			ginContext.Abort()
		}()

		ginContext.Next()
	}
}

func accessLogLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
