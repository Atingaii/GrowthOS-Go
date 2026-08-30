// Package strategycache decorates the authoritative Lottery Strategy reader
// with a best-effort, rebuildable cache projection.
package strategycache

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"strconv"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

const (
	// MaximumProjectionBytes is the largest encoded Strategy projection accepted
	// from or written to the cache. GetRange callers pass this value as the
	// inclusive end offset, so an oversized value returns one extra byte and is
	// rejected without loading the entire Redis value.
	MaximumProjectionBytes int64 = 2 << 20

	DefaultNamespace     = "growthos:development"
	DefaultTTL           = 5 * time.Minute
	DefaultLookupTimeout = 75 * time.Millisecond
	DefaultWriteTimeout  = 75 * time.Millisecond
	DefaultFillTimeout   = 2 * time.Second

	maximumTTL           = 5 * time.Minute
	maximumLookupTimeout = time.Second
	maximumWriteTimeout  = time.Second
	maximumFillTimeout   = 30 * time.Second
	strategyKeySuffix    = ":lottery:strategy:projection:v1:"
)

var (
	ErrReaderNotConfigured = errors.New("lottery strategy cache: reader is not configured")
	ErrOptionsInvalid      = errors.New("lottery strategy cache: options are invalid")

	errNilContext = errors.New("lottery strategy cache: context is nil")
)

