// Package appconfig loads and validates GrowthOS process configuration from a
// caller-provided environment lookup boundary.
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	environmentVariable           = "GROWTHOS_ENVIRONMENT"
	httpAddressVariable           = "GROWTHOS_HTTP_ADDRESS"
	httpShutdownTimeoutVariable   = "GROWTHOS_HTTP_SHUTDOWN_TIMEOUT"
	httpReadHeaderTimeoutVariable = "GROWTHOS_HTTP_READ_HEADER_TIMEOUT"
	httpReadTimeoutVariable       = "GROWTHOS_HTTP_READ_TIMEOUT"
	httpWriteTimeoutVariable      = "GROWTHOS_HTTP_WRITE_TIMEOUT"
	httpIdleTimeoutVariable       = "GROWTHOS_HTTP_IDLE_TIMEOUT"
	logLevelVariable              = "GROWTHOS_LOG_LEVEL"
	logFormatVariable             = "GROWTHOS_LOG_FORMAT"
	mysqlAddressVariable          = "GROWTHOS_MYSQL_ADDRESS"
	mysqlDatabaseVariable         = "GROWTHOS_MYSQL_DATABASE"
	mysqlTLSModeVariable          = "GROWTHOS_MYSQL_TLS_MODE"
	mysqlTLSCAFileVariable        = "GROWTHOS_MYSQL_TLS_CA_FILE"
	mysqlConnectTimeoutVariable   = "GROWTHOS_MYSQL_CONNECT_TIMEOUT"
	mysqlReadTimeoutVariable      = "GROWTHOS_MYSQL_READ_TIMEOUT"
	mysqlWriteTimeoutVariable     = "GROWTHOS_MYSQL_WRITE_TIMEOUT"
	mysqlUserVariable             = "GROWTHOS_MYSQL_USER"
	mysqlPasswordVariable         = "GROWTHOS_MYSQL_PASSWORD"
	mysqlPingTimeoutVariable      = "GROWTHOS_MYSQL_PING_TIMEOUT"
	mysqlMaxOpenConnsVariable     = "GROWTHOS_MYSQL_MAX_OPEN_CONNS"
	mysqlMaxIdleConnsVariable     = "GROWTHOS_MYSQL_MAX_IDLE_CONNS"
	mysqlConnMaxLifetimeVariable  = "GROWTHOS_MYSQL_CONN_MAX_LIFETIME"
	mysqlConnMaxIdleTimeVariable  = "GROWTHOS_MYSQL_CONN_MAX_IDLE_TIME"
	migrationUserVariable         = "GROWTHOS_MYSQL_MIGRATION_USER"
	migrationPasswordVariable     = "GROWTHOS_MYSQL_MIGRATION_PASSWORD"
	migrationReadTimeoutVariable  = "GROWTHOS_MYSQL_MIGRATION_READ_TIMEOUT"
	migrationLockTimeoutVariable  = "GROWTHOS_MYSQL_MIGRATION_LOCK_TIMEOUT"
	migrationStatementVariable    = "GROWTHOS_MYSQL_MIGRATION_STATEMENT_TIMEOUT"
	defaultHTTPAddress            = ":8080"
	defaultHTTPShutdownTimeout    = 5 * time.Second
	defaultHTTPReadHeaderTimeout  = 5 * time.Second
	defaultHTTPReadTimeout        = 15 * time.Second
	defaultHTTPWriteTimeout       = 30 * time.Second
	defaultHTTPIdleTimeout        = 60 * time.Second
	maximumHTTPShutdownTimeout    = 2 * time.Minute
	maximumHTTPReadHeaderTimeout  = 30 * time.Second
	maximumHTTPReadTimeout        = 5 * time.Minute
	maximumHTTPWriteTimeout       = 10 * time.Minute
	maximumHTTPIdleTimeout        = 10 * time.Minute
	defaultMySQLAddress           = "127.0.0.1:3306"
	defaultMySQLDatabase          = "growthos"
	defaultMySQLUser              = "growthos_app"
	defaultMigrationUser          = "growthos_migrator"
	defaultMigrationReadTimeout   = 35 * time.Second
	defaultMySQLConnectTimeout    = 3 * time.Second
	defaultMySQLReadTimeout       = 5 * time.Second
	defaultMySQLWriteTimeout      = 5 * time.Second
	defaultMySQLPingTimeout       = 3 * time.Second
	defaultMySQLMaxOpenConns      = 10
	defaultMySQLMaxIdleConns      = 10
	defaultMySQLConnMaxLifetime   = 3 * time.Minute
	defaultMySQLConnMaxIdleTime   = time.Minute
	defaultMigrationLockTimeout   = 40 * time.Second
	defaultMigrationStatement     = 30 * time.Second
	maximumMySQLConnectTimeout    = 30 * time.Second
	maximumMySQLReadTimeout       = 5 * time.Minute
	maximumMySQLWriteTimeout      = 5 * time.Minute
	maximumMySQLPingTimeout       = 30 * time.Second
	maximumMySQLConnections       = 100
	maximumMySQLConnMaxLifetime   = time.Hour
	maximumMySQLConnMaxIdleTime   = 30 * time.Minute
	maximumMigrationLockTimeout   = 11 * time.Minute
	maximumMigrationStatement     = 10 * time.Minute
	maximumMigrationReadTimeout   = 10*time.Minute + 30*time.Second
	readinessResponseBudget       = time.Second
	migrationTimeoutBudget        = 5 * time.Second
	maximumPasswordBytes          = 1024
)

