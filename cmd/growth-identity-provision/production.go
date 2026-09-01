package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlprovisioner"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/passwordhash"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/jmoiron/sqlx"
)

const redactedProvisionRuntime = "identity provision runtime (redacted)"

var (
	errProvisionRuntimeDependency       = errors.New("identity provision runtime dependency is unavailable")
	errProvisionRuntimeDatabase         = errors.New("identity provision runtime database is unavailable")
	errProvisionRuntimeClock            = errors.New("identity provision runtime clock is invalid")
	errProvisionRuntimeClosed           = errors.New("identity provision runtime is closed")
	errProvisionRuntimeAlreadyAttempted = errors.New("identity provision runtime is one shot")
	errProvisionRuntimeClose            = errors.New("identity provision runtime close failed")
	errProvisionRuntimeContext          = errors.New("identity provision runtime context is required")
)

// provisionRuntime is deliberately narrower than an account repository. It
// can attempt one create and close its one dedicated database pool. The caller
// retains responsibility for a defensive clear of password after Create
// returns. The runtime additionally clears the supplied slice immediately
// after HashEnrollment returns and clears its private copy on every path.
type provisionRuntime interface {
	Create(context.Context, provisionCommand, []byte) error
	Close() error
}

type runtimeFactory func(
	context.Context,
	appconfig.IdentityProvisionerConfig,
) (provisionRuntime, error)

type runtimeDependencies struct {
	NewRuntime runtimeFactory
}

type provisionDatabaseOpener func(
	context.Context,
	mysqlstore.Config,
) (*sqlx.DB, error)

type enrollmentHasher interface {
	HashEnrollment(context.Context, []byte) (passwordhash.Envelope, error)
}

type enrollmentHasherConstructor func(passwordhash.Config) (enrollmentHasher, error)

type accountProvisioner interface {
	Create(context.Context, identity.WorkforceAccount) error
}

type accountProvisionerConstructor func(*sqlx.DB) (accountProvisioner, error)

type provisionClock func() time.Time

// identityProvisionRuntime owns database and nothing else. The mutex makes
// Create and Close mutually exclusive, preserves the one-shot no-retry rule,
// and makes repeated Close calls return the first result without closing the
// pool twice.
type identityProvisionRuntime struct {
	mu          sync.Mutex
	database    *sqlx.DB
	hasher      enrollmentHasher
	provisioner accountProvisioner
	now         provisionClock
	attempted   bool
	closed      bool
	closeErr    error
}

func productionDependencies() runtimeDependencies {
	return runtimeDependencies{
		NewRuntime: newProductionRuntimeFactory(
			mysqlstore.Open,
			newProductionEnrollmentHasher,
			newProductionAccountProvisioner,
			time.Now,
		),
	}
}

func newProductionEnrollmentHasher(config passwordhash.Config) (enrollmentHasher, error) {
	return passwordhash.New(config)
}

func newProductionAccountProvisioner(database *sqlx.DB) (accountProvisioner, error) {
	return mysqlprovisioner.New(database)
}

// newProductionRuntimeFactory keeps every lifecycle decision injectable while
// retaining one owner for the pool. mysqlstore.Open is intentionally used by
// production instead of OpenMigration: the provisioner gets no multi-statement
// DSN and its pool is explicitly limited to one connection.
func newProductionRuntimeFactory(
	openDatabase provisionDatabaseOpener,
	constructHasher enrollmentHasherConstructor,
	constructProvisioner accountProvisionerConstructor,
	now provisionClock,
) runtimeFactory {
	return func(
		ctx context.Context,
		config appconfig.IdentityProvisionerConfig,
	) (provisionRuntime, error) {
		if ctx == nil {
			return nil, errProvisionRuntimeContext
		}
		if openDatabase == nil || constructHasher == nil ||
			constructProvisioner == nil || now == nil {
			return nil, errProvisionRuntimeDependency
		}

		database, err := openDatabase(ctx, mysqlProvisionRuntimeConfig(config.MySQL))
		if err != nil {
			closeUnexpectedProvisionDatabase(database)
			return nil, err
		}
		if database == nil || database.DB == nil {
			return nil, errProvisionRuntimeDatabase
		}

		hasher, err := constructHasher(passwordhash.DefaultConfig())
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		if nilInterface(hasher) {
			_ = database.Close()
			return nil, errProvisionRuntimeDependency
		}

		provisioner, err := constructProvisioner(database)
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		if nilInterface(provisioner) {
			_ = database.Close()
			return nil, errProvisionRuntimeDependency
		}

		return &identityProvisionRuntime{
			database:    database,
			hasher:      hasher,
			provisioner: provisioner,
			now:         now,
		}, nil
	}
}

