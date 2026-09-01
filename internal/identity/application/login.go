package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

// LoginService verifies one local workforce credential and issues a wholly new
// server-side session. It never upgrades or reuses an incoming token.
type LoginService struct {
	clock       Clock
	credentials CredentialReader
	passwords   PasswordVerifier
	admissions  AdmissionController
	entropy     EntropyReader
	issuer      SessionIssuer
	entropyMu   sync.Mutex
}

// LoginDependencies keeps the required trust boundaries explicit.
type LoginDependencies struct {
	Clock       Clock
	Credentials CredentialReader
	Passwords   PasswordVerifier
	Admissions  AdmissionController
	Entropy     EntropyReader
	Issuer      SessionIssuer
}

func NewLoginService(dependencies LoginDependencies) (*LoginService, error) {
	service := &LoginService{
		clock:       dependencies.Clock,
		credentials: dependencies.Credentials,
		passwords:   dependencies.Passwords,
		admissions:  dependencies.Admissions,
		entropy:     dependencies.Entropy,
		issuer:      dependencies.Issuer,
	}
	if service.Validate() != nil {
		return nil, ErrNotConfigured
	}
	return service, nil
}

func (service *LoginService) Validate() error {
	if service == nil || dependencyIsNil(service.clock) ||
		dependencyIsNil(service.credentials) || dependencyIsNil(service.passwords) ||
		dependencyIsNil(service.admissions) || dependencyIsNil(service.entropy) ||
		dependencyIsNil(service.issuer) {
		return ErrNotConfigured
	}
	return nil
}

// Login follows the fixed admission -> credential -> finalize -> entropy ->
// issue sequence. Once BeginAdmission succeeds, it invokes FinalizeAdmission at
// most once on every ordinary return path.
func (service *LoginService) Login(
	ctx context.Context,
	command LoginCommand,
) (IssuedSession, error) {
	defer zeroSecret(command.password)
	defer zeroSecret(command.previousToken)
	if service.Validate() != nil {
		return IssuedSession{}, wrapOperationError(ErrNotConfigured, ErrNotConfigured)
	}
	if ctx == nil || command.Validate() != nil {
		return IssuedSession{}, wrapOperationError(ErrInvalidArgument, ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return IssuedSession{}, wrapOperationError(ErrOperationCanceled, err)
	}
	now := canonicalInstant(service.clock.Now())
	if now.IsZero() {
		return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, errors.New("clock returned zero"))
	}
	deadline, err := admissionDeadline(ctx, now)
	if err != nil {
		return IssuedSession{}, err
	}
	request, err := NewAdmissionRequest(
		command.loginDigest,
		command.sourceDigest,
		now,
		deadline,
	)
	if err != nil {
		return IssuedSession{}, wrapOperationError(ErrInvalidArgument, err)
	}
	grant, err := service.admissions.BeginAdmission(ctx, request)
	if err != nil {
		return IssuedSession{}, classifyBeginAdmissionError(ctx, err)
	}
	receipt, err := newAdmissionReceipt(request, grant)
	if err != nil {
		// An unusable grant cannot safely authorize finalization. Its bounded
		// lease is the fail-closed recovery mechanism.
		return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
	}

	account, outcome, authenticationErr := service.verifyCredential(ctx, command)
	finalizeContext, cancelFinalize := context.WithTimeout(
		context.WithoutCancel(ctx),
		AdmissionFinalizeTimeout,
	)
	finalizeErr := service.admissions.FinalizeAdmission(finalizeContext, receipt, outcome)
	var classifiedFinalizeErr error
	if finalizeErr != nil {
		classifiedFinalizeErr = classifyFinalizeAdmissionError(ctx, finalizeErr)
	}
	cancelFinalize()
	if authenticationErr != nil && errors.Is(authenticationErr, ErrOperationCanceled) {
		return IssuedSession{}, authenticationErr
	}
	if err := ctx.Err(); err != nil {
		return IssuedSession{}, wrapOperationError(ErrOperationCanceled, err)
	}
	if classifiedFinalizeErr != nil {
		return IssuedSession{}, classifiedFinalizeErr
	}
	if authenticationErr != nil {
		return IssuedSession{}, authenticationErr
	}
	previousDigest, hasPrevious := digestOptionalToken(command.previousToken)
	return service.issueNewSession(ctx, account, now, previousDigest, hasPrevious)
}