const (
	redactedAPIConfig       = "growth-api configuration (redacted)"
	redactedMigrationConfig = "growth-migrate configuration (redacted)"
)

// Environment identifies the deployment environment without coupling
// configuration loading to deployment-specific behavior.
type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentStaging     Environment = "staging"
	EnvironmentProduction  Environment = "production"
)

// LogLevel is the minimum structured log severity emitted by the process.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat selects the process log encoder.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// MySQLTLSMode selects how connections authenticate the MySQL server.
type MySQLTLSMode string

const (
	MySQLTLSDisabled       MySQLTLSMode = "disabled"
	MySQLTLSVerifyIdentity MySQLTLSMode = "verify_identity"
)

// Config is the validated configuration for the growth-api process.
type Config struct {
	Environment Environment
	HTTP        HTTPConfig
	Log         LogConfig
	MySQL       MySQLConfig
}

// MigrationConfig is the validated configuration for the migration process.
// It deliberately has no HTTP settings or growth-api credentials.
type MigrationConfig struct {
	Environment Environment
	Log         LogConfig
	MySQL       MigrationMySQLConfig
}

// HTTPConfig controls the HTTP listener and its bounded lifecycle timeouts.
type HTTPConfig struct {
	Address           string
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// LogConfig controls structured logging output.
type LogConfig struct {
	Level  LogLevel
	Format LogFormat
}

// MySQLConnectionConfig contains the non-secret connection settings shared by
// the API and migration processes.
type MySQLConnectionConfig struct {
	Address        string
	Database       string
	TLSMode        MySQLTLSMode
	TLSCAFile      string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

// MySQLConfig controls the API's least-privilege database account and pool.
type MySQLConfig struct {
	MySQLConnectionConfig
	User                  string
	Password              string
	PingTimeout           time.Duration
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
}

// MigrationMySQLConfig controls the separately privileged migration account.
type MigrationMySQLConfig struct {
	MySQLConnectionConfig
	User             string
	Password         string
	LockTimeout      time.Duration
	StatementTimeout time.Duration
}

// String, GoString, LogValue, and MarshalJSON deliberately make the four
// configuration values that can transitively contain a password safe at
// common diagnostic and serialization boundaries. Operational logs should
// select individual non-secret fields explicitly instead of dumping config.
func (Config) String() string   { return redactedAPIConfig }
func (Config) GoString() string { return redactedAPIConfig }
func (Config) LogValue() slog.Value {
	return slog.StringValue(redactedAPIConfig)
}
func (Config) MarshalJSON() ([]byte, error) { return json.Marshal(redactedAPIConfig) }

func (MySQLConfig) String() string   { return redactedAPIConfig }
func (MySQLConfig) GoString() string { return redactedAPIConfig }
func (MySQLConfig) LogValue() slog.Value {
	return slog.StringValue(redactedAPIConfig)
}
func (MySQLConfig) MarshalJSON() ([]byte, error) { return json.Marshal(redactedAPIConfig) }

func (MigrationConfig) String() string   { return redactedMigrationConfig }
func (MigrationConfig) GoString() string { return redactedMigrationConfig }
func (MigrationConfig) LogValue() slog.Value {
	return slog.StringValue(redactedMigrationConfig)
}
func (MigrationConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedMigrationConfig)
}

func (MigrationMySQLConfig) String() string   { return redactedMigrationConfig }
func (MigrationMySQLConfig) GoString() string { return redactedMigrationConfig }
func (MigrationMySQLConfig) LogValue() slog.Value {
	return slog.StringValue(redactedMigrationConfig)
}
func (MigrationMySQLConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedMigrationConfig)
}

