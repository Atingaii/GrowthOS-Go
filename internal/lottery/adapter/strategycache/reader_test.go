package strategycache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestNewReaderValidatesDependenciesAndOptions(t *testing.T) {
	source := readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		return domain.Strategy{}, nil
	})
	store := newMemoryStore()

	reader, err := New(source, store, Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := reader.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if reader.namespace != DefaultNamespace || reader.ttl != DefaultTTL ||
		reader.lookupTimeout != DefaultLookupTimeout || reader.writeTimeout != DefaultWriteTimeout ||
		reader.fillTimeout != DefaultFillTimeout {
		t.Fatalf("default reader options = %#v", reader)
	}

	var nilStore *memoryStore
	if _, err := New(source, nilStore, Options{}); !errors.Is(err, ErrReaderNotConfigured) {
		t.Fatalf("New(typed nil store) error = %v", err)
	}
	var nilSource readerFunc
	if _, err := New(nilSource, store, Options{}); !errors.Is(err, ErrReaderNotConfigured) {
		t.Fatalf("New(typed nil source) error = %v", err)
	}
	var nilObserver *recordingObserver
	if _, err := New(source, store, Options{Observer: nilObserver}); !errors.Is(err, ErrOptionsInvalid) {
		t.Fatalf("New(typed nil observer) error = %v", err)
	}

	invalidOptions := []Options{
		{Namespace: "GrowthOS"},
		{Namespace: "growthos:*"},
		{Namespace: "growthos:preview"},
		{TTL: -time.Second},
		{TTL: 5*time.Minute + time.Nanosecond},
		{LookupTimeout: -time.Second},
		{LookupTimeout: 2 * time.Second},
		{WriteTimeout: -time.Second},
		{WriteTimeout: 2 * time.Second},
		{FillTimeout: -time.Second},
		{FillTimeout: time.Minute},
	}
	for _, options := range invalidOptions {
		if _, err := New(source, store, options); !errors.Is(err, ErrOptionsInvalid) {
			t.Errorf("New(%#v) error = %v, want options invalid", options, err)
		}
	}
}

func TestReaderCacheHitUsesBoundedRangeAndSkipsSource(t *testing.T) {
	strategy := mustStrategy(t, 42, "Cached strategy", []awardInput{
		{id: 7, name: "Cached award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	value, err := encodeProjection(strategy)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	store := newMemoryStore()
	key := strategyKey(DefaultNamespace, strategy.ID())
	store.values[key] = value
	var sourceCalls atomic.Int32
	observer := &recordingObserver{}
	reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		return domain.Strategy{}, errors.New("source must not be called")
	}), store, Options{Observer: observer})

	actual, err := reader.FindByID(context.Background(), strategy.ID())
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	assertSameStrategy(t, actual, strategy)
	if sourceCalls.Load() != 0 {
		t.Fatalf("source calls = %d, want 0", sourceCalls.Load())
	}
	getCalls, _, _ := store.snapshotCalls()
	if len(getCalls) != 1 || getCalls[0].key != key || getCalls[0].start != 0 || getCalls[0].end != MaximumProjectionBytes {
		t.Fatalf("GetRange calls = %#v", getCalls)
	}
	if !observer.contains(ObservationHit) {
		t.Fatalf("observations = %#v, want hit", observer.snapshot())
	}
}

