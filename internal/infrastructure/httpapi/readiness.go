package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
)

// DefaultReadinessTimeout bounds a dependency probe when the caller does not
// provide a positive timeout.
const DefaultReadinessTimeout = 2 * time.Second

// ReadinessChecker is the minimal dependency contract required by /ready.
// *sql.DB satisfies this interface directly.
type ReadinessChecker interface {
	PingContext(context.Context) error
}

type readinessResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

func newReadinessHandler(
	version string,
	clock Clock,
	checker ReadinessChecker,
	timeout time.Duration,
) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = DefaultReadinessTimeout
	}

	return func(ginContext *gin.Context) {
		ginContext.Header("Cache-Control", "no-store")
		if nilReadinessChecker(checker) {
			AbortWithError(ginContext, dependencyUnavailableFault)
			return
		}

		probeContext, cancel := context.WithTimeout(ginContext.Request.Context(), timeout)
		defer cancel()

		if err := checker.PingContext(probeContext); err != nil || probeContext.Err() != nil {
			AbortWithError(ginContext, dependencyUnavailableFault)
			return
		}

		ginContext.JSON(http.StatusOK, readinessResponse{
			Status:    "ready",
			Version:   version,
			Timestamp: clock.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

func nilReadinessChecker(checker ReadinessChecker) bool {
	if checker == nil {
		return true
	}

	value := reflect.ValueOf(checker)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
