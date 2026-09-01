package mysqlprovisioner

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/DATA-DOG/go-sqlmock"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const testCredentialEnvelope = "$argon2id$v=19$m=19456,t=2,p=1$cHJpdmF0ZS1zYWx0$cHJpdmF0ZS1kaWdlc3Q"

func TestNewRejectsMissingDatabaseWithoutTakingOwnership(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		name     string
		database *sqlx.DB
	}{
		{name: "nil"},
		{name: "empty", database: &sqlx.DB{}},
	} {
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			provisioner, err := New(check.database)
			if provisioner != nil || !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("New() = %#v, %v", provisioner, err)
			}
		})
	}

	database, _, closeDatabase := newMockDatabase(t)
	defer closeDatabase()
	provisioner, err := New(database)
	if err != nil || provisioner == nil {
		t.Fatalf("New(valid) = %#v, %v", provisioner, err)
	}
	if err := database.Ping(); err != nil {
		t.Fatalf("New took ownership of or closed caller pool: %v", err)
	}
}

func TestCreateUsesOneExactCreateOnlyTransactionAndClearsEnvelopeCopy(t *testing.T) {
	t.Parallel()
	account := testWorkforceAccount(t)
	provisioner, mock, closeDatabase := newMockProvisioner(t)
	defer closeDatabase()

	capturedEnvelope := &byteCapture{want: []byte(testCredentialEnvelope)}
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(insertWorkforceAccountSQL)).WithArgs(
		account.ID().String(),
		account.LoginName().String(),
		account.PrincipalID().String(),
		capturedEnvelope,
		string(account.Status()),
		uint64(account.CredentialVersion()),
		uint64(account.AuthenticationEpoch()),
		account.CreatedAt(),
		account.UpdatedAt(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := provisioner.Create(context.Background(), account); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	assertExpectations(t, mock)
	if len(capturedEnvelope.got) == 0 {
		t.Fatal("SQL boundary did not receive the credential envelope as bytes")
	}
	for _, value := range capturedEnvelope.got {
		if value != 0 {
			t.Fatal("temporary credential envelope copy was not cleared after Create")
		}
	}
	if got := string(account.CredentialEnvelope().Bytes()); got != testCredentialEnvelope {
		t.Fatal("Create modified the immutable domain account envelope")
	}
}

func TestCreateRejectsInvalidCallsBeforeStartingTransaction(t *testing.T) {
	t.Parallel()
	account := testWorkforceAccount(t)
	provisioner, mock, closeDatabase := newMockProvisioner(t)
	defer closeDatabase()

	checks := []struct {
		name        string
		provisioner *Provisioner
		ctx         context.Context
		account     identity.WorkforceAccount
		want        error
	}{
		{name: "nil context", provisioner: provisioner, account: account, want: ErrInvalidArgument},
		{name: "nil receiver", ctx: context.Background(), account: account, want: ErrNotConfigured},
		{name: "empty receiver", provisioner: &Provisioner{}, ctx: context.Background(), account: account, want: ErrNotConfigured},
		{name: "invalid account", provisioner: provisioner, ctx: context.Background(), want: ErrInvalidArgument},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.provisioner.Create(check.ctx, check.account)
			assertErrorClass(t, err, check.want)
		})
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	err := provisioner.Create(canceledContext, account)
	assertErrorClass(t, err, ErrOperationCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Create() error = %v", err)
	}
	assertExpectations(t, mock)
}

