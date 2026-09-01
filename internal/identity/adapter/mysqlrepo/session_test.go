package mysqlrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/DATA-DOG/go-sqlmock"
	drivermysql "github.com/go-sql-driver/mysql"
)

func TestLoginIssueEvictsExactlyOneDeterministicOldestSession(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	issuedAt := testInstant(10)
	txNow := issuedAt.Add(time.Second)
	entropy, candidateDigest, candidateReference, candidateOperation := issueEntropy(0x61, 0x62, 0x63)
	activeSessions := makeTestActiveSessions(t, account, issuedAt, 5)
	oldest := activeSessions[0]
	repository, mock := newRepositoryMock(t, func() time.Time { return txNow })
	mock.ExpectBegin()
	expectAccountLock(mock, account)
	mock.ExpectQuery(sqlPattern(selectActiveSessionsForUpdateSQL)).WithArgs(
		account.ID().String(), uint64(account.AuthenticationEpoch()), txNow, txNow,
	).WillReturnRows(sessionsRows(activeSessions...))
	expectSessionRevoke(
		mock,
		oldest,
		txNow,
		identity.SessionRevokeReasonConcurrencyLimit,
		"evict:"+candidateOperation,
	)
	expectSessionInsert(
		mock,
		account,
		candidateReference,
		candidateOperation,
		candidateDigest,
		issuedAt,
		txNow,
	)
	mock.ExpectCommit()

	service := mustLoginService(t, issuedAt, account, entropy, repository)
	issued, err := service.Login(context.Background(), mustLoginCommand(t, account, nil))
	if err != nil || issued.Validate() != nil {
		t.Fatalf("Login() = %#v, %v", issued, err)
	}
	rawToken := issued.RawToken()
	defer clearBytes(rawToken)
	if !bytes.Equal(rawToken, bytes.Repeat([]byte{0x61}, identityapp.SessionTokenBytes)) {
		t.Fatal("issued token differs from the confirmed candidate")
	}
	if !strings.Contains(selectActiveSessionsForUpdateSQL,
		"ORDER BY last_seen_at ASC, issued_at ASC, session_ref ASC") {
		t.Fatal("oldest-session order drifted")
	}
	assertMockExpectations(t, mock)
}

func TestLoginPreviousTokenReplacementPrecedesCapacityAndAvoidsEviction(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	issuedAt := testInstant(10)
	txNow := issuedAt.Add(2 * time.Second)
	previousRaw := bytes.Repeat([]byte{0x71}, identityapp.SessionTokenBytes)
	previousHash := sha256.Sum256(previousRaw)
	previousDigest, err := identity.NewTokenDigest(previousHash[:])
	if err != nil {
		t.Fatal(err)
	}
	previous := mustTestSession(
		t,
		account,
		"ses_previous",
		"issue_previous",
		previousDigest,
		issuedAt.Add(-time.Hour),
		issuedAt.Add(-30*time.Second),
	)
	entropy, candidateDigest, candidateReference, candidateOperation := issueEntropy(0x72, 0x73, 0x74)
	repository, mock := newRepositoryMock(t, func() time.Time { return txNow })
	mock.ExpectQuery(sqlPattern(selectSessionAccountIDByDigestSQL)).
		WithArgs(previousDigest.Bytes()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(account.ID().String()))
	mock.ExpectBegin()
	expectAccountLock(mock, account)
	otherActive := makeTestActiveSessions(t, account, issuedAt, 4)
	activeAtCapacity := append([]identity.Session{previous}, otherActive...)
	mock.ExpectQuery(sqlPattern(selectActiveSessionsForUpdateSQL)).WithArgs(
		account.ID().String(), uint64(account.AuthenticationEpoch()), txNow, txNow,
	).WillReturnRows(sessionsRows(activeAtCapacity...))
	mock.ExpectQuery(sqlPattern(selectSessionByDigestForUpdateSQL)).WithArgs(
		previousDigest.Bytes(), account.ID().String(),
	).WillReturnRows(sessionRows(previous))
	expectSessionRevoke(
		mock,
		previous,
		txNow,
		identity.SessionRevokeReasonSecurityResponse,
		"replace:"+candidateOperation,
	)
	expectSessionInsert(
		mock,
		account,
		candidateReference,
		candidateOperation,
		candidateDigest,
		issuedAt,
		txNow,
	)
	mock.ExpectCommit()

	service := mustLoginService(t, issuedAt, account, entropy, repository)
	issued, err := service.Login(context.Background(), mustLoginCommand(t, account, previousRaw))
	if err != nil || issued.Validate() != nil {
		t.Fatalf("Login(previous) = %#v, %v", issued, err)
	}
	assertMockExpectations(t, mock)
}

