package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type readinessProbeFunc func(context.Context) error

func (function readinessProbeFunc) PingContext(ctx context.Context) error {
	return function(ctx)
}

type typedNilReadinessProbe struct{}

func (*typedNilReadinessProbe) PingContext(context.Context) error { return nil }

func TestDualMySQLReadinessProbesBothDependenciesConcurrently(t *testing.T) {
	t.Parallel()
	started := make(chan string, 2)
	release := make(chan struct{})
	probe := func(name string) readinessProbeFunc {
		return func(ctx context.Context) error {
			started <- name
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	readiness, err := newDualMySQLReadiness(
		probe("business"),
		probe("identity"),
		500*time.Millisecond,
		500*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- readiness.PingContext(context.Background()) }()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(200 * time.Millisecond):
			t.Fatal("readiness probes ran sequentially")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
}

func TestDualMySQLReadinessFailureCancelsAndJoinsSibling(t *testing.T) {
	t.Parallel()
	siblingStarted := make(chan struct{})
	siblingReturned := make(chan struct{})
	var calls atomic.Int32
	readiness, err := newDualMySQLReadiness(
		readinessProbeFunc(func(context.Context) error {
			calls.Add(1)
			return errors.New("private business dsn")
		}),
		readinessProbeFunc(func(ctx context.Context) error {
			calls.Add(1)
			close(siblingStarted)
			<-ctx.Done()
			close(siblingReturned)
			return ctx.Err()
		}),
		time.Second,
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := readiness.PingContext(context.Background()); !errors.Is(err, errReadinessUnavailable) ||
		err.Error() != errReadinessUnavailable.Error() {
		t.Fatalf("PingContext() error = %v", err)
	}
	select {
	case <-siblingStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("identity readiness was never invoked")
	}
	select {
	case <-siblingReturned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("PingContext returned before the canceled sibling joined")
	}
	if calls.Load() != 2 {
		t.Fatalf("probe calls = %d, want 2", calls.Load())
	}
}

func TestDualMySQLReadinessEnforcesIndependentTimeout(t *testing.T) {
	t.Parallel()
	readiness, err := newDualMySQLReadiness(
		readinessProbeFunc(func(context.Context) error { return nil }),
		readinessProbeFunc(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}),
		500*time.Millisecond,
		25*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := readiness.PingContext(context.Background()); !errors.Is(err, errReadinessUnavailable) {
		t.Fatalf("PingContext() error = %v", err)
	}
	elapsed := time.Since(started)
	if elapsed < 15*time.Millisecond || elapsed > 300*time.Millisecond {
		t.Fatalf("independent timeout elapsed = %s", elapsed)
	}
}

func TestDualMySQLReadinessRejectsNilTypedNilAndInvalidDurations(t *testing.T) {
	t.Parallel()
	valid := readinessProbeFunc(func(context.Context) error { return nil })
	var typedNil *typedNilReadinessProbe
	for _, test := range []struct {
		name            string
		business        interface{ PingContext(context.Context) error }
		identity        interface{ PingContext(context.Context) error }
		businessTimeout time.Duration
		identityTimeout time.Duration
	}{
		{name: "nil business", identity: valid, businessTimeout: time.Second, identityTimeout: time.Second},
		{name: "typed nil identity", business: valid, identity: typedNil, businessTimeout: time.Second, identityTimeout: time.Second},
		{name: "zero business timeout", business: valid, identity: valid, identityTimeout: time.Second},
		{name: "negative identity timeout", business: valid, identity: valid, businessTimeout: time.Second, identityTimeout: -time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			if readiness, err := newDualMySQLReadiness(
				test.business,
				test.identity,
				test.businessTimeout,
				test.identityTimeout,
			); !errors.Is(err, errReadinessConfiguration) || readiness != nil {
				t.Fatalf("readiness=%#v err=%v", readiness, err)
			}
		})
	}
	if dualMySQLReadinessTimeout(2*time.Second, 3*time.Second) != 3*time.Second ||
		dualMySQLReadinessTimeout(4*time.Second, 3*time.Second) != 4*time.Second {
		t.Fatal("dual readiness outer timeout is not the maximum pool budget")
	}
}

func TestDualMySQLReadinessRejectsCanceledOrNilParentWithoutProbing(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	probe := readinessProbeFunc(func(context.Context) error {
		calls.Add(1)
		return nil
	})
	readiness, err := newDualMySQLReadiness(probe, probe, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, ctx} {
		if err := readiness.PingContext(ctx); !errors.Is(err, errReadinessUnavailable) {
			t.Fatalf("PingContext() error = %v", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("probe calls = %d, want 0", calls.Load())
	}
}
