package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	governance "github.com/Atingaii/GrowthOS-Go/internal/governance/domain"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestResolveReturnsOnlyAuthoritativeHumanPrincipal(t *testing.T) {
	t.Parallel()

	rawToken := make([]byte, SessionTokenBytes)
	for index := range rawToken {
		rawToken[index] = byte(index + 1)
	}
	originalToken := slices.Clone(rawToken)
	digestBytes := sha256.Sum256(rawToken)
	digest, _ := identity.NewTokenDigest(digestBytes[:])
	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	session := mustApplicationSession(
		t,
		account,
		digest,
		"session-resolve-1",
		"issue-resolve-1",
		applicationTestNow.Add(-time.Minute),
	)
	var calls int
	resolver := sessionResolverFunc(func(
		_ context.Context,
		gotDigest identity.TokenDigest,
		now time.Time,
		idleLifetime time.Duration,
		touchWindow time.Duration,
	) (identity.WorkforceAccount, identity.Session, error) {
		calls++
		if !sameTokenDigest(gotDigest, digest) || now != applicationTestNow ||
			idleLifetime != SessionIdleLifetime || touchWindow != SessionTouchWindow {
			t.Fatalf("resolve arguments drifted")
		}
		return account, session, nil
	})
	service, err := NewResolveService(ClockFunc(func() time.Time { return applicationTestNow }), resolver)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := service.Resolve(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if verified.Validate() != nil || verified.Principal().Kind() != governance.PrincipalKindHuman ||
		verified.Principal().ID().String() != account.PrincipalID().String() ||
		verified.SessionReference() != session.Reference() || calls != 1 {
		t.Fatalf("verified output mismatch: %#v", verified)
	}
	if !slices.Equal(rawToken, originalToken) {
		t.Fatal("Resolve mutated caller-owned bearer bytes")
	}
}

func TestResolveFailureMatrixReturnsZeroPrincipal(t *testing.T) {
	t.Parallel()

	rawToken := make([]byte, SessionTokenBytes)
	for index := range rawToken {
		rawToken[index] = 0x33
	}
	digestBytes := sha256.Sum256(rawToken)
	digest, _ := identity.NewTokenDigest(digestBytes[:])
	enabled := mustApplicationAccount(t, identity.AccountStatusEnabled)
	validSession := mustApplicationSession(
		t,
		enabled,
		digest,
		"session-resolve-matrix",
		"issue-resolve-matrix",
		applicationTestNow.Add(-time.Minute),
	)
	disabled := mustApplicationAccount(t, identity.AccountStatusDisabled)
	inactive := mustApplicationSession(
		t,
		enabled,
		digest,
		"session-expired",
		"issue-expired",
		applicationTestNow.Add(-16*time.Minute),
	)
	mismatchedDigestSession := mustApplicationSession(
		t,
		enabled,
		mustApplicationDigest(t, 0x77),
		"session-wrong-digest",
		"issue-wrong-digest",
		applicationTestNow.Add(-time.Minute),
	)
	mismatchedEpochSession := mustSessionWithEpoch(
		t,
		enabled,
		digest,
		12,
		"session-wrong-epoch",
		"issue-wrong-epoch",
		applicationTestNow.Add(-time.Minute),
	)

	cases := []struct {
		name    string
		account identity.WorkforceAccount
		session identity.Session
		repoErr error
		want    error
	}{
		{name: "missing", repoErr: ErrSessionNotFound, want: ErrUnauthenticated},
		{name: "repository inactive", repoErr: ErrSessionInactive, want: ErrUnauthenticated},
		{name: "dependency unavailable", repoErr: ErrDependencyUnavailable, want: ErrAuthenticationUnavailable},
		{name: "disabled account", account: disabled, session: validSession, want: ErrUnauthenticated},
		{name: "epoch mismatch", account: enabled, session: mismatchedEpochSession, want: ErrUnauthenticated},
		{name: "expired", account: enabled, session: inactive, want: ErrUnauthenticated},
		{name: "digest mismatch", account: enabled, session: mismatchedDigestSession, want: ErrAuthenticationUnavailable},
		{name: "zero stored account", session: validSession, want: ErrAuthenticationUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			resolver := sessionResolverFunc(func(
				context.Context,
				identity.TokenDigest,
				time.Time,
				time.Duration,
				time.Duration,
			) (identity.WorkforceAccount, identity.Session, error) {
				return testCase.account, testCase.session, testCase.repoErr
			})
			service, err := NewResolveService(ClockFunc(func() time.Time { return applicationTestNow }), resolver)
			if err != nil {
				t.Fatal(err)
			}
			verified, resolveErr := service.Resolve(context.Background(), rawToken)
			assertZeroVerified(t, verified)
			if !errors.Is(resolveErr, testCase.want) {
				t.Fatalf("resolve error = %v, want %v", resolveErr, testCase.want)
			}
		})
	}

	service, err := NewResolveService(
		ClockFunc(func() time.Time { return applicationTestNow }),
		sessionResolverFunc(func(context.Context, identity.TokenDigest, time.Time, time.Duration, time.Duration) (identity.WorkforceAccount, identity.Session, error) {
			return enabled, validSession, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalidToken := range [][]byte{nil, make([]byte, SessionTokenBytes), make([]byte, SessionTokenBytes-1)} {
		verified, resolveErr := service.Resolve(context.Background(), invalidToken)
		assertZeroVerified(t, verified)
		if !errors.Is(resolveErr, ErrUnauthenticated) {
			t.Fatalf("invalid token error = %v", resolveErr)
		}
	}
}

func TestRevokeCurrentConfirmedAndUnknownOutcomes(t *testing.T) {
	t.Parallel()

	rawToken := make([]byte, SessionTokenBytes)
	for index := range rawToken {
		rawToken[index] = byte(0x80 + index)
	}
	digestBytes := sha256.Sum256(rawToken)
	digest, _ := identity.NewTokenDigest(digestBytes[:])
	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	before := mustApplicationSession(
		t,
		account,
		digest,
		"session-revoke-1",
		"issue-revoke-1",
		applicationTestNow.Add(-time.Minute),
	)

	for _, testCase := range []struct {
		name      string
		writeErr  error
		wantError error
	}{
		{name: "confirmed", wantError: nil},
		{name: "commit unknown", writeErr: WrapDependencyError(ErrCommitOutcomeUnknown, errors.New("private commit detail")), wantError: ErrRevocationIndeterminate},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var captured SessionRevokeAttempt
			service := mustRevokeService(t, RevokeCurrentDependencies{
				Clock: ClockFunc(func() time.Time { return applicationTestNow }),
				Reader: revocationReaderFunc(func(_ context.Context, got identity.TokenDigest) (identity.WorkforceAccount, identity.Session, error) {
					if !sameTokenDigest(got, digest) {
						t.Fatal("revoke lookup digest mismatch")
					}
					return account, before, nil
				}),
				Revoker: sessionRevokerFunc(func(_ context.Context, attempt SessionRevokeAttempt) error {
					captured = attempt
					return testCase.writeErr
				}),
				Entropy: &sequenceEntropy{},
			})
			err := service.RevokeCurrent(context.Background(), rawToken)
			if testCase.wantError == nil {
				if err != nil {
					t.Fatalf("revoke: %v", err)
				}
			} else if !errors.Is(err, testCase.wantError) {
				t.Fatalf("revoke error = %v, want %v", err, testCase.wantError)
			}
			if captured.Validate() != nil {
				t.Fatalf("invalid revoke attempt: %#v", captured)
			}
			revokedAt, reason, operation, revoked := captured.After().Revocation()
			if !revoked || revokedAt != applicationTestNow || reason != identity.SessionRevokeReasonLogout ||
				operation.Validate() != nil || operation == before.IssueOperationRef() {
				t.Fatalf("bad revoke transition: %v/%q/%q/%v", revokedAt, reason, operation, revoked)
			}

			if testCase.wantError == ErrRevocationIndeterminate {
				receipt, ok := SessionCommitReceiptFromError(err)
				if !ok || receipt.Operation() != SessionCommitOperationRevoke {
					t.Fatalf("missing revoke receipt: %#v/%v", receipt, ok)
				}
				if got := ReconcileSessionCommit(receipt, ObserveSessionCommitState(captured.After())); got != SessionCommitReconciliationCommitted {
					t.Fatalf("committed reconcile = %q", got)
				}
				if got := ReconcileSessionCommit(receipt, ObserveSessionCommitState(captured.Before())); got != SessionCommitReconciliationNotCommitted {
					t.Fatalf("not-committed reconcile = %q", got)
				}
				otherOperation, _ := identity.NewOperationRef("revoke-another-operation")
				otherAfter, otherErr := before.Revoke(applicationTestNow, identity.SessionRevokeReasonLogout, otherOperation)
				if otherErr != nil {
					t.Fatal(otherErr)
				}
				if got := ReconcileSessionCommit(receipt, ObserveSessionCommitState(otherAfter)); got != SessionCommitReconciliationIndeterminate {
					t.Fatalf("different same-time operation reconcile = %q", got)
				}
				if got := ReconcileSessionCommit(receipt, ObserveSessionCommitAbsence()); got != SessionCommitReconciliationIndeterminate {
					t.Fatalf("missing revoke observation = %q", got)
				}
			}
		})
	}
}

func TestRevokeCurrentFailureMatrixAndEntropySerialization(t *testing.T) {
	t.Parallel()

	rawToken := make([]byte, SessionTokenBytes)
	for index := range rawToken {
		rawToken[index] = 0x55
	}
	digestBytes := sha256.Sum256(rawToken)
	digest, _ := identity.NewTokenDigest(digestBytes[:])
	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	before := mustApplicationSession(
		t,
		account,
		digest,
		"session-revoke-matrix",
		"issue-revoke-matrix",
		applicationTestNow.Add(-time.Minute),
	)

	cases := []struct {
		name        string
		readerErr   error
		entropyFail int
		writeErr    error
		want        error
		writeCalls  int
	}{
		{name: "not found", readerErr: ErrSessionNotFound, want: ErrUnauthenticated},
		{name: "inactive", readerErr: ErrSessionInactive, want: ErrUnauthenticated},
		{name: "reader unavailable", readerErr: ErrDependencyUnavailable, want: ErrAuthenticationUnavailable},
		{name: "entropy unavailable", entropyFail: 1, want: ErrAuthenticationUnavailable},
		{name: "conditional conflict", writeErr: ErrAccountStateConflict, want: ErrUnauthenticated, writeCalls: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var writeCalls int
			service := mustRevokeService(t, RevokeCurrentDependencies{
				Clock: ClockFunc(func() time.Time { return applicationTestNow }),
				Reader: revocationReaderFunc(func(context.Context, identity.TokenDigest) (identity.WorkforceAccount, identity.Session, error) {
					return account, before, testCase.readerErr
				}),
				Revoker: sessionRevokerFunc(func(context.Context, SessionRevokeAttempt) error {
					writeCalls++
					return testCase.writeErr
				}),
				Entropy: &sequenceEntropy{failAt: testCase.entropyFail},
			})
			err := service.RevokeCurrent(context.Background(), rawToken)
			if !errors.Is(err, testCase.want) || writeCalls != testCase.writeCalls {
				t.Fatalf("error/writeCalls = %v/%d, want %v/%d", err, writeCalls, testCase.want, testCase.writeCalls)
			}
		})
	}

	entropy := &sequenceEntropy{}
	service := mustRevokeService(t, RevokeCurrentDependencies{
		Clock: ClockFunc(func() time.Time { return applicationTestNow }),
		Reader: revocationReaderFunc(func(context.Context, identity.TokenDigest) (identity.WorkforceAccount, identity.Session, error) {
			return account, before, nil
		}),
		Revoker: sessionRevokerFunc(func(context.Context, SessionRevokeAttempt) error { return nil }),
		Entropy: entropy,
	})
	const workers = 32
	var wait sync.WaitGroup
	errorsFound := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := service.RevokeCurrent(context.Background(), rawToken); err != nil {
				errorsFound <- err
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent revoke: %v", err)
	}
	if entropy.overlapping.Load() || entropy.maxActive.Load() != 1 {
		t.Fatalf("revoke entropy reads overlapped: overlap=%v max=%d", entropy.overlapping.Load(), entropy.maxActive.Load())
	}
}

func mustSessionWithEpoch(
	t *testing.T,
	account identity.WorkforceAccount,
	digest identity.TokenDigest,
	epochValue uint64,
	reference string,
	operation string,
	issuedAt time.Time,
) identity.Session {
	t.Helper()
	referenceValue, _ := identity.NewSessionRef(reference)
	operationValue, _ := identity.NewOperationRef(operation)
	epoch, _ := identity.NewAuthenticationEpoch(epochValue)
	session, err := identity.NewSession(
		referenceValue,
		operationValue,
		account.ID(),
		digest,
		epoch,
		issuedAt,
		issuedAt.Add(SessionIdleLifetime),
		issuedAt.Add(SessionAbsoluteLifetime),
	)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func mustRevokeService(t *testing.T, dependencies RevokeCurrentDependencies) *RevokeCurrentService {
	t.Helper()
	service, err := NewRevokeCurrentService(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
