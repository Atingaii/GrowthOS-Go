package strategycache

import (
	"context"
	"sync"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

type flightGroup struct {
	mu      sync.Mutex
	flights map[string]*flight
}

type flight struct {
	done     chan struct{}
	strategy domain.Strategy
	err      error
}

// do coalesces only source fills for one cache key. Every caller waits with its
// own context. The shared fill deliberately drops the leader's cancellation and
// receives its own hard timeout, so one impatient caller cannot poison other
// callers and abandoned work cannot run without a bound.
func (g *flightGroup) do(
	ctx context.Context,
	lifecycle context.Context,
	key string,
	timeout time.Duration,
	fill func(context.Context) (domain.Strategy, error),
) (domain.Strategy, error, bool) {
	if err := ctx.Err(); err != nil {
		return domain.Strategy{}, err, false
	}
	if err := lifecycle.Err(); err != nil {
		return domain.Strategy{}, err, false
	}

	g.mu.Lock()
	if g.flights == nil {
		g.flights = make(map[string]*flight)
	}
	if existing, found := g.flights[key]; found {
		g.mu.Unlock()
		strategy, err := waitForFlight(ctx, existing)
		return strategy, err, true
	}

	current := &flight{done: make(chan struct{})}
	g.flights[key] = current
	g.mu.Unlock()

	go func() {
		fillContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		stopLifecycleCancellation := context.AfterFunc(lifecycle, cancel)
		strategy, err := fill(fillContext)
		stopLifecycleCancellation()
		cancel()

		g.mu.Lock()
		current.strategy = strategy
		current.err = err
		delete(g.flights, key)
		close(current.done)
		g.mu.Unlock()
	}()

	strategy, err := waitForFlight(ctx, current)
	return strategy, err, false
}

func waitForFlight(ctx context.Context, current *flight) (domain.Strategy, error) {
	select {
	case <-ctx.Done():
		return domain.Strategy{}, ctx.Err()
	case <-current.done:
		if err := ctx.Err(); err != nil {
			return domain.Strategy{}, err
		}
		return current.strategy, current.err
	}
}
