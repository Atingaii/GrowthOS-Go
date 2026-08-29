// Package httpapi contains the HTTP inbound adapter for the GrowthOS modular
// monolith. Domain modules can be mounted here without moving HTTP concerns
// into their domain models.
package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// HealthPath is intentionally unversioned because it describes the process,
	// not a versioned business resource.
	HealthPath = "/health"
	// ReadyPath is intentionally unversioned because it describes whether the
	// process can currently serve traffic, not a versioned business resource.
	ReadyPath = "/ready"
)

// RouterOptions contains process metadata dependencies used by HTTP handlers.
type RouterOptions struct {
	Version            string
	Clock              Clock
	Logger             *slog.Logger
	RequestIDGenerator RequestIDGenerator
	ReadinessChecker   ReadinessChecker
	ReadinessTimeout   time.Duration
}

// NewRouter builds the product process's Gin router.
func NewRouter(options RouterOptions) *gin.Engine {
	version := strings.TrimSpace(options.Version)
	if version == "" {
		version = unknownVersion
	}
	clock := options.Clock
	if clock == nil {
		clock = systemClock{}
	}
	logger := options.Logger
	if logger == nil {
		logger = discardLogger()
	}

	router := gin.New()
	router.Use(
		requestIDMiddleware(options.RequestIDGenerator),
		accessLogMiddleware(logger, clock),
		recoveryMiddleware(logger),
	)
	router.HandleMethodNotAllowed = true
	// Do not trust client-controlled forwarding headers until an explicit reverse
	// proxy CIDR allowlist is configured. This keeps future ClientIP-based logs,
	// audit records, and controls from accepting spoofed source addresses.
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	router.NoRoute(func(ginContext *gin.Context) {
		abortWithFaultStatus(ginContext, http.StatusNotFound, routeNotFoundFault)
	})
	router.NoMethod(func(ginContext *gin.Context) {
		abortWithFaultStatus(ginContext, http.StatusMethodNotAllowed, methodNotAllowedFault)
	})
	router.GET(HealthPath, newHealthHandler(version, clock))
	router.GET(ReadyPath, newReadinessHandler(
		version,
		clock,
		options.ReadinessChecker,
		options.ReadinessTimeout,
	))

	return router
}