func mysqlProvisionRuntimeConfig(config appconfig.IdentityProvisionerMySQLConfig) mysqlstore.Config {
	return mysqlstore.Config{
		ConnectionConfig: mysqlstore.ConnectionConfig{
			Address:        config.Address,
			Database:       config.Database,
			User:           config.User,
			Password:       config.Password,
			TLSMode:        mysqlstore.TLSMode(config.TLSMode),
			TLSCAFile:      config.TLSCAFile,
			ConnectTimeout: config.ConnectTimeout,
			ReadTimeout:    config.ReadTimeout,
			WriteTimeout:   config.WriteTimeout,
		},
		PingTimeout:        config.PingTimeout,
		MaxOpenConnections: 1,
		MaxIdleConnections: 1,
	}
}

func (runtime *identityProvisionRuntime) Create(
	ctx context.Context,
	command provisionCommand,
	password []byte,
) error {
	if ctx == nil {
		return errProvisionRuntimeContext
	}
	if runtime == nil {
		return errProvisionRuntimeDependency
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return errProvisionRuntimeClosed
	}
	if runtime.attempted {
		return errProvisionRuntimeAlreadyAttempted
	}
	// Mark the attempt before any fallible work. In particular, an unknown
	// COMMIT outcome must never be retried through this runtime.
	runtime.attempted = true

	if runtime.database == nil || runtime.database.DB == nil ||
		nilInterface(runtime.hasher) || nilInterface(runtime.provisioner) ||
		runtime.now == nil {
		return errProvisionRuntimeDependency
	}
	if err := validateProvisionCommand(command); err != nil {
		return err
	}

	createdAt, err := canonicalProvisionInstant(runtime.now())
	if err != nil {
		return err
	}

	passwordCopy := bytes.Clone(password)
	defer clearProvisionBytes(passwordCopy)
	hashed, err := runtime.hasher.HashEnrollment(ctx, passwordCopy)
	// readEnrollmentPassword transfers a short-lived mutable secret. Clear it
	// at the first boundary where Argon2 no longer needs it; the command layer
	// still defers a second clear as defense in depth.
	clearProvisionBytes(passwordCopy)
	clearProvisionBytes(password)
	if err != nil {
		return err
	}

	encodedEnvelope := []byte(hashed.Encoded())
	defer clearProvisionBytes(encodedEnvelope)
	credentialEnvelope, err := identity.NewPasswordEnvelope(encodedEnvelope)
	if err != nil {
		return err
	}

	credentialVersion, err := identity.NewCredentialVersion(1)
	if err != nil {
		return err
	}
	authenticationEpoch, err := identity.NewAuthenticationEpoch(1)
	if err != nil {
		return err
	}
	account, err := identity.NewWorkforceAccount(
		command.accountID,
		command.loginName,
		command.principalID,
		identity.AccountStatusEnabled,
		credentialVersion,
		authenticationEpoch,
		credentialEnvelope,
		createdAt,
		createdAt,
	)
	if err != nil {
		return err
	}

	// Return the adapter's stable class unchanged. Most importantly,
	// mysqlprovisioner.ErrCommitOutcomeUnknown remains distinguishable and is
	// never converted into a retryable dependency error.
	return runtime.provisioner.Create(ctx, account)
}

func (runtime *identityProvisionRuntime) Close() error {
	if runtime == nil {
		return errProvisionRuntimeDependency
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return runtime.closeErr
	}
	runtime.closed = true
	if runtime.database == nil || runtime.database.DB == nil {
		runtime.closeErr = errProvisionRuntimeDatabase
		return runtime.closeErr
	}
	if err := runtime.database.Close(); err != nil {
		// A driver close error may contain connection detail. The CLI only needs
		// to downgrade success, so retain a stable low-disclosure result.
		runtime.closeErr = errProvisionRuntimeClose
	}
	return runtime.closeErr
}

func (*identityProvisionRuntime) String() string { return redactedProvisionRuntime }

func (*identityProvisionRuntime) GoString() string { return redactedProvisionRuntime }

func (*identityProvisionRuntime) LogValue() slog.Value {
	return slog.StringValue(redactedProvisionRuntime)
}

func (*identityProvisionRuntime) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedProvisionRuntime)
}

func (*identityProvisionRuntime) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedProvisionRuntime)
}

func canonicalProvisionInstant(value time.Time) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, errProvisionRuntimeClock
	}
	return value.Round(0).UTC().Truncate(time.Microsecond), nil
}

func validateProvisionCommand(command provisionCommand) error {
	if err := command.accountID.Validate(); err != nil {
		return err
	}
	if err := command.loginName.Validate(); err != nil {
		return err
	}
	return command.principalID.Validate()
}

func closeUnexpectedProvisionDatabase(database *sqlx.DB) {
	if database != nil && database.DB != nil {
		_ = database.Close()
	}
}

func clearProvisionBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func nilProvisionRuntime(runtime provisionRuntime) bool {
	return nilInterface(runtime)
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
