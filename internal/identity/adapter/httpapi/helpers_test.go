package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/requestguard"
	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/sessioncookie"
	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

const testOrigin = "http://127.0.0.1:8080"

var testNow = time.Date(2026, 9, 1, 1, 2, 3, 456000000, time.UTC)

type stubLogin struct {
	validateErr error
	result      identityapp.IssuedSession
	err         error
	calls       int
}

func (stub *stubLogin) Validate() error { return stub.validateErr }
func (stub *stubLogin) Login(context.Context, identityapp.LoginCommand) (identityapp.IssuedSession, error) {
	stub.calls++
	return stub.result, stub.err
}

type stubResolve struct {
	validateErr error
	result      identityapp.VerifiedSession
	err         error
	calls       int
}

func (stub *stubResolve) Validate() error { return stub.validateErr }
func (stub *stubResolve) Resolve(context.Context, []byte) (identityapp.VerifiedSession, error) {
	stub.calls++
	return stub.result, stub.err
}

type stubRevoke struct {
	validateErr error
	err         error
	calls       int
}

func (stub *stubRevoke) Validate() error { return stub.validateErr }
func (stub *stubRevoke) RevokeCurrent(context.Context, []byte) error {
	stub.calls++
	return stub.err
}

type deadlineResolve struct{ result identityapp.VerifiedSession }

func (*deadlineResolve) Validate() error { return nil }
func (resolver *deadlineResolve) Resolve(
	ctx context.Context,
	_ []byte,
) (identityapp.VerifiedSession, error) {
	<-ctx.Done()
	return resolver.result, nil
}

type deadlineRevoke struct{}

func (*deadlineRevoke) Validate() error { return nil }
func (*deadlineRevoke) RevokeCurrent(ctx context.Context, _ []byte) error {
	<-ctx.Done()
	return nil
}

type stubCSRF struct {
	issueToken string
	issueErr   error
	verifyErr  error
	issued     []identity.TokenDigest
	verified   []identity.TokenDigest
	verifyRaw  []string
}

func (stub *stubCSRF) Issue(digest identity.TokenDigest) (string, error) {
	stub.issued = append(stub.issued, digest)
	return stub.issueToken, stub.issueErr
}
func (stub *stubCSRF) Verify(raw string, digest identity.TokenDigest, _ time.Time) error {
	stub.verifyRaw = append(stub.verifyRaw, raw)
	stub.verified = append(stub.verified, digest)
	return stub.verifyErr
}

type stubDigester struct {
	err error
}

func (stub *stubDigester) DigestLogin(identity.LoginName) (identity.ThrottleDigest, error) {
	if stub.err != nil {
		return identity.ThrottleDigest{}, stub.err
	}
	return mustThrottleDigest(0x31), nil
}
func (stub *stubDigester) DigestSource(netip.Addr) (identity.ThrottleDigest, error) {
	if stub.err != nil {
		return identity.ThrottleDigest{}, stub.err
	}
	return mustThrottleDigest(0x32), nil
}

type fixedClock time.Time

func (clock fixedClock) Now() time.Time { return time.Time(clock) }

type stubCookiePolicy struct {
	origin   string
	read     []byte
	readErr  error
	buildErr error
	clearErr error
}

func (policy *stubCookiePolicy) Validate() error {
	if policy == nil || policy.origin == "" {
		return errors.New("invalid")
	}
	return nil
}
func (policy *stubCookiePolicy) Name() string         { return "stub_session" }
func (policy *stubCookiePolicy) PublicOrigin() string { return policy.origin }
func (policy *stubCookiePolicy) Read(*http.Request) ([]byte, error) {
	return bytes.Clone(policy.read), policy.readErr
}
func (policy *stubCookiePolicy) ReadOptional(request *http.Request) ([]byte, bool, error) {
	if request == nil {
		return nil, false, errors.New("invalid")
	}
	for _, cookie := range request.Cookies() {
		if cookie.Name == policy.Name() {
			return bytes.Clone(policy.read), true, policy.readErr
		}
	}
	return nil, false, nil
}
func (policy *stubCookiePolicy) Build([]byte, time.Time, time.Time) (*http.Cookie, error) {
	if policy.buildErr != nil {
		return nil, policy.buildErr
	}
	return &http.Cookie{Name: policy.Name(), Value: "opaque", Path: "/", HttpOnly: true}, nil
}
func (policy *stubCookiePolicy) Clear() (*http.Cookie, error) {
	if policy.clearErr != nil {
		return nil, policy.clearErr
	}
	return &http.Cookie{Name: policy.Name(), Path: "/", MaxAge: -1, HttpOnly: true}, nil
}

type stubGuard struct {
	origin string
	err    error
}

func (guard *stubGuard) Validate() error {
	if guard == nil || guard.origin == "" {
		return errors.New("invalid")
	}
	return nil
}
func (guard *stubGuard) PublicOrigin() string { return guard.origin }
func (guard *stubGuard) ValidateUnsafe(*http.Request) error {
	return guard.err
}
func (guard *stubGuard) TrustedSource(*http.Request) (netip.Addr, error) {
	if guard.err != nil {
		return netip.Addr{}, guard.err
	}
	return netip.MustParseAddr("127.0.0.1"), nil
}

