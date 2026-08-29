package mysqlstore

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	drivermysql "github.com/go-sql-driver/mysql"
)

// TLSMode is intentionally closed: either transport encryption is disabled or
// the server certificate and hostname are both verified.
type TLSMode string

const (
	TLSDisabled       TLSMode = "disabled"
	TLSVerifyIdentity TLSMode = "verify_identity"
)

const (
	defaultAddress                    = "127.0.0.1:3306"
	defaultConnectTimeout             = 3 * time.Second
	defaultReadTimeout                = 5 * time.Second
	defaultMigrationReadTimeout       = 35 * time.Second
	defaultMigrationStatement         = 30 * time.Second
	defaultMigrationLock              = 40 * time.Second
	defaultWriteTimeout               = 5 * time.Second
	defaultPingTimeout                = 3 * time.Second
	defaultMaxOpen                    = 10
	defaultConnMaxLifetime            = 3 * time.Minute
	defaultConnMaxIdleTime            = time.Minute
	maximumCABytes              int64 = 1 << 20
	maximumAPIReadTimeout             = 5 * time.Minute
	maximumMigrationRead              = 10*time.Minute + 30*time.Second
	maximumMigrationStatement         = 10 * time.Minute
	maximumMigrationLock              = 11 * time.Minute
	migrationTimeoutBudget            = 5 * time.Second
)

const (
	redactedConnectionConfig = "mysql connection configuration (redacted)"
	redactedPoolConfig       = "mysql pool configuration (redacted)"
	redactedMigrationConfig  = "mysql migration configuration (redacted)"
)

var (
	errConfigValue = errors.New("invalid mysql configuration")
	errSystemRoots = errors.New("system certificate roots unavailable")
	errCARead      = errors.New("mysql ca file cannot be read")
	errCAOversize  = errors.New("mysql ca file exceeds size limit")
	errCAInvalid   = errors.New("mysql ca file contains no certificates")
)

// ConnectionConfig contains only connection and transport fields shared by
// the API and migration identities. Callers should populate it from a typed
// configuration boundary, never from a preformatted DSN.
type ConnectionConfig struct {
	Address        string
	Database       string
	User           string
	Password       string
	TLSMode        TLSMode
	TLSCAFile      string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

// Config configures the long-lived application pool.
type Config struct {
	ConnectionConfig

	PingTimeout           time.Duration
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

// MigrationConfig is a distinct type so migration credentials cannot be
// accidentally sourced from the application pool configuration.
type MigrationConfig struct {
	ConnectionConfig
	StatementTimeout time.Duration
	LockTimeout      time.Duration
}

// These methods make secret-bearing configuration safe at the most common
// formatting, structured logging, and JSON boundaries. Callers that need
// diagnostics should log selected non-secret fields explicitly.
func (ConnectionConfig) String() string   { return redactedConnectionConfig }
func (ConnectionConfig) GoString() string { return redactedConnectionConfig }
func (ConnectionConfig) LogValue() slog.Value {
	return slog.StringValue(redactedConnectionConfig)
}
func (ConnectionConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedConnectionConfig)
}

func (Config) String() string   { return redactedPoolConfig }
func (Config) GoString() string { return redactedPoolConfig }
func (Config) LogValue() slog.Value {
	return slog.StringValue(redactedPoolConfig)
}
func (Config) MarshalJSON() ([]byte, error) { return json.Marshal(redactedPoolConfig) }

func (MigrationConfig) String() string   { return redactedMigrationConfig }
func (MigrationConfig) GoString() string { return redactedMigrationConfig }
func (MigrationConfig) LogValue() slog.Value {
	return slog.StringValue(redactedMigrationConfig)
}
func (MigrationConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedMigrationConfig)
}

type normalizedPool struct {
	maxOpen     int
	maxIdle     int
	maxLifetime time.Duration
	maxIdleTime time.Duration
	pingTimeout time.Duration
}

