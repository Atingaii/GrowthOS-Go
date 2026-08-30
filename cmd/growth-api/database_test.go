package main

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/application"
	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestComposeRuntimeSharesOnePoolAcrossReadinessRepositoryAndOwnership(t *testing.T) {
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	database := sqlx.NewDb(sqlDatabase, "mysql")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT strategy_id, name FROM lottery_strategy WHERE strategy_id = ?",
	)).WithArgs(uint64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"strategy_id", "name"}).AddRow(uint64(42), "Composed strategy"),
	)
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT award_id, name, weight, outcome FROM lottery_strategy_award WHERE strategy_id = ? ORDER BY award_id",
	)).WithArgs(uint64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).
			AddRow(uint64(7), "Only award", uint64(1), "reward"),
	)
	mock.ExpectCommit()
	mock.ExpectClose()

	components, err := composeRuntime(context.Background(), database, nil, runtimeConfiguration{}, nil)
	if err != nil {
		t.Fatalf("compose runtime: %v", err)
	}
	if components.database != database {
		t.Fatal("runtime readiness/ownership handle is not the pool supplied to Repository composition")
	}
	selection, err := components.selection.Select(context.Background(), domain.StrategyID(42))
	if err != nil {
		t.Fatalf("select through composed Repository: %v", err)
	}
	if selection.Strategy.ID() != 42 || selection.Award.ID() != 7 {
		t.Fatalf("selection = Strategy %d Award %d, want 42/7", selection.Strategy.ID(), selection.Award.ID())
	}
	if err := components.database.Close(); err != nil {
		t.Fatalf("close composed pool: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("runtime did not use and close exactly the supplied pool: %v", err)
	}
}

func TestComposeRuntimeDecoratesRepositoryOnlyWhenCacheIsEnabled(t *testing.T) {
	sqlDatabase, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create SQL mock: %v", err)
	}
	database := sqlx.NewDb(sqlDatabase, "mysql")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT strategy_id, name FROM lottery_strategy WHERE strategy_id = ?",
	)).WithArgs(uint64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"strategy_id", "name"}).AddRow(uint64(42), "Cached composition"),
	)
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT award_id, name, weight, outcome FROM lottery_strategy_award WHERE strategy_id = ? ORDER BY award_id",
	)).WithArgs(uint64(42)).WillReturnRows(
		sqlmock.NewRows([]string{"award_id", "name", "weight", "outcome"}).
			AddRow(uint64(7), "Only award", uint64(1), "reward"),
	)
	mock.ExpectCommit()
	mock.ExpectClose()

	cache := newStubCacheRuntime()
	config := runtimeConfiguration{
		Environment: appconfig.EnvironmentTest,
		StrategyCache: appconfig.StrategyCacheConfig{
			Enabled:       true,
			TTL:           5 * time.Minute,
			LookupTimeout: 75 * time.Millisecond,
			WriteTimeout:  75 * time.Millisecond,
			FillTimeout:   2 * time.Second,
		},
	}
	components, err := composeRuntime(context.Background(), database, cache, config, nil)
	if err != nil {
		t.Fatalf("compose cached runtime: %v", err)
	}
	if components.cache != cache {
		t.Fatal("runtime did not retain ownership of the supplied cache client")
	}

	for invocation := 0; invocation < 2; invocation++ {
		selection, err := components.selection.Select(context.Background(), 42)
		if err != nil {
			t.Fatalf("selection %d: %v", invocation, err)
		}
		if selection.Strategy.ID() != 42 || selection.Award.ID() != 7 {
			t.Fatalf("selection %d = Strategy %d Award %d", invocation, selection.Strategy.ID(), selection.Award.ID())
		}
	}
	if cache.setCalls != 1 {
		t.Fatalf("cache SET calls = %d, want one cold fill", cache.setCalls)
	}
	if cache.getCalls != 2 {
		t.Fatalf("cache GETRANGE calls = %d, want one per invocation", cache.getCalls)
	}
	if err := components.cache.Close(); err != nil {
		t.Fatalf("close cache: %v", err)
	}
	if err := components.database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("cache did not prevent the second authoritative read: %v", err)
	}
}

func TestComposeRuntimeRejectsCacheOwnershipMismatch(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		cache   strategyCacheRuntime
		want    error
	}{
		{name: "enabled without cache", enabled: true, want: errStrategyCacheRuntimeRequired},
		{name: "disabled with cache", cache: newStubCacheRuntime(), want: errUnexpectedStrategyCacheRuntime},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sqlDatabase, _, err := sqlmock.New()
			if err != nil {
				t.Fatalf("create SQL mock: %v", err)
			}
			database := sqlx.NewDb(sqlDatabase, "mysql")
			components, err := composeRuntime(context.Background(), database, test.cache, runtimeConfiguration{
				Environment: appconfig.EnvironmentDevelopment,
				StrategyCache: appconfig.StrategyCacheConfig{
					Enabled: test.enabled,
				},
			}, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("composeRuntime() error = %v, want %v", err, test.want)
			}
			if components != (runtimeComponents{}) {
				t.Fatalf("partial runtime = %#v, want zero", components)
			}
			_ = database.Close()
		})
	}
}

func TestComposeRuntimeRejectsNilPoolWithoutCreatingPartialRuntime(t *testing.T) {
	components, err := composeRuntime(context.Background(), nil, nil, runtimeConfiguration{}, nil)
	if !errors.Is(err, application.ErrRepositoryNotConfigured) {
		t.Fatalf("composeRuntime(nil) error = %v, want repository not configured", err)
	}
	if components.database != nil || components.cache != nil || components.selection != nil {
		t.Fatalf("partial runtime = %#v, want zero", components)
	}
}

type stubCacheRuntime struct {
	mu          sync.Mutex
	values      map[string][]byte
	getCalls    int
	setCalls    int
	deleteCalls int
	closeCalls  int
	closeErr    error
}

func newStubCacheRuntime() *stubCacheRuntime {
	return &stubCacheRuntime{values: make(map[string][]byte)}
}

func (cache *stubCacheRuntime) GetRange(_ context.Context, key string, start, end int64) ([]byte, bool, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.getCalls++
	value, found := cache.values[key]
	if !found {
		return nil, false, nil
	}
	if start >= int64(len(value)) {
		return []byte{}, true, nil
	}
	last := end + 1
	if last > int64(len(value)) {
		last = int64(len(value))
	}
	return append([]byte(nil), value[start:last]...), true, nil
}

func (cache *stubCacheRuntime) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.setCalls++
	cache.values[key] = append([]byte(nil), value...)
	return nil
}

func (cache *stubCacheRuntime) Del(_ context.Context, key string) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.deleteCalls++
	delete(cache.values, key)
	return nil
}

func (cache *stubCacheRuntime) Close() error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.closeCalls++
	return cache.closeErr
}