// Store is the smallest cache command surface required by the Strategy
// projection. GetRange uses inclusive offsets, matching Redis GETRANGE.
// found=false with a nil error is the only cache-miss representation; an
// existing empty value must return found=true so the reader can delete it as
// corruption.
type Store interface {
	GetRange(ctx context.Context, key string, start, end int64) (value []byte, found bool, err error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// ObservationKind is a stable, low-cardinality cache outcome.
type ObservationKind string

const (
	ObservationHit         ObservationKind = "hit"
	ObservationMiss        ObservationKind = "miss"
	ObservationReadError   ObservationKind = "read_error"
	ObservationCorrupt     ObservationKind = "corrupt"
	ObservationDeleteError ObservationKind = "delete_error"
	ObservationFillLeader  ObservationKind = "fill_leader"
	ObservationFillJoined  ObservationKind = "fill_joined"
	ObservationSourceError ObservationKind = "source_error"
	ObservationWriteOK     ObservationKind = "write_ok"
	ObservationWriteError  ObservationKind = "write_error"
)

// Observation describes one cache outcome without carrying identities, keys,
// payloads, names, infrastructure errors, addresses, or credentials.
type Observation struct {
	Kind     ObservationKind
	Duration time.Duration
}

// Observer receives cache outcomes. Implementations must be concurrency-safe
// because reads and per-key fills execute concurrently.
type Observer interface {
	Observe(context.Context, Observation)
}

// ObserveFunc adapts a function to Observer.
type ObserveFunc func(context.Context, Observation)

func (f ObserveFunc) Observe(ctx context.Context, observation Observation) {
	if f != nil {
		f(ctx, observation)
	}
}

type noopObserver struct{}

func (noopObserver) Observe(context.Context, Observation) {}

// JitterFunc returns a duration in [0, upperInclusive]. Reader defensively
// clamps an out-of-contract implementation, so cache correctness never relies
// on an operational random source.
type JitterFunc func(upperInclusive time.Duration) time.Duration

// Options controls only cache policy. Authoritative read timeouts remain owned
// by the source StrategyReader and its caller.
type Options struct {
	Namespace     string
	TTL           time.Duration
	LookupTimeout time.Duration
	WriteTimeout  time.Duration
	FillTimeout   time.Duration
	Lifecycle     context.Context
	Observer      Observer
	Jitter        JitterFunc
}

// Reader implements cache-aside around an authoritative StrategyReader. Cache
// errors never replace source semantics; caller cancellation still wins.
type Reader struct {
	source        application.StrategyReader
	store         Store
	namespace     string
	ttl           time.Duration
	lookupTimeout time.Duration
	writeTimeout  time.Duration
	fillTimeout   time.Duration
	lifecycle     context.Context
	observer      Observer
	jitter        JitterFunc
	flights       flightGroup
}

var _ application.StrategyReader = (*Reader)(nil)

// New constructs a StrategyReader cache decorator. To run without Redis, the
// composition root must inject the source directly rather than a nil Store.
func New(source application.StrategyReader, store Store, options Options) (*Reader, error) {
	if dependencyIsNil(source) || dependencyIsNil(store) {
		return nil, ErrReaderNotConfigured
	}

	normalized, err := normalizeOptions(options)
	if err != nil {
		return nil, err
	}
	return &Reader{
		source:        source,
		store:         store,
		namespace:     normalized.Namespace,
		ttl:           normalized.TTL,
		lookupTimeout: normalized.LookupTimeout,
		writeTimeout:  normalized.WriteTimeout,
		fillTimeout:   normalized.FillTimeout,
		lifecycle:     normalized.Lifecycle,
		observer:      normalized.Observer,
		jitter:        normalized.Jitter,
	}, nil
}

// Validate lets composition fail before serving traffic when required local
// dependencies were omitted. It does not probe Redis: cache availability is
// deliberately not a startup or readiness requirement.
func (r *Reader) Validate() error {
	if r == nil || dependencyIsNil(r.source) || dependencyIsNil(r.store) ||
		!validNamespace(r.namespace) || r.ttl <= 0 || r.lookupTimeout <= 0 || r.writeTimeout <= 0 ||
		r.fillTimeout <= 0 || dependencyIsNil(r.lifecycle) || dependencyIsNil(r.observer) || r.jitter == nil {
		return ErrReaderNotConfigured
	}
	return nil
}

// FindByID loads a domain-validated projection on a hit. A miss, malformed
// value, oversized value, or cache dependency failure falls back to the
// authoritative source. Successful source reads are returned even if cache
// encoding or write-back fails.
func (r *Reader) FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
	if ctx == nil {
		return domain.Strategy{}, application.WrapRepositoryError(application.ErrRepositoryInvalidArgument, errNilContext)
	}
	if err := r.Validate(); err != nil {
		return domain.Strategy{}, application.WrapRepositoryError(application.ErrRepositoryNotConfigured, err)
	}
	if id == 0 {
		return domain.Strategy{}, domain.ErrStrategyIDRequired
	}
	if err := ctx.Err(); err != nil {
		return domain.Strategy{}, err
	}

	key := strategyKey(r.namespace, id)
	strategy, hit, err := r.read(ctx, key, id)
	if err != nil {
		return domain.Strategy{}, err
	}
	if hit {
		if err := ctx.Err(); err != nil {
			return domain.Strategy{}, err
		}
		return strategy, nil
	}
	if err := ctx.Err(); err != nil {
		return domain.Strategy{}, err
	}

	strategy, err, joined := r.flights.do(ctx, r.lifecycle, key, r.fillTimeout, func(fillContext context.Context) (domain.Strategy, error) {
		return r.fill(fillContext, key, id)
	})
	if joined {
		r.observe(ctx, ObservationFillJoined, 0)
	} else {
		r.observe(ctx, ObservationFillLeader, 0)
	}
	if contextError := ctx.Err(); contextError != nil {
		return domain.Strategy{}, contextError
	}
	return strategy, err
}

func (r *Reader) read(ctx context.Context, key string, id domain.StrategyID) (domain.Strategy, bool, error) {
	started := time.Now()
	operationContext, cancel := context.WithTimeout(ctx, r.lookupTimeout)
	value, found, err := r.store.GetRange(operationContext, key, 0, MaximumProjectionBytes)
	cancel()
	duration := time.Since(started)

	if contextError := ctx.Err(); contextError != nil {
		return domain.Strategy{}, false, contextError
	}
	if err != nil {
		r.observe(ctx, ObservationReadError, duration)
		return domain.Strategy{}, false, nil
	}
	if !found {
		r.observe(ctx, ObservationMiss, duration)
		return domain.Strategy{}, false, nil
	}
	if len(value) == 0 || int64(len(value)) > MaximumProjectionBytes {
		r.observe(ctx, ObservationCorrupt, duration)
		r.deleteCorrupt(ctx, key, id)
		return domain.Strategy{}, false, nil
	}

	strategy, err := decodeProjection(value)
	if err != nil || strategy.ID() != id {
		r.observe(ctx, ObservationCorrupt, duration)
		r.deleteCorrupt(ctx, key, id)
		return domain.Strategy{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return domain.Strategy{}, false, err
	}
	r.observe(ctx, ObservationHit, duration)
	return strategy, true, nil
}

func (r *Reader) deleteCorrupt(ctx context.Context, key string, id domain.StrategyID) {
	if ctx.Err() != nil {
		return
	}
	started := time.Now()
	operationContext, cancel := context.WithTimeout(ctx, r.writeTimeout)
	err := r.store.Del(operationContext, key)
	cancel()
	if err != nil && ctx.Err() == nil {
		r.observe(ctx, ObservationDeleteError, time.Since(started))
	}
}

func (r *Reader) fill(ctx context.Context, key string, id domain.StrategyID) (domain.Strategy, error) {
	started := time.Now()
	strategy, err := r.source.FindByID(ctx, id)
	if contextError := ctx.Err(); contextError != nil {
		r.observe(ctx, ObservationSourceError, time.Since(started))
		return domain.Strategy{}, contextError
	}
	if err != nil {
		r.observe(ctx, ObservationSourceError, time.Since(started))
		return domain.Strategy{}, err
	}

	value, encodeErr := encodeProjection(strategy)
	if encodeErr != nil || strategy.ID() != id {
		// The cache layer must not invent a new source error contract. Existing
		// application validation remains responsible for a broken source result.
		r.observe(ctx, ObservationWriteError, time.Since(started))
		return strategy, nil
	}

	operationContext, cancel := context.WithTimeout(ctx, r.writeTimeout)
	writeStarted := time.Now()
	err = r.store.Set(operationContext, key, value, r.jitteredTTL())
	cancel()
	if err != nil {
		r.observe(ctx, ObservationWriteError, time.Since(writeStarted))
		return strategy, nil
	}
	r.observe(ctx, ObservationWriteOK, time.Since(writeStarted))
	return strategy, nil
}

func (r *Reader) jitteredTTL() time.Duration {
	window := r.ttl / 10
	if window <= 0 {
		return r.ttl
	}
	jitter := r.jitter(window)
	if jitter < 0 {
		jitter = 0
	}
	if jitter > window {
		jitter = window
	}
	return r.ttl - jitter
}

func (r *Reader) observe(ctx context.Context, kind ObservationKind, duration time.Duration) {
	r.observer.Observe(ctx, Observation{Kind: kind, Duration: duration})
}

func normalizeOptions(options Options) (Options, error) {
	if options.Namespace == "" {
		options.Namespace = DefaultNamespace
	}
	if options.TTL == 0 {
		options.TTL = DefaultTTL
	}
	if options.LookupTimeout == 0 {
		options.LookupTimeout = DefaultLookupTimeout
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = DefaultWriteTimeout
	}
	if options.FillTimeout == 0 {
		options.FillTimeout = DefaultFillTimeout
	}
	if options.Lifecycle == nil {
		options.Lifecycle = context.Background()
	} else if dependencyIsNil(options.Lifecycle) {
		return Options{}, ErrOptionsInvalid
	}
	if options.Observer == nil {
		options.Observer = noopObserver{}
	} else if dependencyIsNil(options.Observer) {
		return Options{}, ErrOptionsInvalid
	}
	if options.Jitter == nil {
		options.Jitter = defaultJitter
	}

	if !validNamespace(options.Namespace) ||
		options.TTL < time.Millisecond || options.TTL > maximumTTL ||
		options.LookupTimeout < time.Millisecond || options.LookupTimeout > maximumLookupTimeout ||
		options.WriteTimeout < time.Millisecond || options.WriteTimeout > maximumWriteTimeout ||
		options.FillTimeout < time.Millisecond || options.FillTimeout > maximumFillTimeout {
		return Options{}, ErrOptionsInvalid
	}
	return options, nil
}

func defaultJitter(upperInclusive time.Duration) time.Duration {
	if upperInclusive <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(upperInclusive) + 1))
}

func strategyKey(namespace string, id domain.StrategyID) string {
	return namespace + strategyKeySuffix + strconv.FormatUint(uint64(id), 10)
}

func validNamespace(namespace string) bool {
	switch namespace {
	case "growthos:development", "growthos:test", "growthos:staging", "growthos:production":
		return true
	default:
		return false
	}
}

func dependencyIsNil(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
