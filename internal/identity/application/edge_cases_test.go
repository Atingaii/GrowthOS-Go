package application

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestLoginDoesNotTrustInvalidAdmissionGrant(t *testing.T) {
	t.Parallel()

	var lookupCalls, hashCalls, finalizeCalls int
	service := mustLoginService(t, LoginDependencies{
		Clock: ClockFunc(func() time.Time { return applicationTestNow }),
		Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
			lookupCalls++
			return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
		}),
		Passwords: passwordVerifierStub{
			verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
				hashCalls++
				return NewPasswordVerification(true, false)
			},
		},
		Admissions: admissionControllerStub{
			begin: func(context.Context, AdmissionRequest) (AdmissionGrant, error) {
				return AdmissionGrant{}, nil
			},
			finalize: func(context.Context, AdmissionReceipt, AdmissionFinalOutcome) error {
				finalizeCalls++
				return nil
			},
		},
		Entropy: &sequenceEntropy{},
		Issuer:  sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error { return nil }),
	})
	issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
	assertZeroIssued(t, issued)
	if !errors.Is(err, ErrAuthenticationUnavailable) || lookupCalls != 0 || hashCalls != 0 || finalizeCalls != 0 {
		t.Fatalf("invalid grant result = %v, calls lookup/hash/finalize=%d/%d/%d", err, lookupCalls, hashCalls, finalizeCalls)
	}
}

func TestLoginRetriesOnlyDigestCollisionWithFreshCandidate(t *testing.T) {
	t.Parallel()

	var attempts []identity.Session
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
			attempts = append(attempts, attempt.Session())
			if len(attempts) == 1 {
				return ErrTokenDigestCollision
			}
			return nil
		}),
	})
	issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
	if err != nil || issued.Validate() != nil || len(attempts) != 2 {
		t.Fatalf("collision retry = %#v, %v, attempts=%d", issued, err, len(attempts))
	}
	if attempts[0].Reference() == attempts[1].Reference() ||
		attempts[0].IssueOperationRef() == attempts[1].IssueOperationRef() ||
		sameTokenDigest(attempts[0].TokenDigest(), attempts[1].TokenDigest()) {
		t.Fatal("collision retry reused token, session reference, or operation reference")
	}
	issuedDigest := attempts[1].TokenDigest().Bytes()
	if slices.Equal(issuedDigest, attempts[0].TokenDigest().Bytes()) {
		t.Fatal("retry did not create a wholly fresh candidate")
	}
}

func TestLoginEntropyFailuresNeverReachIssuer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		entropy EntropyReader
	}{
		{name: "token read", entropy: &sequenceEntropy{failAt: 1}},
		{name: "session reference read", entropy: &sequenceEntropy{failAt: 2}},
		{name: "operation reference read", entropy: &sequenceEntropy{failAt: 3}},
		{name: "all zero", entropy: entropyReaderFunc(func(destination []byte) (int, error) {
			clear(destination)
			return len(destination), nil
		})},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var issuerCalls int
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
				Entropy:    testCase.entropy,
				Issuer: sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error {
					issuerCalls++
					return nil
				}),
			})
			issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
			assertZeroIssued(t, issued)
			if !errors.Is(err, ErrAuthenticationUnavailable) || issuerCalls != 0 {
				t.Fatalf("entropy failure = %v, issuerCalls=%d", err, issuerCalls)
			}
		})
	}
}

func TestCallerCancellationWinsAndNeverCarriesCommitReceipt(t *testing.T) {
	t.Parallel()

	for _, writeResult := range []error{nil, WrapDependencyError(ErrCommitOutcomeUnknown, errors.New("late unknown"))} {
		writeResult := writeResult
		t.Run(func() string {
			if writeResult == nil {
				return "confirmed commit after cancellation"
			}
			return "unknown commit after cancellation"
		}(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
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
				Issuer: sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error {
					cancel()
					return writeResult
				}),
			})
			issued, err := service.Login(ctx, mustApplicationCommand(t, nil))
			assertZeroIssued(t, issued)
			if !errors.Is(err, ErrOperationCanceled) {
				t.Fatalf("cancellation priority = %v", err)
			}
			if _, ok := SessionCommitReceiptFromError(err); ok {
				t.Fatal("canceled operation exposed a commit receipt")
			}
		})
	}
}

