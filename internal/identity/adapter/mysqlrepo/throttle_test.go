package mysqlrepo

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"math"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestBeginAdmissionUsesFixedOrderAndAtomicReservation(t *testing.T) {
	t.Parallel()
	now := testInstant(10)
	deadline := now.Add(3 * time.Second)
	loginDigest := mustTestThrottleDigest(t, 0x11)
	sourceDigest := mustTestThrottleDigest(t, 0x22)
	request, err := identityapp.NewAdmissionRequest(loginDigest, sourceDigest, now, deadline)
	if err != nil {
		t.Fatal(err)
	}
	login := mustThrottleRecord(
		t, identity.ThrottleDimensionLogin, loginDigest,
		now.Add(-time.Minute), now.Add(14*time.Minute), 2, 0, 5,
		time.Time{}, time.Time{}, now.Add(-time.Second),
	)
	source := mustThrottleRecord(
		t, identity.ThrottleDimensionSource, sourceDigest,
		now.Add(-time.Minute), now.Add(14*time.Minute), 4, 0, 9,
		time.Time{}, time.Time{}, now.Add(-time.Second),
	)
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	mock.ExpectBegin()
	expectThrottleEnsure(mock, identity.ThrottleDimensionLogin, loginDigest, request)
	expectThrottleEnsure(mock, identity.ThrottleDimensionSource, sourceDigest, request)
	expectThrottleLock(mock, identity.ThrottleDimensionLogin, loginDigest, login)
	expectThrottleLock(mock, identity.ThrottleDimensionSource, sourceDigest, source)
	expectThrottleSave(mock, identity.ThrottleDimensionLogin, loginDigest, 5, 0, now.Add(-time.Second))
	expectThrottleSave(mock, identity.ThrottleDimensionSource, sourceDigest, 9, 0, now.Add(-time.Second))
	mock.ExpectCommit()

	grant, err := repository.BeginAdmission(context.Background(), request)
	if err != nil || grant.Validate() != nil {
		t.Fatalf("BeginAdmission() = %#v, %v", grant, err)
	}
	assertMockExpectations(t, mock)
}

