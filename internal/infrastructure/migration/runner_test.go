package dbmigration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/golang-migrate/migrate/v4"
)

func TestNoMigrationsDoesNotOpenDatabaseOrCreateVersionTable(t *testing.T) {
	t.Parallel()

	connector := &countingConnector{}
	db := sql.OpenDB(connector)
	runner, err := New(context.Background(), fstest.MapFS{
		"sql/README.md": {Data: []byte("future migrations live here")},
	}, db, Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	status, err := runner.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != StatusNoMigrations || status.Version != 0 || status.Latest != 0 {
		t.Fatalf("Status() = %+v", status)
	}
	result, err := runner.Up(context.Background())
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if result.State != ResultNoMigrations {
		t.Fatalf("Up() = %+v", result)
	}
	if got := connector.connects.Load(); got != 0 {
		t.Fatalf("database opened %d times; no-migration path must not connect", got)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := runner.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestNewClosesOwnedDatabaseOnValidationFailure(t *testing.T) {
	t.Parallel()

	connector := &countingConnector{}
	db := sql.OpenDB(connector)
	_, err := New(context.Background(), fstest.MapFS{
		"sql/01_bad.up.sql": {Data: []byte("SELECT 1")},
	}, db, Config{})
	if !IsStage(err, StageSourceInvalid) {
		t.Fatalf("New() error = %v", err)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("owned database remained usable after constructor failure")
	}
	if got := connector.connects.Load(); got != 0 {
		t.Fatalf("closed database attempted %d connections", got)
	}
}

func TestScanVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		files    fstest.MapFS
		want     []uint
		wantFail bool
	}{
		{
			name: "sorted forward migrations",
			files: fstest.MapFS{
				"sql/000010_add_index.up.sql":      {Data: []byte("SELECT 10")},
				"sql/000001_initial_schema.up.sql": {Data: []byte("SELECT 1")},
				"sql/README.md":                    {Data: []byte("ignored")},
			},
			want: []uint{1, 10},
		},
		{
			name: "duplicate version",
			files: fstest.MapFS{
				"sql/000001_first.up.sql":  {Data: []byte("SELECT 1")},
				"sql/000001_second.up.sql": {Data: []byte("SELECT 2")},
			},
			wantFail: true,
		},
		{
			name: "down migration rejected",
			files: fstest.MapFS{
				"sql/000001_first.down.sql": {Data: []byte("DROP TABLE anything")},
			},
			wantFail: true,
		},
		{
			name: "zero version rejected",
			files: fstest.MapFS{
				"sql/000000_first.up.sql": {Data: []byte("SELECT 1")},
			},
			wantFail: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := scanVersions(tc.files, "sql")
			if tc.wantFail {
				if err == nil {
					t.Fatalf("scanVersions() = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanVersions() error = %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("scanVersions() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("scanVersions() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestNormalizeConfigRequiresLockTimeoutBeyondDriverWait(t *testing.T) {
	t.Parallel()

	if _, err := normalizeConfig(Config{
		LockTimeout:        10 * time.Second,
		NetworkReadTimeout: 6 * time.Second,
		StatementTimeout:   time.Second,
	}); !errors.Is(err, errInvalidConfig) {
		t.Fatalf("ten-second lock timeout error = %v", err)
	}
	got, err := normalizeConfig(Config{
		LockTimeout:        11 * time.Second,
		NetworkReadTimeout: 6 * time.Second,
		StatementTimeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("eleven-second lock timeout error = %v", err)
	}
	if got.lockTimeout != 11*time.Second {
		t.Fatalf("lock timeout = %v", got.lockTimeout)
	}
}

func TestRunnerUpOutcomesAndDirtySafety(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		engine      *fakeEngine
		wantState   ResultState
		wantVersion uint
		wantDirty   bool
		wantApply   bool
	}{
		{
			name:        "applied",
			engine:      &fakeEngine{version: 1},
			wantState:   ResultApplied,
			wantVersion: 1,
		},
		{
			name:        "no change is success",
			engine:      &fakeEngine{upErr: migrate.ErrNoChange, version: 1},
			wantState:   ResultNoChange,
			wantVersion: 1,
		},
		{
			name:      "dirty is explicit",
			engine:    &fakeEngine{upErr: migrate.ErrDirty{Version: 7}},
			wantDirty: true,
		},
		{
			name:      "failed statement is reclassified when it left dirty state",
			engine:    &fakeEngine{upErr: errors.New("private sql failure"), version: 1, dirty: true},
			wantDirty: true,
		},
		{
			name:      "clean apply failure remains apply error",
			engine:    &fakeEngine{upErr: errors.New("private pre-apply failure"), version: 1},
			wantApply: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := runnerWithEngine(t, tc.engine)
			result, err := runner.Up(context.Background())
			if tc.wantDirty {
				if !errors.Is(err, ErrDirty) || !IsStage(err, StageDirty) {
					t.Fatalf("Up() error = %v, want safe dirty error", err)
				}
				if err.Error() != string(StageDirty) {
					t.Fatalf("dirty error rendered unsafe detail: %q", err.Error())
				}
				return
			}
			if tc.wantApply {
				if !IsStage(err, StageApply) || err.Error() != string(StageApply) {
					t.Fatalf("Up() error = %v, want safe apply error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Up() error = %v", err)
			}
			if result.State != tc.wantState || result.Version != tc.wantVersion {
				t.Fatalf("Up() = %+v", result)
			}
		})
	}
}

func TestRunnerStatusStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		engine    *fakeEngine
		wantState StatusState
		wantDirty bool
	}{
		{name: "uninitialized", engine: &fakeEngine{versionErr: migrate.ErrNilVersion}, wantState: StatusUninitialized},
		{name: "clean", engine: &fakeEngine{version: 1}, wantState: StatusClean},
		{name: "dirty", engine: &fakeEngine{version: 1, dirty: true}, wantDirty: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := runnerWithEngine(t, tc.engine)
			status, err := runner.Status(context.Background())
			if tc.wantDirty {
				if !errors.Is(err, ErrDirty) {
					t.Fatalf("Status() error = %v, want ErrDirty", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if status.State != tc.wantState || status.Latest != 1 {
				t.Fatalf("Status() = %+v", status)
			}
		})
	}
}

func TestRunnerStatusRejectsUnknownOrNewerDatabaseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version uint
	}{
		{name: "gap in embedded history", version: 2},
		{name: "database newer than binary", version: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := runnerWithVersions(t, &fakeEngine{version: test.version}, 1, 3)
			status, err := runner.Status(context.Background())
			if status != (Status{}) {
				t.Fatalf("Status() = %+v, want zero status on mismatch", status)
			}
			if !errors.Is(err, ErrVersionMismatch) || !IsStage(err, StageVersionMismatch) {
				t.Fatalf("Status() error = %v, want stable version mismatch", err)
			}
			if err.Error() != string(StageVersionMismatch) {
				t.Fatalf("version mismatch rendered unsafe detail: %q", err.Error())
			}
		})
	}

	runner := runnerWithVersions(t, &fakeEngine{version: 1}, 1, 3)
	status, err := runner.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() pending error = %v", err)
	}
	if status.State != StatusPending || status.Version != 1 || status.Latest != 3 {
		t.Fatalf("Status() pending = %+v", status)
	}
}

func TestRunnerCancellationStopsOnlyAtEngineBoundary(t *testing.T) {
	t.Parallel()

	engine := newBlockingEngine()
	runner := runnerWithEngine(t, engine)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runner.Up(ctx)
		done <- err
	}()
	<-engine.started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) || !IsStage(err, StageCancelled) {
			t.Fatalf("Up() cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Up() did not finish after graceful stop boundary")
	}
	if engine.stopCalls.Load() != 1 {
		t.Fatalf("Stop() calls = %d", engine.stopCalls.Load())
	}
	if _, err := runner.Up(context.Background()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("second Up() error = %v, want terminal cancellation", err)
	}
	if _, err := runner.Status(context.Background()); !errors.Is(err, ErrCancelled) {
		t.Fatalf("Status() after cancellation error = %v, want terminal cancellation", err)
	}
	if engine.upCalls.Load() != 1 {
		t.Fatalf("engine Up() calls = %d, cancelled runner must not be reused", engine.upCalls.Load())
	}
}

func TestRunnerCancellationReportsDirtyStateBeforeCancellation(t *testing.T) {
	t.Parallel()

	engine := newBlockingEngine()
	engine.version = 1
	engine.dirtyOnStop = true
	runner := runnerWithEngine(t, engine)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runner.Up(ctx)
		done <- err
	}()
	<-engine.started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, ErrDirty) || !errors.Is(err, ErrCancelled) || !IsStage(err, StageDirty) {
			t.Fatalf("Up() cancellation error = %v, want dirty to take precedence and retain cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Up() did not finish after dirty graceful-stop boundary")
	}
}

