package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlrepo"
	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type testMaintenanceRepository struct {
	result    identityapp.MaintenanceResult
	err       error
	calls     int
	context   context.Context
	operation identityapp.MaintenanceOperation
}

func (repository *testMaintenanceRepository) RunMaintenance(
	ctx context.Context,
	operation identityapp.MaintenanceOperation,
) (identityapp.MaintenanceResult, error) {
	repository.calls++
	repository.context = ctx
	repository.operation = operation
	return repository.result, repository.err
}

type testMaintenanceService struct {
	result  identityapp.MaintenanceResult
	err     error
	calls   int
	context context.Context
}

func (service *testMaintenanceService) Run(
	ctx context.Context,
) (identityapp.MaintenanceResult, error) {
	service.calls++
	service.context = ctx
	return service.result, service.err
}

func TestMySQLMaintenanceRuntimeConfigMapsIdentityAuthorityAndFixedPool(t *testing.T) {
	input := appconfig.IdentityMaintenanceMySQLConfig{
		MySQLConnectionConfig: appconfig.MySQLConnectionConfig{
			Address:        "mysql.identity.internal:3307",
			Database:       "growthos",
			TLSMode:        appconfig.MySQLTLSVerifyIdentity,
			TLSCAFile:      "/run/secrets/identity-mysql-ca.pem",
			ConnectTimeout: 2700 * time.Millisecond,
			ReadTimeout:    7 * time.Second,
			WriteTimeout:   6 * time.Second,
		},
		User:        "growthos_identity",
		Password:    "identity-mysql-secret",
		PingTimeout: 2200 * time.Millisecond,
	}

	got := mysqlMaintenanceRuntimeConfig(input)
	wantConnection := mysqlstore.ConnectionConfig{
		Address:        input.Address,
		Database:       input.Database,
		User:           input.User,
		Password:       input.Password,
		TLSMode:        mysqlstore.TLSMode(input.TLSMode),
		TLSCAFile:      input.TLSCAFile,
		ConnectTimeout: input.ConnectTimeout,
		ReadTimeout:    input.ReadTimeout,
		WriteTimeout:   input.WriteTimeout,
	}
	if got.ConnectionConfig != wantConnection {
		t.Fatalf("connection mapping = %#v, want %#v", got.ConnectionConfig, wantConnection)
	}
	if got.PingTimeout != input.PingTimeout || got.MaxOpenConnections != 1 || got.MaxIdleConnections != 1 {
		t.Fatalf("pool mapping = ping %s max-open %d max-idle %d", got.PingTimeout, got.MaxOpenConnections, got.MaxIdleConnections)
	}
	if got.ConnectionMaxLifetime != 0 || got.ConnectionMaxIdleTime != 0 {
		t.Fatal("maintenance mapping introduced caller-controlled pool lifetime policy")
	}
}

func TestProductionRuntimeFactoryWiresTrustedClockAndOwnsPool(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	repository := &testMaintenanceRepository{result: mustMaintenanceResult(t, 0, 0)}
	service := &testMaintenanceService{result: mustMaintenanceResult(t, 0, 0)}
	wantClock := time.Date(2026, time.September, 2, 8, 9, 10, 123456789, time.FixedZone("server", 8*60*60))
	var openedConfig mysqlstore.Config
	repositoryConstructed := 0
	serviceConstructed := 0

	factory := newProductionRuntimeFactory(
		func(ctx context.Context, config mysqlstore.Config) (*sqlx.DB, error) {
			if ctx == nil {
				t.Fatal("database opener received nil context")
			}
			openedConfig = config
			return database, nil
		},
		func(got *sqlx.DB) (identityapp.MaintenanceRepository, error) {
			repositoryConstructed++
			if got != database {
				t.Fatal("repository constructor received a different pool")
			}
			return repository, nil
		},
		func(clock identityapp.Clock, got identityapp.MaintenanceRepository) (maintenanceService, error) {
			serviceConstructed++
			if got != repository || clock == nil || clock.Now() != wantClock {
				t.Fatal("service constructor did not receive the trusted clock and repository")
			}
			return service, nil
		},
		func() time.Time { return wantClock },
	)
	config := appconfig.IdentityMaintenanceMySQLConfig{
		MySQLConnectionConfig: appconfig.MySQLConnectionConfig{Address: "db:3306", Database: "growthos"},
		User:                  "growthos_identity", Password: "secret", PingTimeout: time.Second,
	}
	runtime, err := factory(context.Background(), config)
	if err != nil || nilMaintenanceRuntime(runtime) {
		t.Fatalf("factory() = runtime %v, error %v", runtime, err)
	}
	if repositoryConstructed != 1 || serviceConstructed != 1 {
		t.Fatalf("constructor calls repository=%d service=%d", repositoryConstructed, serviceConstructed)
	}
	if openedConfig.MaxOpenConnections != 1 || openedConfig.MaxIdleConnections != 1 || openedConfig.User != config.User {
		t.Fatalf("database opener config = %#v", openedConfig)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("factory owner did not close its pool exactly once: %v", err)
	}
}

