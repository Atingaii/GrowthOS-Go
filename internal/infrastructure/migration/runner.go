package dbmigration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	dbmysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const (
	defaultPath             = "sql"
	defaultMigrationsTable  = "schema_migrations"
	defaultLockTimeout      = 40 * time.Second
	defaultNetworkTimeout   = 35 * time.Second
	defaultStatementTimeout = 30 * time.Second
	maximumLockTimeout      = 11 * time.Minute
	maximumNetworkTimeout   = 10*time.Minute + 30*time.Second
	maximumStatementTimeout = 10 * time.Minute
	migrationTimeoutBudget  = 5 * time.Second
)

var (
	errInvalidConfig = errors.New("invalid migration configuration")
	errInvalidSource = errors.New("invalid migration source")
	errRunnerClosed  = errors.New("migration runner closed")

	migrationNamePattern = regexp.MustCompile(`^([0-9]{6})_[a-z][a-z0-9_]{0,63}\.up\.sql$`)
	identifierPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

// Config controls the source directory and finite database execution bounds.
// NetworkReadTimeout must match the dedicated migration connector supplied to
// New. Zero values select one internally consistent set of defaults.
type Config struct {
	Path               string
	MigrationsTable    string
	LockTimeout        time.Duration
	NetworkReadTimeout time.Duration
	StatementTimeout   time.Duration
}

type normalizedConfig struct {
	path               string
	migrationsTable    string
	lockTimeout        time.Duration
	networkReadTimeout time.Duration
	statementTimeout   time.Duration
}

// ResultState describes a successful forward-run outcome.
type ResultState string

const (
	ResultNoMigrations ResultState = "no_migrations"
	ResultNoChange     ResultState = "no_change"
	ResultApplied      ResultState = "applied"
)

// Result is returned only for successful outcomes.
type Result struct {
	State   ResultState
	Version uint
}

// StatusState describes the current forward migration state.
type StatusState string

const (
	StatusNoMigrations  StatusState = "no_migrations"
	StatusUninitialized StatusState = "uninitialized"
	StatusPending       StatusState = "pending"
	StatusClean         StatusState = "clean"
)

// Status intentionally excludes credentials, SQL, and driver error strings.
type Status struct {
	State   StatusState
	Version uint
	Latest  uint
}

type migrationEngine interface {
	Up() error
	Version() (uint, bool, error)
	Stop()
	Close() error
}

type engineFactory func(context.Context, fs.FS, *sql.DB, normalizedConfig) (migrationEngine, error)

// Runner serializes migration operations and owns the supplied *sql.DB from
// the moment New is called, including constructor-failure and no-migration
// paths. Consumers close only Runner.
type Runner struct {
	mu       sync.Mutex
	db       *sql.DB
	engine   migrationEngine
	versions []uint
	closed   bool
	terminal bool
}

// New validates the embedded migration set before opening the adapter. If it
// contains no .up.sql files, no database query is issued and schema_migrations
// is not created.
func New(ctx context.Context, fsys fs.FS, ownedDB *sql.DB, cfg Config) (*Runner, error) {
	return newRunner(ctx, fsys, ownedDB, cfg, productionEngine)
}

func newRunner(ctx context.Context, fsys fs.FS, ownedDB *sql.DB, cfg Config, factory engineFactory) (*Runner, error) {
	if ownedDB == nil {
		return nil, newError(StageConfigInvalid, errInvalidConfig)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = ownedDB.Close()
		}
	}()

	if ctx == nil || fsys == nil || factory == nil {
		return nil, newError(StageConfigInvalid, errInvalidConfig)
	}
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, newError(StageConfigInvalid, err)
	}
	versions, err := scanVersions(fsys, normalized.path)
	if err != nil {
		return nil, newError(StageSourceInvalid, err)
	}

	runner := &Runner{db: ownedDB, versions: versions}
	if len(versions) > 0 {
		engine, err := factory(ctx, fsys, ownedDB, normalized)
		if err != nil {
			if !nilMigrationEngine(engine) {
				_ = engine.Close()
			}
			return nil, newError(StageOpen, err)
		}
		if nilMigrationEngine(engine) {
			return nil, newError(StageOpen, errInvalidConfig)
		}
		runner.engine = engine
	}
	closeOnError = false
	return runner, nil
}

// Up applies every pending .up.sql migration. Context cancellation requests a
// migrate.GracefulStop and waits until the current migration boundary; the
// statement timeout remains the upper bound for an individual SQL execution.
func (r *Runner) Up(ctx context.Context) (Result, error) {
	if r == nil || ctx == nil {
		return Result{}, newError(StageConfigInvalid, errInvalidConfig)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Result{}, newError(StageConfigInvalid, errRunnerClosed)
	}
	if r.terminal {
		return Result{}, cancelledError(errRunnerClosed)
	}
	if len(r.versions) == 0 {
		return Result{State: ResultNoMigrations}, nil
	}
	if err := ctx.Err(); err != nil {
		return Result{}, cancelledError(err)
	}

	done := make(chan error, 1)
	go func() { done <- r.engine.Up() }()

	select {
	case err := <-done:
		return r.finishUp(err)
	case <-ctx.Done():
		r.engine.Stop()
		engineErr := <-done
		r.terminal = true
		if dirtyErr := r.dirtyAfterExecution(engineErr); dirtyErr != nil {
			return Result{}, dirtyError(errors.Join(ErrCancelled, ctx.Err(), dirtyErr))
		}
		return Result{}, cancelledError(errors.Join(ctx.Err(), engineErr))
	}
}

