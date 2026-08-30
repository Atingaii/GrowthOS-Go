package appconfig

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

const (
	lotteryStrategyCacheEnabledVariable = "GROWTHOS_LOTTERY_STRATEGY_CACHE_ENABLED"
	lotteryStrategyCacheTTLVariable     = "GROWTHOS_LOTTERY_STRATEGY_CACHE_TTL"
	lotteryStrategyCacheLookupVariable  = "GROWTHOS_LOTTERY_STRATEGY_CACHE_LOOKUP_TIMEOUT"
	lotteryStrategyCacheWriteVariable   = "GROWTHOS_LOTTERY_STRATEGY_CACHE_WRITE_TIMEOUT"
	lotteryStrategyCacheFillVariable    = "GROWTHOS_LOTTERY_STRATEGY_CACHE_FILL_TIMEOUT"
	redisAddressVariable                = "GROWTHOS_REDIS_ADDRESS"
	redisUsernameVariable               = "GROWTHOS_REDIS_USERNAME"
	redisPasswordVariable               = "GROWTHOS_REDIS_PASSWORD"
	redisPasswordFileVariable           = "GROWTHOS_REDIS_PASSWORD_FILE"
	redisDatabaseVariable               = "GROWTHOS_REDIS_DATABASE"
	redisTLSModeVariable                = "GROWTHOS_REDIS_TLS_MODE"
	redisTLSCAFileVariable              = "GROWTHOS_REDIS_TLS_CA_FILE"
	redisDialTimeoutVariable            = "GROWTHOS_REDIS_DIAL_TIMEOUT"
	redisReadTimeoutVariable            = "GROWTHOS_REDIS_READ_TIMEOUT"
	redisWriteTimeoutVariable           = "GROWTHOS_REDIS_WRITE_TIMEOUT"
	redisPoolTimeoutVariable            = "GROWTHOS_REDIS_POOL_TIMEOUT"
	redisPoolSizeVariable               = "GROWTHOS_REDIS_POOL_SIZE"
	redisMinIdleConnsVariable           = "GROWTHOS_REDIS_MIN_IDLE_CONNS"
	redisConnMaxLifetimeVariable        = "GROWTHOS_REDIS_CONN_MAX_LIFETIME"
	redisConnMaxIdleTimeVariable        = "GROWTHOS_REDIS_CONN_MAX_IDLE_TIME"

	defaultLotteryStrategyCacheTTL   = 5 * time.Minute
	defaultLotteryCacheLookupTimeout = 75 * time.Millisecond
	defaultLotteryCacheWriteTimeout  = 75 * time.Millisecond
	defaultLotteryCacheFillTimeout   = 2 * time.Second
	maximumLotteryStrategyCacheTTL   = 5 * time.Minute
	maximumLotteryCacheLookupTimeout = time.Second
	maximumLotteryCacheWriteTimeout  = time.Second
	maximumLotteryCacheFillTimeout   = 30 * time.Second
	defaultRedisAddress              = "127.0.0.1:6379"
	defaultRedisUsername             = "growthos_api"
	defaultRedisDialTimeout          = 250 * time.Millisecond
	defaultRedisReadTimeout          = 75 * time.Millisecond
	defaultRedisWriteTimeout         = 75 * time.Millisecond
	defaultRedisPoolTimeout          = 100 * time.Millisecond
	defaultRedisPoolSize             = 10
	defaultRedisMinIdleConns         = 0
	defaultRedisConnMaxLifetime      = 15 * time.Minute
	defaultRedisConnMaxIdleTime      = 5 * time.Minute
	maximumRedisOperationTimeout     = 5 * time.Second
	maximumRedisConnections          = 100
	maximumRedisDatabase             = 255
	maximumRedisConnMaxLifetime      = 24 * time.Hour
	maximumRedisConnMaxIdleTime      = time.Hour
	redactedRedisConfig              = "redis configuration (redacted)"
)

var redisUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// RedisTLSMode is intentionally closed: the client either uses an explicitly
// trusted, hostname-verified TLS connection or no TLS at all.
type RedisTLSMode string

