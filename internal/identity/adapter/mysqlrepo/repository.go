package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	// ThrottleRowRetention is the frozen inactive-row retention horizon. It is
	// longer than every admission window and lease, so recovery evidence is not
	// deleted while an application receipt may still exist.
	ThrottleRowRetention = 24 * time.Hour
	// MaximumActiveSessions is enforced under the account row lock.
	MaximumActiveSessions = uint64(5)
)

var (
	errNilDatabase            = errors.New("identity mysql repository database is nil")
	errNilContext             = errors.New("identity mysql repository context is nil")
	errRepositoryNotReady     = errors.New("identity mysql repository is not configured")
	errUnexpectedAffectedRows = errors.New("identity mysql repository affected an unexpected row count")
	errStoredRowInvalid       = errors.New("identity mysql repository stored row is invalid")
	errStoredSessionOverflow  = errors.New("identity mysql repository active session invariant is damaged")
	errClockInvalid           = errors.New("identity mysql repository clock returned an invalid instant")
	errClockRegressed         = errors.New("identity mysql repository clock precedes stored state")
	errAdmissionCapacity      = errors.New("identity mysql repository admission capacity is exhausted")
	errAdmissionReceiptStale  = errors.New("identity mysql repository admission receipt is stale")
	errTokenDigestCollision   = errors.New("identity mysql repository token digest collision")
	errEpochExhausted         = errors.New("identity mysql repository epoch is exhausted")
)

// Repository implements every MySQL-owned Identity application port. The
// composition root retains ownership of the dedicated growthos_identity pool.
type Repository struct {
	database *sqlx.DB
	now      func() time.Time
}

type storedRestoreError struct{ cause error }

func (restoreError *storedRestoreError) Error() string { return errStoredRowInvalid.Error() }
func (restoreError *storedRestoreError) GoString() string {
	return restoreError.Error()
}

var _ identityapp.CredentialReader = (*Repository)(nil)
var _ identityapp.AdmissionController = (*Repository)(nil)
var _ identityapp.SessionIssuer = (*Repository)(nil)
var _ identityapp.SessionResolver = (*Repository)(nil)
var _ identityapp.SessionRevocationReader = (*Repository)(nil)
var _ identityapp.SessionRevoker = (*Repository)(nil)
var _ identityapp.SessionCommitObserver = (*Repository)(nil)

// New constructs the adapter without probing or taking ownership of the pool.
func New(database *sqlx.DB) (*Repository, error) {
	return newRepository(database, time.Now)
}

func newRepository(database *sqlx.DB, now func() time.Time) (*Repository, error) {
	if database == nil || database.DB == nil {
		return nil, dependencyError(identityapp.ErrDependencyUnavailable, errNilDatabase)
	}
	if now == nil {
		return nil, dependencyError(identityapp.ErrDependencyUnavailable, errRepositoryNotReady)
	}
	return &Repository{database: database, now: now}, nil
}

func (repository *Repository) validateCall(ctx context.Context) error {
	if ctx == nil {
		return dependencyError(identityapp.ErrDependencyInvalidArgument, errNilContext)
	}
	if repository == nil || repository.database == nil || repository.database.DB == nil || repository.now == nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, errRepositoryNotReady)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (repository *Repository) currentTime() (time.Time, error) {
	if repository == nil || repository.now == nil {
		return time.Time{}, errRepositoryNotReady
	}
	now := canonicalTime(repository.now())
	if now.IsZero() {
		return time.Time{}, errClockInvalid
	}
	return now, nil
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.Round(0).UTC().Truncate(time.Microsecond)
}

func writeTxOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
}

func readTxOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
}

func dependencyError(class, cause error) error {
	return identityapp.WrapDependencyError(class, cause)
}

func storedIdentityInvalid(cause error) error {
	return dependencyError(identityapp.ErrStoredIdentityInvalid, cause)
}

func classifyOperationError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return dependencyError(identityapp.ErrDependencyUnavailable, err)
}

func classifyWriteCommitError(ctx context.Context, err error) error {
	if canceled := definitelyCanceledTransaction(ctx, err); canceled != nil {
		return canceled
	}
	return dependencyError(identityapp.ErrCommitOutcomeUnknown, err)
}

func classifyReadCommitError(ctx context.Context, err error) error {
	if canceled := definitelyCanceledTransaction(ctx, err); canceled != nil {
		return canceled
	}
	return classifyOperationError(ctx, err)
}

// A network/driver error returned by Commit remains indeterminate even if the
// caller happened to be canceled concurrently. Only an error that proves the
// transaction never reached a successful Commit is downgraded to cancellation.
func definitelyCanceledTransaction(ctx context.Context, commitErr error) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if errors.Is(commitErr, ctx.Err()) || errors.Is(commitErr, sql.ErrTxDone) {
		return ctx.Err()
	}
	return nil
}

func isDuplicateKey(err error, keyName string) bool {
	var mysqlError *drivermysql.MySQLError
	if keyName == "" || !errors.As(err, &mysqlError) || mysqlError == nil || mysqlError.Number != 1062 {
		return false
	}
	reportedKey, ok := mysqlDuplicateKeyName(mysqlError.Message)
	if !ok {
		return false
	}
	if separator := strings.LastIndexByte(reportedKey, '.'); separator >= 0 {
		reportedKey = reportedKey[separator+1:]
	}
	return reportedKey == keyName
}

func mysqlDuplicateKeyName(message string) (string, bool) {
	const marker = " for key "
	start := strings.LastIndex(message, marker)
	if start < 0 {
		return "", false
	}
	suffix := strings.TrimSpace(message[start+len(marker):])
	if len(suffix) < 3 || (suffix[0] != '\'' && suffix[0] != '`') {
		return "", false
	}
	quote := suffix[0]
	end := strings.IndexByte(suffix[1:], quote)
	if end <= 0 {
		return "", false
	}
	return suffix[1 : end+1], true
}

func requireAffectedRows(result sql.Result, expected int64) error {
	if result == nil {
		return errUnexpectedAffectedRows
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != expected {
		return errUnexpectedAffectedRows
	}
	return nil
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func maxTime(values ...time.Time) time.Time {
	var maximum time.Time
	for _, value := range values {
		if maximum.Before(value) {
			maximum = value
		}
	}
	return maximum
}

func digestEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for index := range left {
		difference |= left[index] ^ right[index]
	}
	return difference == 0
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