// LookupFunc matches os.LookupEnv and makes environment reads injectable.
type LookupFunc func(key string) (value string, found bool)

// Default returns the public, non-secret defaults for the growth-api process.
// Password intentionally has no default, so callers must still use Load before
// starting the process.
func Default() Config {
	return Config{
		Environment: EnvironmentDevelopment,
		HTTP: HTTPConfig{
			Address:           defaultHTTPAddress,
			ShutdownTimeout:   defaultHTTPShutdownTimeout,
			ReadHeaderTimeout: defaultHTTPReadHeaderTimeout,
			ReadTimeout:       defaultHTTPReadTimeout,
			WriteTimeout:      defaultHTTPWriteTimeout,
			IdleTimeout:       defaultHTTPIdleTimeout,
		},
		Log: LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
		MySQL: MySQLConfig{
			MySQLConnectionConfig: defaultMySQLConnectionConfig(),
			User:                  defaultMySQLUser,
			PingTimeout:           defaultMySQLPingTimeout,
			MaxOpenConnections:    defaultMySQLMaxOpenConns,
			MaxIdleConnections:    defaultMySQLMaxIdleConns,
			ConnectionMaxLifetime: defaultMySQLConnMaxLifetime,
			ConnectionMaxIdleTime: defaultMySQLConnMaxIdleTime,
		},
	}
}

// DefaultMigration returns the public, non-secret defaults for the migration
// process. Password intentionally has no default.
func DefaultMigration() MigrationConfig {
	defaults := Default()
	connection := defaults.MySQL.MySQLConnectionConfig
	connection.ReadTimeout = defaultMigrationReadTimeout
	return MigrationConfig{
		Environment: defaults.Environment,
		Log:         defaults.Log,
		MySQL: MigrationMySQLConfig{
			MySQLConnectionConfig: connection,
			User:                  defaultMigrationUser,
			LockTimeout:           defaultMigrationLockTimeout,
			StatementTimeout:      defaultMigrationStatement,
		},
	}
}

func defaultMySQLConnectionConfig() MySQLConnectionConfig {
	return MySQLConnectionConfig{
		Address:        defaultMySQLAddress,
		Database:       defaultMySQLDatabase,
		TLSMode:        MySQLTLSDisabled,
		ConnectTimeout: defaultMySQLConnectTimeout,
		ReadTimeout:    defaultMySQLReadTimeout,
		WriteTimeout:   defaultMySQLWriteTimeout,
	}
}