func TestProductionRuntimeUsesOneCanonicalServerClockSnapshot(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	wantResult := mustMaintenanceResult(t, 250, 250)
	repository := &testMaintenanceRepository{result: wantResult}
	location := time.FixedZone("operator-cannot-select", 8*60*60)
	serverTime := time.Date(2026, time.September, 2, 12, 13, 14, 987654321, location)
	clockCalls := 0

	factory := newProductionRuntimeFactory(
		func(context.Context, mysqlstore.Config) (*sqlx.DB, error) { return database, nil },
		func(*sqlx.DB) (identityapp.MaintenanceRepository, error) { return repository, nil },
		newProductionMaintenanceService,
		func() time.Time {
			clockCalls++
			return serverTime
		},
	)
	runtime, err := factory(context.Background(), appconfig.IdentityMaintenanceMySQLConfig{})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}
	ctx := context.WithValue(context.Background(), maintenanceContextKey{}, "operation")
	result, err := runtime.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TotalDeleted() != 500 || clockCalls != 1 || repository.calls != 1 || repository.context != ctx {
		t.Fatalf("run result/calls total=%d clock=%d repo=%d context=%v", result.TotalDeleted(), clockCalls, repository.calls, repository.context)
	}
	wantObserved := serverTime.UTC().Round(0).Truncate(time.Microsecond)
	operation := repository.operation
	if operation.ObservedAt() != wantObserved || operation.ObservedAt().Location() != time.UTC {
		t.Fatalf("observed time = %s, want %s", operation.ObservedAt(), wantObserved)
	}
	if operation.SessionCutoff() != wantObserved.Add(-identityapp.SessionHistoryRetention) ||
		operation.SessionBudget() != 250 || operation.ThrottleBudget() != 250 || operation.Validate() != nil {
		t.Fatalf("server-owned operation = observed %s cutoff %s budgets %d/%d", operation.ObservedAt(), operation.SessionCutoff(), operation.SessionBudget(), operation.ThrottleBudget())
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestProductionRuntimeFactoryClosesPoolOnConstructorFailure(t *testing.T) {
	tests := []struct {
		name                string
		constructRepository maintenanceRepositoryConstructor
		constructService    maintenanceServiceConstructor
		want                error
	}{
		{
			name: "repository error",
			constructRepository: func(*sqlx.DB) (identityapp.MaintenanceRepository, error) {
				return nil, errors.New("repository construction failed")
			},
			constructService: successfulMaintenanceServiceConstructor,
		},
		{
			name: "typed nil repository",
			constructRepository: func(*sqlx.DB) (identityapp.MaintenanceRepository, error) {
				var repository *testMaintenanceRepository
				return repository, nil
			},
			constructService: successfulMaintenanceServiceConstructor,
			want:             errMaintenanceRuntimeDependency,
		},
		{
			name:                "service error",
			constructRepository: successfulMaintenanceRepositoryConstructor,
			constructService: func(identityapp.Clock, identityapp.MaintenanceRepository) (maintenanceService, error) {
				return nil, errors.New("service construction failed")
			},
		},
		{
			name:                "typed nil service",
			constructRepository: successfulMaintenanceRepositoryConstructor,
			constructService: func(identityapp.Clock, identityapp.MaintenanceRepository) (maintenanceService, error) {
				var service *testMaintenanceService
				return service, nil
			},
			want: errMaintenanceRuntimeDependency,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, mock := newMaintenanceTestDatabase(t)
			mock.ExpectClose()
			factory := newProductionRuntimeFactory(
				func(context.Context, mysqlstore.Config) (*sqlx.DB, error) { return database, nil },
				test.constructRepository,
				test.constructService,
				time.Now,
			)
			runtime, err := factory(context.Background(), appconfig.IdentityMaintenanceMySQLConfig{})
			if runtime != nil || err == nil {
				t.Fatalf("factory(constructor failure) = runtime %v, error %v", runtime, err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("factory error = %v, want %v", err, test.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("constructor failure did not close pool exactly once: %v", err)
			}
		})
	}
}

func TestProductionRuntimeFactoryClosesUnexpectedPoolOnOpenFailure(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	privateCause := errors.New("PRIVATE_OPEN_DSN_CAUSE")
	constructorCalls := 0
	factory := newProductionRuntimeFactory(
		func(context.Context, mysqlstore.Config) (*sqlx.DB, error) { return database, privateCause },
		func(*sqlx.DB) (identityapp.MaintenanceRepository, error) {
			constructorCalls++
			return &testMaintenanceRepository{}, nil
		},
		successfulMaintenanceServiceConstructor,
		time.Now,
	)
	runtime, err := factory(context.Background(), appconfig.IdentityMaintenanceMySQLConfig{})
	if runtime != nil || err != privateCause || constructorCalls != 0 {
		t.Fatalf("factory(open failure) = runtime %v error %v constructor=%d", runtime, err, constructorCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("open failure did not close returned pool once: %v", err)
	}
}

func TestProductionRuntimeFactoryRejectsNilDependenciesAndContextBeforeOpening(t *testing.T) {
	validOpener := func(context.Context, mysqlstore.Config) (*sqlx.DB, error) {
		t.Fatal("opener must not run when an earlier dependency is invalid")
		return nil, nil
	}
	tests := []struct {
		name                string
		opener              maintenanceDatabaseOpener
		constructRepository maintenanceRepositoryConstructor
		constructService    maintenanceServiceConstructor
		now                 func() time.Time
	}{
		{name: "all nil"},
		{name: "opener nil", constructRepository: successfulMaintenanceRepositoryConstructor, constructService: successfulMaintenanceServiceConstructor, now: time.Now},
		{name: "repository constructor nil", opener: validOpener, constructService: successfulMaintenanceServiceConstructor, now: time.Now},
		{name: "service constructor nil", opener: validOpener, constructRepository: successfulMaintenanceRepositoryConstructor, now: time.Now},
		{name: "clock nil", opener: validOpener, constructRepository: successfulMaintenanceRepositoryConstructor, constructService: successfulMaintenanceServiceConstructor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := newProductionRuntimeFactory(test.opener, test.constructRepository, test.constructService, test.now)
			runtime, err := factory(context.Background(), appconfig.IdentityMaintenanceMySQLConfig{})
			if runtime != nil || !errors.Is(err, errMaintenanceRuntimeDependency) {
				t.Fatalf("factory(nil dependency) = runtime %v error %v", runtime, err)
			}
		})
	}

	factory := newProductionRuntimeFactory(validOpener, successfulMaintenanceRepositoryConstructor, successfulMaintenanceServiceConstructor, time.Now)
	if runtime, err := factory(nil, appconfig.IdentityMaintenanceMySQLConfig{}); runtime != nil || !errors.Is(err, errMaintenanceRuntimeContext) {
		t.Fatalf("factory(nil context) = runtime %v error %v", runtime, err)
	}
}

func TestProductionRuntimeFactoryRejectsNilOrUnusableDatabaseSuccess(t *testing.T) {
	tests := []struct {
		name     string
		database *sqlx.DB
	}{
		{name: "nil"},
		{name: "nil sql owner", database: &sqlx.DB{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			constructorCalls := 0
			factory := newProductionRuntimeFactory(
				func(context.Context, mysqlstore.Config) (*sqlx.DB, error) { return test.database, nil },
				func(*sqlx.DB) (identityapp.MaintenanceRepository, error) {
					constructorCalls++
					return &testMaintenanceRepository{}, nil
				},
				successfulMaintenanceServiceConstructor,
				time.Now,
			)
			runtime, err := factory(context.Background(), appconfig.IdentityMaintenanceMySQLConfig{})
			if runtime != nil || !errors.Is(err, errMaintenanceRuntimeDatabase) || constructorCalls != 0 {
				t.Fatalf("factory(unusable database) = runtime %v error %v constructors=%d", runtime, err, constructorCalls)
			}
		})
	}
}

func TestMaintenanceRuntimeReturnsReviewedBudgetResults(t *testing.T) {
	tests := []struct {
		name      string
		sessions  int
		throttles int
		total     int
	}{
		{name: "session budget", sessions: 250, total: 250},
		{name: "throttle budget", throttles: 250, total: 250},
		{name: "full operation", sessions: 250, throttles: 250, total: 500},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, mock := newMaintenanceTestDatabase(t)
			mock.ExpectClose()
			service := &testMaintenanceService{result: mustMaintenanceResult(t, test.sessions, test.throttles)}
			runtime := &identityMaintenanceRuntime{database: database, service: service}
			result, err := runtime.Run(context.Background())
			if err != nil || result.SessionsDeleted() != test.sessions ||
				result.ThrottlesDeleted() != test.throttles || result.TotalDeleted() != test.total ||
				result.TotalDeleted() > identityapp.MaintenanceMaximumRows {
				t.Fatalf("Run() result sessions=%d throttles=%d total=%d error=%v", result.SessionsDeleted(), result.ThrottlesDeleted(), result.TotalDeleted(), err)
			}
			if service.calls != 1 {
				t.Fatalf("service calls = %d, want 1", service.calls)
			}
			if err := runtime.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("database close expectation: %v", err)
			}
		})
	}
}

