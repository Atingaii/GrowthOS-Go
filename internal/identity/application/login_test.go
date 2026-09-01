package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	governance "github.com/Atingaii/GrowthOS-Go/internal/governance/domain"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestLoginSuccessUsesStrictSequenceAndFreshSession(t *testing.T) {
	t.Parallel()

	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	oldToken := make([]byte, SessionTokenBytes)
	for index := range oldToken {
		oldToken[index] = 0x7a
	}
	command := mustApplicationCommand(t, oldToken)
	var events []string
	addEvent := func(event string) { events = append(events, event) }
	entropy := &sequenceEntropy{onRead: func() { addEvent("entropy") }}
	var capturedAttempt SessionIssueAttempt
	service := mustLoginService(t, LoginDependencies{
		Clock: ClockFunc(func() time.Time { return applicationTestNow }),
		Credentials: credentialReaderFunc(func(_ context.Context, login identity.LoginName) (identity.WorkforceAccount, error) {
			addEvent("lookup")
			if login != account.LoginName() {
				t.Fatalf("lookup login = %q", login)
			}
			return account, nil
		}),
		Passwords: passwordVerifierStub{
			verifyLogin: func(_ context.Context, password []byte, envelope string) (PasswordVerification, error) {
				addEvent("verify")
				if string(password) != "correct horse battery staple" ||
					envelope != string(account.CredentialEnvelope().Bytes()) {
					t.Fatalf("verifier received wrong credential inputs")
				}
				return NewPasswordVerification(true, false)
			},
		},
		Admissions: admissionControllerStub{
			begin: func(_ context.Context, request AdmissionRequest) (AdmissionGrant, error) {
				addEvent("begin")
				loginEpoch, _ := identity.NewAdmissionEpoch(1)
				sourceEpoch, _ := identity.NewAdmissionEpoch(2)
				return NewAdmissionGrant(loginEpoch, sourceEpoch, request.Deadline())
			},
			finalize: func(_ context.Context, _ AdmissionReceipt, outcome AdmissionFinalOutcome) error {
				addEvent("finalize:" + string(outcome))
				return nil
			},
		},
		Entropy: entropy,
		Issuer: sessionIssuerFunc(func(_ context.Context, attempt SessionIssueAttempt) error {
			addEvent("issue")
			capturedAttempt = attempt
			return nil
		}),
	})

	issued, err := service.Login(context.Background(), command)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := issued.Validate(); err != nil {
		t.Fatalf("issued session: %v", err)
	}
	wantEvents := []string{
		"begin", "lookup", "verify", "finalize:success",
		"entropy", "entropy", "entropy", "issue",
	}
	if !slices.Equal(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if capturedAttempt.Validate() != nil || capturedAttempt.Account().ID() != account.ID() {
		t.Fatalf("invalid issue attempt: %#v", capturedAttempt)
	}
	previousDigest, ok := capturedAttempt.PreviousTokenDigest()
	if !ok {
		t.Fatal("valid prior token was not carried as a revoke hint")
	}
	wantPrevious := sha256.Sum256(oldToken)
	if !slices.Equal(previousDigest.Bytes(), wantPrevious[:]) {
		t.Fatal("prior-token hint digest mismatch")
	}
	rawToken := issued.RawToken()
	if len(rawToken) != SessionTokenBytes || slices.Equal(rawToken, oldToken) {
		t.Fatal("new session reused or omitted the incoming token")
	}
	wantNewDigest := sha256.Sum256(rawToken)
	if !slices.Equal(capturedAttempt.Session().TokenDigest().Bytes(), wantNewDigest[:]) {
		t.Fatal("issued token does not match persisted digest")
	}
	rawToken[0] ^= 0xff
	if slices.Equal(rawToken, issued.RawToken()) {
		t.Fatal("RawToken did not return a defensive copy")
	}
	verified := issued.VerifiedSession()
	if verified.Principal().Kind() != governance.PrincipalKindHuman ||
		verified.Principal().ID().String() != account.PrincipalID().String() ||
		verified.SessionReference() != capturedAttempt.Session().Reference() {
		t.Fatalf("server-derived identity mismatch: %#v", verified)
	}
	if !allZero(command.Password()) || !allZero(command.previousToken) {
		t.Fatal("consumed LoginCommand retained secret bytes")
	}
}

func TestLoginUnknownWrongAndDisabledAreIndistinguishable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		reader        CredentialReader
		verification  PasswordVerification
		wantRealHash  int
		wantDummyHash int
	}{
		{
			name: "unknown account",
			reader: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
				return identity.WorkforceAccount{}, WrapDependencyError(ErrAccountNotFound, errors.New("private lookup detail"))
			}),
			wantDummyHash: 1,
		},
		{
			name: "wrong password",
			reader: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
				return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
			}),
			verification: mustPasswordVerification(t, false, false),
			wantRealHash: 1,
		},
		{
			name: "disabled account",
			reader: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
				return mustApplicationAccount(t, identity.AccountStatusDisabled), nil
			}),
			verification: mustPasswordVerification(t, true, false),
			wantRealHash: 1,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var realHashes, dummyHashes, issuerCalls int
			var finalOutcome AdmissionFinalOutcome
			service := mustLoginService(t, LoginDependencies{
				Clock:       ClockFunc(func() time.Time { return applicationTestNow }),
				Credentials: testCase.reader,
				Passwords: passwordVerifierStub{
					verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
						realHashes++
						return testCase.verification, nil
					},
					verifyUnknown: func(context.Context, []byte) error {
						dummyHashes++
						return nil
					},
				},
				Admissions: successfulAdmission(t, &finalOutcome),
				Entropy:    &sequenceEntropy{},
				Issuer: sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error {
					issuerCalls++
					return nil
				}),
			})
			issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
			assertZeroIssued(t, issued)
			if !errors.Is(err, ErrAuthenticationFailed) || err.Error() != ErrAuthenticationFailed.Error() {
				t.Fatalf("public error = %q, %v", err, err)
			}
			if realHashes != testCase.wantRealHash || dummyHashes != testCase.wantDummyHash ||
				issuerCalls != 0 || finalOutcome != AdmissionFinalOutcomeFailure {
				t.Fatalf("real/dummy/issue/outcome = %d/%d/%d/%q", realHashes, dummyHashes, issuerCalls, finalOutcome)
			}
		})
	}
}

