package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/mysqlprovisioner"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/passwordhash"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	mysqlstore "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/mysql"
	"github.com/Atingaii/GrowthOS-Go/internal/platform/appconfig"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type testEnrollmentHasher struct {
	envelope         passwordhash.Envelope
	err              error
	calls            int
	context          context.Context
	passwordArgument []byte
	passwordSnapshot []byte
}

func (hasher *testEnrollmentHasher) HashEnrollment(
	ctx context.Context,
	password []byte,
) (passwordhash.Envelope, error) {
	hasher.calls++
	hasher.context = ctx
	hasher.passwordArgument = password
	hasher.passwordSnapshot = bytes.Clone(password)
	return hasher.envelope, hasher.err
}

type testAccountProvisioner struct {
	err      error
	calls    int
	context  context.Context
	account  identity.WorkforceAccount
	onCreate func()
}

func (provisioner *testAccountProvisioner) Create(
	ctx context.Context,
	account identity.WorkforceAccount,
) error {
	provisioner.calls++
	provisioner.context = ctx
	provisioner.account = account
	if provisioner.onCreate != nil {
		provisioner.onCreate()
	}
	return provisioner.err
}

func TestMySQLProvisionRuntimeConfigMapsDedicatedIdentityAndSingleConnectionPool(t *testing.T) {
	input := appconfig.IdentityProvisionerMySQLConfig{
		MySQLConnectionConfig: appconfig.MySQLConnectionConfig{
			Address:        "mysql.identity.internal:3307",
			Database:       "growthos_identity",
			TLSMode:        appconfig.MySQLTLSVerifyIdentity,
			TLSCAFile:      "/run/secrets/identity-mysql-ca.pem",
			ConnectTimeout: 2700 * time.Millisecond,
			ReadTimeout:    7 * time.Second,
			WriteTimeout:   6 * time.Second,
		},
		User:        "growthos_identity_provisioner",
		Password:    "provisioner-mysql-secret",
		PingTimeout: 2200 * time.Millisecond,
	}

	got := mysqlProvisionRuntimeConfig(input)
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
		t.Fatal("mysqlProvisionRuntimeConfig() did not map every validated dedicated connection field")
	}
	if got.PingTimeout != input.PingTimeout ||
		got.MaxOpenConnections != 1 || got.MaxIdleConnections != 1 {
		t.Fatalf("mysqlProvisionRuntimeConfig() pool = ping %s, open %d, idle %d", got.PingTimeout, got.MaxOpenConnections, got.MaxIdleConnections)
	}
	if got.ConnectionMaxLifetime != 0 || got.ConnectionMaxIdleTime != 0 {
		t.Fatal("mysqlProvisionRuntimeConfig() overrode mysqlstore's reviewed lifetime defaults")
	}

	formatted := fmt.Sprintf("%v|%+v|%#v", got, got, got)
	if strings.Contains(formatted, input.Password) {
		t.Fatal("mysqlstore.Config formatting exposed the provisioner password")
	}
}

