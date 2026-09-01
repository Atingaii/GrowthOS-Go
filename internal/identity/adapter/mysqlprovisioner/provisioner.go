package mysqlprovisioner

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"reflect"
	"strings"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const insertWorkforceAccountSQL = `
	INSERT INTO identity_workforce_account
		(account_id, login_name, principal_id, password_envelope,
		 account_status, credential_version, authentication_epoch,
		 created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

const redactedProvisioner = "identity mysql provisioner (redacted)"

var errUnexpectedAffectedRows = errors.New("identity mysql provisioner affected an unexpected row count")

// Provisioner owns no database lifecycle and exposes only create semantics.
// The composition root must supply and close a separately privileged pool.
type Provisioner struct {
	database *sqlx.DB
}

// New constructs the create-only adapter without probing or taking ownership
// of the supplied database pool.
func New(database *sqlx.DB) (*Provisioner, error) {
	if database == nil || database.DB == nil {
		return nil, newError(ErrNotConfigured, errors.New("database handle is unavailable"))
	}
	return &Provisioner{database: database}, nil
}

func (*Provisioner) String() string { return redactedProvisioner }

func (provisioner *Provisioner) GoString() string { return provisioner.String() }

func (*Provisioner) LogValue() slog.Value { return slog.StringValue(redactedProvisioner) }

// Create inserts exactly one already-validated workforce account. It never
// retries, updates, replaces, or treats a duplicate as idempotent success.
func (provisioner *Provisioner) Create(
	ctx context.Context,
	account identity.WorkforceAccount,
) error {
	if ctx == nil {
		return newError(ErrInvalidArgument, errors.New("context is nil"))
	}
	if provisioner == nil || provisioner.database == nil || provisioner.database.DB == nil {
		return newError(ErrNotConfigured, errors.New("database handle is unavailable"))
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err)
	}
	if err := account.Validate(); err != nil {
		return newError(ErrInvalidArgument, err)
	}

	credentialEnvelope := account.CredentialEnvelope().Bytes()
	defer clear(credentialEnvelope)

	transaction, err := provisioner.database.BeginTxx(
		ctx,
		writeTxOptions(),
	)
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	defer func() { _ = transaction.Rollback() }()

	result, err := transaction.ExecContext(
		ctx,
		insertWorkforceAccountSQL,
		account.ID().String(),
		account.LoginName().String(),
		account.PrincipalID().String(),
		credentialEnvelope,
		string(account.Status()),
		uint64(account.CredentialVersion()),
		uint64(account.AuthenticationEpoch()),
		account.CreatedAt(),
		account.UpdatedAt(),
	)
	if err != nil {
		if knownAccountIdentityConflict(err) {
			return newError(ErrAlreadyExists, err)
		}
		return classifyOperationError(ctx, err)
	}
	if err := requireOneAffectedRow(result); err != nil {
		return newError(ErrDependencyUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return canceledError(err)
	}
	if err := transaction.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

func writeTxOptions() *sql.TxOptions {
	return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
}

func requireOneAffectedRow(result sql.Result) error {
	if resultIsNil(result) {
		return errUnexpectedAffectedRows
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errUnexpectedAffectedRows
	}
	return nil
}

func resultIsNil(result sql.Result) bool {
	if result == nil {
		return true
	}
	value := reflect.ValueOf(result)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func classifyOperationError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return canceledError(ctx.Err())
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return canceledError(err)
	}
	return newError(ErrDependencyUnavailable, err)
}

func classifyWriteCommitError(ctx context.Context, commitErr error) error {
	if cancellation := definitelyCanceledTransaction(ctx, commitErr); cancellation != nil {
		return canceledError(cancellation)
	}
	return newError(ErrCommitOutcomeUnknown, commitErr)
}

// A concurrent context cancellation does not prove that COMMIT failed. Only a
// matching context error or sql.ErrTxDone while the context is canceled is
// sufficient evidence to downgrade an otherwise indeterminate write outcome.
func definitelyCanceledTransaction(ctx context.Context, commitErr error) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if errors.Is(commitErr, ctx.Err()) || errors.Is(commitErr, sql.ErrTxDone) {
		return ctx.Err()
	}
	return nil
}

func knownAccountIdentityConflict(err error) bool {
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError == nil || mysqlError.Number != 1062 {
		return false
	}
	keyName, ok := mysqlDuplicateKeyName(mysqlError.Message)
	if !ok {
		return false
	}
	if separator := strings.LastIndexByte(keyName, '.'); separator >= 0 {
		keyName = keyName[separator+1:]
	}
	switch keyName {
	case "PRIMARY",
		"uq_identity_workforce_account_login",
		"uq_identity_workforce_account_principal":
		return true
	default:
		return false
	}
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