func (r *Runner) finishUp(err error) (Result, error) {
	if errors.Is(err, migrate.ErrNoChange) {
		version, versionErr := r.cleanVersion()
		if versionErr != nil {
			return Result{}, versionErr
		}
		return Result{State: ResultNoChange, Version: version}, nil
	}
	if err != nil {
		if dirtyErr := r.dirtyAfterExecution(err); dirtyErr != nil {
			return Result{}, dirtyError(dirtyErr)
		}
		return Result{}, classifyEngineError(StageApply, err)
	}
	version, versionErr := r.cleanVersion()
	if versionErr != nil {
		return Result{}, versionErr
	}
	return Result{State: ResultApplied, Version: version}, nil
}

func (r *Runner) dirtyAfterExecution(executionErr error) error {
	var dirty migrate.ErrDirty
	if errors.As(executionErr, &dirty) {
		return errors.Join(ErrDirty, executionErr)
	}
	version, isDirty, versionErr := r.engine.Version()
	if isDirty {
		return errors.Join(ErrDirty, executionErr, versionErr, migrate.ErrDirty{Version: int(version)})
	}
	return nil
}

// Status reports the applied and latest embedded versions. A dirty database is
// returned as the explicit safe ErrDirty path rather than as ordinary status.
func (r *Runner) Status(ctx context.Context) (Status, error) {
	if r == nil || ctx == nil {
		return Status{}, newError(StageConfigInvalid, errInvalidConfig)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return Status{}, newError(StageConfigInvalid, errRunnerClosed)
	}
	if r.terminal {
		return Status{}, cancelledError(errRunnerClosed)
	}
	if err := ctx.Err(); err != nil {
		return Status{}, cancelledError(err)
	}
	if len(r.versions) == 0 {
		return Status{State: StatusNoMigrations}, nil
	}
	latest := r.versions[len(r.versions)-1]
	version, dirty, err := r.engine.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return Status{State: StatusUninitialized, Latest: latest}, nil
	}
	if err != nil {
		return Status{}, classifyEngineError(StageStatus, err)
	}
	if dirty {
		return Status{}, dirtyError(migrate.ErrDirty{Version: int(version)})
	}
	if !containsVersion(r.versions, version) {
		return Status{}, newError(StageVersionMismatch, errors.Join(ErrVersionMismatch, errInvalidSource))
	}
	if err := ctx.Err(); err != nil {
		return Status{}, cancelledError(err)
	}
	state := StatusClean
	if version < latest {
		state = StatusPending
	}
	return Status{State: state, Version: version, Latest: latest}, nil
}