func TestMaintenanceRuntimeIsOneShotAndNeverRetriesCommitUnknown(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	service := &testMaintenanceService{err: identityapp.ErrCommitOutcomeUnknown}
	runtime := &identityMaintenanceRuntime{database: database, service: service}

	firstResult, firstErr := runtime.Run(context.Background())
	if firstResult != (identityapp.MaintenanceResult{}) || firstErr != identityapp.ErrCommitOutcomeUnknown {
		t.Fatalf("first Run() = result %#v error %v", firstResult, firstErr)
	}
	if _, err := runtime.Run(context.Background()); !errors.Is(err, errMaintenanceRuntimeAlreadyAttempted) {
		t.Fatalf("second Run() error = %v, want no-retry boundary", err)
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d, want exactly one", service.calls)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := runtime.Run(context.Background()); !errors.Is(err, errMaintenanceRuntimeClosed) {
		t.Fatalf("Run(after Close) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestMaintenanceRuntimePassesExactContextWithoutHiddenLoop(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	service := &testMaintenanceService{result: mustMaintenanceResult(t, 3, 4)}
	runtime := &identityMaintenanceRuntime{database: database, service: service}
	ctx := context.WithValue(context.Background(), maintenanceContextKey{}, "exact")
	result, err := runtime.Run(ctx)
	if err != nil || result.TotalDeleted() != 7 || service.calls != 1 || service.context != ctx {
		t.Fatalf("Run() result=%d error=%v calls=%d context=%v", result.TotalDeleted(), err, service.calls, service.context)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestMaintenanceRuntimeRejectsInvalidClockBeforeRepositoryWork(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	repository := &testMaintenanceRepository{}
	service, err := newProductionMaintenanceService(
		identityapp.ClockFunc(func() time.Time { return time.Time{} }),
		repository,
	)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	runtime := &identityMaintenanceRuntime{database: database, service: service}
	_, err = runtime.Run(context.Background())
	if !errors.Is(err, identityapp.ErrAuthenticationUnavailable) || repository.calls != 0 {
		t.Fatalf("Run(zero clock) error=%v repository calls=%d", err, repository.calls)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestMaintenanceRuntimeDefendsNilContextOwnerAndTypedNilService(t *testing.T) {
	var nilRuntime *identityMaintenanceRuntime
	if _, err := nilRuntime.Run(context.Background()); !errors.Is(err, errMaintenanceRuntimeDependency) {
		t.Fatalf("nil runtime Run() error = %v", err)
	}
	if err := nilRuntime.Close(); !errors.Is(err, errMaintenanceRuntimeDependency) {
		t.Fatalf("nil runtime Close() error = %v", err)
	}
	if !nilMaintenanceRuntime(nil) || !nilMaintenanceRuntime(nilRuntime) || nilMaintenanceRuntime(&identityMaintenanceRuntime{}) {
		t.Fatal("nilMaintenanceRuntime did not distinguish nil and non-nil owners")
	}

	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	service := &testMaintenanceService{result: mustMaintenanceResult(t, 0, 0)}
	runtime := &identityMaintenanceRuntime{database: database, service: service}
	if _, err := runtime.Run(nil); !errors.Is(err, errMaintenanceRuntimeContext) {
		t.Fatalf("Run(nil context) error = %v", err)
	}
	if service.calls != 0 {
		t.Fatal("Run(nil context) consumed the one-shot service")
	}
	if _, err := runtime.Run(context.Background()); err != nil || service.calls != 1 {
		t.Fatalf("Run(valid after nil) error=%v calls=%d", err, service.calls)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}

	database, mock = newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	var typedNilService *testMaintenanceService
	runtime = &identityMaintenanceRuntime{database: database, service: typedNilService}
	if _, err := runtime.Run(context.Background()); !errors.Is(err, errMaintenanceRuntimeDependency) {
		t.Fatalf("Run(typed nil service) error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestMaintenanceRuntimeCloseIsExactlyOnceAndLowDisclosure(t *testing.T) {
	database, mock := newMaintenanceTestDatabase(t)
	privateCause := errors.New("driver close exposed mysql://identity:private-password@db")
	mock.ExpectClose().WillReturnError(privateCause)
	runtime := &identityMaintenanceRuntime{database: database}

	first := runtime.Close()
	second := runtime.Close()
	if first != errMaintenanceRuntimeClose || second != errMaintenanceRuntimeClose {
		t.Fatalf("Close() errors = %v and %v, want stable class", first, second)
	}
	if strings.Contains(first.Error(), "private-password") || errors.Is(first, privateCause) {
		t.Fatal("Close() exposed or unwrapped its driver cause")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("Close() did not close exactly once: %v", err)
	}
}

func TestMaintenanceRuntimeFormattingNeverTraversesSensitiveDependencies(t *testing.T) {
	const (
		serviceMarker = "runtime-service-private-marker"
		dsnMarker     = "runtime-dsn-private-marker"
	)
	runtime := &identityMaintenanceRuntime{
		service:  &testMaintenanceService{err: errors.New(serviceMarker)},
		closeErr: errors.New(dsnMarker),
	}
	formatted := fmt.Sprintf("%s|%q|%v|%+v|%#v", runtime, runtime, runtime, runtime, runtime)
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("json.Marshal(runtime) error = %v", err)
	}
	var logged bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))
	logger.Info("test", slog.Any("runtime", runtime))
	combined := formatted + string(encoded) + logged.String()
	for _, marker := range []string{serviceMarker, dsnMarker} {
		if strings.Contains(combined, marker) {
			t.Fatalf("runtime diagnostic boundary exposed %q", marker)
		}
	}
	if !strings.Contains(combined, redactedMaintenanceRuntime) {
		t.Fatal("runtime diagnostic boundary omitted the reviewed redaction marker")
	}
}

func TestProductionDependenciesAndConstructorsUseReviewedAdapters(t *testing.T) {
	if productionDependencies().NewRuntime == nil {
		t.Fatal("productionDependencies omitted the runtime factory")
	}
	database, mock := newMaintenanceTestDatabase(t)
	mock.ExpectClose()
	repository, err := newProductionMaintenanceRepository(database)
	if err != nil || nilInterface(repository) {
		t.Fatalf("repository constructor = %v, error %v", repository, err)
	}
	if _, ok := repository.(*mysqlrepo.Repository); !ok {
		t.Fatalf("repository type = %T, want *mysqlrepo.Repository", repository)
	}
	service, err := newProductionMaintenanceService(identityapp.ClockFunc(time.Now), repository)
	if err != nil || nilInterface(service) {
		t.Fatalf("service constructor = %v, error %v", service, err)
	}
	if _, ok := service.(*identityapp.MaintenanceService); !ok {
		t.Fatalf("service type = %T, want *application.MaintenanceService", service)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("database.Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("constructors unexpectedly took pool ownership: %v", err)
	}
}

type maintenanceContextKey struct{}

func successfulMaintenanceRepositoryConstructor(*sqlx.DB) (identityapp.MaintenanceRepository, error) {
	return &testMaintenanceRepository{}, nil
}

func successfulMaintenanceServiceConstructor(
	identityapp.Clock,
	identityapp.MaintenanceRepository,
) (maintenanceService, error) {
	return &testMaintenanceService{}, nil
}

func newMaintenanceTestDatabase(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	return sqlx.NewDb(database, "sqlmock"), mock
}
