package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

var applicationTestNow = time.Date(2026, 9, 1, 8, 0, 0, 123456000, time.UTC)

func mustApplicationAccount(t *testing.T, status identity.AccountStatus) identity.WorkforceAccount {
	t.Helper()
	id, err := identity.NewAccountID("account-1")
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginName("operator-1")
	if err != nil {
		t.Fatal(err)
	}
	principalID, err := identity.NewPrincipalID("principal-1")
	if err != nil {
		t.Fatal(err)
	}
	version, err := identity.NewCredentialVersion(7)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := identity.NewAuthenticationEpoch(11)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := identity.NewPasswordEnvelope([]byte("$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := identity.NewWorkforceAccount(
		id,
		login,
		principalID,
		status,
		version,
		epoch,
		envelope,
		applicationTestNow.Add(-24*time.Hour),
		applicationTestNow.Add(-time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func mustApplicationDigest(t *testing.T, marker byte) identity.TokenDigest {
	t.Helper()
	value := make([]byte, identity.DigestBytes)
	for index := range value {
		value[index] = marker
	}
	digest, err := identity.NewTokenDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustApplicationThrottleDigest(t *testing.T, marker byte) identity.ThrottleDigest {
	t.Helper()
	value := make([]byte, identity.DigestBytes)
	for index := range value {
		value[index] = marker
	}
	digest, err := identity.NewThrottleDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func mustApplicationSession(
	t *testing.T,
	account identity.WorkforceAccount,
	digest identity.TokenDigest,
	reference string,
	operation string,
	issuedAt time.Time,
) identity.Session {
	t.Helper()
	sessionReference, err := identity.NewSessionRef(reference)
	if err != nil {
		t.Fatal(err)
	}
	operationReference, err := identity.NewOperationRef(operation)
	if err != nil {
		t.Fatal(err)
	}
	session, err := identity.NewSession(
		sessionReference,
		operationReference,
		account.ID(),
		digest,
		account.AuthenticationEpoch(),
		issuedAt,
		issuedAt.Add(SessionIdleLifetime),
		issuedAt.Add(SessionAbsoluteLifetime),
	)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func mustApplicationCommand(t *testing.T, previousToken []byte) LoginCommand {
	t.Helper()
	login, err := identity.NewLoginName("operator-1")
	if err != nil {
		t.Fatal(err)
	}
	command, err := NewLoginCommand(
		login,
		[]byte("correct horse battery staple"),
		mustApplicationThrottleDigest(t, 0x11),
		mustApplicationThrottleDigest(t, 0x22),
		previousToken,
	)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func assertZeroIssued(t *testing.T, issued IssuedSession) {
	t.Helper()
	if issued != (IssuedSession{}) {
		t.Fatalf("failure returned a nonzero issued session: %#v", issued)
	}
}

func assertZeroVerified(t *testing.T, verified VerifiedSession) {
	t.Helper()
	if verified != (VerifiedSession{}) {
		t.Fatalf("failure returned a nonzero verified session: %#v", verified)
	}
}

type credentialReaderFunc func(context.Context, identity.LoginName) (identity.WorkforceAccount, error)

func (function credentialReaderFunc) FindByLogin(
	ctx context.Context,
	login identity.LoginName,
) (identity.WorkforceAccount, error) {
	return function(ctx, login)
}

type passwordVerifierStub struct {
	verifyLogin   func(context.Context, []byte, string) (PasswordVerification, error)
	verifyUnknown func(context.Context, []byte) error
}

func (stub passwordVerifierStub) VerifyLogin(
	ctx context.Context,
	password []byte,
	envelope string,
) (PasswordVerification, error) {
	if stub.verifyLogin == nil {
		return PasswordVerification{}, errors.New("unexpected VerifyLogin")
	}
	return stub.verifyLogin(ctx, password, envelope)
}

func (stub passwordVerifierStub) VerifyUnknownLogin(
	ctx context.Context,
	password []byte,
) error {
	if stub.verifyUnknown == nil {
		return errors.New("unexpected VerifyUnknownLogin")
	}
	return stub.verifyUnknown(ctx, password)
}

type admissionControllerStub struct {
	begin    func(context.Context, AdmissionRequest) (AdmissionGrant, error)
	finalize func(context.Context, AdmissionReceipt, AdmissionFinalOutcome) error
}

func (stub admissionControllerStub) BeginAdmission(
	ctx context.Context,
	request AdmissionRequest,
) (AdmissionGrant, error) {
	return stub.begin(ctx, request)
}

func (stub admissionControllerStub) FinalizeAdmission(
	ctx context.Context,
	receipt AdmissionReceipt,
	outcome AdmissionFinalOutcome,
) error {
	return stub.finalize(ctx, receipt, outcome)
}

type sessionIssuerFunc func(context.Context, SessionIssueAttempt) error

func (function sessionIssuerFunc) IssueSession(
	ctx context.Context,
	attempt SessionIssueAttempt,
) error {
	return function(ctx, attempt)
}

type entropyReaderFunc func([]byte) (int, error)

func (function entropyReaderFunc) Read(destination []byte) (int, error) {
	return function(destination)
}

type sessionResolverFunc func(
	context.Context,
	identity.TokenDigest,
	time.Time,
	time.Duration,
	time.Duration,
) (identity.WorkforceAccount, identity.Session, error)

func (function sessionResolverFunc) ResolveAndTouch(
	ctx context.Context,
	digest identity.TokenDigest,
	now time.Time,
	idleLifetime time.Duration,
	touchWindow time.Duration,
) (identity.WorkforceAccount, identity.Session, error) {
	return function(ctx, digest, now, idleLifetime, touchWindow)
}

type revocationReaderFunc func(
	context.Context,
	identity.TokenDigest,
) (identity.WorkforceAccount, identity.Session, error)

func (function revocationReaderFunc) FindForRevocation(
	ctx context.Context,
	digest identity.TokenDigest,
) (identity.WorkforceAccount, identity.Session, error) {
	return function(ctx, digest)
}

type sessionRevokerFunc func(context.Context, SessionRevokeAttempt) error

func (function sessionRevokerFunc) RevokeSession(
	ctx context.Context,
	attempt SessionRevokeAttempt,
) error {
	return function(ctx, attempt)
}

// sequenceEntropy produces distinct, nonzero full reads. It deliberately
// detects overlapping calls so tests can prove the service-side serialization
// promised by the EntropyReader port.
type sequenceEntropy struct {
	mu          sync.Mutex
	next        byte
	failAt      int
	calls       int
	active      atomic.Int32
	maxActive   atomic.Int32
	overlapping atomic.Bool
	onRead      func()
}

func (entropy *sequenceEntropy) Read(destination []byte) (int, error) {
	active := entropy.active.Add(1)
	defer entropy.active.Add(-1)
	for {
		maximum := entropy.maxActive.Load()
		if active <= maximum || entropy.maxActive.CompareAndSwap(maximum, active) {
			break
		}
	}
	if active > 1 {
		entropy.overlapping.Store(true)
	}
	if entropy.onRead != nil {
		entropy.onRead()
	}

	entropy.mu.Lock()
	defer entropy.mu.Unlock()
	entropy.calls++
	if entropy.failAt > 0 && entropy.calls == entropy.failAt {
		return 0, errors.New("entropy unavailable")
	}
	if entropy.next == 0 {
		entropy.next = 1
	}
	for index := range destination {
		destination[index] = entropy.next
	}
	entropy.next++
	return len(destination), nil
}

func successfulAdmission(t *testing.T, finalized *AdmissionFinalOutcome) AdmissionController {
	t.Helper()
	return admissionControllerStub{
		begin: func(_ context.Context, request AdmissionRequest) (AdmissionGrant, error) {
			loginEpoch, err := identity.NewAdmissionEpoch(3)
			if err != nil {
				t.Fatal(err)
			}
			sourceEpoch, err := identity.NewAdmissionEpoch(5)
			if err != nil {
				t.Fatal(err)
			}
			return NewAdmissionGrant(loginEpoch, sourceEpoch, request.Deadline())
		},
		finalize: func(_ context.Context, receipt AdmissionReceipt, outcome AdmissionFinalOutcome) error {
			if receipt.Validate() != nil {
				t.Fatal("invalid admission receipt")
			}
			if finalized != nil {
				*finalized = outcome
			}
			return nil
		},
	}
}