func TestLoginFinalizesNeutralWithUncanceledCleanupContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), testContextKey{}, "preserved"))
	defer cancel()
	var finalized int
	service := mustLoginService(t, LoginDependencies{
		Clock: ClockFunc(func() time.Time { return applicationTestNow }),
		Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
			return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
		}),
		Passwords: passwordVerifierStub{
			verifyLogin: func(hashContext context.Context, _ []byte, _ string) (PasswordVerification, error) {
				cancel()
				return PasswordVerification{}, hashContext.Err()
			},
		},
		Admissions: admissionControllerStub{
			begin: func(_ context.Context, request AdmissionRequest) (AdmissionGrant, error) {
				epoch, _ := identity.NewAdmissionEpoch(1)
				return NewAdmissionGrant(epoch, epoch, request.Deadline())
			},
			finalize: func(cleanupContext context.Context, receipt AdmissionReceipt, outcome AdmissionFinalOutcome) error {
				finalized++
				if cleanupContext.Err() != nil || cleanupContext.Value(testContextKey{}) != "preserved" ||
					outcome != AdmissionFinalOutcomeNeutral || receipt.Validate() != nil {
					t.Fatalf("bad cleanup context/outcome: err=%v value=%v outcome=%q", cleanupContext.Err(), cleanupContext.Value(testContextKey{}), outcome)
				}
				return nil
			},
		},
		Entropy: &sequenceEntropy{},
		Issuer:  sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error { return nil }),
	})
	issued, err := service.Login(ctx, mustApplicationCommand(t, nil))
	assertZeroIssued(t, issued)
	if !errors.Is(err, ErrOperationCanceled) || finalized != 1 {
		t.Fatalf("cancel result = %v, finalized=%d", err, finalized)
	}
}

type testContextKey struct{}