const (
	RedisTLSDisabled       RedisTLSMode = "disabled"
	RedisTLSVerifyIdentity RedisTLSMode = "verify_identity"
)

// StrategyCacheConfig controls the Lottery-owned cache-aside policy. Redis is
// only an accelerator; MySQL remains the source of truth in every mode.
type StrategyCacheConfig struct {
	Enabled       bool
	TTL           time.Duration
	LookupTimeout time.Duration
	WriteTimeout  time.Duration
	FillTimeout   time.Duration
}

// RedisConfig contains the secret-bearing connection and pool settings used by
// the optional Lottery strategy cache.
type RedisConfig struct {
	Address               string
	Username              string
	Password              string
	Database              int
	TLSMode               RedisTLSMode
	TLSCAFile             string
	DialTimeout           time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	PoolTimeout           time.Duration
	PoolSize              int
	MinIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

func (RedisConfig) String() string   { return redactedRedisConfig }
func (RedisConfig) GoString() string { return redactedRedisConfig }
func (RedisConfig) LogValue() slog.Value {
	return slog.StringValue(redactedRedisConfig)
}
func (RedisConfig) MarshalJSON() ([]byte, error) { return json.Marshal(redactedRedisConfig) }

func defaultStrategyCacheConfig() StrategyCacheConfig {
	return StrategyCacheConfig{
		Enabled:       false,
		TTL:           defaultLotteryStrategyCacheTTL,
		LookupTimeout: defaultLotteryCacheLookupTimeout,
		WriteTimeout:  defaultLotteryCacheWriteTimeout,
		FillTimeout:   defaultLotteryCacheFillTimeout,
	}
}

func defaultRedisConfig() RedisConfig {
	return RedisConfig{
		Address:               defaultRedisAddress,
		Username:              defaultRedisUsername,
		Database:              0,
		TLSMode:               RedisTLSDisabled,
		DialTimeout:           defaultRedisDialTimeout,
		ReadTimeout:           defaultRedisReadTimeout,
		WriteTimeout:          defaultRedisWriteTimeout,
		PoolTimeout:           defaultRedisPoolTimeout,
		PoolSize:              defaultRedisPoolSize,
		MinIdleConnections:    defaultRedisMinIdleConns,
		ConnectionMaxLifetime: defaultRedisConnMaxLifetime,
		ConnectionMaxIdleTime: defaultRedisConnMaxIdleTime,
	}
}

func loadStrategyCacheAndRedis(
	lookup LookupFunc,
	environment Environment,
	environmentValid bool,
	selectionTimeout time.Duration,
	selectionTimeoutValid bool,
	cache *StrategyCacheConfig,
	redis *RedisConfig,
	problems *[]error,
) {
	enabledValid := loadBoolean(lookup, lotteryStrategyCacheEnabledVariable, &cache.Enabled, problems)
	loadDuration(lookup, lotteryStrategyCacheTTLVariable, maximumLotteryStrategyCacheTTL, &cache.TTL, problems)
	lookupValid := loadDuration(lookup, lotteryStrategyCacheLookupVariable, maximumLotteryCacheLookupTimeout, &cache.LookupTimeout, problems)
	writeValid := loadDuration(lookup, lotteryStrategyCacheWriteVariable, maximumLotteryCacheWriteTimeout, &cache.WriteTimeout, problems)
	fillValid := loadDuration(lookup, lotteryStrategyCacheFillVariable, maximumLotteryCacheFillTimeout, &cache.FillTimeout, problems)

	if value, found := suppliedValue(lookup, redisAddressVariable, problems); found {
		if validMySQLAddress(value) {
			redis.Address = value
		} else {
			*problems = append(*problems, fmt.Errorf("%s must be a valid host:port with a non-empty host and a port from 1 to 65535", redisAddressVariable))
		}
	}
	if value, found := suppliedValue(lookup, redisUsernameVariable, problems); found {
		if redisUsernamePattern.MatchString(value) {
			redis.Username = value
		} else {
			*problems = append(*problems, fmt.Errorf("%s must match [A-Za-z0-9][A-Za-z0-9._-]{0,63}", redisUsernameVariable))
		}
	}
	loadRedisPassword(lookup, cache.Enabled && enabledValid, &redis.Password, problems)
	loadInteger(lookup, redisDatabaseVariable, 0, maximumRedisDatabase, &redis.Database, problems)

	tlsModeValid := true
	if value, present := lookup(redisTLSModeVariable); present {
		if strings.TrimSpace(value) == "" {
			*problems = append(*problems, fmt.Errorf("%s must not be empty", redisTLSModeVariable))
			tlsModeValid = false
		} else {
			mode := RedisTLSMode(value)
			switch mode {
			case RedisTLSDisabled, RedisTLSVerifyIdentity:
				redis.TLSMode = mode
			default:
				tlsModeValid = false
				*problems = append(*problems, fmt.Errorf("%s must be one of disabled, verify_identity", redisTLSModeVariable))
			}
		}
	}
	loadOptionalString(lookup, redisTLSCAFileVariable, &redis.TLSCAFile, problems)
	loadDuration(lookup, redisDialTimeoutVariable, maximumRedisOperationTimeout, &redis.DialTimeout, problems)
	loadDuration(lookup, redisReadTimeoutVariable, maximumRedisOperationTimeout, &redis.ReadTimeout, problems)
	loadDuration(lookup, redisWriteTimeoutVariable, maximumRedisOperationTimeout, &redis.WriteTimeout, problems)
	loadDuration(lookup, redisPoolTimeoutVariable, maximumRedisOperationTimeout, &redis.PoolTimeout, problems)
	poolSizeValid := loadInteger(lookup, redisPoolSizeVariable, 1, maximumRedisConnections, &redis.PoolSize, problems)
	minIdleValid := loadInteger(lookup, redisMinIdleConnsVariable, 0, maximumRedisConnections, &redis.MinIdleConnections, problems)
	loadDuration(lookup, redisConnMaxLifetimeVariable, maximumRedisConnMaxLifetime, &redis.ConnectionMaxLifetime, problems)
	loadDuration(lookup, redisConnMaxIdleTimeVariable, maximumRedisConnMaxIdleTime, &redis.ConnectionMaxIdleTime, problems)

	if poolSizeValid && minIdleValid && redis.MinIdleConnections > redis.PoolSize {
		*problems = append(*problems, fmt.Errorf("%s must be no greater than %s", redisMinIdleConnsVariable, redisPoolSizeVariable))
	}
	if tlsModeValid && redis.TLSMode == RedisTLSDisabled && redis.TLSCAFile != "" {
		*problems = append(*problems, fmt.Errorf("%s must not be set when %s is disabled", redisTLSCAFileVariable, redisTLSModeVariable))
	}
	if environmentValid && enabledValid && cache.Enabled &&
		(environment == EnvironmentStaging || environment == EnvironmentProduction) &&
		redis.TLSMode != RedisTLSVerifyIdentity {
		*problems = append(*problems, fmt.Errorf("%s must be verify_identity when %s is true in staging and production", redisTLSModeVariable, lotteryStrategyCacheEnabledVariable))
	}
	if selectionTimeoutValid && lookupValid && writeValid && fillValid && cache.Enabled &&
		cache.LookupTimeout+cache.FillTimeout+cache.WriteTimeout > selectionTimeout {
		*problems = append(*problems, fmt.Errorf(
			"%s, %s, and %s together must be no greater than %s",
			lotteryStrategyCacheLookupVariable,
			lotteryStrategyCacheFillVariable,
			lotteryStrategyCacheWriteVariable,
			lotterySelectionTimeoutVariable,
		))
	}
}

func loadRedisPassword(lookup LookupFunc, required bool, destination *string, problems *[]error) {
	_, valuePresent := lookup(redisPasswordVariable)
	_, filePresent := lookup(redisPasswordFileVariable)
	if !valuePresent && !filePresent {
		if required {
			*problems = append(*problems, fmt.Errorf("exactly one of %s or %s is required when %s is true", redisPasswordVariable, redisPasswordFileVariable, lotteryStrategyCacheEnabledVariable))
		}
		return
	}
	loadRequiredPassword(lookup, redisPasswordVariable, redisPasswordFileVariable, destination, problems)
}