func TestReaderMissFillsAndUsesSubtractiveTTLJitter(t *testing.T) {
	strategy := mustStrategy(t, 42, "Source strategy", []awardInput{
		{id: 7, name: "Source award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	store := newMemoryStore()
	var sourceCalls atomic.Int32
	reader := mustReader(t, readerFunc(func(_ context.Context, id domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		if id != strategy.ID() {
			return domain.Strategy{}, fmt.Errorf("unexpected id %d", id)
		}
		return strategy, nil
	}), store, Options{
		TTL: 5 * time.Minute,
		Jitter: func(upper time.Duration) time.Duration {
			if upper != 30*time.Second {
				t.Errorf("jitter upper = %s, want 30s", upper)
			}
			return upper
		},
	})

	actual, err := reader.FindByID(context.Background(), strategy.ID())
	if err != nil {
		t.Fatalf("cold FindByID() error = %v", err)
	}
	assertSameStrategy(t, actual, strategy)
	if sourceCalls.Load() != 1 {
		t.Fatalf("source calls = %d, want 1", sourceCalls.Load())
	}
	_, setCalls, _ := store.snapshotCalls()
	if len(setCalls) != 1 || setCalls[0].key != strategyKey(DefaultNamespace, strategy.ID()) || setCalls[0].ttl != 4*time.Minute+30*time.Second {
		t.Fatalf("Set calls = %#v", setCalls)
	}
	cached, err := decodeProjection(setCalls[0].value)
	if err != nil {
		t.Fatalf("decode written projection: %v", err)
	}
	assertSameStrategy(t, cached, strategy)

	actual, err = reader.FindByID(context.Background(), strategy.ID())
	if err != nil {
		t.Fatalf("warm FindByID() error = %v", err)
	}
	assertSameStrategy(t, actual, strategy)
	if sourceCalls.Load() != 1 {
		t.Fatalf("source calls after hit = %d, want 1", sourceCalls.Load())
	}
}

func TestReaderCacheFailuresNeverReplaceSuccessfulSourceRead(t *testing.T) {
	strategy := mustStrategy(t, 42, "Source strategy", []awardInput{
		{id: 7, name: "Source award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	tests := map[string]func(*memoryStore){
		"read failure":  func(store *memoryStore) { store.getErr = errors.New("redis unavailable") },
		"write failure": func(store *memoryStore) { store.setErr = errors.New("redis full") },
	}
	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			store := newMemoryStore()
			configure(store)
			reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
				return strategy, nil
			}), store, Options{})

			actual, err := reader.FindByID(context.Background(), strategy.ID())
			if err != nil {
				t.Fatalf("FindByID() error = %v", err)
			}
			assertSameStrategy(t, actual, strategy)
		})
	}
}

func TestReaderCacheOperationTimeoutFallsBackWhileCallerRemainsActive(t *testing.T) {
	strategy := mustStrategy(t, 42, "Source strategy", []awardInput{
		{id: 7, name: "Source award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	store := newMemoryStore()
	store.getRange = func(ctx context.Context, _ string, _, _ int64) ([]byte, bool, error) {
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		return strategy, nil
	}), store, Options{LookupTimeout: 20 * time.Millisecond})

	actual, err := reader.FindByID(context.Background(), strategy.ID())
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	assertSameStrategy(t, actual, strategy)
}

func TestReaderDoesNotNegativeCacheSourceErrors(t *testing.T) {
	store := newMemoryStore()
	var sourceCalls atomic.Int32
	reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		return domain.Strategy{}, application.WrapRepositoryError(application.ErrStrategyNotFound, errors.New("missing"))
	}), store, Options{})

	for attempt := 0; attempt < 2; attempt++ {
		_, err := reader.FindByID(context.Background(), 42)
		if !errors.Is(err, application.ErrStrategyNotFound) {
			t.Fatalf("attempt %d error = %v", attempt, err)
		}
	}
	if sourceCalls.Load() != 2 {
		t.Fatalf("source calls = %d, want 2", sourceCalls.Load())
	}
	_, setCalls, _ := store.snapshotCalls()
	if len(setCalls) != 0 {
		t.Fatalf("negative result was cached: %#v", setCalls)
	}
}