func TestProductionRuntimeFactoryMapsConfigurationAndTransfersExactlyOneOwner(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	ctx := context.WithValue(context.Background(), testContextKey{}, "factory")
	input := appconfig.IdentityProvisionerConfig{
		MySQL: appconfig.IdentityProvisionerMySQLConfig{
			MySQLConnectionConfig: appconfig.MySQLConnectionConfig{
				Address:        "127.0.0.1:3306",
				Database:       "growthos",
				TLSMode:        appconfig.MySQLTLSDisabled,
				ConnectTimeout: 3 * time.Second,
				ReadTimeout:    5 * time.Second,
				WriteTimeout:   5 * time.Second,
			},
			User:        "growthos_identity_provisioner",
			Password:    "database-secret",
			PingTimeout: 2 * time.Second,
		},
		OperationTimeout: 3 * time.Second,
	}
	hasher := &testEnrollmentHasher{}
	provisioner := &testAccountProvisioner{}
	var openedContext context.Context
	var openedConfig mysqlstore.Config
	var hasherConfig passwordhash.Config
	var provisionerDatabase *sqlx.DB

	factory := newProductionRuntimeFactory(
		func(gotContext context.Context, gotConfig mysqlstore.Config) (*sqlx.DB, error) {
			openedContext = gotContext
			openedConfig = gotConfig
			return database, nil
		},
		func(config passwordhash.Config) (enrollmentHasher, error) {
			hasherConfig = config
			return hasher, nil
		},
		func(gotDatabase *sqlx.DB) (accountProvisioner, error) {
			provisionerDatabase = gotDatabase
			return provisioner, nil
		},
		func() time.Time { return time.Now() },
	)

	runtime, err := factory(ctx, input)
	if err != nil || nilProvisionRuntime(runtime) {
		t.Fatalf("factory() = runtime %v, error %v", runtime, err)
	}
	if openedContext != ctx || openedConfig != mysqlProvisionRuntimeConfig(input.MySQL) {
		t.Fatal("factory did not pass the lifecycle context and exact provisioner MySQL mapping")
	}
	if provisionerDatabase != database {
		t.Fatal("provisioner constructor did not receive the uniquely owned pool")
	}
	defaults := passwordhash.DefaultConfig()
	if hasherConfig.MaxConcurrent != defaults.MaxConcurrent ||
		hasherConfig.AcquireTimeout != defaults.AcquireTimeout ||
		hasherConfig.Entropy == nil {
		t.Fatal("hasher constructor did not receive the reviewed enrollment defaults")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database ownership expectation: %v", err)
	}
}

func TestProductionRuntimeFactoryClosesUnexpectedDatabaseFromOpenErrorOnce(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	openCause := errors.New("private opener cause")
	constructorCalled := false
	factory := newProductionRuntimeFactory(
		func(context.Context, mysqlstore.Config) (*sqlx.DB, error) {
			return database, openCause
		},
		func(passwordhash.Config) (enrollmentHasher, error) {
			constructorCalled = true
			return &testEnrollmentHasher{}, nil
		},
		func(*sqlx.DB) (accountProvisioner, error) {
			constructorCalled = true
			return &testAccountProvisioner{}, nil
		},
		time.Now,
	)

	runtime, err := factory(context.Background(), appconfig.IdentityProvisionerConfig{})
	if runtime != nil || !errors.Is(err, openCause) {
		t.Fatalf("factory(open error) = runtime %v, error %v", runtime, err)
	}
	if constructorCalled {
		t.Fatal("factory invoked a constructor after database open failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database returned with open error was not closed exactly once: %v", err)
	}
}

func TestProductionRuntimeFactoryReleasesDatabaseForEveryConstructorFailure(t *testing.T) {
	tests := []struct {
		name                 string
		constructHasher      enrollmentHasherConstructor
		constructProvisioner accountProvisionerConstructor
		want                 error
	}{
		{
			name: "hasher error",
			constructHasher: func(passwordhash.Config) (enrollmentHasher, error) {
				return nil, errors.New("hasher construction failed")
			},
			constructProvisioner: successfulProvisionerConstructor,
		},
		{
			name: "typed nil hasher",
			constructHasher: func(passwordhash.Config) (enrollmentHasher, error) {
				var hasher *testEnrollmentHasher
				return hasher, nil
			},
			constructProvisioner: successfulProvisionerConstructor,
			want:                 errProvisionRuntimeDependency,
		},
		{
			name:            "provisioner error",
			constructHasher: successfulHasherConstructor,
			constructProvisioner: func(*sqlx.DB) (accountProvisioner, error) {
				return nil, errors.New("provisioner construction failed")
			},
		},
		{
			name:            "typed nil provisioner",
			constructHasher: successfulHasherConstructor,
			constructProvisioner: func(*sqlx.DB) (accountProvisioner, error) {
				var provisioner *testAccountProvisioner
				return provisioner, nil
			},
			want: errProvisionRuntimeDependency,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, mock := newProvisionTestDatabase(t)
			mock.ExpectClose()
			factory := newProductionRuntimeFactory(
				func(context.Context, mysqlstore.Config) (*sqlx.DB, error) {
					return database, nil
				},
				test.constructHasher,
				test.constructProvisioner,
				time.Now,
			)
			runtime, err := factory(context.Background(), appconfig.IdentityProvisionerConfig{})
			if runtime != nil || err == nil {
				t.Fatalf("factory(constructor failure) = runtime %v, error %v", runtime, err)
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("factory error = %v, want %v", err, test.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("constructor failure did not close the pool exactly once: %v", err)
			}
		})
	}
}

