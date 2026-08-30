package redisstore

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestOpenConstructsLazyClientWithoutPing(t *testing.T) {
	t.Parallel()

	client, err := Open(Config{
		Address:  "127.0.0.1:1",
		Username: "growthos_cache",
		Password: "not-logged-password",
	})
	if err != nil {
		t.Fatalf("Open() contacted an unavailable Redis endpoint: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestClientGetRangeDistinguishesHitMissAndFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    *redis.StringCmd
		want      string
		wantFound bool
		wantCause error
	}{
		{name: "hit", result: redis.NewStringResult(`{"format":1}`, nil), want: `{"format":1}`, wantFound: true},
		{name: "empty value is still a hit", result: redis.NewStringResult("", nil), wantFound: true},
		{name: "miss", result: redis.NewStringResult("", redis.Nil)},
		{name: "failure", result: redis.NewStringResult("", context.DeadlineExceeded), wantCause: context.DeadlineExceeded},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			commands := &stubCommands{getRangeResult: test.result}
			client := &Client{commands: commands}
			payload, found, err := client.GetRange(context.Background(), "growthos:test:key", 0, 1024)
			if test.wantCause != nil {
				var safeErr *Error
				if !errors.As(err, &safeErr) || safeErr.Stage() != StageGetRange || !errors.Is(err, test.wantCause) {
					t.Fatalf("GetRange() error = %v, want safe wrapped cause", err)
				}
				if found || payload != nil {
					t.Fatalf("failure returned payload=%q found=%v", payload, found)
				}
				return
			}
			if err != nil || found != test.wantFound || string(payload) != test.want {
				t.Fatalf("GetRange() = %q,%v,%v, want %q,%v,nil", payload, found, err, test.want, test.wantFound)
			}
			if commands.getRangeKey != "growthos:test:key" || commands.getRangeStart != 0 || commands.getRangeEnd != 1024 {
				t.Fatal("GetRange() did not preserve the exact bounded command")
			}
		})
	}
}

func TestClientSetPreservesPayloadAndAtomicExpiration(t *testing.T) {
	t.Parallel()

	commands := &stubCommands{setResult: redis.NewStatusResult("OK", nil)}
	client := &Client{commands: commands}
	payload := []byte(`{"format":1}`)
	if err := client.Set(context.Background(), "growthos:test:key", payload, 3*time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if commands.setKey != "growthos:test:key" || commands.setExpiration != 3*time.Minute {
		t.Fatal("Set() did not preserve key and expiration")
	}
	stored, ok := commands.setValue.([]byte)
	if !ok || string(stored) != string(payload) {
		t.Fatalf("Set() value = %#v, want payload bytes", commands.setValue)
	}
}

func TestClientDelRemovesExactlyOneKey(t *testing.T) {
	t.Parallel()

	commands := &stubCommands{delResult: redis.NewIntResult(1, nil)}
	client := &Client{commands: commands}
	if err := client.Del(context.Background(), "growthos:test:key"); err != nil {
		t.Fatalf("Del() error = %v", err)
	}
	if len(commands.delKeys) != 1 || commands.delKeys[0] != "growthos:test:key" {
		t.Fatalf("Del() keys = %#v, want one exact key", commands.delKeys)
	}
}

func TestClientCommandsRejectInvalidCallsBeforeRedis(t *testing.T) {
	t.Parallel()

	commands := &stubCommands{}
	client := &Client{commands: commands}
	longKey := strings.Repeat("k", maximumKeyBytes+1)
	tests := []struct {
		name  string
		stage Stage
		call  func() error
	}{
		{name: "get nil context", stage: StageGetRange, call: func() error { _, _, err := client.GetRange(nil, "key", 0, 1); return err }},
		{name: "get invalid key", stage: StageGetRange, call: func() error { _, _, err := client.GetRange(context.Background(), "bad key", 0, 1); return err }},
		{name: "get oversized key", stage: StageGetRange, call: func() error { _, _, err := client.GetRange(context.Background(), longKey, 0, 1); return err }},
		{name: "get invalid range", stage: StageGetRange, call: func() error { _, _, err := client.GetRange(context.Background(), "key", 2, 1); return err }},
		{name: "set nil context", stage: StageSet, call: func() error { return client.Set(nil, "key", []byte("value"), time.Second) }},
		{name: "set empty payload", stage: StageSet, call: func() error { return client.Set(context.Background(), "key", nil, time.Second) }},
		{name: "set persistent ttl", stage: StageSet, call: func() error { return client.Set(context.Background(), "key", []byte("value"), 0) }},
		{name: "delete invalid key", stage: StageDelete, call: func() error { return client.Del(context.Background(), "bad\nkey") }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			var safeErr *Error
			if !errors.As(err, &safeErr) || safeErr.Stage() != test.stage {
				t.Fatalf("error = %v, want stage %s", err, test.stage)
			}
		})
	}
	if commands.getRangeCalls != 0 || commands.setCalls != 0 || commands.delCalls != 0 {
		t.Fatalf("invalid calls reached Redis: get=%d set=%d del=%d", commands.getRangeCalls, commands.setCalls, commands.delCalls)
	}
}