func TestCreateClassifiesEveryPreCommitFailureAndRollsBack(t *testing.T) {
	t.Parallel()
	account := testWorkforceAccount(t)
	privateBegin := errors.New("private begin dsn detail")

	t.Run("begin", func(t *testing.T) {
		provisioner, mock, closeDatabase := newMockProvisioner(t)
		defer closeDatabase()
		mock.ExpectBegin().WillReturnError(privateBegin)
		err := provisioner.Create(context.Background(), account)
		assertErrorWithCause(t, err, ErrDependencyUnavailable, privateBegin)
		assertExpectations(t, mock)
	})

	for _, check := range []struct {
		name       string
		result     driver.Result
		wantCause  error
		wantClass  error
		cancelCall bool
	}{
		{
			name:      "rows affected unavailable",
			wantCause: errors.New("private rows affected detail"),
			wantClass: ErrDependencyUnavailable,
		},
		{name: "zero rows", result: sqlmock.NewResult(0, 0), wantClass: ErrDependencyUnavailable},
		{name: "two rows", result: sqlmock.NewResult(0, 2), wantClass: ErrDependencyUnavailable},
		{name: "canceled before commit", wantClass: ErrOperationCanceled, cancelCall: true},
	} {
		t.Run(check.name, func(t *testing.T) {
			provisioner, mock, closeDatabase := newMockProvisioner(t)
			defer closeDatabase()
			ctx := context.Background()
			result := check.result
			if check.wantCause != nil {
				result = sqlmock.NewErrorResult(check.wantCause)
			}
			if check.cancelCall {
				cancelContext, cancel := context.WithCancel(context.Background())
				ctx = cancelContext
				result = cancelingResult{cancel: cancel, affected: 1}
			}
			mock.ExpectBegin()
			mock.ExpectExec(sqlPattern(insertWorkforceAccountSQL)).
				WillReturnResult(result)
			mock.ExpectRollback()
			err := provisioner.Create(ctx, account)
			assertErrorClass(t, err, check.wantClass)
			if check.wantCause != nil {
				var typed *Error
				if !errors.As(err, &typed) || typed.Cause() != check.wantCause {
					t.Fatal("RowsAffected cause was not retained for trusted inspection")
				}
			}
			if check.cancelCall && !errors.Is(err, context.Canceled) {
				t.Fatal("pre-commit cancellation omitted context class")
			}
			assertExpectations(t, mock)
		})
	}
}

func TestCreateMapsOnlyReviewedAccountUniqueKeysToAlreadyExists(t *testing.T) {
	t.Parallel()
	account := testWorkforceAccount(t)
	for _, check := range []struct {
		name    string
		message string
	}{
		{name: "account primary", message: "Duplicate entry 'private-account' for key 'PRIMARY'"},
		{name: "qualified login", message: "Duplicate entry 'private-login' for key 'identity_workforce_account.uq_identity_workforce_account_login'"},
		{name: "backtick principal", message: "Duplicate entry 'private-principal' for key `growthos.identity_workforce_account.uq_identity_workforce_account_principal`"},
	} {
		t.Run(check.name, func(t *testing.T) {
			provisioner, mock, closeDatabase := newMockProvisioner(t)
			defer closeDatabase()
			cause := &drivermysql.MySQLError{Number: 1062, Message: check.message}
			mock.ExpectBegin()
			mock.ExpectExec(sqlPattern(insertWorkforceAccountSQL)).WillReturnError(cause)
			mock.ExpectRollback()
			err := provisioner.Create(context.Background(), account)
			assertErrorWithCause(t, err, ErrAlreadyExists, cause)
			if strings.Contains(err.Error(), "private-") {
				t.Fatalf("duplicate rendering leaked entry: %v", err)
			}
			assertExpectations(t, mock)
		})
	}
}