func TestProductionRuntimeFactoryRejectsNilDependenciesBeforeOpeningDatabase(t *testing.T) {
	validOpener := func(context.Context, mysqlstore.Config) (*sqlx.DB, error) {
		t.Fatal("opener must not run when a dependency is nil")
		return nil, nil
	}
	tests := []struct {
		name                 string
		opener               provisionDatabaseOpener
		constructHasher      enrollmentHasherConstructor
		constructProvisioner accountProvisionerConstructor
		now                  provisionClock
	}{
		{name: "all nil"},
		{name: "opener nil", constructHasher: successfulHasherConstructor, constructProvisioner: successfulProvisionerConstructor, now: time.Now},
		{name: "hasher constructor nil", opener: validOpener, constructProvisioner: successfulProvisionerConstructor, now: time.Now},
		{name: "provisioner constructor nil", opener: validOpener, constructHasher: successfulHasherConstructor, now: time.Now},
		{name: "clock nil", opener: validOpener, constructHasher: successfulHasherConstructor, constructProvisioner: successfulProvisionerConstructor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := newProductionRuntimeFactory(test.opener, test.constructHasher, test.constructProvisioner, test.now)
			runtime, err := factory(context.Background(), appconfig.IdentityProvisionerConfig{})
			if runtime != nil || !errors.Is(err, errProvisionRuntimeDependency) {
				t.Fatalf("factory(nil dependency) = runtime %v, error %v", runtime, err)
			}
		})
	}

	factory := newProductionRuntimeFactory(validOpener, successfulHasherConstructor, successfulProvisionerConstructor, time.Now)
	if runtime, err := factory(nil, appconfig.IdentityProvisionerConfig{}); runtime != nil || !errors.Is(err, errProvisionRuntimeContext) {
		t.Fatalf("factory(nil context) = runtime %v, error %v", runtime, err)
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
			constructorCalled := false
			factory := newProductionRuntimeFactory(
				func(context.Context, mysqlstore.Config) (*sqlx.DB, error) {
					return test.database, nil
				},
				func(passwordhash.Config) (enrollmentHasher, error) {
					constructorCalled = true
					return &testEnrollmentHasher{}, nil
				},
				successfulProvisionerConstructor,
				time.Now,
			)
			runtime, err := factory(context.Background(), appconfig.IdentityProvisionerConfig{})
			if runtime != nil || !errors.Is(err, errProvisionRuntimeDatabase) {
				t.Fatalf("factory(unusable database) = runtime %v, error %v", runtime, err)
			}
			if constructorCalled {
				t.Fatal("factory constructed adapters around an unusable database")
			}
		})
	}
}

