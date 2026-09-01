package passwordhash

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestWorkGateCancellationDoesNotLeakGoroutines(t *testing.T) {
	gate := newWorkGate(1)
	if err := gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("occupy gate error = %v", err)
	}
	before := runtime.NumGoroutine()
	for index := 0; index < 1000; index++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := gate.acquire(ctx, time.Second); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled acquire %d error = %v", index, err)
		}
	}
	after := runtime.NumGoroutine()
	gate.release()
	if after > before+1 {
		t.Fatalf("goroutines grew from %d to %d", before, after)
	}
}

func TestWorkGateEnforcesCapacityAndWaitBudget(t *testing.T) {
	t.Parallel()

	gate := newWorkGate(2)
	if err := gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	if err := gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("second acquire error = %v", err)
	}
	started := time.Now()
	err := gate.acquire(context.Background(), 10*time.Millisecond)
	if !errors.Is(err, ErrHashingUnavailable) {
		t.Fatalf("capacity acquire error = %v", err)
	}
	if time.Since(started) < 5*time.Millisecond {
		t.Fatalf("capacity acquire did not honor a bounded wait")
	}

	gate.release()
	if err := gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("acquire after release error = %v", err)
	}
	gate.release()
	gate.release()
}

func TestWorkGateCancellationWinsWhileBlocked(t *testing.T) {
	t.Parallel()

	gate := newWorkGate(1)
	if err := gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("occupy gate error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := gate.acquire(ctx, time.Second)
	gate.release()
	if !errors.Is(err, ErrHashingUnavailable) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquire error = %v", err)
	}
}

func TestWorkGateDeadlineWinsWhileBlocked(t *testing.T) {
	t.Parallel()

	gate := newWorkGate(1)
	if err := gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("occupy gate error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := gate.acquire(ctx, time.Second)
	gate.release()
	if !errors.Is(err, ErrHashingUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline acquire error = %v", err)
	}
}