func TestBeginAdmissionFailsClosedWhenEnsuredRowCannotBeLocked(t *testing.T) {
	t.Parallel()
	now := testInstant(10)
	loginDigest := mustTestThrottleDigest(t, 0x23)
	sourceDigest := mustTestThrottleDigest(t, 0x24)
	request, err := identityapp.NewAdmissionRequest(
		loginDigest,
		sourceDigest,
		now,
		now.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	repository, mock := newRepositoryMock(t, func() time.Time { return now })
	mock.ExpectBegin()
	expectThrottleEnsure(mock, identity.ThrottleDimensionLogin, loginDigest, request)
	expectThrottleEnsure(mock, identity.ThrottleDimensionSource, sourceDigest, request)
	mock.ExpectQuery(sqlPattern(selectThrottleForUpdateSQL)).
		WithArgs(string(identity.ThrottleDimensionLogin), loginDigest.Bytes()).
		WillReturnRows(sqlmock.NewRows(throttleColumns()))
	mock.ExpectRollback()

	grant, err := repository.BeginAdmission(context.Background(), request)
	if grant != (identityapp.AdmissionGrant{}) {
		t.Fatalf("unexpected grant = %#v", grant)
	}
	assertSafeDependencyError(t, err, identityapp.ErrStoredIdentityInvalid)
	assertMockExpectations(t, mock)
}

func TestBeginAdmissionRechecksLeaseAfterBothLocks(t *testing.T) {
	t.Parallel()
	admittedAt := testInstant(10)
	deadline := admittedAt.Add(identityapp.MaximumAdmissionLease)
	loginDigest := mustTestThrottleDigest(t, 0x25)
	sourceDigest := mustTestThrottleDigest(t, 0x26)
	request, err := identityapp.NewAdmissionRequest(loginDigest, sourceDigest, admittedAt, deadline)
	if err != nil {
		t.Fatal(err)
	}
	source := mustThrottleRecord(
		t, identity.ThrottleDimensionSource, sourceDigest,
		admittedAt.Add(-time.Minute), admittedAt.Add(14*time.Minute), 0, 0, 3,
		time.Time{}, time.Time{}, admittedAt.Add(-time.Second),
	)

	t.Run("elapsed lease rolls back without state change", func(t *testing.T) {
		login := mustThrottleRecord(
			t, identity.ThrottleDimensionLogin, loginDigest,
			admittedAt.Add(-time.Minute), admittedAt.Add(14*time.Minute), 0, 0, 2,
			time.Time{}, time.Time{}, admittedAt.Add(-time.Second),
		)
		repository, mock := newRepositoryMock(t, func() time.Time { return deadline })
		mock.ExpectBegin()
		expectThrottleEnsure(mock, identity.ThrottleDimensionLogin, loginDigest, request)
		expectThrottleEnsure(mock, identity.ThrottleDimensionSource, sourceDigest, request)
		expectThrottleLock(mock, identity.ThrottleDimensionLogin, loginDigest, login)
		expectThrottleLock(mock, identity.ThrottleDimensionSource, sourceDigest, source)
		mock.ExpectRollback()
		grant, err := repository.BeginAdmission(context.Background(), request)
		if grant != (identityapp.AdmissionGrant{}) {
			t.Fatalf("unexpected grant = %#v", grant)
		}
		assertSafeDependencyError(t, err, identityapp.ErrAdmissionStale)
		assertMockExpectations(t, mock)
	})

	loginWithExpiredBatch := mustThrottleRecord(
		t, identity.ThrottleDimensionLogin, loginDigest,
		admittedAt.Add(-time.Minute), admittedAt.Add(14*time.Minute), 1, 1, 7,
		admittedAt, time.Time{}, admittedAt.Add(-time.Second),
	)
	for _, check := range []struct {
		name        string
		commitError error
		wantClass   error
	}{
		{name: "expired recovery is fenced before stale", wantClass: identityapp.ErrAdmissionStale},
		{name: "recovery commit acknowledgement lost", commitError: errors.New("private commit loss"), wantClass: identityapp.ErrCommitOutcomeUnknown},
	} {
		t.Run(check.name, func(t *testing.T) {
			repository, mock := newRepositoryMock(t, func() time.Time { return deadline })
			mock.ExpectBegin()
			expectThrottleEnsure(mock, identity.ThrottleDimensionLogin, loginDigest, request)
			expectThrottleEnsure(mock, identity.ThrottleDimensionSource, sourceDigest, request)
			expectThrottleLock(mock, identity.ThrottleDimensionLogin, loginDigest, loginWithExpiredBatch)
			expectThrottleLock(mock, identity.ThrottleDimensionSource, sourceDigest, source)
			expectThrottleSave(mock, identity.ThrottleDimensionLogin, loginDigest, 7, 1, admittedAt.Add(-time.Second))
			if check.commitError == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectCommit().WillReturnError(check.commitError)
			}
			grant, err := repository.BeginAdmission(context.Background(), request)
			if grant != (identityapp.AdmissionGrant{}) {
				t.Fatalf("unexpected grant = %#v", grant)
			}
			assertSafeDependencyError(t, err, check.wantClass)
			assertMockExpectations(t, mock)
		})
	}
}