// Load applies known environment overrides to Default and validates every
// supplied value. It returns all independent validation failures together.
func Load(lookup LookupFunc) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("load configuration: lookup function is required")
	}

	config := Default()
	var problems []error

	environmentValid := loadEnvironment(lookup, &config.Environment, &problems)
	if value, found := suppliedValue(lookup, httpAddressVariable, &problems); found {
		if validAddress(value) {
			config.HTTP.Address = value
		} else {
			problems = append(problems, fmt.Errorf("%s must be a valid host:port with a port from 1 to 65535", httpAddressVariable))
		}
	}

	loadDuration(lookup, httpShutdownTimeoutVariable, maximumHTTPShutdownTimeout, &config.HTTP.ShutdownTimeout, &problems)
	loadDuration(lookup, httpReadHeaderTimeoutVariable, maximumHTTPReadHeaderTimeout, &config.HTTP.ReadHeaderTimeout, &problems)
	loadDuration(lookup, httpReadTimeoutVariable, maximumHTTPReadTimeout, &config.HTTP.ReadTimeout, &problems)
	httpWriteTimeoutValid := loadDuration(lookup, httpWriteTimeoutVariable, maximumHTTPWriteTimeout, &config.HTTP.WriteTimeout, &problems)
	loadDuration(lookup, httpIdleTimeoutVariable, maximumHTTPIdleTimeout, &config.HTTP.IdleTimeout, &problems)

	loadLog(lookup, &config.Log, &problems)
	tlsModeValid, _ := loadMySQLConnection(
		lookup,
		&config.MySQL.MySQLConnectionConfig,
		mysqlReadTimeoutVariable,
		maximumMySQLReadTimeout,
		&problems,
	)
	loadMySQLUser(lookup, mysqlUserVariable, &config.MySQL.User, &problems)
	loadRequiredPassword(lookup, mysqlPasswordVariable, &config.MySQL.Password, &problems)
	mysqlPingTimeoutValid := loadDuration(lookup, mysqlPingTimeoutVariable, maximumMySQLPingTimeout, &config.MySQL.PingTimeout, &problems)
	maxOpenValid := loadInteger(lookup, mysqlMaxOpenConnsVariable, 1, maximumMySQLConnections, &config.MySQL.MaxOpenConnections, &problems)
	maxIdleValid := loadInteger(lookup, mysqlMaxIdleConnsVariable, 0, maximumMySQLConnections, &config.MySQL.MaxIdleConnections, &problems)
	loadDuration(lookup, mysqlConnMaxLifetimeVariable, maximumMySQLConnMaxLifetime, &config.MySQL.ConnectionMaxLifetime, &problems)
	loadDuration(lookup, mysqlConnMaxIdleTimeVariable, maximumMySQLConnMaxIdleTime, &config.MySQL.ConnectionMaxIdleTime, &problems)
	if maxOpenValid && maxIdleValid && config.MySQL.MaxIdleConnections > config.MySQL.MaxOpenConnections {
		problems = append(problems, fmt.Errorf("%s must be no greater than %s", mysqlMaxIdleConnsVariable, mysqlMaxOpenConnsVariable))
	}
	if httpWriteTimeoutValid && mysqlPingTimeoutValid &&
		(config.HTTP.WriteTimeout <= readinessResponseBudget ||
			config.MySQL.PingTimeout > config.HTTP.WriteTimeout-readinessResponseBudget) {
		problems = append(problems, fmt.Errorf("%s plus a %s response budget must be no greater than %s", mysqlPingTimeoutVariable, readinessResponseBudget, httpWriteTimeoutVariable))
	}
	validateDeploymentTLS(config.Environment, config.MySQL.MySQLConnectionConfig, environmentValid, tlsModeValid, &problems)

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("load configuration: %w", errors.Join(problems...))
	}
	return config, nil
}

// LoadMigration loads only the settings used by the migration process. HTTP
// settings and API account settings are intentionally ignored so unrelated
// process configuration cannot prevent a migration from starting.
func LoadMigration(lookup LookupFunc) (MigrationConfig, error) {
	if lookup == nil {
		return MigrationConfig{}, errors.New("load migration configuration: lookup function is required")
	}

	config := DefaultMigration()
	var problems []error

	environmentValid := loadEnvironment(lookup, &config.Environment, &problems)
	loadLog(lookup, &config.Log, &problems)
	tlsModeValid, migrationReadTimeoutValid := loadMySQLConnection(
		lookup,
		&config.MySQL.MySQLConnectionConfig,
		migrationReadTimeoutVariable,
		maximumMigrationReadTimeout,
		&problems,
	)
	loadMySQLUser(lookup, migrationUserVariable, &config.MySQL.User, &problems)
	loadRequiredPassword(lookup, migrationPasswordVariable, &config.MySQL.Password, &problems)
	migrationLockTimeoutValid := loadDurationRange(lookup, migrationLockTimeoutVariable, 11*time.Second, maximumMigrationLockTimeout, &config.MySQL.LockTimeout, &problems)
	migrationStatementTimeoutValid := loadDuration(lookup, migrationStatementVariable, maximumMigrationStatement, &config.MySQL.StatementTimeout, &problems)
	if migrationReadTimeoutValid && migrationStatementTimeoutValid &&
		(config.MySQL.ReadTimeout <= migrationTimeoutBudget ||
			config.MySQL.StatementTimeout > config.MySQL.ReadTimeout-migrationTimeoutBudget) {
		problems = append(problems, fmt.Errorf("%s plus a %s network budget must be no greater than %s", migrationStatementVariable, migrationTimeoutBudget, migrationReadTimeoutVariable))
	}
	if migrationLockTimeoutValid && migrationReadTimeoutValid &&
		(config.MySQL.LockTimeout <= migrationTimeoutBudget ||
			config.MySQL.ReadTimeout > config.MySQL.LockTimeout-migrationTimeoutBudget) {
		problems = append(problems, fmt.Errorf("%s plus a %s lock cleanup budget must be no greater than %s", migrationReadTimeoutVariable, migrationTimeoutBudget, migrationLockTimeoutVariable))
	}
	validateDeploymentTLS(config.Environment, config.MySQL.MySQLConnectionConfig, environmentValid, tlsModeValid, &problems)

	if len(problems) > 0 {
		return MigrationConfig{}, fmt.Errorf("load migration configuration: %w", errors.Join(problems...))
	}
	return config, nil
}

