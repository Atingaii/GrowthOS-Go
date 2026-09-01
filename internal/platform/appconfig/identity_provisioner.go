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
	identityProvisionerMySQLUserVariable         = "GROWTHOS_IDENTITY_PROVISIONER_MYSQL_USER"
	identityProvisionerMySQLPasswordVariable     = "GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD"
	identityProvisionerMySQLPasswordFileVariable = "GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PASSWORD_FILE"
	identityProvisionerMySQLReadTimeoutVariable  = "GROWTHOS_IDENTITY_PROVISIONER_MYSQL_READ_TIMEOUT"
	identityProvisionerMySQLPingTimeoutVariable  = "GROWTHOS_IDENTITY_PROVISIONER_MYSQL_PING_TIMEOUT"
	identityProvisionerOperationTimeoutVariable  = "GROWTHOS_IDENTITY_PROVISIONER_OPERATION_TIMEOUT"

	defaultIdentityProvisionerMySQLUser        = "growthos_identity_provisioner"
	defaultIdentityProvisionerMySQLReadTimeout = 5 * time.Second
	defaultIdentityProvisionerMySQLPingTimeout = 3 * time.Second
	defaultIdentityProvisionerOperationTimeout = 3 * time.Second

	minimumIdentityProvisionerOperationTimeout = time.Second
	maximumIdentityProvisionerOperationTimeout = 30 * time.Second
	identityProvisionerNetworkBudget           = time.Second

	redactedIdentityProvisionerConfig = "identity provisioner configuration (redacted)"
)

// IdentityProvisionerConfig is the validated configuration for the dedicated
// one-shot workforce-account provisioner. It deliberately has no HTTP,
// Lottery, runtime Identity, Redis, or migration settings.
type IdentityProvisionerConfig struct {
	Environment      Environment
	Log              LogConfig
	MySQL            IdentityProvisionerMySQLConfig
	OperationTimeout time.Duration
}

// IdentityProvisionerMySQLConfig owns the provisioner's separate
// least-privilege credential. Endpoint, schema, TLS, connect, and write policy
// remain shared deployment settings; read and ping budgets are process-owned.
type IdentityProvisionerMySQLConfig struct {
	MySQLConnectionConfig
	User        string
	Password    string
	PingTimeout time.Duration
}

// Every common diagnostic boundary is redacted because these values contain
// the provisioner database password transitively.
func (IdentityProvisionerConfig) String() string   { return redactedIdentityProvisionerConfig }
func (IdentityProvisionerConfig) GoString() string { return redactedIdentityProvisionerConfig }
func (IdentityProvisionerConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityProvisionerConfig)
}
func (IdentityProvisionerConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityProvisionerConfig)
}
func (IdentityProvisionerConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityProvisionerConfig)
}

func (IdentityProvisionerMySQLConfig) String() string {
	return redactedIdentityProvisionerConfig
}
func (IdentityProvisionerMySQLConfig) GoString() string {
	return redactedIdentityProvisionerConfig
}
func (IdentityProvisionerMySQLConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedIdentityProvisionerConfig)
}
func (IdentityProvisionerMySQLConfig) LogValue() slog.Value {
	return slog.StringValue(redactedIdentityProvisionerConfig)
}
func (IdentityProvisionerMySQLConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIdentityProvisionerConfig)
}

// DefaultIdentityProvisioner returns only public, non-secret defaults. The
// database password has no default, so callers must use
// LoadIdentityProvisioner before opening a connection.
func DefaultIdentityProvisioner() IdentityProvisionerConfig {
	defaults := Default()
	connection := defaults.MySQL.MySQLConnectionConfig
	connection.ReadTimeout = defaultIdentityProvisionerMySQLReadTimeout
	return IdentityProvisionerConfig{
		Environment: defaults.Environment,
		Log:         defaults.Log,
		MySQL: IdentityProvisionerMySQLConfig{
			MySQLConnectionConfig: connection,
			User:                  defaultIdentityProvisionerMySQLUser,
			PingTimeout:           defaultIdentityProvisionerMySQLPingTimeout,
		},
		OperationTimeout: defaultIdentityProvisionerOperationTimeout,
	}
}

// LoadIdentityProvisioner reads only the common deployment boundary and the
// provisioner-specific account and budgets. Unrelated process variables are
// intentionally ignored so they cannot prevent this one-shot command from
// starting or cause it to consume another process's credential.
func LoadIdentityProvisioner(lookup LookupFunc) (IdentityProvisionerConfig, error) {
	if lookup == nil {
		return IdentityProvisionerConfig{}, errors.New("load identity provisioner configuration: lookup function is required")
	}

	config := DefaultIdentityProvisioner()
	var problems []error

	environmentValid := loadEnvironment(lookup, &config.Environment, &problems)
	loadLog(lookup, &config.Log, &problems)
	tlsModeValid, readTimeoutValid, writeTimeoutValid := loadMySQLConnection(
		lookup,
		&config.MySQL.MySQLConnectionConfig,
		identityProvisionerMySQLReadTimeoutVariable,
		maximumMySQLReadTimeout,
		&problems,
	)
	loadMySQLUser(
		lookup,
		identityProvisionerMySQLUserVariable,
		&config.MySQL.User,
		&problems,
	)
	loadRequiredPassword(
		lookup,
		identityProvisionerMySQLPasswordVariable,
		identityProvisionerMySQLPasswordFileVariable,
		&config.MySQL.Password,
		&problems,
	)
	loadDuration(
		lookup,
		identityProvisionerMySQLPingTimeoutVariable,
		maximumMySQLPingTimeout,
		&config.MySQL.PingTimeout,
		&problems,
	)
	operationTimeoutValid := loadDurationRange(
		lookup,
		identityProvisionerOperationTimeoutVariable,
		minimumIdentityProvisionerOperationTimeout,
		maximumIdentityProvisionerOperationTimeout,
		&config.OperationTimeout,
		&problems,
	)

	if operationTimeoutValid && readTimeoutValid &&
		(config.MySQL.ReadTimeout <= identityProvisionerNetworkBudget ||
			config.OperationTimeout > config.MySQL.ReadTimeout-identityProvisionerNetworkBudget) {
		problems = append(problems, fmt.Errorf(
			"%s plus a %s network cancellation/cleanup budget must be no greater than %s",
			identityProvisionerOperationTimeoutVariable,
			identityProvisionerNetworkBudget,
			identityProvisionerMySQLReadTimeoutVariable,
		))
	}
	if operationTimeoutValid && writeTimeoutValid &&
		(config.MySQL.WriteTimeout <= identityProvisionerNetworkBudget ||
			config.OperationTimeout > config.MySQL.WriteTimeout-identityProvisionerNetworkBudget) {
		problems = append(problems, fmt.Errorf(
			"%s plus a %s network cancellation/cleanup budget must be no greater than %s",
			identityProvisionerOperationTimeoutVariable,
			identityProvisionerNetworkBudget,
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
		return IdentityProvisionerConfig{}, fmt.Errorf(
			"load identity provisioner configuration: %w",
			errors.Join(problems...),
		)
	}
	return config, nil
}