func validStubDependencies(t *testing.T) Dependencies {
	t.Helper()
	cookies, err := sessioncookie.NewDevelopment(testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := requestguard.New(testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	return Dependencies{
		Login:    &stubLogin{err: identityapp.ErrAuthenticationFailed},
		Resolve:  &stubResolve{err: identityapp.ErrUnauthenticated},
		Revoke:   &stubRevoke{},
		Cookies:  cookies,
		CSRF:     &stubCSRF{issueToken: "csrf-token"},
		Guard:    guard,
		Digester: &stubDigester{},
		Clock:    fixedClock(testNow),
	}
}

func mustThrottleDigest(marker byte) identity.ThrottleDigest {
	value := bytes.Repeat([]byte{marker}, identity.DigestBytes)
	digest, err := identity.NewThrottleDigest(value)
	if err != nil {
		panic(err)
	}
	return digest
}

func mustAccount(t *testing.T) identity.WorkforceAccount {
	t.Helper()
	id, _ := identity.NewAccountID("account-1")
	login, _ := identity.NewLoginName("operator-1")
	principal, _ := identity.NewPrincipalID("operator-1")
	credentialVersion, _ := identity.NewCredentialVersion(1)
	epoch, _ := identity.NewAuthenticationEpoch(1)
	envelope, _ := identity.NewPasswordEnvelope([]byte("valid-envelope"))
	account, err := identity.NewWorkforceAccount(
		id, login, principal, identity.AccountStatusEnabled, credentialVersion,
		epoch, envelope, testNow.Add(-time.Hour), testNow.Add(-time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func mustSession(t *testing.T, rawToken []byte) identity.Session {
	t.Helper()
	account := mustAccount(t)
	digestValue := sha256.Sum256(rawToken)
	digest, _ := identity.NewTokenDigest(digestValue[:])
	reference, _ := identity.NewSessionRef("session-1")
	operation, _ := identity.NewOperationRef("issue-1")
	session, err := identity.NewSession(
		reference, operation, account.ID(), digest, account.AuthenticationEpoch(),
		testNow, testNow.Add(identityapp.SessionIdleLifetime),
		testNow.Add(identityapp.SessionAbsoluteLifetime),
	)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type resolverPort struct {
	account identity.WorkforceAccount
	session identity.Session
	err     error
}

func (port *resolverPort) ResolveAndTouch(
	context.Context,
	identity.TokenDigest,
	time.Time,
	time.Duration,
	time.Duration,
) (identity.WorkforceAccount, identity.Session, error) {
	return port.account, port.session, port.err
}

func mustVerified(t *testing.T, rawToken []byte) identityapp.VerifiedSession {
	t.Helper()
	service, err := identityapp.NewResolveService(
		fixedClock(testNow.Add(time.Second)),
		&resolverPort{account: mustAccount(t), session: mustSession(t, rawToken)},
	)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := service.Resolve(context.Background(), rawToken)
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

type credentialPort struct{ account identity.WorkforceAccount }

func (port *credentialPort) FindByLogin(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
	return port.account, nil
}

type passwordPort struct{}

func (*passwordPort) VerifyLogin(context.Context, []byte, string) (identityapp.PasswordVerification, error) {
	return identityapp.NewPasswordVerification(true, false)
}
func (*passwordPort) VerifyUnknownLogin(context.Context, []byte) error { return nil }

type admissionPort struct{}

func (*admissionPort) BeginAdmission(
	_ context.Context,
	request identityapp.AdmissionRequest,
) (identityapp.AdmissionGrant, error) {
	loginEpoch, _ := identity.NewAdmissionEpoch(1)
	sourceEpoch, _ := identity.NewAdmissionEpoch(1)
	return identityapp.NewAdmissionGrant(loginEpoch, sourceEpoch, request.Deadline())
}
func (*admissionPort) FinalizeAdmission(
	context.Context,
	identityapp.AdmissionReceipt,
	identityapp.AdmissionFinalOutcome,
) error {
	return nil
}

type repeatingEntropy struct {
	mu     sync.Mutex
	marker byte
}

func (entropy *repeatingEntropy) Read(destination []byte) (int, error) {
	entropy.mu.Lock()
	defer entropy.mu.Unlock()
	for index := range destination {
		destination[index] = entropy.marker
		entropy.marker++
		if entropy.marker == 0 {
			entropy.marker = 1
		}
	}
	return len(destination), nil
}

type issuerPort struct {
	attempts []identityapp.SessionIssueAttempt
	err      error
}

func (port *issuerPort) IssueSession(_ context.Context, attempt identityapp.SessionIssueAttempt) error {
	port.attempts = append(port.attempts, attempt)
	return port.err
}

func mustLoginService(t *testing.T, issuer *issuerPort) *identityapp.LoginService {
	t.Helper()
	service, err := identityapp.NewLoginService(identityapp.LoginDependencies{
		Clock:       fixedClock(testNow),
		Credentials: &credentialPort{account: mustAccount(t)},
		Passwords:   &passwordPort{},
		Admissions:  &admissionPort{},
		Entropy:     &repeatingEntropy{marker: 1},
		Issuer:      issuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func validLoginRequest(body io.Reader) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, SessionPath, body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", testOrigin)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.RemoteAddr = "127.0.0.1:41000"
	return request
}