func normalizeConnection(in ConnectionConfig, migration bool) (ConnectionConfig, error) {
	out := in
	if out.Address == "" {
		out.Address = defaultAddress
	}
	if out.TLSMode == "" {
		out.TLSMode = TLSDisabled
	}
	if out.ConnectTimeout == 0 {
		out.ConnectTimeout = defaultConnectTimeout
	}
	if out.ReadTimeout == 0 {
		out.ReadTimeout = defaultReadTimeout
		if migration {
			out.ReadTimeout = defaultMigrationReadTimeout
		}
	}
	if out.WriteTimeout == 0 {
		out.WriteTimeout = defaultWriteTimeout
	}

	host, rawPort, err := net.SplitHostPort(out.Address)
	if err != nil || host == "" {
		return ConnectionConfig{}, errConfigValue
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return ConnectionConfig{}, errConfigValue
	}
	if !validPlainField(out.User, 32) || !validPlainField(out.Database, 64) {
		return ConnectionConfig{}, errConfigValue
	}
	if out.Password == "" || len(out.Password) > 1024 {
		return ConnectionConfig{}, errConfigValue
	}
	if err := validDuration(out.ConnectTimeout, time.Minute); err != nil {
		return ConnectionConfig{}, err
	}
	maximumReadTimeout := maximumAPIReadTimeout
	if migration {
		maximumReadTimeout = maximumMigrationRead
	}
	if err := validDuration(out.ReadTimeout, maximumReadTimeout); err != nil {
		return ConnectionConfig{}, err
	}
	if err := validDuration(out.WriteTimeout, 5*time.Minute); err != nil {
		return ConnectionConfig{}, err
	}

	switch out.TLSMode {
	case TLSDisabled:
		if out.TLSCAFile != "" {
			return ConnectionConfig{}, errConfigValue
		}
	case TLSVerifyIdentity:
		// An optional CA augments, rather than replaces, the host trust store.
	default:
		return ConnectionConfig{}, errConfigValue
	}

	return out, nil
}

func normalizePool(in Config) (normalizedPool, error) {
	out := normalizedPool{
		maxOpen:     in.MaxOpenConnections,
		maxIdle:     in.MaxIdleConnections,
		maxLifetime: in.ConnectionMaxLifetime,
		maxIdleTime: in.ConnectionMaxIdleTime,
		pingTimeout: in.PingTimeout,
	}
	if out.maxOpen == 0 {
		out.maxOpen = defaultMaxOpen
	}
	if out.maxLifetime == 0 {
		out.maxLifetime = defaultConnMaxLifetime
	}
	if out.maxIdleTime == 0 {
		out.maxIdleTime = defaultConnMaxIdleTime
	}
	if out.pingTimeout == 0 {
		out.pingTimeout = defaultPingTimeout
	}

	if out.maxOpen < 1 || out.maxOpen > 1000 || out.maxIdle < 0 || out.maxIdle > out.maxOpen {
		return normalizedPool{}, errConfigValue
	}
	if err := validDuration(out.maxLifetime, 24*time.Hour); err != nil {
		return normalizedPool{}, err
	}
	if err := validDuration(out.maxIdleTime, 24*time.Hour); err != nil {
		return normalizedPool{}, err
	}
	if err := validDuration(out.pingTimeout, time.Minute); err != nil {
		return normalizedPool{}, err
	}
	return out, nil
}

func validPlainField(value string, maxRunes int) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func validDuration(value, maximum time.Duration) error {
	if value <= 0 || value > maximum {
		return errConfigValue
	}
	return nil
}