func TestLoginInputClockAndDeadlineFailuresStayBeforeDependencies(t *testing.T) {
	t.Parallel()

	var beginCalls int
	dependencies := func(clock Clock) LoginDependencies {
		return LoginDependencies{
			Clock: clock,
			Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
				return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
			}),
			Passwords: passwordVerifierStub{
				verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
					return NewPasswordVerification(true, false)
				},
			},
			Admissions: admissionControllerStub{
				begin: func(context.Context, AdmissionRequest) (AdmissionGrant, error) {
					beginCalls++
					return AdmissionGrant{}, nil
				},
				finalize: func(context.Context, AdmissionReceipt, AdmissionFinalOutcome) error { return nil },
			},
			Entropy: &sequenceEntropy{},
			Issuer:  sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error { return nil }),
		}
	}

	zeroClockService := mustLoginService(t, dependencies(ClockFunc(func() time.Time { return time.Time{} })))
	issued, err := zeroClockService.Login(context.Background(), mustApplicationCommand(t, nil))
	assertZeroIssued(t, issued)
	if !errors.Is(err, ErrAuthenticationUnavailable) {
		t.Fatalf("zero clock = %v", err)
	}

	futureClockService := mustLoginService(t, dependencies(ClockFunc(func() time.Time {
		return canonicalInstant(time.Now()).Add(time.Hour)
	})))
	deadlineContext, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	issued, err = futureClockService.Login(deadlineContext, mustApplicationCommand(t, nil))
	assertZeroIssued(t, issued)
	if !errors.Is(err, ErrOperationCanceled) || beginCalls != 0 {
		t.Fatalf("deadline before server time = %v, beginCalls=%d", err, beginCalls)
	}

	var nilService *LoginService
	command := mustApplicationCommand(t, nil)
	issued, err = nilService.Login(context.Background(), command)
	assertZeroIssued(t, issued)
	if !errors.Is(err, ErrNotConfigured) || !allZero(command.Password()) {
		t.Fatalf("nil service = %v, command retained secret=%v", err, !allZero(command.Password()))
	}
}

func TestAdmissionAndCredentialDependencyClassesAreStable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		beginErr  error
		readerErr error
		dummyErr  error
		finalErr  error
		want      error
		outcome   AdmissionFinalOutcome
	}{
		{name: "begin unavailable", beginErr: ErrDependencyUnavailable, want: ErrAuthenticationUnavailable},
		{name: "begin commit unknown", beginErr: ErrCommitOutcomeUnknown, want: ErrCommitOutcomeUnknown},
		{name: "reader unavailable", readerErr: ErrDependencyUnavailable, want: ErrAuthenticationUnavailable, outcome: AdmissionFinalOutcomeNeutral},
		{name: "dummy unavailable", readerErr: ErrAccountNotFound, dummyErr: ErrDependencyUnavailable, want: ErrAuthenticationUnavailable, outcome: AdmissionFinalOutcomeNeutral},
		{name: "stale finalize", finalErr: ErrAdmissionStale, want: ErrAuthenticationUnavailable, outcome: AdmissionFinalOutcomeSuccess},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var finalized AdmissionFinalOutcome
			service := mustLoginService(t, LoginDependencies{
				Clock: ClockFunc(func() time.Time { return applicationTestNow }),
				Credentials: credentialReaderFunc(func(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
					if testCase.readerErr != nil {
						return identity.WorkforceAccount{}, testCase.readerErr
					}
					return mustApplicationAccount(t, identity.AccountStatusEnabled), nil
				}),
				Passwords: passwordVerifierStub{
					verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
						return NewPasswordVerification(true, false)
					},
					verifyUnknown: func(context.Context, []byte) error { return testCase.dummyErr },
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
						finalized = outcome
						return testCase.finalErr
					},
				},
				Entropy: &sequenceEntropy{},
				Issuer:  sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error { return nil }),
			})
			issued, err := service.Login(context.Background(), mustApplicationCommand(t, nil))
			assertZeroIssued(t, issued)
			if !errors.Is(err, testCase.want) || finalized != testCase.outcome {
				t.Fatalf("error/outcome = %v/%q, want %v/%q", err, finalized, testCase.want, testCase.outcome)
			}
		})
	}
}
