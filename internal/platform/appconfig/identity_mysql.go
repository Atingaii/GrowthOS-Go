package appconfig

import (
	"fmt"
	"time"
)

const (
	identityMySQLUserVariable            = "GROWTHOS_IDENTITY_MYSQL_USER"
	identityMySQLPasswordVariable        = "GROWTHOS_IDENTITY_MYSQL_PASSWORD"
	identityMySQLPasswordFileVariable    = "GROWTHOS_IDENTITY_MYSQL_PASSWORD_FILE"
	identityMySQLReadTimeoutVariable     = "GROWTHOS_IDENTITY_MYSQL_READ_TIMEOUT"
	identityMySQLPingTimeoutVariable     = "GROWTHOS_IDENTITY_MYSQL_PING_TIMEOUT"
	identityMySQLMaxOpenConnsVariable    = "GROWTHOS_IDENTITY_MYSQL_MAX_OPEN_CONNS"
	identityMySQLMaxIdleConnsVariable    = "GROWTHOS_IDENTITY_MYSQL_MAX_IDLE_CONNS"
	identityMySQLConnMaxLifetimeVariable = "GROWTHOS_IDENTITY_MYSQL_CONN_MAX_LIFETIME"
	identityMySQLConnMaxIdleTimeVariable = "GROWTHOS_IDENTITY_MYSQL_CONN_MAX_IDLE_TIME"
	defaultIdentityMySQLUser             = "growthos_identity"
	defaultIdentityMySQLReadTimeout      = 5 * time.Second
	defaultIdentityMySQLPingTimeout      = 3 * time.Second
	defaultIdentityMySQLMaxOpenConns     = 10
	defaultIdentityMySQLMaxIdleConns     = 10
	defaultIdentityMySQLConnMaxLifetime  = 3 * time.Minute
	defaultIdentityMySQLConnMaxIdleTime  = time.Minute
)

func defaultIdentityMySQLConfig() MySQLConfig {
	connection := defaultMySQLConnectionConfig()
	connection.ReadTimeout = defaultIdentityMySQLReadTimeout
	return MySQLConfig{
		MySQLConnectionConfig: connection,
		User:                  defaultIdentityMySQLUser,
		PingTimeout:           defaultIdentityMySQLPingTimeout,
		MaxOpenConnections:    defaultIdentityMySQLMaxOpenConns,
		MaxIdleConnections:    defaultIdentityMySQLMaxIdleConns,
		ConnectionMaxLifetime: defaultIdentityMySQLConnMaxLifetime,
		ConnectionMaxIdleTime: defaultIdentityMySQLConnMaxIdleTime,
	}
}

func loadIdentityMySQL(
	lookup LookupFunc,
	shared MySQLConnectionConfig,
	httpWriteTimeout time.Duration,
	httpWriteTimeoutValid bool,
	destination *MySQLConfig,
	problems *[]error,
) bool {
	// Identity shares the deployment endpoint, schema, TLS policy and bounded
	// connect/write timeouts, but has its own runtime account, read budget and
	// connection pool. A separate MySQLConfig lets composition create a distinct
	// pool/DSN without accidentally reusing growthos_app credentials.
	readTimeout := destination.ReadTimeout
	destination.MySQLConnectionConfig = shared
	destination.ReadTimeout = readTimeout

	userValid := loadMySQLUser(lookup, identityMySQLUserVariable, &destination.User, problems)
	loadRequiredPassword(
		lookup,
		identityMySQLPasswordVariable,
		identityMySQLPasswordFileVariable,
		&destination.Password,
		problems,
	)
	loadDuration(
		lookup,
		identityMySQLReadTimeoutVariable,
		maximumMySQLReadTimeout,
		&destination.ReadTimeout,
		problems,
	)
	pingTimeoutValid := loadDuration(
		lookup,
		identityMySQLPingTimeoutVariable,
		maximumMySQLPingTimeout,
		&destination.PingTimeout,
		problems,
	)
	maxOpenValid := loadInteger(
		lookup,
		identityMySQLMaxOpenConnsVariable,
		1,
		maximumMySQLConnections,
		&destination.MaxOpenConnections,
		problems,
	)
	maxIdleValid := loadInteger(
		lookup,
		identityMySQLMaxIdleConnsVariable,
		0,
		maximumMySQLConnections,
		&destination.MaxIdleConnections,
		problems,
	)
	loadDuration(
		lookup,
		identityMySQLConnMaxLifetimeVariable,
		maximumMySQLConnMaxLifetime,
		&destination.ConnectionMaxLifetime,
		problems,
	)
	loadDuration(
		lookup,
		identityMySQLConnMaxIdleTimeVariable,
		maximumMySQLConnMaxIdleTime,
		&destination.ConnectionMaxIdleTime,
		problems,
	)

	if maxOpenValid && maxIdleValid && destination.MaxIdleConnections > destination.MaxOpenConnections {
		*problems = append(*problems, fmt.Errorf(
			"%s must be no greater than %s",
			identityMySQLMaxIdleConnsVariable,
			identityMySQLMaxOpenConnsVariable,
		))
	}
	if httpWriteTimeoutValid && pingTimeoutValid &&
		(httpWriteTimeout <= readinessResponseBudget ||
			destination.PingTimeout > httpWriteTimeout-readinessResponseBudget) {
		*problems = append(*problems, fmt.Errorf(
			"%s plus a %s response budget must be no greater than %s",
			identityMySQLPingTimeoutVariable,
			readinessResponseBudget,
			httpWriteTimeoutVariable,
		))
	}
	return userValid
}