func TestLoginFailureClassesFinalizeOnceAndReturnZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		beginErr         error
		verifyErr        error
		finalizeErr      error
		entropyFailAt    int
		issueErrors      []error
		wantClass        error
		wantFinalize     int
		wantFinalOutcome AdmissionFinalOutcome
		wantIssueCalls   int
	}{
		{name: "admission rejected", beginErr: ErrAdmissionRejected, wantClass: ErrAuthenticationThrottled},
		{name: "verifier unavailable", verifyErr: ErrDependencyUnavailable, wantClass: ErrAuthenticationUnavailable, wantFinalize: 1, wantFinalOutcome: AdmissionFinalOutcomeNeutral},
		{name: "finalize commit unknown", finalizeErr: ErrCommitOutcomeUnknown, wantClass: ErrCommitOutcomeUnknown, wantFinalize: 1, wantFinalOutcome: AdmissionFinalOutcomeSuccess},
		{name: "entropy unavailable", entropyFailAt: 1, wantClass: ErrAuthenticationUnavailable, wantFinalize: 1, wantFinalOutcome: AdmissionFinalOutcomeSuccess},
		{name: "account recheck conflict", issueErrors: []error{ErrAccountStateConflict}, wantClass: ErrAuthenticationFailed, wantFinalize: 1, wantFinalOutcome: AdmissionFinalOutcomeSuccess, wantIssueCalls: 1},
		{name: "three digest collisions", issueErrors: []error{ErrTokenDigestCollision, ErrTokenDigestCollision, ErrTokenDigestCollision}, wantClass: ErrAuthenticationUnavailable, wantFinalize: 1, wantFinalOutcome: AdmissionFinalOutcomeSuccess, wantIssueCalls: MaximumIssueAttempts},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var finalizeCalls, issueCalls int
			var finalOutcome AdmissionFinalOutcome
			service := mustLoginService(t, LoginDependencies{
				Clock: ClockFunc(func() time.Time { return applicationTestNow }),
				Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
					return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
				}),
				Passwords: passwordVerifierStub{
					verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
						if testCase.verifyErr != nil {
							return PasswordVerification{}, testCase.verifyErr
						}
						return NewPasswordVerification(true, false)
					},
				},
				Admissions: admissionControllerStub{
					begin: func(_ context.Context, request AdmissionRequest) (AdmissionGrant, error) {
						if testCase.beginErr != nil {
							return AdmissionGrant{}, testCase.beginErr
						}
						epoch, _ := identity.NewAdmissionEpoch(1)
						return NewAdmissionGrant(epoch, epoch, request.Deadline())
					},
					finalize: func(_ context.Context, _ AdmissionReceipt, outcome AdmissionFinalOutcome) error {
						finalizeCalls++
						finalOutcome = outcome
						return testCase.finalizeErr
					},
				},
				Entropy: &sequenceEntropy{failAt: testCase.entropyFailAt},
				Issuer: sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error {
					var result error
					if issueCalls < len(testCase.issueErrors) {
						result = testCase.issueErrors[issueCalls]
					}
					issueCalls++
					return result
				}),
			})
			issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
			assertZeroIssued(t, issued)
			if !errors.Is(err, testCase.wantClass) {
				t.Fatalf("error = %v, want class %v", err, testCase.wantClass)
			}
			if finalizeCalls != testCase.wantFinalize || issueCalls != testCase.wantIssueCalls {
				t.Fatalf("finalize/issue = %d/%d, want %d/%d", finalizeCalls, issueCalls, testCase.wantFinalize, testCase.wantIssueCalls)
			}
			if testCase.wantFinalize > 0 && finalOutcome != testCase.wantFinalOutcome {
				t.Fatalf("final outcome = %q, want %q", finalOutcome, testCase.wantFinalOutcome)
			}
		})
	}
}

func TestLoginIssueCommitUnknownReturnsOnlyDefensiveReceipt(t *testing.T) {
	t.Parallel()

	var candidate identity.Session
	service := mustLoginService(t, LoginDependencies{
		Clock: ClockFunc(func() time.Time { return applicationTestNow }),
		Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
			return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
		}),
		Passwords: passwordVerifierStub{
			verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
				return NewPasswordVerification(true, false)
			},
		},
		Admissions: successfulAdmission(t, nil),
		Entropy:    &sequenceEntropy{},
		Issuer: sessionIssuerFunc(func(_ context.Context, attempt SessionIssueAttempt) error {
			candidate = attempt.Session()
			return WrapDependencyError(ErrCommitOutcomeUnknown, errors.New("driver: connection lost after COMMIT"))
		}),
	})
	issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
	assertZeroIssued(t, issued)
	if !errors.Is(err, ErrCommitOutcomeUnknown) || strings.Contains(err.Error(), "driver") {
		t.Fatalf("commit error disclosure = %q", err)
	}
	receipt, ok := SessionCommitReceiptFromError(fmt.Errorf("outer: %w", err))
	if !ok || receipt.Operation() != SessionCommitOperationIssue || receipt.Validate() != nil {
		t.Fatalf("missing issue receipt: %#v/%v", receipt, ok)
	}
	if reconciliation := ReconcileSessionCommit(receipt, ObserveSessionCommitState(candidate)); reconciliation != SessionCommitReconciliationCommitted {
		t.Fatalf("exact observation = %q", reconciliation)
	}
	if issued.Validate() == nil || len(issued.RawToken()) != SessionTokenBytes {
		t.Fatal("diagnostic reconciliation recovered a bearer output")
	}
}