func TestBeginAdmissionCommitsExpiredRecoveryBeforeOtherDimensionRejects(t *testing.T) {
	t.Parallel()
	now := testInstant(10)
	deadline := now.Add(3 * time.Second)
	loginDigest := mustTestThrottleDigest(t, 0x31)
	sourceDigest := mustTestThrottleDigest(t, 0x32)
	request, err := identityapp.NewAdmissionRequest(loginDigest, sourceDigest, now, deadline)
	if err != nil {
		t.Fatal(err)
	}
	login := mustThrottleRecord(
		t, identity.ThrottleDimensionLogin, loginDigest,
		now.Add(-time.Minute), now.Add(14*time.Minute), 1, 1, 7,
		now, time.Time{}, now.Add(-time.Second),
	)
	source := mustThrottleRecord(
		t, identity.ThrottleDimensionSource, sourceDigest,
		now.Add(-time.Minute), now.Add(14*time.Minute), identityapp.SourceFailureThreshold, 0, 4,
		time.Time{}, now.Add(30*time.Second), now.Add(-time.Second),
	)

	for _, check := range []struct {
		name        string
		commitError error
		wantClass   error
	}{
		{name: "recovery committed then rejected", wantClass: identityapp.ErrAdmissionRejected},
		{name: "recovery commit acknowledgement lost", commitError: errors.New("private commit detail"), wantClass: identityapp.ErrCommitOutcomeUnknown},
	} {
		t.Run(check.name, func(t *testing.T) {
			repository, mock := newRepositoryMock(t, func() time.Time { return now })
			mock.ExpectBegin()
			expectThrottleEnsure(mock, identity.ThrottleDimensionLogin, loginDigest, request)
			expectThrottleEnsure(mock, identity.ThrottleDimensionSource, sourceDigest, request)
			expectThrottleLock(mock, identity.ThrottleDimensionLogin, loginDigest, login)
			expectThrottleLock(mock, identity.ThrottleDimensionSource, sourceDigest, source)
			expectThrottleSave(mock, identity.ThrottleDimensionLogin, loginDigest, 7, 1, now.Add(-time.Second))
			if check.commitError == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectCommit().WillReturnError(check.commitError)
			}
			_, err := repository.BeginAdmission(context.Background(), request)
			assertSafeDependencyError(t, err, check.wantClass)
			assertMockExpectations(t, mock)
		})
	}
}

func TestFinalizeThrottleFailureBackoffAndSourceSuccessRearm(t *testing.T) {
	t.Parallel()
	now := testInstant(10)
	deadline := now.Add(2 * time.Second)
	policy := identityapp.V1AdmissionPolicy()

	t.Run("fifth login failure arms initial backoff", func(t *testing.T) {
		digest := mustTestThrottleDigest(t, 0x41)
		record := mustThrottleRecord(
			t, identity.ThrottleDimensionLogin, digest,
			now.Add(-time.Minute), now.Add(14*time.Minute), 4, 1, 10,
			deadline, time.Time{}, now.Add(-time.Second),
		)
		next, err := finalizeThrottle(
			record,
			throttleKey{dimension: identity.ThrottleDimensionLogin, digest: digest},
			mustEpoch(t, 10),
			deadline,
			identityapp.AdmissionFinalOutcomeFailure,
			now,
			policy,
		)
		if err != nil {
			t.Fatal(err)
		}
		blockedUntil, ok := next.state.BlockedUntil()
		if next.state.FailureCount() != 5 || next.state.InflightCount() != 0 ||
			next.state.AdmissionEpoch() != 11 || !ok ||
			blockedUntil != now.Add(identityapp.AuthenticationInitialBackoff) {
			t.Fatalf("next state = %#v", next.state)
		}
	})

	t.Run("successful source probe rearms without washing failures", func(t *testing.T) {
		digest := mustTestThrottleDigest(t, 0x42)
		record := mustThrottleRecord(
			t, identity.ThrottleDimensionSource, digest,
			now.Add(-time.Minute), now.Add(14*time.Minute), identityapp.SourceFailureThreshold, 1, 20,
			deadline, now.Add(-time.Microsecond), now.Add(-time.Second),
		)
		next, err := finalizeThrottle(
			record,
			throttleKey{dimension: identity.ThrottleDimensionSource, digest: digest},
			mustEpoch(t, 20),
			deadline,
			identityapp.AdmissionFinalOutcomeSuccess,
			now,
			policy,
		)
		if err != nil {
			t.Fatal(err)
		}
		blockedUntil, ok := next.state.BlockedUntil()
		if next.state.FailureCount() != identityapp.SourceFailureThreshold || !ok ||
			blockedUntil != now.Add(identityapp.AuthenticationInitialBackoff) {
			t.Fatalf("source success state = %#v", next.state)
		}
		key := throttleKey{dimension: identity.ThrottleDimensionSource, digest: digest}
		if allowed, err := throttleAllowsReservation(next, key, policy, now.Add(time.Second)); err != nil || allowed {
			t.Fatalf("immediate admission = %v, %v", allowed, err)
		}
		if allowed, err := throttleAllowsReservation(next, key, policy, blockedUntil); err != nil || !allowed {
			t.Fatalf("boundary admission = %v, %v", allowed, err)
		}
	})
}

