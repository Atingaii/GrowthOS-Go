package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/strategycache"
)

const strategyCacheWarningInterval = 10 * time.Second

type observationThrottle struct {
	lastLogged time.Time
	suppressed int
}

// strategyCacheObserver keeps routine cache outcomes at debug level and
// rate-limits warning classes independently. A long Redis outage therefore
// remains diagnosable without emitting one warning per request.
type strategyCacheObserver struct {
	logger *slog.Logger
	now    func() time.Time
	window time.Duration

	mu       sync.Mutex
	warnings map[strategycache.ObservationKind]observationThrottle
}

func newStrategyCacheObserver(logger *slog.Logger) strategycache.Observer {
	return &strategyCacheObserver{
		logger:   logger,
		now:      time.Now,
		window:   strategyCacheWarningInterval,
		warnings: make(map[strategycache.ObservationKind]observationThrottle),
	}
}

func (observer *strategyCacheObserver) Observe(ctx context.Context, observation strategycache.Observation) {
	if observer == nil || observer.logger == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !warningObservation(observation.Kind) {
		observer.logger.LogAttrs(
			ctx,
			slog.LevelDebug,
			"lottery strategy cache outcome",
			slog.String("cache_outcome", string(observation.Kind)),
			slog.Int64("duration_ms", observation.Duration.Milliseconds()),
		)
		return
	}

	suppressed, emit := observer.admitWarning(observation.Kind)
	if !emit {
		return
	}
	observer.logger.LogAttrs(
		ctx,
		slog.LevelWarn,
		"lottery strategy cache degraded",
		slog.String("cache_outcome", string(observation.Kind)),
		slog.Int64("duration_ms", observation.Duration.Milliseconds()),
		slog.Int("suppressed_since_last", suppressed),
	)
}

func (observer *strategyCacheObserver) admitWarning(kind strategycache.ObservationKind) (int, bool) {
	now := time.Now()
	if observer.now != nil {
		now = observer.now()
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.warnings == nil {
		observer.warnings = make(map[strategycache.ObservationKind]observationThrottle)
	}
	window := observer.window
	if window <= 0 {
		window = strategyCacheWarningInterval
	}

	state := observer.warnings[kind]
	elapsed := now.Sub(state.lastLogged)
	if !state.lastLogged.IsZero() && elapsed >= 0 && elapsed < window {
		state.suppressed++
		observer.warnings[kind] = state
		return 0, false
	}
	suppressed := state.suppressed
	observer.warnings[kind] = observationThrottle{lastLogged: now}
	return suppressed, true
}

func warningObservation(kind strategycache.ObservationKind) bool {
	switch kind {
	case strategycache.ObservationReadError,
		strategycache.ObservationDeleteError,
		strategycache.ObservationWriteError:
		return true
	default:
		return false
	}
}