func loadEnvironment(lookup LookupFunc, destination *Environment, problems *[]error) bool {
	value, present := lookup(environmentVariable)
	if !present {
		return true
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", environmentVariable))
		return false
	}
	environment := Environment(value)
	if !validEnvironment(environment) {
		*problems = append(*problems, fmt.Errorf("%s must be one of development, test, staging, production", environmentVariable))
		return false
	}
	*destination = environment
	return true
}

func loadLog(lookup LookupFunc, destination *LogConfig, problems *[]error) {
	if value, found := suppliedValue(lookup, logLevelVariable, problems); found {
		level := LogLevel(value)
		if validLogLevel(level) {
			destination.Level = level
		} else {
			*problems = append(*problems, fmt.Errorf("%s must be one of debug, info, warn, error", logLevelVariable))
		}
	}
	if value, found := suppliedValue(lookup, logFormatVariable, problems); found {
		format := LogFormat(value)
		if validLogFormat(format) {
			destination.Format = format
		} else {
			*problems = append(*problems, fmt.Errorf("%s must be one of json, text", logFormatVariable))
		}
	}
}

func loadMySQLConnection(
	lookup LookupFunc,
	destination *MySQLConnectionConfig,
	readTimeoutVariable string,
	maximumReadTimeout time.Duration,
	problems *[]error,
) (bool, bool) {
	if value, found := suppliedValue(lookup, mysqlAddressVariable, problems); found {
		if validMySQLAddress(value) {
			destination.Address = value
		} else {
			*problems = append(*problems, fmt.Errorf("%s must be a valid host:port with a non-empty host and a port from 1 to 65535", mysqlAddressVariable))
		}
	}
	if value, found := suppliedValue(lookup, mysqlDatabaseVariable, problems); found {
		if validMySQLIdentifier(value) {
			destination.Database = value
		} else {
			*problems = append(*problems, fmt.Errorf("%s must match [a-z][a-z0-9_]{0,63}", mysqlDatabaseVariable))
		}
	}

	tlsModeValid := true
	if value, present := lookup(mysqlTLSModeVariable); present {
		if strings.TrimSpace(value) == "" {
			*problems = append(*problems, fmt.Errorf("%s must not be empty", mysqlTLSModeVariable))
			tlsModeValid = false
		} else {
			mode := MySQLTLSMode(value)
			if validMySQLTLSMode(mode) {
				destination.TLSMode = mode
			} else {
				tlsModeValid = false
				*problems = append(*problems, fmt.Errorf("%s must be one of disabled, verify_identity", mysqlTLSModeVariable))
			}
		}
	}
	loadOptionalString(lookup, mysqlTLSCAFileVariable, &destination.TLSCAFile, problems)
	loadDuration(lookup, mysqlConnectTimeoutVariable, maximumMySQLConnectTimeout, &destination.ConnectTimeout, problems)
	readTimeoutValid := loadDuration(lookup, readTimeoutVariable, maximumReadTimeout, &destination.ReadTimeout, problems)
	loadDuration(lookup, mysqlWriteTimeoutVariable, maximumMySQLWriteTimeout, &destination.WriteTimeout, problems)
	return tlsModeValid, readTimeoutValid
}

func validateDeploymentTLS(
	environment Environment,
	config MySQLConnectionConfig,
	environmentValid bool,
	tlsModeValid bool,
	problems *[]error,
) {
	if tlsModeValid && config.TLSMode == MySQLTLSDisabled && config.TLSCAFile != "" {
		*problems = append(*problems, fmt.Errorf("%s must not be set when %s is disabled", mysqlTLSCAFileVariable, mysqlTLSModeVariable))
	}
	if environmentValid && tlsModeValid &&
		(environment == EnvironmentStaging || environment == EnvironmentProduction) &&
		config.TLSMode != MySQLTLSVerifyIdentity {
		*problems = append(*problems, fmt.Errorf("%s must be verify_identity in staging and production", mysqlTLSModeVariable))
	}
}

func loadOptionalString(lookup LookupFunc, variable string, destination *string, problems *[]error) {
	if value, found := suppliedValue(lookup, variable, problems); found {
		*destination = value
	}
}