func TestLoginFailsClosedWhenStoredActiveCountExceedsFive(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	issuedAt := testInstant(10)
	txNow := issuedAt.Add(time.Second)
	entropy, _, _, _ := issueEntropy(0x31, 0x32, 0x33)
	repository, mock := newRepositoryMock(t, func() time.Time { return txNow })
	mock.ExpectBegin()
	expectAccountLock(mock, account)
	mock.ExpectQuery(sqlPattern(selectActiveSessionsForUpdateSQL)).WillReturnRows(
		sessionsRows(makeTestActiveSessions(t, account, issuedAt, 6)...),
	)
	mock.ExpectRollback()

	service := mustLoginService(t, issuedAt, account, entropy, repository)
	issued, err := service.Login(context.Background(), mustLoginCommand(t, account, nil))
	if !errors.Is(err, identityapp.ErrAuthenticationUnavailable) || issued != (identityapp.IssuedSession{}) {
		t.Fatalf("Login(damaged count) = %#v, %v", issued, err)
	}
	assertMockExpectations(t, mock)
}

func TestLoginHintCannotConcealStoredActiveCountOverflow(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	issuedAt := testInstant(10)
	txNow := issuedAt.Add(time.Second)
	previousRaw := bytes.Repeat([]byte{0x34}, identityapp.SessionTokenBytes)
	previousHash := sha256.Sum256(previousRaw)
	previousDigest, err := identity.NewTokenDigest(previousHash[:])
	if err != nil {
		t.Fatal(err)
	}
	previous := mustTestSession(
		t,
		account,
		"ses_overflow_hint",
		"issue_overflow_hint",
		previousDigest,
		issuedAt.Add(-time.Hour),
		issuedAt.Add(-time.Minute),
	)
	active := append(
		[]identity.Session{previous},
		makeTestActiveSessions(t, account, issuedAt, 5)...,
	)
	entropy, _, _, _ := issueEntropy(0x35, 0x36, 0x37)
	repository, mock := newRepositoryMock(t, func() time.Time { return txNow })
	mock.ExpectQuery(sqlPattern(selectSessionAccountIDByDigestSQL)).
		WithArgs(previousDigest.Bytes()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(account.ID().String()))
	mock.ExpectBegin()
	expectAccountLock(mock, account)
	mock.ExpectQuery(sqlPattern(selectActiveSessionsForUpdateSQL)).WithArgs(
		account.ID().String(), uint64(account.AuthenticationEpoch()), txNow, txNow,
	).WillReturnRows(sessionsRows(active...))
	mock.ExpectRollback()

	service := mustLoginService(t, issuedAt, account, entropy, repository)
	issued, err := service.Login(context.Background(), mustLoginCommand(t, account, previousRaw))
	if !errors.Is(err, identityapp.ErrAuthenticationUnavailable) || issued != (identityapp.IssuedSession{}) {
		t.Fatalf("Login(damaged count with hint) = %#v, %v", issued, err)
	}
	assertMockExpectations(t, mock)
}