func TestThrottleBoundariesFenceStaleAndNeverWrap(t *testing.T) {
	t.Parallel()
	now := testInstant(10)
	deadline := now.Add(2 * time.Second)
	digest := mustTestThrottleDigest(t, 0x51)
	record := mustThrottleRecord(
		t, identity.ThrottleDimensionLogin, digest,
		now.Add(-time.Minute), now.Add(14*time.Minute), math.MaxUint32-1, 1, 33,
		deadline, now.Add(time.Minute), now.Add(-time.Second),
	)
	_, err := finalizeThrottle(
		record,
		throttleKey{dimension: identity.ThrottleDimensionLogin, digest: digest},
		mustEpoch(t, 32),
		deadline,
		identityapp.AdmissionFinalOutcomeNeutral,
		now,
		identityapp.V1AdmissionPolicy(),
	)
	assertSafeDependencyError(t, err, identityapp.ErrAdmissionStale)

	if got := authenticationBackoff(math.MaxUint32, 5, 30*time.Second, 15*time.Minute); got != 15*time.Minute {
		t.Fatalf("capped backoff = %s", got)
	}
	if _, err := restoreThrottle(storedThrottle{
		dimension:         string(identity.ThrottleDimensionLogin),
		digest:            digest.Bytes(),
		windowStartedAt:   now,
		windowExpiresAt:   now.Add(time.Minute),
		failureCount:      math.MaxUint32,
		inflightCount:     1,
		admissionEpoch:    1,
		inflightExpiresAt: sql.NullTime{Time: deadline, Valid: true},
		updatedAt:         now,
		rowExpiresAt:      now.Add(ThrottleRowRetention),
	}); err == nil {
		t.Fatal("overflow aggregate restored successfully")
	}
}