// Close releases the source adapter, its dedicated connection, and the owned
// pool. It is idempotent.
func (r *Runner) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true

	var closeErrors []error
	if r.engine != nil {
		if err := r.engine.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	if r.db != nil {
		if err := r.db.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	if len(closeErrors) > 0 {
		return newError(StageClose, errors.Join(closeErrors...))
	}
	return nil
}

func (r *Runner) cleanVersion() (uint, error) {
	version, dirty, err := r.engine.Version()
	if err != nil {
		return 0, classifyEngineError(StageStatus, err)
	}
	if dirty {
		return 0, dirtyError(migrate.ErrDirty{Version: int(version)})
	}
	if !containsVersion(r.versions, version) {
		return 0, newError(StageVersionMismatch, errors.Join(ErrVersionMismatch, errInvalidSource))
	}
	return version, nil
}

func normalizeConfig(in Config) (normalizedConfig, error) {
	out := normalizedConfig{
		path:               in.Path,
		migrationsTable:    in.MigrationsTable,
		lockTimeout:        in.LockTimeout,
		networkReadTimeout: in.NetworkReadTimeout,
		statementTimeout:   in.StatementTimeout,
	}
	if out.path == "" {
		out.path = defaultPath
	}
	if out.migrationsTable == "" {
		out.migrationsTable = defaultMigrationsTable
	}
	if out.lockTimeout == 0 {
		out.lockTimeout = defaultLockTimeout
	}
	if out.networkReadTimeout == 0 {
		out.networkReadTimeout = defaultNetworkTimeout
	}
	if out.statementTimeout == 0 {
		out.statementTimeout = defaultStatementTimeout
	}
	if !fs.ValidPath(out.path) || out.path == "." || strings.HasPrefix(out.path, "/") {
		return normalizedConfig{}, errInvalidConfig
	}
	if !identifierPattern.MatchString(out.migrationsTable) {
		return normalizedConfig{}, errInvalidConfig
	}
	// The upstream MySQL adapter executes GET_LOCK with a fixed ten-second
	// server wait in a goroutine. The network deadline must finish before the
	// outer lock deadline, while statement cancellation must win before the
	// network deadline. Fixed margins cover scheduling and cleanup work.
	if out.lockTimeout < 11*time.Second || out.lockTimeout > maximumLockTimeout {
		return normalizedConfig{}, errInvalidConfig
	}
	if out.networkReadTimeout <= 0 || out.networkReadTimeout > maximumNetworkTimeout {
		return normalizedConfig{}, errInvalidConfig
	}
	if out.statementTimeout <= 0 || out.statementTimeout > maximumStatementTimeout {
		return normalizedConfig{}, errInvalidConfig
	}
	if out.networkReadTimeout <= migrationTimeoutBudget ||
		out.statementTimeout > out.networkReadTimeout-migrationTimeoutBudget {
		return normalizedConfig{}, errInvalidConfig
	}
	if out.lockTimeout <= migrationTimeoutBudget ||
		out.networkReadTimeout > out.lockTimeout-migrationTimeoutBudget {
		return normalizedConfig{}, errInvalidConfig
	}
	return out, nil
}

func containsVersion(versions []uint, version uint) bool {
	index := sort.Search(len(versions), func(index int) bool { return versions[index] >= version })
	return index < len(versions) && versions[index] == version
}

func scanVersions(fsys fs.FS, path string) ([]uint, error) {
	entries, err := fs.ReadDir(fsys, path)
	if err != nil {
		return nil, errInvalidSource
	}
	seen := make(map[uint]struct{})
	versions := make([]uint, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, errInvalidSource
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".down.sql") {
			return nil, errInvalidSource
		}
		if !strings.HasSuffix(name, ".up.sql") {
			if strings.HasSuffix(name, ".sql") {
				return nil, errInvalidSource
			}
			continue
		}
		matches := migrationNamePattern.FindStringSubmatch(name)
		if matches == nil {
			return nil, errInvalidSource
		}
		rawVersion, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil || rawVersion == 0 {
			return nil, errInvalidSource
		}
		version := uint(rawVersion)
		if uint64(version) != rawVersion {
			return nil, errInvalidSource
		}
		if _, duplicate := seen[version]; duplicate {
			return nil, errInvalidSource
		}
		seen[version] = struct{}{}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions, nil
}

func nilMigrationEngine(engine migrationEngine) bool {
	if engine == nil {
		return true
	}
	value := reflect.ValueOf(engine)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func classifyEngineError(stage Stage, err error) error {
	var dirty migrate.ErrDirty
	if errors.As(err, &dirty) {
		return dirtyError(err)
	}
	return newError(stage, err)
}

func dirtyError(cause error) error {
	return newError(StageDirty, errors.Join(ErrDirty, cause))
}

func cancelledError(cause error) error {
	return newError(StageCancelled, errors.Join(ErrCancelled, cause))
}

type migrateEngine struct {
	migrate *migrate.Migrate
}

func productionEngine(ctx context.Context, fsys fs.FS, db *sql.DB, cfg normalizedConfig) (migrationEngine, error) {
	sourceDriver, err := iofs.New(fsys, cfg.path)
	if err != nil {
		return nil, err
	}
	closeSource := true
	defer func() {
		if closeSource {
			_ = sourceDriver.Close()
		}
	}()

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	closeConn := true
	defer func() {
		if closeConn {
			_ = conn.Close()
		}
	}()

	databaseDriver, err := dbmysql.WithConnection(ctx, conn, &dbmysql.Config{
		MigrationsTable:  cfg.migrationsTable,
		NoLock:           false,
		StatementTimeout: cfg.statementTimeout,
	})
	if err != nil {
		return nil, err
	}
	migrator, err := migrate.NewWithInstance("iofs", sourceDriver, "mysql", databaseDriver)
	if err != nil {
		_ = databaseDriver.Close()
		return nil, err
	}
	migrator.LockTimeout = cfg.lockTimeout
	closeSource = false
	closeConn = false
	return &migrateEngine{migrate: migrator}, nil
}

func (e *migrateEngine) Up() error {
	return e.migrate.Up()
}

func (e *migrateEngine) Version() (uint, bool, error) {
	return e.migrate.Version()
}

func (e *migrateEngine) Stop() {
	select {
	case e.migrate.GracefulStop <- true:
	default:
	}
}

func (e *migrateEngine) Close() error {
	sourceErr, databaseErr := e.migrate.Close()
	return errors.Join(sourceErr, databaseErr)
}

// String formatting is intentionally limited to public result values; Runner
// and Config have no String method to avoid accidental credential propagation.
func (s ResultState) String() string { return fmt.Sprint(string(s)) }
func (s StatusState) String() string { return fmt.Sprint(string(s)) }