func TestResolveAndTouchCommitsAuthoritativeExtension(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	issuedAt := testInstant(0).Add(-10 * time.Minute)
	now := testInstant(10)
	session := mustTestSession(
		t,
		account,
		"ses_resolve",
		"issue_resolve",
		mustTestTokenDigest(t, 0x45),
		issuedAt,
		issuedAt,
	)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	mock.ExpectQuery(sqlPattern(selectSessionAccountIDByDigestSQL)).WithArgs(session.TokenDigest().Bytes()).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(account.ID().String()))
	mock.ExpectBegin()
	expectAccountLock(mock, account)
	mock.ExpectQuery(sqlPattern(selectSessionByDigestForUpdateSQL)).WithArgs(
		session.TokenDigest().Bytes(), account.ID().String(),
	).WillReturnRows(sessionRows(session))
	next, err := session.Touch(now, identityapp.SessionIdleLifetime)
	if err != nil {
		t.Fatal(err)
	}
	expectSessionTouch(mock, session, next)
	mock.ExpectCommit()

	gotAccount, gotSession, err := repository.ResolveAndTouch(
		context.Background(),
		session.TokenDigest(),
		now,
		identityapp.SessionIdleLifetime,
		identityapp.SessionTouchWindow,
	)
	if err != nil || !accountsEqual(gotAccount, account) || gotSession.LastSeenAt() != now ||
		gotSession.IdleExpiresAt() != next.IdleExpiresAt() {
		t.Fatalf("ResolveAndTouch() = %#v, %#v, %v", gotAccount, gotSession, err)
	}
	assertMockExpectations(t, mock)
}