func TestLoginDrivesAtomicFinalizeAndRechecksDeadlineAfterLocks(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	admittedAt := testInstant(10)
	deadline := admittedAt.Add(identityapp.MaximumAdmissionLease)
	finalizedAt := admittedAt.Add(time.Second)
	loginDigest := mustTestThrottleDigest(t, 0x61)
	sourceDigest := mustTestThrottleDigest(t, 0x62)
	request, err := identityapp.NewAdmissionRequest(loginDigest, sourceDigest, admittedAt, deadline)
	if err != nil {
		t.Fatal(err)
	}
	loginBefore := mustThrottleRecord(
		t, identity.ThrottleDimensionLogin, loginDigest,
		admittedAt, admittedAt.Add(identityapp.AuthenticationObservationWindow), 0, 0, 1,
		time.Time{}, time.Time{}, admittedAt,
	)
	sourceBefore := mustThrottleRecord(
		t, identity.ThrottleDimensionSource, sourceDigest,
		admittedAt, admittedAt.Add(identityapp.AuthenticationObservationWindow), 0, 0, 1,
		time.Time{}, time.Time{}, admittedAt,
	)
	loginReserved := mustThrottleRecord(
		t, identity.ThrottleDimensionLogin, loginDigest,
		admittedAt, admittedAt.Add(identityapp.AuthenticationObservationWindow), 0, 1, 1,
		deadline, time.Time{}, admittedAt,
	)
	sourceReserved := mustThrottleRecord(
		t, identity.ThrottleDimensionSource, sourceDigest,
		admittedAt, admittedAt.Add(identityapp.AuthenticationObservationWindow), 0, 1, 1,
		deadline, time.Time{}, admittedAt,
	)

	checks := []struct {
		name       string
		now        func() time.Time
		wantClass  error
		writeFinal bool
	}{
		{
			name:       "failure finalizes both rows",
			now:        func() time.Time { return finalizedAt },
			wantClass:  identityapp.ErrAuthenticationFailed,
			writeFinal: true,
		},
		{
			name: "lock wait crosses individual lease",
			now: func() func() time.Time {
				calls := 0
				return func() time.Time {
					calls++
					switch calls {
					case 1:
						return admittedAt
					case 2:
						return deadline.Add(-time.Microsecond)
					default:
						return deadline
					}
				}
			}(),
			wantClass: identityapp.ErrAuthenticationUnavailable,
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			repository, mock := newRepositoryMock(t, check.now)
			// Begin reservation.
			mock.ExpectBegin()
			expectThrottleEnsure(mock, identity.ThrottleDimensionLogin, loginDigest, request)
			expectThrottleEnsure(mock, identity.ThrottleDimensionSource, sourceDigest, request)
			expectThrottleLock(mock, identity.ThrottleDimensionLogin, loginDigest, loginBefore)
			expectThrottleLock(mock, identity.ThrottleDimensionSource, sourceDigest, sourceBefore)
			expectThrottleSave(mock, identity.ThrottleDimensionLogin, loginDigest, 1, 0, admittedAt)
			expectThrottleSave(mock, identity.ThrottleDimensionSource, sourceDigest, 1, 0, admittedAt)
			mock.ExpectCommit()
			// Finalize locks both dimensions before its second deadline check.
			mock.ExpectBegin()
			expectThrottleLock(mock, identity.ThrottleDimensionLogin, loginDigest, loginReserved)
			expectThrottleLock(mock, identity.ThrottleDimensionSource, sourceDigest, sourceReserved)
			if check.writeFinal {
				expectThrottleSave(mock, identity.ThrottleDimensionLogin, loginDigest, 1, 1, admittedAt)
				expectThrottleSave(mock, identity.ThrottleDimensionSource, sourceDigest, 1, 1, admittedAt)
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			service, err := identityapp.NewLoginService(identityapp.LoginDependencies{
				Clock: identityapp.ClockFunc(func() time.Time { return admittedAt }),
				Credentials: credentialReaderFunc(func(
					context.Context,
					identity.LoginName,
				) (identity.WorkforceAccount, error) {
					return account, nil
				}),
				Passwords:  passwordVerifierResult{matched: false},
				Admissions: repository,
				Entropy:    &staticEntropy{reader: bytes.NewReader(nil)},
				Issuer: sessionIssuerFunc(func(
					context.Context,
					identityapp.SessionIssueAttempt,
				) error {
					return nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			command, err := identityapp.NewLoginCommand(
				account.LoginName(),
				[]byte("wrong password"),
				loginDigest,
				sourceDigest,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			issued, loginErr := service.Login(context.Background(), command)
			if issued != (identityapp.IssuedSession{}) || !errors.Is(loginErr, check.wantClass) {
				t.Fatalf("Login() = %#v, %v", issued, loginErr)
			}
			assertMockExpectations(t, mock)
		})
	}
}

func expectThrottleEnsure(
	mock sqlmock.Sqlmock,
	dimension identity.ThrottleDimension,
	digest identity.ThrottleDigest,
	request identityapp.AdmissionRequest,
) {
	mock.ExpectExec(sqlPattern(ensureThrottleSQL)).WithArgs(
		string(dimension),
		digest.Bytes(),
		request.AdmittedAt(),
		request.AdmittedAt().Add(request.Policy().ObservationWindow()),
		request.AdmittedAt(),
		request.AdmittedAt().Add(ThrottleRowRetention),
	).WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectThrottleLock(
	mock sqlmock.Sqlmock,
	dimension identity.ThrottleDimension,
	digest identity.ThrottleDigest,
	record throttleRecord,
) {
	mock.ExpectQuery(sqlPattern(selectThrottleForUpdateSQL)).
		WithArgs(string(dimension), digest.Bytes()).
		WillReturnRows(throttleRows(record))
}

func expectThrottleSave(
	mock sqlmock.Sqlmock,
	dimension identity.ThrottleDimension,
	digest identity.ThrottleDigest,
	epoch uint64,
	inflight uint32,
	updatedAt time.Time,
) {
	mock.ExpectExec(sqlPattern(updateThrottleSQL)).WithArgs(
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), string(dimension), digest.Bytes(), epoch, inflight, updatedAt,
	).WillReturnResult(sqlmock.NewResult(0, 1))
}
