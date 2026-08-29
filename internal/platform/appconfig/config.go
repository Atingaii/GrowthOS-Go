// Package appconfig loads and validates GrowthOS process configuration from a
// caller-provided environment lookup boundary.
package appconfig

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
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

// Config is the validated configuration for the growth-api process.
type Config struct {
	Environment Environment
	HTTP        HTTPConfig
	Log         LogConfig
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

// LookupFunc matches os.LookupEnv and makes environment reads injectable.
type LookupFunc func(key string) (value string, found bool)

// Default returns safe standalone defaults for the growth-api process.
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

	if value, found := suppliedValue(lookup, environmentVariable, &problems); found {
		environment := Environment(value)
		if validEnvironment(environment) {
			config.Environment = environment
		} else {
			problems = append(problems, fmt.Errorf("%s must be one of development, test, staging, production", environmentVariable))
		}
	}
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
	loadDuration(lookup, httpWriteTimeoutVariable, maximumHTTPWriteTimeout, &config.HTTP.WriteTimeout, &problems)
	loadDuration(lookup, httpIdleTimeoutVariable, maximumHTTPIdleTimeout, &config.HTTP.IdleTimeout, &problems)

	if value, found := suppliedValue(lookup, logLevelVariable, &problems); found {
		level := LogLevel(value)
		if validLogLevel(level) {
			config.Log.Level = level
		} else {
			problems = append(problems, fmt.Errorf("%s must be one of debug, info, warn, error", logLevelVariable))
		}
	}
	if value, found := suppliedValue(lookup, logFormatVariable, &problems); found {
		format := LogFormat(value)
		if validLogFormat(format) {
			config.Log.Format = format
		} else {
			problems = append(problems, fmt.Errorf("%s must be one of json, text", logFormatVariable))
		}
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("load configuration: %w", errors.Join(problems...))
	}
	return config, nil
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
) {
	value, found := suppliedValue(lookup, variable, problems)
	if !found {
		return
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 || duration > maximum {
		*problems = append(*problems, fmt.Errorf(
			"%s must be a duration greater than 0 and no more than %s",
			variable,
			maximum,
		))
		return
	}
	*destination = duration
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