func TestRunnerCloseOwnsEngineAndDatabase(t *testing.T) {
	t.Parallel()

	engine := &fakeEngine{version: 1}
	runner := runnerWithEngine(t, engine)
	if err := runner.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if engine.closeCalls.Load() != 1 {
		t.Fatalf("engine Close() calls = %d", engine.closeCalls.Load())
	}
	if err := runner.db.PingContext(context.Background()); err == nil {
		t.Fatal("owned database remained open")
	}
}

func runnerWithEngine(t *testing.T, engine *fakeEngine) *Runner {
	return runnerWithVersions(t, engine, 1)
}

func runnerWithVersions(t *testing.T, engine *fakeEngine, versions ...uint) *Runner {
	t.Helper()
	files := make(fstest.MapFS, len(versions))
	for _, version := range versions {
		name := "sql/" + sixDigitVersion(version) + "_test.up.sql"
		files[name] = &fstest.MapFile{Data: []byte("SELECT 1")}
	}
	db := sql.OpenDB(&countingConnector{})
	runner, err := newRunner(context.Background(), files, db, Config{}, func(context.Context, fs.FS, *sql.DB, normalizedConfig) (migrationEngine, error) {
		return engine, nil
	})
	if err != nil {
		t.Fatalf("newRunner() error = %v", err)
	}
	t.Cleanup(func() { _ = runner.Close() })
	return runner
}