func TestProvisionRuntimeCreatesFixedCanonicalWorkforceAccountAndClearsPrivateCopies(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	envelope := validProvisionTestEnvelope(t)
	hasher := &testEnrollmentHasher{envelope: envelope}
	provisioner := &testAccountProvisioner{}
	location := time.FixedZone("sensitive-local-zone", 8*60*60)
	clockValue := time.Date(2026, time.September, 2, 9, 10, 11, 987654321, location)
	runtime := &identityProvisionRuntime{
		database:    database,
		hasher:      hasher,
		provisioner: provisioner,
		now:         func() time.Time { return clockValue },
	}
	command := validProvisionTestCommand()
	password := []byte("correct horse battery staple")
	wantPassword := bytes.Clone(password)
	defer clearProvisionBytes(password)
	defer clearProvisionBytes(wantPassword)
	provisioner.onCreate = func() {
		if !allProvisionBytesZero(hasher.passwordArgument) || !allProvisionBytesZero(password) {
			t.Error("durable Create observed password bytes retained after HashEnrollment")
		}
	}
	ctx := context.WithValue(context.Background(), testContextKey{}, "create")

	if err := runtime.Create(ctx, command, password); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if hasher.calls != 1 || hasher.context != ctx || !bytes.Equal(hasher.passwordSnapshot, wantPassword) {
		t.Fatal("Create() did not call enrollment hashing once with the lifecycle context and password")
	}
	defer clearProvisionBytes(hasher.passwordSnapshot)
	if !allProvisionBytesZero(hasher.passwordArgument) {
		t.Fatal("Create() retained its private password copy after returning")
	}
	if !allProvisionBytesZero(password) {
		t.Fatal("Create() did not clear the supplied password immediately after hashing")
	}
	if provisioner.calls != 1 || provisioner.context != ctx {
		t.Fatal("Create() did not call the create-only adapter once with the lifecycle context")
	}

	account := provisioner.account
	if account.ID() != command.accountID || account.LoginName() != command.loginName ||
		account.PrincipalID() != command.principalID {
		t.Fatal("Create() changed one of the reviewed account identities")
	}
	if account.Status() != identity.AccountStatusEnabled ||
		account.CredentialVersion() != identity.CredentialVersion(1) ||
		account.AuthenticationEpoch() != identity.AuthenticationEpoch(1) {
		t.Fatal("Create() did not fix enabled/version-one/epoch-one lifecycle state")
	}
	wantTime := clockValue.Round(0).UTC().Truncate(time.Microsecond)
	if account.CreatedAt() != wantTime || account.UpdatedAt() != wantTime ||
		account.CreatedAt().Location() != time.UTC {
		t.Fatalf("Create() times = %s and %s, want same canonical %s", account.CreatedAt(), account.UpdatedAt(), wantTime)
	}
	gotEnvelope := account.CredentialEnvelope().Bytes()
	defer clearProvisionBytes(gotEnvelope)
	if string(gotEnvelope) != envelope.Encoded() {
		t.Fatal("Create() did not persist the enrollment hasher's exact validated envelope")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestProvisionRuntimeIsOneShotAndNeverRetriesUnknownCommitOutcome(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	hasher := &testEnrollmentHasher{envelope: validProvisionTestEnvelope(t)}
	provisioner := &testAccountProvisioner{err: mysqlprovisioner.ErrCommitOutcomeUnknown}
	runtime := &identityProvisionRuntime{
		database: database, hasher: hasher, provisioner: provisioner, now: time.Now,
	}
	password := []byte("correct horse battery staple")
	defer clearProvisionBytes(password)

	first := runtime.Create(context.Background(), validProvisionTestCommand(), password)
	if first != mysqlprovisioner.ErrCommitOutcomeUnknown {
		t.Fatalf("first Create() error = %v, want unchanged ErrCommitOutcomeUnknown", first)
	}
	second := runtime.Create(context.Background(), validProvisionTestCommand(), password)
	if !errors.Is(second, errProvisionRuntimeAlreadyAttempted) {
		t.Fatalf("second Create() error = %v, want no-retry boundary", second)
	}
	if hasher.calls != 1 || provisioner.calls != 1 {
		t.Fatalf("one-shot calls = hash %d, provision %d", hasher.calls, provisioner.calls)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := runtime.Create(context.Background(), validProvisionTestCommand(), password); !errors.Is(err, errProvisionRuntimeClosed) {
		t.Fatalf("Create(after close) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestProvisionRuntimeRejectsInvalidClockBeforeHashing(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	hasher := &testEnrollmentHasher{envelope: validProvisionTestEnvelope(t)}
	provisioner := &testAccountProvisioner{}
	runtime := &identityProvisionRuntime{
		database: database, hasher: hasher, provisioner: provisioner,
		now: func() time.Time { return time.Time{} },
	}

	err := runtime.Create(context.Background(), validProvisionTestCommand(), []byte("not retained by fake"))
	if !errors.Is(err, errProvisionRuntimeClock) {
		t.Fatalf("Create(zero clock) error = %v", err)
	}
	if hasher.calls != 0 || provisioner.calls != 0 {
		t.Fatal("Create(zero clock) performed sensitive or durable work")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestProvisionRuntimeRejectsInvalidCommandBeforeClockOrHashing(t *testing.T) {
	tests := []struct {
		name    string
		command provisionCommand
	}{
		{name: "account id", command: provisionCommand{
			loginName: identity.LoginName("alex.rivera"), principalID: identity.PrincipalID("principal.alex"),
		}},
		{name: "login name", command: provisionCommand{
			accountID: identity.AccountID("account.alex"), principalID: identity.PrincipalID("principal.alex"),
		}},
		{name: "principal id", command: provisionCommand{
			accountID: identity.AccountID("account.alex"), loginName: identity.LoginName("alex.rivera"),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, mock := newProvisionTestDatabase(t)
			mock.ExpectClose()
			hasher := &testEnrollmentHasher{envelope: validProvisionTestEnvelope(t)}
			provisioner := &testAccountProvisioner{}
			clockCalled := false
			runtime := &identityProvisionRuntime{
				database: database, hasher: hasher, provisioner: provisioner,
				now: func() time.Time {
					clockCalled = true
					return time.Now()
				},
			}
			password := []byte("caller-clears-on-pre-hash-rejection")
			defer clearProvisionBytes(password)
			if err := runtime.Create(context.Background(), test.command, password); err == nil {
				t.Fatal("Create(invalid command) error = nil")
			}
			if clockCalled || hasher.calls != 0 || provisioner.calls != 0 {
				t.Fatal("Create(invalid command) performed clock, Argon2, or durable work")
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

func TestProvisionRuntimePassesHasherFailureWithoutCallingProvisioner(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	hashCause := passwordhash.ErrPasswordRejected
	hasher := &testEnrollmentHasher{err: hashCause}
	provisioner := &testAccountProvisioner{}
	runtime := &identityProvisionRuntime{
		database: database, hasher: hasher, provisioner: provisioner, now: time.Now,
	}
	password := []byte("short")
	defer clearProvisionBytes(password)

	err := runtime.Create(context.Background(), validProvisionTestCommand(), password)
	if err != hashCause {
		t.Fatalf("Create(hash failure) error = %v, want unchanged %v", err, hashCause)
	}
	if provisioner.calls != 0 {
		t.Fatal("Create(hash failure) called the durable adapter")
	}
	if !allProvisionBytesZero(hasher.passwordArgument) || !allProvisionBytesZero(password) {
		t.Fatal("Create(hash failure) did not clear both private and supplied password slices")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestProvisionRuntimeDefendsNilAndTypedNilBoundaries(t *testing.T) {
	var nilRuntime *identityProvisionRuntime
	if err := nilRuntime.Create(context.Background(), validProvisionTestCommand(), nil); !errors.Is(err, errProvisionRuntimeDependency) {
		t.Fatalf("nil runtime Create() error = %v", err)
	}
	if err := nilRuntime.Close(); !errors.Is(err, errProvisionRuntimeDependency) {
		t.Fatalf("nil runtime Close() error = %v", err)
	}
	if !nilProvisionRuntime(nil) || !nilProvisionRuntime(nilRuntime) {
		t.Fatal("nilProvisionRuntime did not reject nil and typed-nil owners")
	}
	if nilProvisionRuntime(&identityProvisionRuntime{}) {
		t.Fatal("nilProvisionRuntime rejected a non-nil owner")
	}

	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	var typedNilHasher *testEnrollmentHasher
	runtime := &identityProvisionRuntime{
		database:    database,
		hasher:      typedNilHasher,
		provisioner: &testAccountProvisioner{},
		now:         time.Now,
	}
	if err := runtime.Create(context.Background(), validProvisionTestCommand(), nil); !errors.Is(err, errProvisionRuntimeDependency) {
		t.Fatalf("Create(typed nil hasher) error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}

	database, mock = newProvisionTestDatabase(t)
	mock.ExpectClose()
	var typedNilProvisioner *testAccountProvisioner
	runtime = &identityProvisionRuntime{
		database:    database,
		hasher:      &testEnrollmentHasher{},
		provisioner: typedNilProvisioner,
		now:         time.Now,
	}
	if err := runtime.Create(context.Background(), validProvisionTestCommand(), nil); !errors.Is(err, errProvisionRuntimeDependency) {
		t.Fatalf("Create(typed nil provisioner) error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("database close expectation: %v", err)
	}
}

func TestProvisionRuntimeCloseIsExactlyOnceAndLowDisclosureOnFailure(t *testing.T) {
	database, mock := newProvisionTestDatabase(t)
	privateCause := errors.New("driver close exposed dsn://private-password")
	mock.ExpectClose().WillReturnError(privateCause)
	runtime := &identityProvisionRuntime{database: database}

	first := runtime.Close()
	second := runtime.Close()
	if first != errProvisionRuntimeClose || second != errProvisionRuntimeClose {
		t.Fatalf("Close() errors = %v and %v, want stable low-disclosure class", first, second)
	}
	if strings.Contains(first.Error(), "private-password") || errors.Is(first, privateCause) {
		t.Fatal("Close() exposed or unwrapped a driver close cause")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("Close() did not close exactly once: %v", err)
	}
}

func TestProvisionRuntimeFormattingNeverTraversesSensitiveDependencies(t *testing.T) {
	const (
		passwordMarker = "runtime-password-marker"
		envelopeMarker = "runtime-envelope-marker"
		dsnMarker      = "runtime-dsn-marker"
	)
	runtime := &identityProvisionRuntime{
		hasher: &testEnrollmentHasher{
			passwordSnapshot: []byte(passwordMarker),
			err:              errors.New(envelopeMarker),
		},
		provisioner: &testAccountProvisioner{err: errors.New(dsnMarker)},
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
	for _, marker := range []string{passwordMarker, envelopeMarker, dsnMarker} {
		if strings.Contains(combined, marker) {
			t.Fatalf("runtime diagnostic boundary exposed %q", marker)
		}
	}
	if !strings.Contains(combined, redactedProvisionRuntime) {
		t.Fatal("runtime diagnostic boundary omitted the reviewed redaction marker")
	}
}

func TestProductionDependenciesExposeOnlyRuntimeFactory(t *testing.T) {
	dependencies := productionDependencies()
	if dependencies.NewRuntime == nil {
		t.Fatal("productionDependencies() omitted the provision runtime factory")
	}
}

func TestProductionConstructorsWirePasswordEnrollmentAndCreateOnlyAdapter(t *testing.T) {
	hasher, err := newProductionEnrollmentHasher(passwordhash.DefaultConfig())
	if err != nil || nilInterface(hasher) {
		t.Fatalf("newProductionEnrollmentHasher() = hasher %v, error %v", hasher, err)
	}
	if _, ok := hasher.(*passwordhash.Hasher); !ok {
		t.Fatalf("newProductionEnrollmentHasher() type = %T, want *passwordhash.Hasher", hasher)
	}

	database, mock := newProvisionTestDatabase(t)
	mock.ExpectClose()
	provisioner, err := newProductionAccountProvisioner(database)
	if err != nil || nilInterface(provisioner) {
		t.Fatalf("newProductionAccountProvisioner() = provisioner %v, error %v", provisioner, err)
	}
	if _, ok := provisioner.(*mysqlprovisioner.Provisioner); !ok {
		t.Fatalf("newProductionAccountProvisioner() type = %T, want *mysqlprovisioner.Provisioner", provisioner)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("database.Close() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("adapter constructor took database ownership: %v", err)
	}
}

type testContextKey struct{}

func successfulHasherConstructor(passwordhash.Config) (enrollmentHasher, error) {
	return &testEnrollmentHasher{}, nil
}

func successfulProvisionerConstructor(*sqlx.DB) (accountProvisioner, error) {
	return &testAccountProvisioner{}, nil
}

func newProvisionTestDatabase(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	return sqlx.NewDb(database, "sqlmock"), mock
}

func validProvisionTestEnvelope(t *testing.T) passwordhash.Envelope {
	t.Helper()
	salt := base64.RawStdEncoding.EncodeToString(make([]byte, passwordhash.SaltBytes))
	output := base64.RawStdEncoding.EncodeToString(make([]byte, passwordhash.OutputBytes))
	encoded := "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$" + output
	envelope, err := passwordhash.ParseEnvelope(encoded)
	if err != nil {
		t.Fatalf("passwordhash.ParseEnvelope(test fixture) error = %v", err)
	}
	return envelope
}

func validProvisionTestCommand() provisionCommand {
	return provisionCommand{
		accountID:    identity.AccountID("account.alex"),
		loginName:    identity.LoginName("alex.rivera"),
		principalID:  identity.PrincipalID("principal.alex"),
		passwordFile: "/run/secrets/alex-enrollment-password",
	}
}

func allProvisionBytesZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
