package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/adapter/strategycache"
)

func TestStrategyCacheObserverEmitsOnlyLowCardinalityOutcomeFields(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	observer := newStrategyCacheObserver(logger)

	observer.Observe(context.Background(), strategycache.Observation{
		Kind:     strategycache.ObservationHit,
		Duration: 12 * time.Millisecond,
	})
	rendered := output.String()
	if !strings.Contains(rendered, `"cache_outcome":"hit"`) ||
		!strings.Contains(rendered, `"duration_ms":12`) {
		t.Fatalf("debug observation = %q", rendered)
	}
	for _, forbidden := range []string{"strategy_id", "cache_key", "payload", "password", "redis_address"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("observation exposed forbidden field %q: %s", forbidden, rendered)
		}
	}
}

func TestStrategyCacheObserverRateLimitsEachWarningKind(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	current := time.Unix(1_700_000_000, 0)
	observer := &strategyCacheObserver{
		logger:   logger,
		now:      func() time.Time { return current },
		window:   strategyCacheWarningInterval,
		warnings: make(map[strategycache.ObservationKind]observationThrottle),
	}

	observation := strategycache.Observation{Kind: strategycache.ObservationReadError}
	observer.Observe(context.Background(), observation)
	observer.Observe(context.Background(), observation)
	observer.Observe(context.Background(), strategycache.Observation{Kind: strategycache.ObservationWriteError})
	if lines := nonEmptyLines(output.String()); len(lines) != 2 {
		t.Fatalf("warning lines = %d, want one per kind; output = %q", len(lines), output.String())
	}

	current = current.Add(strategyCacheWarningInterval)
	observer.Observe(context.Background(), observation)
	lines := nonEmptyLines(output.String())
	if len(lines) != 3 || !strings.Contains(lines[2], `"suppressed_since_last":1`) {
		t.Fatalf("post-window warning did not report suppression: %q", output.String())
	}
}

func TestNilStrategyCacheObserverIsSafe(t *testing.T) {
	var observer *strategyCacheObserver
	observer.Observe(nil, strategycache.Observation{Kind: strategycache.ObservationReadError})
}