func (service *LoginService) verifyCredential(
	ctx context.Context,
	command LoginCommand,
) (identity.WorkforceAccount, AdmissionFinalOutcome, error) {
	password := command.Password()
	defer zeroSecret(password)
	account, err := service.credentials.FindByLogin(ctx, command.loginName)
	if err != nil {
		if canceled := canceledOperationError(ctx, err); canceled != nil {
			return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral, canceled
		}
		if !errors.Is(err, ErrAccountNotFound) {
			return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral,
				wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		if verifyErr := service.passwords.VerifyUnknownLogin(ctx, password); verifyErr != nil {
			if canceled := canceledOperationError(ctx, verifyErr); canceled != nil {
				return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral, canceled
			}
			return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral,
				wrapOperationError(ErrAuthenticationUnavailable, verifyErr)
		}
		return identity.WorkforceAccount{}, AdmissionFinalOutcomeFailure,
			wrapOperationError(ErrAuthenticationFailed, ErrAuthenticationFailed)
	}
	if account.Validate() != nil {
		return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral,
			wrapOperationError(ErrAuthenticationUnavailable, ErrStoredIdentityInvalid)
	}

	encodedBytes := account.CredentialEnvelope().Bytes()
	defer zeroSecret(encodedBytes)
	verification, verifyErr := service.passwords.VerifyLogin(
		ctx,
		password,
		string(encodedBytes),
	)
	if verifyErr != nil {
		if canceled := canceledOperationError(ctx, verifyErr); canceled != nil {
			return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral, canceled
		}
		return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral,
			wrapOperationError(ErrAuthenticationUnavailable, verifyErr)
	}
	if verification.Validate() != nil {
		return identity.WorkforceAccount{}, AdmissionFinalOutcomeNeutral,
			wrapOperationError(ErrAuthenticationUnavailable, errors.New("password verifier contract violation"))
	}
	if account.Status() != identity.AccountStatusEnabled || !verification.Matched() {
		return identity.WorkforceAccount{}, AdmissionFinalOutcomeFailure,
			wrapOperationError(ErrAuthenticationFailed, ErrAuthenticationFailed)
	}
	return account, AdmissionFinalOutcomeSuccess, nil
}

func (service *LoginService) issueNewSession(
	ctx context.Context,
	account identity.WorkforceAccount,
	now time.Time,
	previousDigest identity.TokenDigest,
	hasPrevious bool,
) (IssuedSession, error) {
	for attemptNumber := 0; attemptNumber < MaximumIssueAttempts; attemptNumber++ {
		if err := ctx.Err(); err != nil {
			return IssuedSession{}, wrapOperationError(ErrOperationCanceled, err)
		}
		rawToken, sessionReference, operationReference, err := service.generateTokenAndReferences()
		if err != nil {
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		if err := ctx.Err(); err != nil {
			clear(rawToken)
			return IssuedSession{}, wrapOperationError(ErrOperationCanceled, err)
		}
		digestBytes := sha256.Sum256(rawToken)
		digest, err := identity.NewTokenDigest(digestBytes[:])
		if err != nil {
			clear(rawToken)
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		session, err := identity.NewSession(
			sessionReference,
			operationReference,
			account.ID(),
			digest,
			account.AuthenticationEpoch(),
			now,
			now.Add(SessionIdleLifetime),
			now.Add(SessionAbsoluteLifetime),
		)
		if err != nil {
			clear(rawToken)
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		principal, err := principalFromAccount(account)
		if err != nil {
			clear(rawToken)
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		verified, err := newVerifiedSession(principal, session)
		if err != nil {
			clear(rawToken)
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		issued, err := newIssuedSession(verified, rawToken)
		if err != nil {
			clear(rawToken)
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		issueAttempt, err := newSessionIssueAttempt(account, session, previousDigest, hasPrevious)
		if err != nil {
			clear(rawToken)
			clear(issued.rawToken[:])
			return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
		}
		writeErr := service.issuer.IssueSession(ctx, issueAttempt)
		if writeErr == nil {
			if err := ctx.Err(); err != nil {
				clear(rawToken)
				clear(issued.rawToken[:])
				return IssuedSession{}, wrapOperationError(ErrOperationCanceled, err)
			}
			clear(rawToken)
			return issued, nil
		}
		clear(rawToken)
		clear(issued.rawToken[:])
		if canceled := canceledOperationError(ctx, writeErr); canceled != nil {
			return IssuedSession{}, canceled
		}
		if errors.Is(writeErr, ErrTokenDigestCollision) {
			continue
		}
		if errors.Is(writeErr, ErrCommitOutcomeUnknown) {
			receipt, receiptErr := newIssueCommitReceipt(session)
			if receiptErr != nil {
				return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, receiptErr)
			}
			return IssuedSession{}, wrapOperationErrorWithReceipt(
				ErrCommitOutcomeUnknown,
				writeErr,
				receipt,
			)
		}
		if errors.Is(writeErr, ErrAccountStateConflict) {
			return IssuedSession{}, wrapOperationError(ErrAuthenticationFailed, writeErr)
		}
		return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, writeErr)
	}
	return IssuedSession{}, wrapOperationError(ErrAuthenticationUnavailable, ErrTokenDigestCollision)
}

func (service *LoginService) generateTokenAndReferences() (
	[]byte,
	identity.SessionRef,
	identity.OperationRef,
	error,
) {
	service.entropyMu.Lock()
	defer service.entropyMu.Unlock()

	rawToken := make([]byte, SessionTokenBytes)
	if _, err := io.ReadFull(service.entropy, rawToken); err != nil || allZero(rawToken) {
		clear(rawToken)
		if err != nil {
			return nil, "", "", err
		}
		return nil, "", "", errors.New("entropy returned an all-zero token")
	}
	referenceEntropy := make([]byte, SessionReferenceEntropyBytes)
	if _, err := io.ReadFull(service.entropy, referenceEntropy); err != nil || allZero(referenceEntropy) {
		clear(rawToken)
		clear(referenceEntropy)
		if err != nil {
			return nil, "", "", err
		}
		return nil, "", "", errors.New("entropy returned an all-zero session reference")
	}
	reference, err := identity.NewSessionRef("ses_" + hex.EncodeToString(referenceEntropy))
	clear(referenceEntropy)
	if err != nil {
		clear(rawToken)
		return nil, "", "", err
	}
	operationEntropy := make([]byte, OperationReferenceEntropyBytes)
	if _, err := io.ReadFull(service.entropy, operationEntropy); err != nil || allZero(operationEntropy) {
		clear(rawToken)
		clear(operationEntropy)
		if err != nil {
			return nil, "", "", err
		}
		return nil, "", "", errors.New("entropy returned an all-zero operation reference")
	}
	operationReference, err := identity.NewOperationRef(
		"issue_" + hex.EncodeToString(operationEntropy),
	)
	clear(operationEntropy)
	if err != nil {
		clear(rawToken)
		return nil, "", "", err
	}
	return rawToken, reference, operationReference, nil
}

func admissionDeadline(ctx context.Context, now time.Time) (time.Time, error) {
	deadline := now.Add(MaximumAdmissionLease)
	if callerDeadline, ok := ctx.Deadline(); ok {
		callerDeadline = canonicalInstant(callerDeadline)
		if callerDeadline.Before(deadline) {
			deadline = callerDeadline
		}
	}
	if !now.Before(deadline) {
		return time.Time{}, wrapOperationError(ErrOperationCanceled, context.DeadlineExceeded)
	}
	return deadline, nil
}

func digestOptionalToken(rawToken []byte) (identity.TokenDigest, bool) {
	if len(rawToken) != SessionTokenBytes {
		return identity.TokenDigest{}, false
	}
	digestBytes := sha256.Sum256(rawToken)
	digest, err := identity.NewTokenDigest(digestBytes[:])
	if err != nil {
		return identity.TokenDigest{}, false
	}
	return digest, true
}

func classifyBeginAdmissionError(ctx context.Context, err error) error {
	if canceled := canceledOperationError(ctx, err); canceled != nil {
		return canceled
	}
	if errors.Is(err, ErrAdmissionRejected) {
		return wrapOperationError(ErrAuthenticationThrottled, err)
	}
	if errors.Is(err, ErrCommitOutcomeUnknown) {
		return wrapOperationError(ErrCommitOutcomeUnknown, err)
	}
	return wrapOperationError(ErrAuthenticationUnavailable, err)
}

func classifyFinalizeAdmissionError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return wrapOperationError(ErrOperationCanceled, ctx.Err())
	}
	if errors.Is(err, ErrCommitOutcomeUnknown) {
		return wrapOperationError(ErrCommitOutcomeUnknown, err)
	}
	return wrapOperationError(ErrAuthenticationUnavailable, err)
}