func TestReaderDeletesCorruptKeyPreciselyBeforeRebuilding(t *testing.T) {
	strategy := mustStrategy(t, 42, "Source strategy", []awardInput{
		{id: 7, name: "Source award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	validOther, err := encodeProjection(mustStrategy(t, 99, "Other strategy", []awardInput{
		{id: 1, name: "Other award", weight: 1, outcome: domain.AwardOutcomeReward},
	}))
	if err != nil {
		t.Fatalf("encode other fixture: %v", err)
	}
	oversized := make([]byte, MaximumProjectionBytes+1)
	for index := range oversized {
		oversized[index] = 'x'
	}
	tests := map[string][]byte{
		"empty":          {},
		"malformed":      []byte(`{"schema":`),
		"wrong identity": []byte(`{"schema":"growthos.lottery.strategy.projection","schema_version":1,"strategy":{"id":"43","name":"Wrong","awards":[{"id":"1","name":"Award","weight":"1","outcome":"reward"}]}}`),
		"oversized":      oversized,
	}

	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			store := newMemoryStore()
			targetKey := strategyKey(DefaultNamespace, strategy.ID())
			otherKey := strategyKey(DefaultNamespace, 99)
			store.values[targetKey] = append([]byte(nil), corrupt...)
			store.values[otherKey] = append([]byte(nil), validOther...)
			reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
				return strategy, nil
			}), store, Options{})

			actual, err := reader.FindByID(context.Background(), strategy.ID())
			if err != nil {
				t.Fatalf("FindByID() error = %v", err)
			}
			assertSameStrategy(t, actual, strategy)
			_, _, deleteCalls := store.snapshotCalls()
			if len(deleteCalls) != 1 || deleteCalls[0] != targetKey {
				t.Fatalf("Del calls = %#v, want exact target %q", deleteCalls, targetKey)
			}
			store.mu.Lock()
			otherAfter := append([]byte(nil), store.values[otherKey]...)
			targetAfter := append([]byte(nil), store.values[targetKey]...)
			store.mu.Unlock()
			if string(otherAfter) != string(validOther) {
				t.Fatal("precise delete changed a different cache key")
			}
			if rebuilt, err := decodeProjection(targetAfter); err != nil {
				t.Fatalf("target was not rebuilt after delete: %v", err)
			} else {
				assertSameStrategy(t, rebuilt, strategy)
			}
		})
	}
}

func TestReaderDeleteFailureDoesNotPreventCorruptValueRepair(t *testing.T) {
	strategy := mustStrategy(t, 42, "Source strategy", []awardInput{
		{id: 7, name: "Source award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	store := newMemoryStore()
	key := strategyKey(DefaultNamespace, strategy.ID())
	store.values[key] = []byte(`{"broken":true}`)
	store.deleteErr = errors.New("delete unavailable")
	observer := &recordingObserver{}
	reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		return strategy, nil
	}), store, Options{Observer: observer})

	actual, err := reader.FindByID(context.Background(), strategy.ID())
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	assertSameStrategy(t, actual, strategy)
	store.mu.Lock()
	repaired := append([]byte(nil), store.values[key]...)
	store.mu.Unlock()
	if decoded, err := decodeProjection(repaired); err != nil {
		t.Fatalf("unconditional Set did not repair value after Del failure: %v", err)
	} else {
		assertSameStrategy(t, decoded, strategy)
	}
	if !observer.contains(ObservationDeleteError) {
		t.Fatalf("observations = %#v, want delete error", observer.snapshot())
	}
}

func TestReaderDoesNotCacheMismatchedSourceIdentity(t *testing.T) {
	sourceStrategy := mustStrategy(t, 43, "Wrong strategy", []awardInput{
		{id: 7, name: "Award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	store := newMemoryStore()
	reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		return sourceStrategy, nil
	}), store, Options{})

	actual, err := reader.FindByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	assertSameStrategy(t, actual, sourceStrategy)
	_, setCalls, _ := store.snapshotCalls()
	if len(setCalls) != 0 {
		t.Fatalf("mismatched source Strategy was cached: %#v", setCalls)
	}
}

func TestReaderClampsOutOfContractJitter(t *testing.T) {
	source := readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		return domain.Strategy{}, nil
	})
	for name, test := range map[string]struct {
		jitter JitterFunc
		want   time.Duration
	}{
		"negative":  {jitter: func(time.Duration) time.Duration { return -time.Minute }, want: 5 * time.Minute},
		"too large": {jitter: func(time.Duration) time.Duration { return time.Minute }, want: 4*time.Minute + 30*time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			reader := mustReader(t, source, newMemoryStore(), Options{TTL: 5 * time.Minute, Jitter: test.jitter})
			if actual := reader.jitteredTTL(); actual != test.want {
				t.Fatalf("jitteredTTL() = %s, want %s", actual, test.want)
			}
		})
	}
}

func TestReaderCallerCancellationWinsCacheFailure(t *testing.T) {
	store := newMemoryStore()
	store.getRange = func(ctx context.Context, _ string, _, _ int64) ([]byte, bool, error) {
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	var sourceCalls atomic.Int32
	reader := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		return domain.Strategy{}, nil
	}), store, Options{LookupTimeout: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := reader.FindByID(ctx, 42)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FindByID() error = %v, want caller deadline", err)
	}
	if sourceCalls.Load() != 0 {
		t.Fatalf("source calls = %d, want 0", sourceCalls.Load())
	}
}

