package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

type RevokeCurrentService struct {
	clock     Clock
	reader    SessionRevocationReader
	revoker   SessionRevoker
	entropy   EntropyReader
	entropyMu sync.Mutex
}

type RevokeCurrentDependencies struct {
	Clock   Clock
	Reader  SessionRevocationReader
	Revoker SessionRevoker
	Entropy EntropyReader
}

func NewRevokeCurrentService(
	dependencies RevokeCurrentDependencies,
) (*RevokeCurrentService, error) {
	service := &RevokeCurrentService{
		clock:   dependencies.Clock,
		reader:  dependencies.Reader,
		revoker: dependencies.Revoker,
		entropy: dependencies.Entropy,
	}
	if service.Validate() != nil {
		return nil, ErrNotConfigured
	}
	return service, nil
}

func (service *RevokeCurrentService) Validate() error {
	if service == nil || dependencyIsNil(service.clock) ||
		dependencyIsNil(service.reader) || dependencyIsNil(service.revoker) ||
		dependencyIsNil(service.entropy) {
		return ErrNotConfigured
	}
	return nil
}

// RevokeCurrent returns nil only after the revoke COMMIT is confirmed.
func (service *RevokeCurrentService) RevokeCurrent(
	ctx context.Context,
	rawToken []byte,
) error {
	if service.Validate() != nil {
		return wrapOperationError(ErrNotConfigured, ErrNotConfigured)
	}
	if ctx == nil {
		return wrapOperationError(ErrInvalidArgument, ErrInvalidArgument)
	}
	if len(rawToken) != SessionTokenBytes || allZero(rawToken) {
		return wrapOperationError(ErrUnauthenticated, ErrUnauthenticated)
	}
	if err := ctx.Err(); err != nil {
		return wrapOperationError(ErrOperationCanceled, err)
	}
	now := canonicalInstant(service.clock.Now())
	if now.IsZero() {
		return wrapOperationError(ErrAuthenticationUnavailable, errors.New("clock returned zero"))
	}
	digestBytes := sha256.Sum256(rawToken)
	digest, err := identity.NewTokenDigest(digestBytes[:])
	if err != nil {
		return wrapOperationError(ErrUnauthenticated, err)
	}
	account, before, err := service.reader.FindForRevocation(ctx, digest)
	if err != nil {
		return classifyResolveError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return wrapOperationError(ErrOperationCanceled, err)
	}
	if account.Validate() != nil || before.Validate() != nil ||
		account.ID() != before.AccountID() || !sameTokenDigest(digest, before.TokenDigest()) {
		return wrapOperationError(ErrAuthenticationUnavailable, ErrStoredIdentityInvalid)
	}
	if account.Status() != identity.AccountStatusEnabled ||
		account.AuthenticationEpoch() != before.AuthenticationEpoch() {
		return wrapOperationError(ErrUnauthenticated, ErrSessionInactive)
	}
	active, activeErr := before.ActiveAt(now)
	if activeErr != nil {
		return wrapOperationError(ErrAuthenticationUnavailable, activeErr)
	}
	if !active {
		return wrapOperationError(ErrUnauthenticated, ErrSessionInactive)
	}
	operationRef, err := service.newRevokeOperationRef()
	if err != nil {
		return wrapOperationError(ErrAuthenticationUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return wrapOperationError(ErrOperationCanceled, err)
	}
	after, err := before.Revoke(now, identity.SessionRevokeReasonLogout, operationRef)
	if err != nil {
		return wrapOperationError(ErrAuthenticationUnavailable, err)
	}
	attempt, err := newSessionRevokeAttempt(account, before, after)
	if err != nil {
		return wrapOperationError(ErrAuthenticationUnavailable, err)
	}
	receipt, err := newRevokeCommitReceipt(before, after)
	if err != nil {
		return wrapOperationError(ErrAuthenticationUnavailable, err)
	}
	err = service.revoker.RevokeSession(ctx, attempt)
	if err == nil {
		if canceled := ctx.Err(); canceled != nil {
			return wrapOperationError(ErrOperationCanceled, canceled)
		}
		return nil
	}
	if canceled := canceledOperationError(ctx, err); canceled != nil {
		return canceled
	}
	if errors.Is(err, ErrCommitOutcomeUnknown) {
		return wrapOperationErrorWithReceipt(ErrRevocationIndeterminate, err, receipt)
	}
	if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrSessionInactive) ||
		errors.Is(err, ErrAccountStateConflict) {
		return wrapOperationError(ErrUnauthenticated, err)
	}
	return wrapOperationError(ErrAuthenticationUnavailable, err)
}

func (service *RevokeCurrentService) newRevokeOperationRef() (identity.OperationRef, error) {
	service.entropyMu.Lock()
	defer service.entropyMu.Unlock()

	value := make([]byte, OperationReferenceEntropyBytes)
	if _, err := io.ReadFull(service.entropy, value); err != nil || allZero(value) {
		clear(value)
		if err != nil {
			return "", err
		}
		return "", errors.New("entropy returned an all-zero operation reference")
	}
	reference, err := identity.NewOperationRef("revoke_" + hex.EncodeToString(value))
	clear(value)
	return reference, err
}
