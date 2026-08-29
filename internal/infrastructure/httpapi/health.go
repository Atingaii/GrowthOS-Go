package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const unknownVersion = "unknown"

// Clock is the minimal time dependency needed by the health endpoint.
// Keeping it explicit makes the response deterministic in tests.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function, such as time.Now, to Clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (clock ClockFunc) Now() time.Time {
	return clock()
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

type healthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}

func newHealthHandler(version string, clock Clock) gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Header("Cache-Control", "no-store")
		context.JSON(http.StatusOK, healthResponse{
			Status:    "ok",
			Version:   version,
			Timestamp: clock.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}