func TestFindForRevocationUsesOneReadSnapshotAndReadCommitIsNeverWriteUnknown(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	session := mustTestSession(
		t,
		account,
		"ses_read_revoke",
		"issue_read_revoke",
		mustTestTokenDigest(t, 0x49),
		testInstant(0).Add(-time.Minute),
		testInstant(0).Add(-time.Minute),
	)
	for _, check := range []struct {
		name        string
		commitError error
	}{
		{name: "committed snapshot"},
		{name: "read commit failure", commitError: errors.New("private read commit failure")},
	} {
		t.Run(check.name, func(t *testing.T) {
			repository, mock := newRepositoryMock(t, func() time.Time { return testInstant(1) })
			mock.ExpectBegin()
			mock.ExpectQuery(sqlPattern(selectSessionByDigestSQL)).WithArgs(session.TokenDigest().Bytes()).
				WillReturnRows(sessionRows(session))
			mock.ExpectQuery(sqlPattern(selectAccountByIDSQL)).WithArgs(account.ID().String()).
				WillReturnRows(accountRows(account))
			if check.commitError == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectCommit().WillReturnError(check.commitError)
			}
			gotAccount, gotSession, err := repository.FindForRevocation(context.Background(), session.TokenDigest())
			if check.commitError == nil {
				if err != nil || !accountsEqual(gotAccount, account) ||
					!sameImmutableSessionIdentity(gotSession, session) {
					t.Fatalf("FindForRevocation() = %#v, %#v, %v", gotAccount, gotSession, err)
				}
			} else {
				assertSafeDependencyError(t, err, identityapp.ErrDependencyUnavailable)
				if errors.Is(err, identityapp.ErrCommitOutcomeUnknown) {
					t.Fatal("read-only commit failure classified as unknown write outcome")
				}
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestTouchThenRevokeUsesCurrentRowAndCannotFakeLogout(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	plannedAt := testInstant(10)
	txNow := plannedAt.Add(2 * time.Second)
	rawToken := bytes.Repeat([]byte{0x51}, identityapp.SessionTokenBytes)
	digestBytes := sha256.Sum256(rawToken)
	digest, err := identity.NewTokenDigest(digestBytes[:])
	if err != nil {
		t.Fatal(err)
	}
	before := mustTestSession(
		t,
		account,
		"ses_logout",
		"issue_logout",
		digest,
		plannedAt.Add(-10*time.Minute),
		plannedAt.Add(-2*time.Minute),
	)
	current, err := before.Touch(plannedAt.Add(time.Second), identityapp.SessionIdleLifetime)
	if err != nil {
		t.Fatal(err)
	}
	revokeEntropy := bytes.Repeat([]byte{0x52}, identityapp.OperationReferenceEntropyBytes)
	revokeOperation := "revoke_" + strings.Repeat("52", identityapp.OperationReferenceEntropyBytes)

	for _, check := range []struct {
		name          string
		commitError   error
		wantClass     error
		reconcileRead bool
	}{
		{name: "confirmed", wantClass: nil},
		{name: "commit unknown stays indeterminate after touch", commitError: errors.New("private commit loss"), wantClass: identityapp.ErrRevocationIndeterminate, reconcileRead: true},
	} {
		t.Run(check.name, func(t *testing.T) {
			repository, mock := newRepositoryMock(t, func() time.Time { return txNow })
			mock.ExpectBegin()
			expectAccountLock(mock, account)
			mock.ExpectQuery(sqlPattern(selectSessionByReferenceForUpdateSQL)).WithArgs(
				before.Reference().String(), account.ID().String(),
			).WillReturnRows(sessionRows(current))
			expectSessionRevoke(
				mock,
				current,
				txNow,
				identity.SessionRevokeReasonLogout,
				revokeOperation,
			)
			if check.commitError == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectCommit().WillReturnError(check.commitError)
			}
			service, err := identityapp.NewRevokeCurrentService(identityapp.RevokeCurrentDependencies{
				Clock:   identityapp.ClockFunc(func() time.Time { return plannedAt }),
				Reader:  revocationReaderStub{account: account, session: before},
				Revoker: repository,
				Entropy: &staticEntropy{reader: bytes.NewReader(revokeEntropy)},
			})
			if err != nil {
				t.Fatal(err)
			}
			revokeErr := service.RevokeCurrent(context.Background(), rawToken)
			if check.wantClass == nil {
				if revokeErr != nil {
					t.Fatalf("RevokeCurrent() error = %v", revokeErr)
				}
			} else if !errors.Is(revokeErr, check.wantClass) {
				t.Fatalf("RevokeCurrent() error = %v, want %v", revokeErr, check.wantClass)
			}
			if check.reconcileRead {
				receipt, ok := identityapp.SessionCommitReceiptFromError(revokeErr)
				if !ok {
					t.Fatal("commit-unknown revoke omitted receipt")
				}
				storedAfter, err := current.Revoke(txNow, identity.SessionRevokeReasonLogout, identity.OperationRef(revokeOperation))
				if err != nil {
					t.Fatal(err)
				}
				mock.ExpectQuery(sqlPattern(selectSessionByReferenceSQL)).WithArgs(before.Reference().String()).
					WillReturnRows(sessionRows(storedAfter))
				observation, err := repository.ObserveSessionCommit(context.Background(), receipt)
				if err != nil {
					t.Fatal(err)
				}
				if got := identityapp.ReconcileSessionCommit(receipt, observation); got != identityapp.SessionCommitReconciliationIndeterminate {
					t.Fatalf("reconciliation = %s", got)
				}
			}
			assertMockExpectations(t, mock)
		})
	}
}

func TestOnlyTokenDigestUniqueCollisionIsRetryable(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	session := mustTestSession(
		t,
		account,
		"ses_collision",
		"issue_collision",
		mustTestTokenDigest(t, 0x66),
		testInstant(0),
		testInstant(0),
	)
	checks := []struct {
		name      string
		key       string
		wantClass error
	}{
		{name: "token digest", key: "uq_identity_session_token_digest", wantClass: identityapp.ErrTokenDigestCollision},
		{name: "session reference", key: "PRIMARY", wantClass: identityapp.ErrDependencyUnavailable},
		{name: "issue operation", key: "uq_identity_session_issue_operation", wantClass: identityapp.ErrDependencyUnavailable},
		{name: "similar key name", key: "old_uq_identity_session_token_digest_copy", wantClass: identityapp.ErrDependencyUnavailable},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			repository, mock := newRepositoryMock(t, func() time.Time { return testInstant(1) })
			mock.ExpectBegin()
			tx, err := repository.database.BeginTxx(context.Background(), writeTxOptions())
			if err != nil {
				t.Fatal(err)
			}
			const privateEntry = "sentinel-token-digest-entry"
			mock.ExpectExec(sqlPattern(insertSessionSQL)).WillReturnError(&drivermysql.MySQLError{
				Number:  1062,
				Message: "Duplicate entry '" + privateEntry + "' for key 'identity_session." + check.key + "'",
			})
			err = insertSession(context.Background(), tx, session, testInstant(1))
			assertSafeDependencyError(t, err, check.wantClass)
			if check.wantClass == identityapp.ErrTokenDigestCollision {
				var dependency *identityapp.DependencyError
				if !errors.As(err, &dependency) || dependency.Cause() != errTokenDigestCollision {
					t.Fatalf("collision cause = %#v", err)
				}
				encoded, marshalErr := json.Marshal(dependency)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				rendered := fmt.Sprintf(
					"%v|%+v|%#v|%q|%s|%s|%s",
					err,
					err,
					err,
					err,
					dependency.Cause(),
					dependency.LogValue().String(),
					encoded,
				)
				if strings.Contains(rendered, privateEntry) || strings.Contains(rendered, "Duplicate entry") {
					t.Fatalf("duplicate entry leaked through collision: %s", rendered)
				}
			}
			mock.ExpectRollback()
			_ = tx.Rollback()
			assertMockExpectations(t, mock)
		})
	}
}

func expectAccountLock(mock sqlmock.Sqlmock, account identity.WorkforceAccount) {
	mock.ExpectQuery(sqlPattern(selectAccountForUpdateSQL)).WithArgs(account.ID().String()).
		WillReturnRows(accountRows(account))
}

func expectSessionInsert(
	mock sqlmock.Sqlmock,
	account identity.WorkforceAccount,
	reference string,
	operation string,
	digest identity.TokenDigest,
	issuedAt time.Time,
	updatedAt time.Time,
) {
	mock.ExpectExec(sqlPattern(insertSessionSQL)).WithArgs(
		reference,
		operation,
		account.ID().String(),
		digest.Bytes(),
		uint64(account.AuthenticationEpoch()),
		issuedAt,
		issuedAt,
		issuedAt.Add(identityapp.SessionIdleLifetime),
		issuedAt.Add(identityapp.SessionAbsoluteLifetime),
		updatedAt,
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectSessionRevoke(
	mock sqlmock.Sqlmock,
	session identity.Session,
	revokedAt time.Time,
	reason identity.SessionRevokeReason,
	operation string,
) {
	mock.ExpectExec(sqlPattern(updateSessionRevocationSQL)).WithArgs(
		revokedAt,
		string(reason),
		operation,
		revokedAt,
		session.Reference().String(),
		session.AccountID().String(),
		session.IssueOperationRef().String(),
		session.TokenDigest().Bytes(),
		uint64(session.AuthenticationEpoch()),
		session.IssuedAt(),
		session.AbsoluteExpiresAt(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectSessionTouch(
	mock sqlmock.Sqlmock,
	before identity.Session,
	after identity.Session,
) {
	mock.ExpectExec(sqlPattern(updateSessionTouchSQL)).WithArgs(
		after.LastSeenAt(),
		after.IdleExpiresAt(),
		after.LastSeenAt(),
		before.Reference().String(),
		before.AccountID().String(),
		before.TokenDigest().Bytes(),
		uint64(before.AuthenticationEpoch()),
		before.IssuedAt(),
		before.LastSeenAt(),
		before.IdleExpiresAt(),
		before.AbsoluteExpiresAt(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

func makeTestActiveSessions(
	t *testing.T,
	account identity.WorkforceAccount,
	now time.Time,
	count int,
) []identity.Session {
	t.Helper()
	sessions := make([]identity.Session, 0, count)
	for index := 0; index < count; index++ {
		sessions = append(sessions, mustTestSession(
			t,
			account,
			"ses_active_"+strconv.Itoa(index),
			"issue_active_"+strconv.Itoa(index),
			mustTestTokenDigest(t, byte(0x20+index)),
			now.Add(-30*time.Minute).Add(time.Duration(index)*time.Second),
			now.Add(-5*time.Minute).Add(time.Duration(index)*time.Second),
		))
	}
	return sessions
}