func TestLoginCommandDefensiveValidation(t *testing.T) {
	t.Parallel()

	login, _ := identity.NewLoginName("operator-1")
	password := []byte("secret-password")
	previous := make([]byte, SessionTokenBytes)
	for index := range previous {
		previous[index] = 0x44
	}
	command, err := NewLoginCommand(
		login,
		password,
		mustApplicationThrottleDigest(t, 1),
		mustApplicationThrottleDigest(t, 2),
		previous,
	)
	if err != nil {
		t.Fatal(err)
	}
	password[0] = 'X'
	previous[0] = 0x99
	if string(command.Password()) != "secret-password" || command.previousToken[0] != 0x44 {
		t.Fatal("constructor did not defensively copy secrets")
	}
	extracted := command.Password()
	extracted[0] = 'Y'
	if string(command.Password()) != "secret-password" {
		t.Fatal("Password accessor exposed internal storage")
	}

	invalidPasswords := [][]byte{
		nil,
		{},
		{0xff},
		[]byte(strings.Repeat("a", 129)),
		[]byte(strings.Repeat("界", 129)),
	}
	for _, invalid := range invalidPasswords {
		result, resultErr := NewLoginCommand(
			login,
			invalid,
			mustApplicationThrottleDigest(t, 1),
			mustApplicationThrottleDigest(t, 2),
			nil,
		)
		if resultErr == nil || result.loginName != "" || result.password != nil ||
			result.loginDigest.Validate() == nil || result.sourceDigest.Validate() == nil ||
			result.previousToken != nil {
			t.Fatalf("invalid password accepted: len=%d", len(invalid))
		}
	}
	allZeroPrevious := make([]byte, SessionTokenBytes)
	if result, resultErr := NewLoginCommand(
		login,
		[]byte("valid"),
		mustApplicationThrottleDigest(t, 1),
		mustApplicationThrottleDigest(t, 2),
		allZeroPrevious,
	); resultErr == nil || result.loginName != "" || result.password != nil ||
		result.previousToken != nil {
		t.Fatal("all-zero prior bearer hint accepted")
	}
}

func TestLoginSerializesInjectedEntropy(t *testing.T) {
	t.Parallel()

	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	entropy := &sequenceEntropy{}
	service := mustLoginService(t, LoginDependencies{
		Clock: ClockFunc(func() time.Time { return applicationTestNow }),
		Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
			return account, nil
		}),
		Passwords: passwordVerifierStub{
			verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
				return NewPasswordVerification(true, false)
			},
		},
		Admissions: concurrentAdmissionStub(t),
		Entropy:    entropy,
		Issuer:     sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error { return nil }),
	})

	const workers = 48
	var wait sync.WaitGroup
	errorsFound := make(chan error, workers)
	commands := make([]LoginCommand, workers)
	for index := range commands {
		commands[index] = mustApplicationCommand(t, nil)
	}
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(command LoginCommand) {
			defer wait.Done()
			issued, err := service.Login(context.Background(), command)
			if err != nil {
				errorsFound <- err
				return
			}
			if issued.Validate() != nil {
				errorsFound <- errors.New("invalid issued session")
			}
		}(commands[index])
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent login: %v", err)
	}
	if entropy.overlapping.Load() || entropy.maxActive.Load() != 1 {
		t.Fatalf("entropy reads overlapped: overlap=%v max=%d", entropy.overlapping.Load(), entropy.maxActive.Load())
	}
}

func mustLoginService(t *testing.T, dependencies LoginDependencies) *LoginService {
	t.Helper()
	service, err := NewLoginService(dependencies)
	if err != nil {
		t.Fatalf("new login service: %v", err)
	}
	return service
}

func mustPasswordVerification(t *testing.T, matched, needsRehash bool) PasswordVerification {
	t.Helper()
	verification, err := NewPasswordVerification(matched, needsRehash)
	if err != nil {
		t.Fatal(err)
	}
	return verification
}

func concurrentAdmissionStub(t *testing.T) AdmissionController {
	t.Helper()
	return admissionControllerStub{
		begin: func(_ context.Context, request AdmissionRequest) (AdmissionGrant, error) {
			epoch, err := identity.NewAdmissionEpoch(1)
			if err != nil {
				return AdmissionGrant{}, err
			}
			return NewAdmissionGrant(epoch, epoch, request.Deadline())
		},
		finalize: func(_ context.Context, receipt AdmissionReceipt, outcome AdmissionFinalOutcome) error {
			if receipt.Validate() != nil || !outcome.Valid() {
				return ErrDependencyInvalidArgument
			}
			return nil
		},
	}
}