func sixDigitVersion(version uint) string {
	const digits = "000000"
	raw := fmt.Sprint(version)
	return digits[:len(digits)-len(raw)] + raw
}

type fakeEngine struct {
	upErr       error
	version     uint
	dirty       bool
	dirtyOnStop bool
	versionErr  error
	started     chan struct{}
	release     chan struct{}
	stopOnce    sync.Once
	stopCalls   atomic.Int32
	closeCalls  atomic.Int32
	upCalls     atomic.Int32
}

func newBlockingEngine() *fakeEngine {
	return &fakeEngine{started: make(chan struct{}), release: make(chan struct{})}
}

func (e *fakeEngine) Up() error {
	e.upCalls.Add(1)
	if e.started != nil {
		close(e.started)
		<-e.release
	}
	return e.upErr
}

func (e *fakeEngine) Version() (uint, bool, error) {
	return e.version, e.dirty, e.versionErr
}

func (e *fakeEngine) Stop() {
	e.stopCalls.Add(1)
	if e.dirtyOnStop {
		e.dirty = true
	}
	if e.release != nil {
		e.stopOnce.Do(func() { close(e.release) })
	}
}

func (e *fakeEngine) Close() error {
	e.closeCalls.Add(1)
	return nil
}

type countingConnector struct {
	connects atomic.Int32
}

func (c *countingConnector) Connect(context.Context) (driver.Conn, error) {
	c.connects.Add(1)
	return nil, errors.New("unexpected database connection")
}

func (c *countingConnector) Driver() driver.Driver { return countingDriver{} }

type countingDriver struct{}

func (countingDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("unexpected database connection")
}
