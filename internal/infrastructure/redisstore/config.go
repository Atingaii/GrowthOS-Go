package redisstore

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

// TLSMode is intentionally closed. Opportunistic encryption is not supported:
// deployments either use their explicitly trusted internal boundary or verify
// both the Redis certificate chain and endpoint identity.
type TLSMode string

const (
	TLSDisabled       TLSMode = "disabled"
	TLSVerifyIdentity TLSMode = "verify_identity"
)

const (
	defaultAddress                     = "127.0.0.1:6379"
	defaultDialTimeout                 = time.Second
	defaultReadTimeout                 = 250 * time.Millisecond
	defaultWriteTimeout                = 250 * time.Millisecond
	defaultPoolTimeout                 = 500 * time.Millisecond
	defaultPoolSize                    = 16
	defaultConnectionMaxLifetime       = 30 * time.Minute
	defaultConnectionMaxIdleTime       = 5 * time.Minute
	maximumCABytes               int64 = 1 << 20
	maximumPasswordBytes               = 1024
	maximumUsernameRunes               = 128
	maximumDatabase                    = 255
	maximumPoolSize                    = 1000
)

const redactedConfig = "redis configuration (redacted)"

var (
	errConfigValue = errors.New("invalid redis configuration")
	errSystemRoots = errors.New("system certificate roots unavailable")
	errCARead      = errors.New("redis ca file cannot be read")
	errCAOversize  = errors.New("redis ca file exceeds size limit")
	errCAInvalid   = errors.New("redis ca file contains no certificates")
)

// Config contains the connection, transport, and bounded pool policy for one
// long-lived Redis application identity. Password is deliberately a separate
// field rather than part of a URL so diagnostics never need to parse a
// secret-bearing endpoint.
type Config struct {
	Address               string
	Username              string
	Password              string
	Database              int
	TLSMode               TLSMode
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

// These methods cover the common formatting, structured logging, and JSON
// boundaries. Operational logs should select reviewed non-secret fields rather
// than logging Config wholesale.
func (Config) String() string   { return redactedConfig }
func (Config) GoString() string { return redactedConfig }
func (Config) LogValue() slog.Value {
	return slog.StringValue(redactedConfig)
}
func (Config) MarshalJSON() ([]byte, error) { return json.Marshal(redactedConfig) }

func redisOptions(in Config) (*redis.Options, error) {
	cfg, err := normalizeConfig(in)
	if err != nil {
		return nil, newError(StageConfigInvalid, err)
	}

	options := &redis.Options{
		Network:               "tcp",
		Addr:                  cfg.Address,
		Protocol:              2,
		Username:              cfg.Username,
		Password:              cfg.Password,
		DB:                    cfg.Database,
		MaxRetries:            -1,
		DialTimeout:           cfg.DialTimeout,
		DialerRetries:         1,
		ReadTimeout:           cfg.ReadTimeout,
		WriteTimeout:          cfg.WriteTimeout,
		ContextTimeoutEnabled: true,
		PoolSize:              cfg.PoolSize,
		MaxIdleConns:          cfg.PoolSize,
		MaxActiveConns:        cfg.PoolSize,
		PoolTimeout:           cfg.PoolTimeout,
		MinIdleConns:          cfg.MinIdleConnections,
		ConnMaxLifetime:       cfg.ConnectionMaxLifetime,
		ConnMaxIdleTime:       cfg.ConnectionMaxIdleTime,
		DisableIdentity:       true,
	}
	if cfg.TLSMode == TLSVerifyIdentity {
		options.TLSConfig, err = verifiedTLSConfig(cfg.Address, cfg.TLSCAFile)
		if err != nil {
			return nil, err
		}
	}
	return options, nil
}

func normalizeConfig(in Config) (Config, error) {
	out := in
	if out.Address == "" {
		out.Address = defaultAddress
	}
	if out.TLSMode == "" {
		out.TLSMode = TLSDisabled
	}
	if out.DialTimeout == 0 {
		out.DialTimeout = defaultDialTimeout
	}
	if out.ReadTimeout == 0 {
		out.ReadTimeout = defaultReadTimeout
	}
	if out.WriteTimeout == 0 {
		out.WriteTimeout = defaultWriteTimeout
	}
	if out.PoolTimeout == 0 {
		out.PoolTimeout = defaultPoolTimeout
	}
	if out.PoolSize == 0 {
		out.PoolSize = defaultPoolSize
	}
	if out.ConnectionMaxLifetime == 0 {
		out.ConnectionMaxLifetime = defaultConnectionMaxLifetime
	}
	if out.ConnectionMaxIdleTime == 0 {
		out.ConnectionMaxIdleTime = defaultConnectionMaxIdleTime
	}

	host, rawPort, err := net.SplitHostPort(out.Address)
	if err != nil || host == "" || strings.TrimSpace(host) != host {
		return Config{}, errConfigValue
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errConfigValue
	}
	if !validPlainField(out.Username, maximumUsernameRunes) {
		return Config{}, errConfigValue
	}
	if out.Password == "" || len(out.Password) > maximumPasswordBytes {
		return Config{}, errConfigValue
	}
	if out.Database < 0 || out.Database > maximumDatabase {
		return Config{}, errConfigValue
	}
	if out.PoolSize < 1 || out.PoolSize > maximumPoolSize ||
		out.MinIdleConnections < 0 || out.MinIdleConnections > out.PoolSize {
		return Config{}, errConfigValue
	}
	for _, duration := range []struct {
		value   time.Duration
		maximum time.Duration
	}{
		{out.DialTimeout, 30 * time.Second},
		{out.ReadTimeout, time.Minute},
		{out.WriteTimeout, time.Minute},
		{out.PoolTimeout, time.Minute},
		{out.ConnectionMaxLifetime, 24 * time.Hour},
		{out.ConnectionMaxIdleTime, 24 * time.Hour},
	} {
		if duration.value <= 0 || duration.value > duration.maximum {
			return Config{}, errConfigValue
		}
	}

	switch out.TLSMode {
	case TLSDisabled:
		if out.TLSCAFile != "" {
			return Config{}, errConfigValue
		}
	case TLSVerifyIdentity:
		// An optional private CA augments the host trust store.
	default:
		return Config{}, errConfigValue
	}

	return out, nil
}

func validPlainField(value string, maxRunes int) bool {
	if value == "" || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maxRunes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !unicode.IsPrint(character) {
			return false
		}
	}
	return true
}

func verifiedTLSConfig(address, caFile string) (*tls.Config, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return nil, newError(StageConfigInvalid, errConfigValue)
	}

	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, newError(StageTLSRoots, errSystemRoots)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	if caFile != "" {
		pemBytes, readErr := readBoundedCA(caFile)
		if readErr != nil {
			return nil, readErr
		}
		if !roots.AppendCertsFromPEM(pemBytes) {
			return nil, newError(StageTLSCA, errCAInvalid)
		}
	}

	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         host,
		RootCAs:            roots,
		InsecureSkipVerify: false,
	}, nil
}

func readBoundedCA(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, newError(StageTLSCA, errCARead)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maximumCABytes+1))
	if err != nil {
		return nil, newError(StageTLSCA, errCARead)
	}
	if int64(len(contents)) > maximumCABytes {
		return nil, newError(StageTLSCA, errCAOversize)
	}
	return contents, nil
}