func TestReaderCoalescesConcurrentColdReadsPerKey(t *testing.T) {
	strategy := mustStrategy(t, 42, "Concurrent strategy", []awardInput{
		{id: 7, name: "Concurrent award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	store := newMemoryStore()
	store.getNotified = make(chan struct{}, 64)
	started := make(chan struct{})
	release := make(chan struct{})
	var sourceCalls atomic.Int32
	var startOnce sync.Once
	reader := mustReader(t, readerFunc(func(ctx context.Context, _ domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		startOnce.Do(func() { close(started) })
		select {
		case <-release:
			return strategy, nil
		case <-ctx.Done():
			return domain.Strategy{}, ctx.Err()
		}
	}), store, Options{FillTimeout: time.Second})

	const callers = 32
	results := make(chan error, callers)
	for index := 0; index < callers; index++ {
		go func() {
			actual, err := reader.FindByID(context.Background(), strategy.ID())
			if err == nil {
				assertSameStrategyConcurrent(results, actual, strategy)
				return
			}
			results <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("source fill did not start")
	}
	for index := 0; index < callers; index++ {
		select {
		case <-store.getNotified:
		case <-time.After(time.Second):
			t.Fatalf("only %d callers reached cache lookup", index)
		}
	}
	close(release)
	for index := 0; index < callers; index++ {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("caller %d error = %v", index, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("caller %d did not finish", index)
		}
	}
	if sourceCalls.Load() != 1 {
		t.Fatalf("source calls = %d, want one coalesced fill", sourceCalls.Load())
	}
}

func TestReaderRunsDifferentKeyFillsIndependently(t *testing.T) {
	strategies := map[domain.StrategyID]domain.Strategy{
		42: mustStrategy(t, 42, "First strategy", []awardInput{
			{id: 7, name: "First award", weight: 1, outcome: domain.AwardOutcomeReward},
		}),
		43: mustStrategy(t, 43, "Second strategy", []awardInput{
			{id: 8, name: "Second award", weight: 1, outcome: domain.AwardOutcomeReward},
		}),
	}
	started := make(chan domain.StrategyID, len(strategies))
	release := make(chan struct{})
	reader := mustReader(t, readerFunc(func(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
		started <- id
		select {
		case <-release:
			return strategies[id], nil
		case <-ctx.Done():
			return domain.Strategy{}, ctx.Err()
		}
	}), newMemoryStore(), Options{FillTimeout: time.Second})

	results := make(chan error, len(strategies))
	for id := range strategies {
		id := id
		go func() {
			actual, err := reader.FindByID(context.Background(), id)
			if err == nil && actual.ID() != id {
				err = fmt.Errorf("StrategyID = %d, want %d", actual.ID(), id)
			}
			results <- err
		}()
	}

	seen := make(map[domain.StrategyID]struct{}, len(strategies))
	for range strategies {
		select {
		case id := <-started:
			seen[id] = struct{}{}
		case <-time.After(time.Second):
			t.Fatal("different-key fills blocked each other")
		}
	}
	if len(seen) != len(strategies) {
		t.Fatalf("started fills = %#v, want both keys", seen)
	}
	close(release)
	for range strategies {
		if err := <-results; err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
	}
}

func TestReaderCallerCancellationDoesNotPoisonSharedFill(t *testing.T) {
	strategy := mustStrategy(t, 42, "Shared strategy", []awardInput{
		{id: 7, name: "Shared award", weight: 3, outcome: domain.AwardOutcomeReward},
	})
	store := newMemoryStore()
	started := make(chan struct{})
	release := make(chan struct{})
	var sourceCalls atomic.Int32
	reader := mustReader(t, readerFunc(func(ctx context.Context, _ domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		select {
		case <-started:
		default:
			close(started)
		}
		select {
		case <-release:
			return strategy, nil
		case <-ctx.Done():
			return domain.Strategy{}, ctx.Err()
		}
	}), store, Options{FillTimeout: time.Second})

	leaderContext, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := reader.FindByID(leaderContext, strategy.ID())
		leaderResult <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared fill did not start")
	}
	cancelLeader()
	select {
	case err := <-leaderResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled leader did not return independently")
	}

	waiterResult := make(chan struct {
		strategy domain.Strategy
		err      error
	}, 1)
	go func() {
		actual, err := reader.FindByID(context.Background(), strategy.ID())
		waiterResult <- struct {
			strategy domain.Strategy
			err      error
		}{actual, err}
	}()
	close(release)
	select {
	case result := <-waiterResult:
		if result.err != nil {
			t.Fatalf("waiter error = %v", result.err)
		}
		assertSameStrategy(t, result.strategy, strategy)
	case <-time.After(time.Second):
		t.Fatal("waiter did not receive shared fill")
	}
	if sourceCalls.Load() != 1 {
		t.Fatalf("source calls = %d, want 1", sourceCalls.Load())
	}
}

func TestReaderBoundsAbandonedFill(t *testing.T) {
	store := newMemoryStore()
	var sourceCalls atomic.Int32
	reader := mustReader(t, readerFunc(func(ctx context.Context, _ domain.StrategyID) (domain.Strategy, error) {
		sourceCalls.Add(1)
		<-ctx.Done()
		return domain.Strategy{}, ctx.Err()
	}), store, Options{FillTimeout: 20 * time.Millisecond})

	_, err := reader.FindByID(context.Background(), 42)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FindByID() error = %v, want bounded fill deadline", err)
	}
	if sourceCalls.Load() != 1 {
		t.Fatalf("source calls = %d, want 1", sourceCalls.Load())
	}
}

func TestReaderLifecycleCancellationStopsSharedFill(t *testing.T) {
	lifecycle, stopLifecycle := context.WithCancel(context.Background())
	started := make(chan struct{})
	reader := mustReader(t, readerFunc(func(ctx context.Context, _ domain.StrategyID) (domain.Strategy, error) {
		close(started)
		<-ctx.Done()
		return domain.Strategy{}, ctx.Err()
	}), newMemoryStore(), Options{Lifecycle: lifecycle, FillTimeout: time.Second})

	result := make(chan error, 1)
	go func() {
		_, err := reader.FindByID(context.Background(), 42)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("shared fill did not start")
	}
	stopLifecycle()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("FindByID() error = %v, want lifecycle cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lifecycle cancellation did not stop shared fill")
	}
}

func TestReaderRejectsInvalidArgumentsAndUnconfiguredZeroValue(t *testing.T) {
	reader := &Reader{}
	if _, err := reader.FindByID(context.Background(), 42); !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("zero Reader error = %v", err)
	}

	configured := mustReader(t, readerFunc(func(context.Context, domain.StrategyID) (domain.Strategy, error) {
		return domain.Strategy{}, nil
	}), newMemoryStore(), Options{})
	if _, err := configured.FindByID(nil, 42); !errors.Is(err, application.ErrRepositoryInvalidArgument) {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := configured.FindByID(context.Background(), 0); !errors.Is(err, domain.ErrStrategyIDRequired) {
		t.Fatalf("zero ID error = %v", err)
	}
}

type readerFunc func(context.Context, domain.StrategyID) (domain.Strategy, error)

func (f readerFunc) FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
	return f(ctx, id)
}

