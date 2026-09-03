package appconfig

import (
	"fmt"
	"time"
)

const (
	governanceMySQLUserVariable            = "GROWTHOS_GOVERNANCE_MYSQL_USER"
	governanceMySQLPasswordVariable        = "GROWTHOS_GOVERNANCE_MYSQL_PASSWORD"
	governanceMySQLPasswordFileVariable    = "GROWTHOS_GOVERNANCE_MYSQL_PASSWORD_FILE"
	governanceMySQLReadTimeoutVariable     = "GROWTHOS_GOVERNANCE_MYSQL_READ_TIMEOUT"
	governanceMySQLPingTimeoutVariable     = "GROWTHOS_GOVERNANCE_MYSQL_PING_TIMEOUT"
	governanceMySQLMaxOpenConnsVariable    = "GROWTHOS_GOVERNANCE_MYSQL_MAX_OPEN_CONNS"
	governanceMySQLMaxIdleConnsVariable    = "GROWTHOS_GOVERNANCE_MYSQL_MAX_IDLE_CONNS"
	governanceMySQLConnMaxLifetimeVariable = "GROWTHOS_GOVERNANCE_MYSQL_CONN_MAX_LIFETIME"
	governanceMySQLConnMaxIdleTimeVariable = "GROWTHOS_GOVERNANCE_MYSQL_CONN_MAX_IDLE_TIME"
	defaultGovernanceMySQLUser             = "growthos_governance"
	defaultGovernanceMySQLReadTimeout      = 5 * time.Second
	defaultGovernanceMySQLPingTimeout      = 3 * time.Second
	defaultGovernanceMySQLMaxOpenConns     = 10
	defaultGovernanceMySQLMaxIdleConns     = 10
	defaultGovernanceMySQLConnMaxLifetime  = 3 * time.Minute
	defaultGovernanceMySQLConnMaxIdleTime  = time.Minute
)

func defaultGovernanceMySQLConfig() MySQLConfig {
	connection := defaultMySQLConnectionConfig()
	connection.ReadTimeout = defaultGovernanceMySQLReadTimeout
	return MySQLConfig{
		MySQLConnectionConfig: connection,
		User:                  defaultGovernanceMySQLUser,
		PingTimeout:           defaultGovernanceMySQLPingTimeout,
		MaxOpenConnections:    defaultGovernanceMySQLMaxOpenConns,
		MaxIdleConnections:    defaultGovernanceMySQLMaxIdleConns,
		ConnectionMaxLifetime: defaultGovernanceMySQLConnMaxLifetime,
		ConnectionMaxIdleTime: defaultGovernanceMySQLConnMaxIdleTime,
	}
}

func loadGovernanceMySQL(
	lookup LookupFunc,
	shared MySQLConnectionConfig,
	httpWriteTimeout time.Duration,
	httpWriteTimeoutValid bool,
	destination *MySQLConfig,
	problems *[]error,
) (bool, bool) {
	// Governance shares the deployment endpoint, schema, TLS policy and bounded
	// connect/write timeouts. Its credential and pool remain distinct because
	// policy reads and authorization-audit inserts have different grants from
	// both business data and Identity session authority.
	readTimeout := destination.ReadTimeout
	destination.MySQLConnectionConfig = shared
	destination.ReadTimeout = readTimeout

	userValid := loadMySQLUser(lookup, governanceMySQLUserVariable, &destination.User, problems)
	loadRequiredPassword(
		lookup,
		governanceMySQLPasswordVariable,
		governanceMySQLPasswordFileVariable,
		&destination.Password,
		problems,
	)
	readTimeoutValid := loadDuration(
		lookup,
		governanceMySQLReadTimeoutVariable,
		maximumMySQLReadTimeout,
		&destination.ReadTimeout,
		problems,
	)
	pingTimeoutValid := loadDuration(
		lookup,
		governanceMySQLPingTimeoutVariable,
		maximumMySQLPingTimeout,
		&destination.PingTimeout,
		problems,
	)
	maxOpenValid := loadInteger(
		lookup,
		governanceMySQLMaxOpenConnsVariable,
		1,
		maximumMySQLConnections,
		&destination.MaxOpenConnections,
		problems,
	)
	maxIdleValid := loadInteger(
		lookup,
		governanceMySQLMaxIdleConnsVariable,
		0,
		maximumMySQLConnections,
		&destination.MaxIdleConnections,
		problems,
	)
	loadDuration(
		lookup,
		governanceMySQLConnMaxLifetimeVariable,
		maximumMySQLConnMaxLifetime,
		&destination.ConnectionMaxLifetime,
		problems,
	)
	loadDuration(
		lookup,
		governanceMySQLConnMaxIdleTimeVariable,
		maximumMySQLConnMaxIdleTime,
		&destination.ConnectionMaxIdleTime,
		problems,
	)

	if maxOpenValid && maxIdleValid && destination.MaxIdleConnections > destination.MaxOpenConnections {
		*problems = append(*problems, fmt.Errorf(
			"%s must be no greater than %s",
			governanceMySQLMaxIdleConnsVariable,
			governanceMySQLMaxOpenConnsVariable,
		))
	}
	if httpWriteTimeoutValid && pingTimeoutValid &&
		(httpWriteTimeout <= readinessResponseBudget ||
			destination.PingTimeout > httpWriteTimeout-readinessResponseBudget) {
		*problems = append(*problems, fmt.Errorf(
			"%s plus a %s response budget must be no greater than %s",
			governanceMySQLPingTimeoutVariable,
			readinessResponseBudget,
			httpWriteTimeoutVariable,
		))
	}
	return userValid, readTimeoutValid
}
