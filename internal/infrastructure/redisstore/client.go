package redisstore

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

const maximumKeyBytes = 512

var (
	errClientUnavailable = errors.New("redis client is unavailable")
	errNilContext        = errors.New("redis context is nil")
	errInvalidKey        = errors.New("redis key is invalid")
	errInvalidRange      = errors.New("redis range is invalid")
	errInvalidPayload    = errors.New("redis payload is invalid")
	errInvalidExpiration = errors.New("redis expiration is invalid")
)

type commandClient interface {
	GetRange(ctx context.Context, key string, start, end int64) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Close() error
}

// Client is the narrow Redis resource shared by cache adapters. It deliberately
// exposes only the commands needed by the first Strategy projection cache.
type Client struct {
	commands  commandClient
	closeOnce sync.Once
	closeErr  error
}

// Open validates local configuration and constructs the Redis pool without
// issuing a PING or synchronously proving Redis availability. With the default
// MinIdleConnections=0, dialing is deferred until a command. A positive value
// allows go-redis to attempt background pool prewarming; failures from that
// prewarming are not returned by Open and never make Redis a startup or
// readiness authority. Ownership of the returned Client transfers to caller.
func Open(cfg Config) (*Client, error) {
	options, err := redisOptions(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{commands: redis.NewClient(options)}, nil
}

// GetRange returns at most the requested byte range. A missing key is a normal
// cache outcome represented by found=false and err=nil; all operational errors
// remain inspectable behind a safely rendered Error.
func (c *Client) GetRange(
	ctx context.Context,
	key string,
	start int64,
	end int64,
) (payload []byte, found bool, err error) {
	if ctx == nil {
		return nil, false, newError(StageGetRange, errNilContext)
	}
	if c == nil || c.commands == nil {
		return nil, false, newError(StageGetRange, errClientUnavailable)
	}
	if !validKey(key) {
		return nil, false, newError(StageGetRange, errInvalidKey)
	}
	if start < 0 || end < start {
		return nil, false, newError(StageGetRange, errInvalidRange)
	}

	payload, err = c.commands.GetRange(ctx, key, start, end).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, newError(StageGetRange, err)
	}
	return payload, true, nil
}

// Set stores one cache payload and its expiration atomically. Persistent cache
// entries and empty payloads are rejected at this boundary.
func (c *Client) Set(
	ctx context.Context,
	key string,
	payload []byte,
	expiration time.Duration,
) error {
	if ctx == nil {
		return newError(StageSet, errNilContext)
	}
	if c == nil || c.commands == nil {
		return newError(StageSet, errClientUnavailable)
	}
	if !validKey(key) {
		return newError(StageSet, errInvalidKey)
	}
	if len(payload) == 0 {
		return newError(StageSet, errInvalidPayload)
	}
	if expiration <= 0 {
		return newError(StageSet, errInvalidExpiration)
	}
	if err := c.commands.Set(ctx, key, payload, expiration).Err(); err != nil {
		return newError(StageSet, err)
	}
	return nil
}

// Del removes exactly one key. It is intentionally not variadic so a caller
// cannot turn corruption repair into an accidental namespace-wide operation.
func (c *Client) Del(ctx context.Context, key string) error {
	if ctx == nil {
		return newError(StageDelete, errNilContext)
	}
	if c == nil || c.commands == nil {
		return newError(StageDelete, errClientUnavailable)
	}
	if !validKey(key) {
		return newError(StageDelete, errInvalidKey)
	}
	if err := c.commands.Del(ctx, key).Err(); err != nil {
		return newError(StageDelete, err)
	}
	return nil
}

// Close releases every pooled connection. It is idempotent so partial startup
// cleanup and normal shutdown can share one ownership path safely.
func (c *Client) Close() error {
	if c == nil || c.commands == nil {
		return newError(StageClose, errClientUnavailable)
	}
	c.closeOnce.Do(func() {
		if err := c.commands.Close(); err != nil {
			c.closeErr = newError(StageClose, err)
		}
	})
	return c.closeErr
}

func validKey(key string) bool {
	if key == "" || len(key) > maximumKeyBytes || !utf8.ValidString(key) {
		return false
	}
	for _, character := range key {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
