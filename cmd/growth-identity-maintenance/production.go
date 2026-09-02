package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlrepo"
	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/jmoiron/sqlx"
)

const redactedMaintenanceRuntime = "identity maintenance runtime (redacted)"

var (
	errMaintenanceRuntimeDependency       = errors.New("identity maintenance runtime dependency is unavailable")
	errMaintenanceRuntimeDatabase         = errors.New("identity maintenance runtime database is unavailable")
	errMaintenanceRuntimeContext          = errors.New("identity maintenance runtime context is required")
	errMaintenanceRuntimeClosed           = errors.New("identity maintenance runtime is closed")
	errMaintenanceRuntimeAlreadyAttempted = errors.New("identity maintenance runtime is one shot")
	errMaintenanceRuntimeResult           = errors.New("identity maintenance runtime result is invalid")
	errMaintenanceRuntimeClose            = errors.New("identity maintenance runtime close failed")
)

type maintenanceRuntime interface {
	Run(context.Context) (identityapp.MaintenanceResult, error)
	Close() error
}

type runtimeFactory func(
	context.Context,
	appconfig.IdentityMaintenanceMySQLConfig,
) (maintenanceRuntime, error)

type runtimeDependencies struct {
	NewRuntime runtimeFactory
}

type maintenanceDatabaseOpener func(
	context.Context,
	mysqlstore.Config,
) (*sqlx.DB, error)

type maintenanceRepositoryConstructor func(
	*sqlx.DB,
) (identityapp.MaintenanceRepository, error)

type maintenanceService interface {
	Run(context.Context) (identityapp.MaintenanceResult, error)
}

type maintenanceServiceConstructor func(
	identityapp.Clock,
	identityapp.MaintenanceRepository,
) (maintenanceService, error)

// identityMaintenanceRuntime owns exactly one database pool and permits one
// application operation. A mutex serializes Run and Close, while attempted is
// set before fallible work so commit uncertainty can never be retried through
// the same process runtime.
type identityMaintenanceRuntime struct {
	mu        sync.Mutex
	database  *sqlx.DB
	service   maintenanceService
	attempted bool
	closed    bool
	closeErr  error
}

func productionDependencies() runtimeDependencies {
	return runtimeDependencies{
		NewRuntime: newProductionRuntimeFactory(
			mysqlstore.Open,
			newProductionMaintenanceRepository,
			newProductionMaintenanceService,
			time.Now,
		),
	}
}

func newProductionMaintenanceRepository(
	database *sqlx.DB,
) (identityapp.MaintenanceRepository, error) {
	return mysqlrepo.New(database)
}

func newProductionMaintenanceService(
	clock identityapp.Clock,
	repository identityapp.MaintenanceRepository,
) (maintenanceService, error) {
	return identityapp.NewMaintenanceService(clock, repository)
}

// newProductionRuntimeFactory uses the ordinary single-statement pool opener,
// never the migration opener. The fixed pool size prevents operator input or
// environment variables from turning one bounded operation into concurrent
// database work.
func newProductionRuntimeFactory(
	openDatabase maintenanceDatabaseOpener,
	constructRepository maintenanceRepositoryConstructor,
	constructService maintenanceServiceConstructor,
	now func() time.Time,
) runtimeFactory {
	return func(
		ctx context.Context,
		config appconfig.IdentityMaintenanceMySQLConfig,
	) (maintenanceRuntime, error) {
		if ctx == nil {
			return nil, errMaintenanceRuntimeContext
		}
		if openDatabase == nil || constructRepository == nil ||
			constructService == nil || now == nil {
			return nil, errMaintenanceRuntimeDependency
		}

		database, err := openDatabase(ctx, mysqlMaintenanceRuntimeConfig(config))
		if err != nil {
			closeUnexpectedMaintenanceDatabase(database)
			return nil, err
		}
		if database == nil || database.DB == nil {
			return nil, errMaintenanceRuntimeDatabase
		}

		repository, err := constructRepository(database)
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		if nilInterface(repository) {
			_ = database.Close()
			return nil, errMaintenanceRuntimeDependency
		}

		// The trusted process clock is bound here. The CLI grammar has no way to
		// supply time, cutoff, retention, or row budgets.
		service, err := constructService(identityapp.ClockFunc(now), repository)
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		if nilInterface(service) {
			_ = database.Close()
			return nil, errMaintenanceRuntimeDependency
		}

		return &identityMaintenanceRuntime{
			database: database,
			service:  service,
		}, nil
	}
}

func mysqlMaintenanceRuntimeConfig(
	config appconfig.IdentityMaintenanceMySQLConfig,
) mysqlstore.Config {
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

func (runtime *identityMaintenanceRuntime) Run(
	ctx context.Context,
) (identityapp.MaintenanceResult, error) {
	if ctx == nil {
		return identityapp.MaintenanceResult{}, errMaintenanceRuntimeContext
	}
	if runtime == nil {
		return identityapp.MaintenanceResult{}, errMaintenanceRuntimeDependency
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return identityapp.MaintenanceResult{}, errMaintenanceRuntimeClosed
	}
	if runtime.attempted {
		return identityapp.MaintenanceResult{}, errMaintenanceRuntimeAlreadyAttempted
	}
	// Mark the attempt before validating or calling the service. Every runtime
	// is one shot; there is no internal retry for any failure class.
	runtime.attempted = true
	if runtime.database == nil || runtime.database.DB == nil || nilInterface(runtime.service) {
		return identityapp.MaintenanceResult{}, errMaintenanceRuntimeDependency
	}

	result, err := runtime.service.Run(ctx)
	if err != nil {
		return identityapp.MaintenanceResult{}, err
	}
	if result.Validate() != nil || result.TotalDeleted() > identityapp.MaintenanceMaximumRows {
		return identityapp.MaintenanceResult{}, errMaintenanceRuntimeResult
	}
	return result, nil
}

func (runtime *identityMaintenanceRuntime) Close() error {
	if runtime == nil {
		return errMaintenanceRuntimeDependency
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return runtime.closeErr
	}
	runtime.closed = true
	if runtime.database == nil || runtime.database.DB == nil {
		runtime.closeErr = errMaintenanceRuntimeDatabase
		return runtime.closeErr
	}
	if err := runtime.database.Close(); err != nil {
		// Driver details may contain a host or account. The command only needs a
		// stable signal that success must be downgraded.
		runtime.closeErr = errMaintenanceRuntimeClose
	}
	return runtime.closeErr
}

func (*identityMaintenanceRuntime) String() string { return redactedMaintenanceRuntime }

func (*identityMaintenanceRuntime) GoString() string { return redactedMaintenanceRuntime }

func (*identityMaintenanceRuntime) LogValue() slog.Value {
	return slog.StringValue(redactedMaintenanceRuntime)
}

func (*identityMaintenanceRuntime) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedMaintenanceRuntime)
}

func (*identityMaintenanceRuntime) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedMaintenanceRuntime)
}

func closeUnexpectedMaintenanceDatabase(database *sqlx.DB) {
	if database != nil && database.DB != nil {
		_ = database.Close()
	}
}

func nilMaintenanceRuntime(runtime maintenanceRuntime) bool {
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
