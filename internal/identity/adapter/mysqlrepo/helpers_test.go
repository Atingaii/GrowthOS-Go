package mysqlrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func testInstant(second int) time.Time {
	return time.Date(2026, 9, 1, 12, 0, second, 123456000, time.UTC)
}

func mustTestAccount(t *testing.T) identity.WorkforceAccount {
	t.Helper()
	accountID, err := identity.NewAccountID("account:operator1")
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginName("operator.one")
	if err != nil {
		t.Fatal(err)
	}
	principalID, err := identity.NewPrincipalID("principal:operator1")
	if err != nil {
		t.Fatal(err)
	}
	credentialVersion, err := identity.NewCredentialVersion(3)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := identity.NewAuthenticationEpoch(7)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := identity.NewPasswordEnvelope([]byte("$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0MTIzNA$ZGlnZXN0ZGlnZXN0ZGlnZXN0ZGlnZXN0MTIzNDU2Nzg"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := identity.NewWorkforceAccount(
		accountID,
		login,
		principalID,
		identity.AccountStatusEnabled,
		credentialVersion,
		epoch,
		envelope,
		testInstant(0).Add(-24*time.Hour),
		testInstant(0).Add(-time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func mustTestTokenDigest(t *testing.T, marker byte) identity.TokenDigest {
	t.Helper()
	value := bytes.Repeat([]byte{marker}, identity.DigestBytes)
	digest, err := identity.NewTokenDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustTestThrottleDigest(t *testing.T, marker byte) identity.ThrottleDigest {
	t.Helper()
	value := bytes.Repeat([]byte{marker}, identity.DigestBytes)
	digest, err := identity.NewThrottleDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustTestSession(
	t *testing.T,
	account identity.WorkforceAccount,
	reference string,
	issueRef string,
	digest identity.TokenDigest,
	issuedAt time.Time,
	lastSeenAt time.Time,
) identity.Session {
	t.Helper()
	sessionRef, err := identity.NewSessionRef(reference)
	if err != nil {
		t.Fatal(err)
	}
	operationRef, err := identity.NewOperationRef(issueRef)
	if err != nil {
		t.Fatal(err)
	}
	session, err := identity.RestoreSession(
		sessionRef,
		operationRef,
		account.ID(),
		digest,
		account.AuthenticationEpoch(),
		issuedAt,
		lastSeenAt,
		lastSeenAt.Add(identityapp.SessionIdleLifetime),
		issuedAt.Add(identityapp.SessionAbsoluteLifetime),
		time.Time{},
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func newRepositoryMock(
	t *testing.T,
	now func() time.Time,
) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository, err := newRepository(sqlx.NewDb(database, "sqlmock"), now)
	if err != nil {
		t.Fatal(err)
	}
	return repository, mock
}

func accountColumns() []string {
	return []string{
		"account_id", "login_name", "principal_id", "password_envelope",
		"account_status", "credential_version", "authentication_epoch",
		"created_at", "updated_at",
	}
}

func accountRows(account identity.WorkforceAccount) *sqlmock.Rows {
	envelope := account.CredentialEnvelope().Bytes()
	return sqlmock.NewRows(accountColumns()).AddRow(
		account.ID().String(),
		account.LoginName().String(),
		account.PrincipalID().String(),
		envelope,
		string(account.Status()),
		uint64(account.CredentialVersion()),
		uint64(account.AuthenticationEpoch()),
		account.CreatedAt(),
		account.UpdatedAt(),
	)
}

func sessionColumns() []string {
	return []string{
		"session_ref", "issue_operation_ref", "account_id", "token_digest",
		"authentication_epoch", "issued_at", "last_seen_at", "idle_expires_at",
		"absolute_expires_at", "revoked_at", "revoke_reason",
		"revoke_operation_ref", "updated_at",
	}
}

func sessionRows(session identity.Session) *sqlmock.Rows {
	return sessionsRows(session)
}

func sessionsRows(sessions ...identity.Session) *sqlmock.Rows {
	rows := sqlmock.NewRows(sessionColumns())
	for _, session := range sessions {
		revokedAt, reason, operationRef, revoked := session.Revocation()
		var revokedAtValue any
		var reasonValue any
		var operationValue any
		updatedAt := session.LastSeenAt()
		if revoked {
			revokedAtValue = revokedAt
			reasonValue = string(reason)
			operationValue = operationRef.String()
			updatedAt = revokedAt
		}
		rows.AddRow(
			session.Reference().String(),
			session.IssueOperationRef().String(),
			session.AccountID().String(),
			session.TokenDigest().Bytes(),
			uint64(session.AuthenticationEpoch()),
			session.IssuedAt(),
			session.LastSeenAt(),
			session.IdleExpiresAt(),
			session.AbsoluteExpiresAt(),
			revokedAtValue,
			reasonValue,
			operationValue,
			updatedAt,
		)
	}
	return rows
}

func throttleColumns() []string {
	return []string{
		"dimension", "subject_digest", "window_started_at", "window_expires_at",
		"failure_count", "inflight_count", "admission_epoch",
		"inflight_expires_at", "blocked_until", "updated_at", "row_expires_at",
	}
}

func throttleRows(record throttleRecord) *sqlmock.Rows {
	inflightExpiresAt, hasInflight := record.state.InflightExpiresAt()
	blockedUntil, hasBlock := record.state.BlockedUntil()
	var inflightValue any
	if hasInflight {
		inflightValue = inflightExpiresAt
	}
	var blockedValue any
	if hasBlock {
		blockedValue = blockedUntil
	}
	return sqlmock.NewRows(throttleColumns()).AddRow(
		string(record.state.Dimension()),
		record.state.Digest().Bytes(),
		record.state.WindowStartedAt(),
		record.state.WindowExpiresAt(),
		uint64(record.state.FailureCount()),
		uint64(record.state.InflightCount()),
		uint64(record.state.AdmissionEpoch()),
		inflightValue,
		blockedValue,
		record.updatedAt,
		record.rowExpiresAt,
	)
}

func mustThrottleRecord(
	t *testing.T,
	dimension identity.ThrottleDimension,
	digest identity.ThrottleDigest,
	startedAt time.Time,
	expiresAt time.Time,
	failures uint32,
	inflight uint32,
	epochValue uint64,
	inflightExpiresAt time.Time,
	blockedUntil time.Time,
	updatedAt time.Time,
) throttleRecord {
	t.Helper()
	epoch, err := identity.NewAdmissionEpoch(epochValue)
	if err != nil {
		t.Fatal(err)
	}
	state, err := identity.NewThrottleState(
		dimension,
		digest,
		startedAt,
		expiresAt,
		failures,
		inflight,
		epoch,
		inflightExpiresAt,
		blockedUntil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return throttleRecord{
		state:        state,
		updatedAt:    updatedAt,
		rowExpiresAt: maxTime(updatedAt.Add(ThrottleRowRetention), expiresAt, inflightExpiresAt, blockedUntil),
	}
}

func sqlPattern(statement string) string { return regexp.QuoteMeta(statement) }

func assertMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type credentialReaderFunc func(context.Context, identity.LoginName) (identity.WorkforceAccount, error)

func (function credentialReaderFunc) FindByLogin(
	ctx context.Context,
	login identity.LoginName,
) (identity.WorkforceAccount, error) {
	return function(ctx, login)
}

type passwordVerifierStub struct{}

func (passwordVerifierStub) VerifyLogin(
	context.Context,
	[]byte,
	string,
) (identityapp.PasswordVerification, error) {
	return identityapp.NewPasswordVerification(true, false)
}

func (passwordVerifierStub) VerifyUnknownLogin(context.Context, []byte) error { return nil }

type passwordVerifierResult struct{ matched bool }

func (stub passwordVerifierResult) VerifyLogin(
	context.Context,
	[]byte,
	string,
) (identityapp.PasswordVerification, error) {
	return identityapp.NewPasswordVerification(stub.matched, false)
}

func (passwordVerifierResult) VerifyUnknownLogin(context.Context, []byte) error { return nil }

type admissionStub struct {
	loginEpoch  identity.AdmissionEpoch
	sourceEpoch identity.AdmissionEpoch
}

func (stub admissionStub) BeginAdmission(
	_ context.Context,
	request identityapp.AdmissionRequest,
) (identityapp.AdmissionGrant, error) {
	return identityapp.NewAdmissionGrant(stub.loginEpoch, stub.sourceEpoch, request.Deadline())
}

func (admissionStub) FinalizeAdmission(
	context.Context,
	identityapp.AdmissionReceipt,
	identityapp.AdmissionFinalOutcome,
) error {
	return nil
}

type staticEntropy struct{ reader io.Reader }

func (entropy *staticEntropy) Read(destination []byte) (int, error) {
	return entropy.reader.Read(destination)
}

type sessionIssuerFunc func(context.Context, identityapp.SessionIssueAttempt) error

func (function sessionIssuerFunc) IssueSession(
	ctx context.Context,
	attempt identityapp.SessionIssueAttempt,
) error {
	return function(ctx, attempt)
}

func issueEntropy(token, reference, operation byte) (*staticEntropy, identity.TokenDigest, string, string) {
	material := append([]byte{}, bytes.Repeat([]byte{token}, identityapp.SessionTokenBytes)...)
	material = append(material, bytes.Repeat([]byte{reference}, identityapp.SessionReferenceEntropyBytes)...)
	material = append(material, bytes.Repeat([]byte{operation}, identityapp.OperationReferenceEntropyBytes)...)
	rawToken := bytes.Repeat([]byte{token}, identityapp.SessionTokenBytes)
	digestBytes := sha256.Sum256(rawToken)
	digest, _ := identity.NewTokenDigest(digestBytes[:])
	return &staticEntropy{reader: bytes.NewReader(material)}, digest,
		"ses_" + hex.EncodeToString(bytes.Repeat([]byte{reference}, identityapp.SessionReferenceEntropyBytes)),
		"issue_" + hex.EncodeToString(bytes.Repeat([]byte{operation}, identityapp.OperationReferenceEntropyBytes))
}

func mustEpoch(t *testing.T, value uint64) identity.AdmissionEpoch {
	t.Helper()
	epoch, err := identity.NewAdmissionEpoch(value)
	if err != nil {
		t.Fatal(err)
	}
	return epoch
}

func mustLoginCommand(
	t *testing.T,
	account identity.WorkforceAccount,
	previous []byte,
) identityapp.LoginCommand {
	t.Helper()
	command, err := identityapp.NewLoginCommand(
		account.LoginName(),
		[]byte("correct horse battery staple"),
		mustTestThrottleDigest(t, 0x41),
		mustTestThrottleDigest(t, 0x42),
		previous,
	)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func mustLoginService(
	t *testing.T,
	clockAt time.Time,
	account identity.WorkforceAccount,
	entropy identityapp.EntropyReader,
	issuer identityapp.SessionIssuer,
) *identityapp.LoginService {
	t.Helper()
	service, err := identityapp.NewLoginService(identityapp.LoginDependencies{
		Clock: identityapp.ClockFunc(func() time.Time { return clockAt }),
		Credentials: credentialReaderFunc(func(
			context.Context,
			identity.LoginName,
		) (identity.WorkforceAccount, error) {
			return account, nil
		}),
		Passwords: passwordVerifierStub{},
		Admissions: admissionStub{
			loginEpoch:  mustEpoch(t, 1),
			sourceEpoch: mustEpoch(t, 1),
		},
		Entropy: entropy,
		Issuer:  issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type revocationReaderStub struct {
	account identity.WorkforceAccount
	session identity.Session
	err     error
}

func (stub revocationReaderStub) FindForRevocation(
	context.Context,
	identity.TokenDigest,
) (identity.WorkforceAccount, identity.Session, error) {
	return stub.account, stub.session, stub.err
}

func assertSafeDependencyError(t *testing.T, err error, class error) {
	t.Helper()
	if !errors.Is(err, class) {
		t.Fatalf("error = %v, want class %v", err, class)
	}
	if err.Error() != class.Error() {
		t.Fatalf("rendered error = %q, want %q", err.Error(), class.Error())
	}
}