func TestCreateFailsClosedForUnknownDuplicatePermissionAndDriverErrors(t *testing.T) {
	t.Parallel()
	account := testWorkforceAccount(t)
	for _, check := range []struct {
		name  string
		cause error
		want  error
	}{
		{
			name:  "unknown future unique key",
			cause: &drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry 'private' for key 'uq_identity_workforce_account_future'"},
			want:  ErrDependencyUnavailable,
		},
		{
			name:  "similar key name",
			cause: &drivermysql.MySQLError{Number: 1062, Message: "Duplicate entry 'private' for key 'old_uq_identity_workforce_account_login_copy'"},
			want:  ErrDependencyUnavailable,
		},
		{
			name:  "malformed duplicate message",
			cause: &drivermysql.MySQLError{Number: 1062, Message: "private malformed duplicate"},
			want:  ErrDependencyUnavailable,
		},
		{
			name:  "non duplicate mysql error",
			cause: &drivermysql.MySQLError{Number: 1142, Message: "private INSERT permission detail"},
			want:  ErrDependencyUnavailable,
		},
		{name: "driver failure", cause: errors.New("private driver dsn detail"), want: ErrDependencyUnavailable},
		{name: "context cancellation", cause: context.Canceled, want: ErrOperationCanceled},
		{name: "context deadline", cause: context.DeadlineExceeded, want: ErrOperationCanceled},
	} {
		t.Run(check.name, func(t *testing.T) {
			provisioner, mock, closeDatabase := newMockProvisioner(t)
			defer closeDatabase()
			mock.ExpectBegin()
			mock.ExpectExec(sqlPattern(insertWorkforceAccountSQL)).WillReturnError(check.cause)
			mock.ExpectRollback()
			err := provisioner.Create(context.Background(), account)
			assertErrorWithCause(t, err, check.want, check.cause)
			assertExpectations(t, mock)
		})
	}
}

func TestCreateCommitFailureIsUnknownAndNeverRetried(t *testing.T) {
	t.Parallel()
	account := testWorkforceAccount(t)
	provisioner, mock, closeDatabase := newMockProvisioner(t)
	defer closeDatabase()
	privateCommit := errors.New("private commit acknowledgement with envelope")
	mock.ExpectBegin()
	mock.ExpectExec(sqlPattern(insertWorkforceAccountSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(privateCommit)

	err := provisioner.Create(context.Background(), account)
	assertErrorWithCause(t, err, ErrCommitOutcomeUnknown, privateCommit)
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "envelope") {
		t.Fatalf("commit error leaked private cause: %v", err)
	}
	assertExpectations(t, mock)
}

func TestCommitCancellationRequiresProofThatCommitDidNotSucceed(t *testing.T) {
	t.Parallel()
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	deadlineContext, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer deadlineCancel()

	for _, check := range []struct {
		name       string
		ctx        context.Context
		commitErr  error
		wantClass  error
		wantCtxErr error
	}{
		{name: "matching cancellation", ctx: canceledContext, commitErr: context.Canceled, wantClass: ErrOperationCanceled, wantCtxErr: context.Canceled},
		{name: "matching deadline", ctx: deadlineContext, commitErr: context.DeadlineExceeded, wantClass: ErrOperationCanceled, wantCtxErr: context.DeadlineExceeded},
		{name: "transaction already done", ctx: canceledContext, commitErr: sql.ErrTxDone, wantClass: ErrOperationCanceled, wantCtxErr: context.Canceled},
		{name: "concurrent cancel plus network error", ctx: canceledContext, commitErr: errors.New("private network acknowledgement"), wantClass: ErrCommitOutcomeUnknown},
		{name: "driver cancellation without canceled context", ctx: context.Background(), commitErr: context.Canceled, wantClass: ErrCommitOutcomeUnknown},
	} {
		t.Run(check.name, func(t *testing.T) {
			err := classifyWriteCommitError(check.ctx, check.commitErr)
			assertErrorClass(t, err, check.wantClass)
			if check.wantCtxErr != nil && !errors.Is(err, check.wantCtxErr) {
				t.Fatalf("commit classification omitted context class %v", check.wantCtxErr)
			}
		})
	}
}

func TestWriteTransactionIsReadCommittedAndSQLCannotMutateExistingRows(t *testing.T) {
	t.Parallel()
	options := writeTxOptions()
	if options == nil || options.Isolation != sql.LevelReadCommitted || options.ReadOnly {
		t.Fatalf("write transaction options = %#v", options)
	}

	normalized := " " + strings.ToLower(strings.Join(strings.Fields(insertWorkforceAccountSQL), " ")) + " "
	if strings.Count(normalized, " insert into ") != 1 ||
		strings.Count(insertWorkforceAccountSQL, "?") != 9 {
		t.Fatalf("create SQL is not one exact nine-column insert: %s", normalized)
	}
	for _, forbidden := range []string{
		" replace ",
		" on duplicate ",
		" update identity_workforce_account ",
		" delete from ",
		" select ",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("create SQL contains forbidden mutation %q: %s", forbidden, normalized)
		}
	}
}