func TestClientCommandFailuresRenderOnlyStableStage(t *testing.T) {
	t.Parallel()

	secretCause := errors.New("redis://user:password@customer-cache/private-key")
	commands := &stubCommands{
		setResult: redis.NewStatusResult("", secretCause),
		delResult: redis.NewIntResult(0, secretCause),
	}
	client := &Client{commands: commands}
	tests := []struct {
		stage Stage
		call  func() error
	}{
		{stage: StageSet, call: func() error {
			return client.Set(context.Background(), "growthos:test:key", []byte("secret-payload"), time.Second)
		}},
		{stage: StageDelete, call: func() error { return client.Del(context.Background(), "growthos:test:key") }},
	}
	for _, test := range tests {
		err := test.call()
		if !errors.Is(err, secretCause) || err.Error() != string(test.stage) {
			t.Fatalf("error = %v, want safe stage with inspectable cause", err)
		}
		for _, forbidden := range []string{"password", "customer-cache", "private-key", "secret-payload"} {
			if strings.Contains(err.Error(), forbidden) {
				t.Fatalf("error leaked %q: %q", forbidden, err)
			}
		}
	}
}

func TestClientCloseIsConcurrentAndIdempotent(t *testing.T) {
	t.Parallel()

	commands := &stubCommands{}
	client := &Client{commands: commands}
	const callers = 32
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			if err := client.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		}()
	}
	wait.Wait()
	if got := commands.closeCalls.Load(); got != 1 {
		t.Fatalf("underlying Close calls = %d, want 1", got)
	}
}

type stubCommands struct {
	getRangeResult *redis.StringCmd
	setResult      *redis.StatusCmd
	delResult      *redis.IntCmd

	getRangeKey   string
	getRangeStart int64
	getRangeEnd   int64
	getRangeCalls int
	setKey        string
	setValue      any
	setExpiration time.Duration
	setCalls      int
	delKeys       []string
	delCalls      int
	closeCalls    atomic.Int64
}

func (s *stubCommands) GetRange(_ context.Context, key string, start, end int64) *redis.StringCmd {
	s.getRangeCalls++
	s.getRangeKey = key
	s.getRangeStart = start
	s.getRangeEnd = end
	if s.getRangeResult == nil {
		return redis.NewStringResult("", nil)
	}
	return s.getRangeResult
}

func (s *stubCommands) Set(_ context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	s.setCalls++
	s.setKey = key
	s.setValue = value
	s.setExpiration = expiration
	if s.setResult == nil {
		return redis.NewStatusResult("OK", nil)
	}
	return s.setResult
}

func (s *stubCommands) Del(_ context.Context, keys ...string) *redis.IntCmd {
	s.delCalls++
	s.delKeys = append([]string(nil), keys...)
	if s.delResult == nil {
		return redis.NewIntResult(0, nil)
	}
	return s.delResult
}

func (s *stubCommands) Close() error {
	s.closeCalls.Add(1)
	return nil
}