type getCall struct {
	key        string
	start, end int64
}

type setCall struct {
	key   string
	value []byte
	ttl   time.Duration
}

type memoryStore struct {
	mu          sync.Mutex
	values      map[string][]byte
	getCalls    []getCall
	setCalls    []setCall
	deleteCalls []string
	getErr      error
	setErr      error
	deleteErr   error
	getRange    func(context.Context, string, int64, int64) ([]byte, bool, error)
	getNotified chan struct{}
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: make(map[string][]byte)}
}

func (s *memoryStore) GetRange(ctx context.Context, key string, start, end int64) ([]byte, bool, error) {
	s.mu.Lock()
	s.getCalls = append(s.getCalls, getCall{key: key, start: start, end: end})
	getRange := s.getRange
	notify := s.getNotified
	err := s.getErr
	value, found := s.values[key]
	value = append([]byte(nil), value...)
	s.mu.Unlock()
	if notify != nil {
		notify <- struct{}{}
	}
	if getRange != nil {
		return getRange(ctx, key, start, end)
	}
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	if start < 0 || end < start || start >= int64(len(value)) {
		return []byte{}, true, nil
	}
	last := end + 1
	if last > int64(len(value)) {
		last = int64(len(value))
	}
	return append([]byte(nil), value[start:last]...), true, nil
}