func TestDuplicateClassifierRejectsTypedNilMySQLError(t *testing.T) {
	t.Parallel()
	var mysqlError *drivermysql.MySQLError
	var err error = mysqlError
	if knownAccountIdentityConflict(err) {
		t.Fatal("typed-nil MySQL error was classified as an account conflict")
	}
}

func TestAffectedRowsRejectsNilTypedNilAndDriverError(t *testing.T) {
	t.Parallel()
	if err := requireOneAffectedRow(nil); !errors.Is(err, errUnexpectedAffectedRows) {
		t.Fatalf("nil result error = %v", err)
	}
	var typedNil *staticResult
	if err := requireOneAffectedRow(typedNil); !errors.Is(err, errUnexpectedAffectedRows) {
		t.Fatalf("typed nil result error = %v", err)
	}
	private := errors.New("private result detail")
	if err := requireOneAffectedRow(staticResult{rowsErr: private}); err != private {
		t.Fatalf("RowsAffected error = %v", err)
	}
}

func newMockProvisioner(t *testing.T) (*Provisioner, sqlmock.Sqlmock, func()) {
	t.Helper()
	database, mock, closeDatabase := newMockDatabase(t)
	provisioner, err := New(database)
	if err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	return provisioner, mock, closeDatabase
}

func newMockDatabase(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	return sqlx.NewDb(database, "sqlmock"), mock, func() { _ = database.Close() }
}

func testWorkforceAccount(t *testing.T) identity.WorkforceAccount {
	t.Helper()
	accountID, err := identity.NewAccountID("account:l32:operator")
	if err != nil {
		t.Fatal(err)
	}
	loginName, err := identity.NewLoginName("operator_l32")
	if err != nil {
		t.Fatal(err)
	}
	principalID, err := identity.NewPrincipalID("principal:l32:operator")
	if err != nil {
		t.Fatal(err)
	}
	credentialVersion, err := identity.NewCredentialVersion(1)
	if err != nil {
		t.Fatal(err)
	}
	authenticationEpoch, err := identity.NewAuthenticationEpoch(1)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := identity.NewPasswordEnvelope([]byte(testCredentialEnvelope))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 34, 56, 123456000, time.UTC)
	account, err := identity.NewWorkforceAccount(
		accountID,
		loginName,
		principalID,
		identity.AccountStatusEnabled,
		credentialVersion,
		authenticationEpoch,
		envelope,
		now,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func sqlPattern(statement string) string {
	return regexp.QuoteMeta(statement)
}

func assertExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func assertErrorClass(t *testing.T, err, class error) {
	t.Helper()
	if !errors.Is(err, class) {
		t.Fatalf("error = %v, want class %v", err, class)
	}
	if err.Error() != class.Error() {
		t.Fatalf("rendered error = %q, want %q", err, class)
	}
}

func assertErrorWithCause(t *testing.T, err, class, cause error) {
	t.Helper()
	assertErrorClass(t, err, class)
	var typed *Error
	if !errors.As(err, &typed) || typed.Cause() != cause {
		t.Fatalf("trusted cause = %#v, want %#v", typed, cause)
	}
}

type byteCapture struct {
	want []byte
	got  []byte
}

func (capture *byteCapture) Match(value driver.Value) bool {
	bytesValue, ok := value.([]byte)
	if !ok || string(bytesValue) != string(capture.want) {
		return false
	}
	capture.got = bytesValue
	return true
}

type cancelingResult struct {
	cancel   context.CancelFunc
	affected int64
}

func (result cancelingResult) LastInsertId() (int64, error) { return 0, nil }

func (result cancelingResult) RowsAffected() (int64, error) {
	result.cancel()
	return result.affected, nil
}

type staticResult struct {
	rows    int64
	rowsErr error
}

func (result staticResult) LastInsertId() (int64, error) { return 0, nil }

func (result staticResult) RowsAffected() (int64, error) {
	return result.rows, result.rowsErr
}
