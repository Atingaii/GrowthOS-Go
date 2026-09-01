package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"
)

const (
	identityMaintenanceMySQLReadTimeoutVariable = "GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_READ_TIMEOUT"
	identityMaintenanceMySQLPingTimeoutVariable = "GROWTHOS_IDENTITY_MAINTENANCE_MYSQL_PING_TIMEOUT"
	identityMaintenanceOperationTimeoutVariable = "GROWTHOS_IDENTITY_MAINTENANCE_OPERATION_TIMEOUT"

	defaultIdentityMaintenanceMySQLReadTimeout = 5 * time.Second
	defaultIdentityMaintenanceMySQLPingTimeout = 3 * time.Second
	defaultIdentityMaintenanceOperationTimeout = 3 * time.Second

	minimumIdentityMaintenanceOperationTimeout = time.Second
	maximumIdentityMaintenanceOperationTimeout = 30 * time.Second
	identityMaintenanceNetworkBudget           = time.Second

	redactedIdentityMaintenanceConfig = "identity maintenance configuration (redacted)"
)

// IdentityMaintenanceConfig is the validated configuration boundary for
// bounded, one-shot Identity maintenance. It deliberately excludes HTTP,
// Lottery, Redis, password hashing, browser-security keys, migrations,
// provisioning credentials, and connection-pool policy.
type IdentityMaintenanceConfig struct {
	Environment      Environment
	Log              LogConfig
	MySQL            IdentityMaintenanceMySQLConfig
	OperationTimeout time.Duration
}

// IdentityMaintenanceMySQLConfig reuses the runtime Identity database
// credential while retaining only the connection and per-operation settings a
// one-shot maintenance process needs. The process opens at most one connection
// in its composition root; pool tuning is intentionally not configurable here.
type IdentityMaintenanceMySQLConfig struct {
	MySQLConnectionConfig
	User        string
	Password    string
	PingTimeout time.Duration
}

func (IdentityMaintenanceConfig) String() string   { return redactedIdentityMaintenanceConfig }
func (IdentityMaintenanceConfig) GoString() string { return redactedIdentityMaintenanceConfig }
func (IdentityMaintenanceConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityMaintenanceConfig)
}
func (IdentityMaintenanceConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityMaintenanceConfig)
}
func (IdentityMaintenanceConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityMaintenanceConfig)
}

func (IdentityMaintenanceMySQLConfig) String() string {
	return redactedIdentityMaintenanceConfig
}
func (IdentityMaintenanceMySQLConfig) GoString() string {
	return redactedIdentityMaintenanceConfig
}
func (IdentityMaintenanceMySQLConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityMaintenanceConfig)
}
func (IdentityMaintenanceMySQLConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityMaintenanceConfig)
}
func (IdentityMaintenanceMySQLConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityMaintenanceConfig)
}

// DefaultIdentityMaintenance returns public defaults only. The Identity
// database password has no default, so callers must use
// LoadIdentityMaintenance before opening a database connection.
func DefaultIdentityMaintenance() IdentityMaintenanceConfig {
	defaults := Default()
	connection := defaults.MySQL.MySQLConnectionConfig
	connection.ReadTimeout = defaultIdentityMaintenanceMySQLReadTimeout
	return IdentityMaintenanceConfig{
		Environment: defaults.Environment,
		Log:         defaults.Log,
		MySQL: IdentityMaintenanceMySQLConfig{
			MySQLConnectionConfig: connection,
			User:                  defaultIdentityMySQLUser,
			PingTimeout:           defaultIdentityMaintenanceMySQLPingTimeout,
		},
		OperationTimeout: defaultIdentityMaintenanceOperationTimeout,
	}
}

// LoadIdentityMaintenance reads only the shared deployment connection, the
// runtime Identity database credential, and maintenance-owned budgets.
// Variables owned by other processes are intentionally never requested.
func LoadIdentityMaintenance(lookup LookupFunc) (IdentityMaintenanceConfig, error) {
	if lookup == nil {
		return IdentityMaintenanceConfig{}, errors.New("load identity maintenance configuration: lookup function is required")
	}

	config := DefaultIdentityMaintenance()
	var problems []error

	environmentValid := loadEnvironment(lookup, &config.Environment, &problems)
	loadLog(lookup, &config.Log, &problems)
	tlsModeValid, readTimeoutValid, writeTimeoutValid := loadMySQLConnection(
		lookup,
		&config.MySQL.MySQLConnectionConfig,
		identityMaintenanceMySQLReadTimeoutVariable,
		maximumMySQLReadTimeout,
		&problems,
	)
	loadMySQLUser(lookup, identityMySQLUserVariable, &config.MySQL.User, &problems)
	loadRequiredPassword(
		lookup,
		identityMySQLPasswordVariable,
		identityMySQLPasswordFileVariable,
		&config.MySQL.Password,
		&problems,
	)
	loadDuration(
		lookup,
		identityMaintenanceMySQLPingTimeoutVariable,
		maximumMySQLPingTimeout,
		&config.MySQL.PingTimeout,
		&problems,
	)
	operationTimeoutValid := loadDurationRange(
		lookup,
		identityMaintenanceOperationTimeoutVariable,
		minimumIdentityMaintenanceOperationTimeout,
		maximumIdentityMaintenanceOperationTimeout,
		&config.OperationTimeout,
		&problems,
	)

	if operationTimeoutValid && readTimeoutValid &&
		(config.MySQL.ReadTimeout <= identityMaintenanceNetworkBudget ||
			config.OperationTimeout > config.MySQL.ReadTimeout-identityMaintenanceNetworkBudget) {
		problems = append(problems, fmt.Errorf(
			"%s plus a %s network cancellation/cleanup budget must be no greater than %s",
			identityMaintenanceOperationTimeoutVariable,
			identityMaintenanceNetworkBudget,
			identityMaintenanceMySQLReadTimeoutVariable,
		))
	}
	if operationTimeoutValid && writeTimeoutValid &&
		(config.MySQL.WriteTimeout <= identityMaintenanceNetworkBudget ||
			config.OperationTimeout > config.MySQL.WriteTimeout-identityMaintenanceNetworkBudget) {
		problems = append(problems, fmt.Errorf(
			"%s plus a %s network cancellation/cleanup budget must be no greater than %s",
			identityMaintenanceOperationTimeoutVariable,
			identityMaintenanceNetworkBudget,
			mysqlWriteTimeoutVariable,
		))
	}

	validateDeploymentTLS(
		config.Environment,
		config.MySQL.MySQLConnectionConfig,
		environmentValid,
		tlsModeValid,
		&problems,
	)

	if len(problems) > 0 {
		return IdentityMaintenanceConfig{}, fmt.Errorf(
			"load identity maintenance configuration: %w",
			errors.Join(problems...),
		)
	}
	return config, nil
}