func (s *memoryStore) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls = append(s.setCalls, setCall{key: key, value: append([]byte(nil), value...), ttl: ttl})
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = append([]byte(nil), value...)
	return nil
}

func (s *memoryStore) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCalls = append(s.deleteCalls, key)
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.values, key)
	return nil
}

func (s *memoryStore) snapshotCalls() ([]getCall, []setCall, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gets := append([]getCall(nil), s.getCalls...)
	sets := make([]setCall, len(s.setCalls))
	for index, call := range s.setCalls {
		sets[index] = call
		sets[index].value = append([]byte(nil), call.value...)
	}
	deletes := append([]string(nil), s.deleteCalls...)
	return gets, sets, deletes
}

type recordingObserver struct {
	mu           sync.Mutex
	observations []Observation
}

func (o *recordingObserver) Observe(_ context.Context, observation Observation) {
	o.mu.Lock()
	o.observations = append(o.observations, observation)
	o.mu.Unlock()
}

func (o *recordingObserver) snapshot() []Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Observation(nil), o.observations...)
}

func (o *recordingObserver) contains(kind ObservationKind) bool {
	for _, observation := range o.snapshot() {
		if observation.Kind == kind {
			return true
		}
	}
	return false
}

type awardInput struct {
	id      domain.AwardID
	name    string
	weight  domain.Weight
	outcome domain.AwardOutcome
}

func mustStrategy(t *testing.T, id domain.StrategyID, name string, inputs []awardInput) domain.Strategy {
	t.Helper()
	awards := make([]domain.Award, 0, len(inputs))
	for _, input := range inputs {
		award, err := domain.NewAward(input.id, input.name, input.weight, input.outcome)
		if err != nil {
			t.Fatalf("NewAward(%d): %v", input.id, err)
		}
		awards = append(awards, award)
	}
	strategy, err := domain.NewStrategy(id, name, awards)
	if err != nil {
		t.Fatalf("NewStrategy(%d): %v", id, err)
	}
	return strategy
}

func mustReader(t *testing.T, source application.StrategyReader, store Store, options Options) *Reader {
	t.Helper()
	reader, err := New(source, store, options)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return reader
}

func assertSameStrategy(t *testing.T, actual, expected domain.Strategy) {
	t.Helper()
	if actual.ID() != expected.ID() || actual.Name() != expected.Name() || actual.TotalWeight() != expected.TotalWeight() {
		t.Fatalf("Strategy header = (%d,%q,%d), want (%d,%q,%d)",
			actual.ID(), actual.Name(), actual.TotalWeight(),
			expected.ID(), expected.Name(), expected.TotalWeight())
	}
	actualAwards := actual.Awards()
	expectedAwards := expected.Awards()
	if len(actualAwards) != len(expectedAwards) {
		t.Fatalf("award count = %d, want %d", len(actualAwards), len(expectedAwards))
	}
	for index := range actualAwards {
		actualAward := actualAwards[index]
		expectedAward := expectedAwards[index]
		if actualAward.ID() != expectedAward.ID() || actualAward.Name() != expectedAward.Name() ||
			actualAward.Weight() != expectedAward.Weight() || actualAward.Outcome() != expectedAward.Outcome() {
			t.Fatalf("award %d differs: got (%d,%q,%d,%q), want (%d,%q,%d,%q)", index,
				actualAward.ID(), actualAward.Name(), actualAward.Weight(), actualAward.Outcome(),
				expectedAward.ID(), expectedAward.Name(), expectedAward.Weight(), expectedAward.Outcome())
		}
	}
}

func assertSameStrategyConcurrent(results chan<- error, actual, expected domain.Strategy) {
	if actual.ID() != expected.ID() || actual.Name() != expected.Name() || actual.TotalWeight() != expected.TotalWeight() ||
		len(actual.Awards()) != len(expected.Awards()) {
		results <- fmt.Errorf("strategy differs: got %d/%q, want %d/%q", actual.ID(), actual.Name(), expected.ID(), expected.Name())
		return
	}
	results <- nil
}
