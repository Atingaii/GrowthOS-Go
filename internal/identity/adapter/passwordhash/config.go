package passwordhash

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"time"
)

const (
	DefaultMaxConcurrent  = 2
	MinimumMaxConcurrent  = 1
	MaximumMaxConcurrent  = 4
	DefaultAcquireTimeout = 250 * time.Millisecond
	MinimumAcquireTimeout = time.Millisecond
	MaximumAcquireTimeout = time.Second
)

// Config controls only the process resource boundary and enrollment entropy.
// A zero Config selects the documented defaults. Consequently an explicit
// zero AcquireTimeout is also interpreted as the default; use a positive value
// in the exported [MinimumAcquireTimeout, MaximumAcquireTimeout] range to
// override it.
type Config struct {
	MaxConcurrent  int
	AcquireTimeout time.Duration
	Entropy        io.Reader
}

// DefaultConfig returns the v1 process defaults. Entropy is crypto/rand.Reader.
func DefaultConfig() Config {
	return Config{
		MaxConcurrent:  DefaultMaxConcurrent,
		AcquireTimeout: DefaultAcquireTimeout,
		Entropy:        rand.Reader,
	}
}

func (c Config) String() string {
	return fmt.Sprintf(
		"passwordhash.Config{MaxConcurrent:%d, AcquireTimeout:%s, Entropy:[REDACTED]}",
		c.MaxConcurrent,
		c.AcquireTimeout,
	)
}

func (c Config) GoString() string { return c.String() }

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("max_concurrent", c.MaxConcurrent),
		slog.Duration("acquire_timeout", c.AcquireTimeout),
		slog.String("entropy", "[REDACTED]"),
	)
}

type normalizedConfig struct {
	maxConcurrent  int
	acquireTimeout time.Duration
	entropy        io.Reader
}

func normalizeConfig(config Config) (normalizedConfig, error) {
	maximum := config.MaxConcurrent
	if maximum == 0 {
		maximum = DefaultMaxConcurrent
	}
	if maximum < MinimumMaxConcurrent || maximum > MaximumMaxConcurrent {
		return normalizedConfig{}, ErrInvalidConfiguration
	}

	wait := config.AcquireTimeout
	if wait == 0 {
		wait = DefaultAcquireTimeout
	}
	if wait < MinimumAcquireTimeout || wait > MaximumAcquireTimeout {
		return normalizedConfig{}, ErrInvalidConfiguration
	}

	entropy := config.Entropy
	if entropy == nil {
		entropy = rand.Reader
	} else if isNilInterface(entropy) {
		return normalizedConfig{}, ErrInvalidConfiguration
	}

	return normalizedConfig{
		maxConcurrent:  maximum,
		acquireTimeout: wait,
		entropy:        entropy,
	}, nil
}

func isNilInterface(value any) bool {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type workGate struct {
	slots chan struct{}
}

func newWorkGate(capacity int) *workGate {
	return &workGate{slots: make(chan struct{}, capacity)}
}

func (g *workGate) acquire(ctx context.Context, wait time.Duration) error {
	if g == nil || ctx == nil || wait < MinimumAcquireTimeout || wait > MaximumAcquireTimeout {
		return ErrInvalidConfiguration
	}
	if err := ctx.Err(); err != nil {
		return hashingUnavailable(err)
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case g.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return hashingUnavailable(ctx.Err())
	case <-timer.C:
		return ErrHashingUnavailable
	}
}

func (g *workGate) release() {
	<-g.slots
}

var processWorkGate struct {
	sync.Mutex
	gate     *workGate
	capacity int
}

// sharedWorkGate makes the first valid process configuration authoritative.
// Later constructors share it; a conflicting capacity fails startup instead of
// silently creating another semaphore and bypassing the process budget.
func sharedWorkGate(capacity int) (*workGate, error) {
	processWorkGate.Lock()
	defer processWorkGate.Unlock()

	if processWorkGate.gate == nil {
		processWorkGate.gate = newWorkGate(capacity)
		processWorkGate.capacity = capacity
		return processWorkGate.gate, nil
	}
	if processWorkGate.capacity != capacity {
		return nil, ErrInvalidConfiguration
	}
	return processWorkGate.gate, nil
}