func loadMySQLUser(lookup LookupFunc, variable string, destination *string, problems *[]error) {
	value, present := lookup(variable)
	if !present {
		return
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", variable))
		return
	}
	if !validMySQLUser(value) {
		*problems = append(*problems, fmt.Errorf("%s must contain 1 to 32 printable Unicode characters, no control characters, and no leading or trailing whitespace", variable))
		return
	}
	*destination = value
}

func loadRequiredPassword(lookup LookupFunc, variable string, destination *string, problems *[]error) {
	value, found := lookup(variable)
	if !found {
		*problems = append(*problems, fmt.Errorf("%s is required", variable))
		return
	}
	if len(value) == 0 {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", variable))
		return
	}
	if len(value) > maximumPasswordBytes {
		*problems = append(*problems, fmt.Errorf("%s must be no more than %d bytes", variable, maximumPasswordBytes))
		return
	}
	*destination = value
}

func loadInteger(
	lookup LookupFunc,
	variable string,
	minimum int,
	maximum int,
	destination *int,
	problems *[]error,
) bool {
	value, present := lookup(variable)
	if !present {
		return true
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", variable))
		return false
	}
	integer, err := strconv.Atoi(value)
	if err != nil || integer < minimum || integer > maximum {
		*problems = append(*problems, fmt.Errorf("%s must be an integer from %d to %d", variable, minimum, maximum))
		return false
	}
	*destination = integer
	return true
}

func suppliedValue(lookup LookupFunc, variable string, problems *[]error) (string, bool) {
	value, found := lookup(variable)
	if !found {
		return "", false
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", variable))
		return "", false
	}
	return value, true
}

func loadDuration(
	lookup LookupFunc,
	variable string,
	maximum time.Duration,
	destination *time.Duration,
	problems *[]error,
) bool {
	value, present := lookup(variable)
	if !present {
		return true
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", variable))
		return false
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 || duration > maximum {
		*problems = append(*problems, fmt.Errorf(
			"%s must be a duration greater than 0 and no more than %s",
			variable,
			maximum,
		))
		return false
	}
	*destination = duration
	return true
}

func loadDurationRange(
	lookup LookupFunc,
	variable string,
	minimum time.Duration,
	maximum time.Duration,
	destination *time.Duration,
	problems *[]error,
) bool {
	value, present := lookup(variable)
	if !present {
		return true
	}
	if strings.TrimSpace(value) == "" {
		*problems = append(*problems, fmt.Errorf("%s must not be empty", variable))
		return false
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < minimum || duration > maximum {
		*problems = append(*problems, fmt.Errorf(
			"%s must be a duration from %s to %s",
			variable,
			minimum,
			maximum,
		))
		return false
	}
	*destination = duration
	return true
}

func validEnvironment(environment Environment) bool {
	switch environment {
	case EnvironmentDevelopment, EnvironmentTest, EnvironmentStaging, EnvironmentProduction:
		return true
	default:
		return false
	}
}

func validLogLevel(level LogLevel) bool {
	switch level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return true
	default:
		return false
	}
}

func validLogFormat(format LogFormat) bool {
	switch format {
	case LogFormatJSON, LogFormatText:
		return true
	default:
		return false
	}
}

func validMySQLTLSMode(mode MySQLTLSMode) bool {
	switch mode {
	case MySQLTLSDisabled, MySQLTLSVerifyIdentity:
		return true
	default:
		return false
	}
}

func validMySQLIdentifier(identifier string) bool {
	if len(identifier) == 0 || len(identifier) > 64 || identifier[0] < 'a' || identifier[0] > 'z' {
		return false
	}
	for _, character := range identifier[1:] {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' {
			return false
		}
	}
	return true
}

func validMySQLUser(user string) bool {
	if !utf8.ValidString(user) {
		return false
	}
	runes := []rune(user)
	if len(runes) == 0 || len(runes) > 32 || unicode.IsSpace(runes[0]) || unicode.IsSpace(runes[len(runes)-1]) {
		return false
	}
	for _, character := range runes {
		if !unicode.IsPrint(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validMySQLAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && host != "" && validAddress(address)
}

func validAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return false
	}
	return host == "" || net.ParseIP(host) != nil || validHostname(host)
}

func validHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') &&
				(character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') &&
				character != '-' {
				return false
			}
		}
	}
	return true
}