func driverConfig(in ConnectionConfig, migration bool) (*drivermysql.Config, error) {
	cfg, err := normalizeConnection(in, migration)
	if err != nil {
		return nil, newError(StageConfigInvalid, err)
	}

	driverCfg := drivermysql.NewConfig()
	// The driver's default logger writes unstructured connection errors directly
	// to stderr. That bypasses GrowthOS's JSON logging and may render raw driver
	// causes. Readiness and lifecycle logs already expose the safe operational
	// signal, so keep driver-internal logging silent at this boundary.
	driverCfg.Logger = &drivermysql.NopLogger{}
	driverCfg.User = cfg.User
	driverCfg.Passwd = cfg.Password
	driverCfg.Net = "tcp"
	driverCfg.Addr = cfg.Address
	driverCfg.DBName = cfg.Database
	// Params contains SQL system variables and is rendered as SET key=value by
	// the driver. Charset must use the driver's dedicated option; treating it
	// as a generic parameter would emit the invalid `SET charset = utf8mb4`.
	driverCfg.Params = map[string]string{"time_zone": "'+00:00'"}
	if err := driverCfg.Apply(drivermysql.Charset("utf8mb4", "")); err != nil {
		return nil, newError(StageConfigInvalid, err)
	}
	driverCfg.Loc = time.UTC
	driverCfg.Timeout = cfg.ConnectTimeout
	driverCfg.ReadTimeout = cfg.ReadTimeout
	driverCfg.WriteTimeout = cfg.WriteTimeout
	driverCfg.ParseTime = true
	driverCfg.MultiStatements = migration

	// Keep every opt-in mechanism that can weaken transport, authentication,
	// file access, or query boundaries disabled. The MySQL 8.4 baseline uses
	// caching_sha2_password; legacy native, cleartext, and old password plugins
	// are not accepted by this connector.
	driverCfg.AllowAllFiles = false
	driverCfg.AllowCleartextPasswords = false
	driverCfg.AllowFallbackToPlaintext = false
	driverCfg.AllowNativePasswords = false
	driverCfg.AllowOldPasswords = false
	driverCfg.InterpolateParams = false
	driverCfg.ColumnsWithAlias = false
	driverCfg.ClientFoundRows = false
	driverCfg.RejectReadOnly = true
	driverCfg.CheckConnLiveness = true

	if cfg.TLSMode == TLSVerifyIdentity {
		tlsCfg, tlsErr := verifiedTLSConfig(cfg.Address, cfg.TLSCAFile)
		if tlsErr != nil {
			return nil, tlsErr
		}
		driverCfg.TLS = tlsCfg
	}

	return driverCfg, nil
}

func migrationDriverConfig(in MigrationConfig) (*drivermysql.Config, error) {
	connection, err := normalizeConnection(in.ConnectionConfig, true)
	if err != nil {
		return nil, newError(StageConfigInvalid, err)
	}
	statementTimeout := in.StatementTimeout
	if statementTimeout == 0 {
		statementTimeout = defaultMigrationStatement
	}
	lockTimeout := in.LockTimeout
	if lockTimeout == 0 {
		lockTimeout = defaultMigrationLock
	}
	if err := validDuration(statementTimeout, maximumMigrationStatement); err != nil {
		return nil, newError(StageConfigInvalid, err)
	}
	if lockTimeout < 11*time.Second || lockTimeout > maximumMigrationLock {
		return nil, newError(StageConfigInvalid, errConfigValue)
	}
	if connection.ReadTimeout <= migrationTimeoutBudget ||
		statementTimeout > connection.ReadTimeout-migrationTimeoutBudget {
		return nil, newError(StageConfigInvalid, errConfigValue)
	}
	if lockTimeout <= migrationTimeoutBudget ||
		connection.ReadTimeout > lockTimeout-migrationTimeoutBudget {
		return nil, newError(StageConfigInvalid, errConfigValue)
	}
	return driverConfig(connection, true)
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

func readBoundedCA(path string) ([]byte, error) {
	file, err := os.Open(path)
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

// String makes the non-secret enum convenient in explicitly selected logs.
func (m TLSMode) String() string {
	return fmt.Sprint(string(m))
}
