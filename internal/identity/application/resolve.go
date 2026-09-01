package application

import (
	"context"
	"crypto/sha256"
	"errors"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

type ResolveService struct {
	clock    Clock
	resolver SessionResolver
}

func NewResolveService(clock Clock, resolver SessionResolver) (*ResolveService, error) {
	service := &ResolveService{clock: clock, resolver: resolver}
	if service.Validate() != nil {
		return nil, ErrNotConfigured
	}
	return service, nil
}

func (service *ResolveService) Validate() error {
	if service == nil || dependencyIsNil(service.clock) || dependencyIsNil(service.resolver) {
		return ErrNotConfigured
	}
	return nil
}

func (service *ResolveService) Resolve(
	ctx context.Context,
	rawToken []byte,
) (VerifiedSession, error) {
	if service.Validate() != nil {
		return VerifiedSession{}, wrapOperationError(ErrNotConfigured, ErrNotConfigured)
	}
	if ctx == nil {
		return VerifiedSession{}, wrapOperationError(ErrInvalidArgument, ErrInvalidArgument)
	}
	if len(rawToken) != SessionTokenBytes || allZero(rawToken) {
		return VerifiedSession{}, wrapOperationError(ErrUnauthenticated, ErrUnauthenticated)
	}
	if err := ctx.Err(); err != nil {
		return VerifiedSession{}, wrapOperationError(ErrOperationCanceled, err)
	}
	now := canonicalInstant(service.clock.Now())
	if now.IsZero() {
		return VerifiedSession{}, wrapOperationError(ErrAuthenticationUnavailable, errors.New("clock returned zero"))
	}
	digestBytes := sha256.Sum256(rawToken)
	digest, err := identity.NewTokenDigest(digestBytes[:])
	if err != nil {
		return VerifiedSession{}, wrapOperationError(ErrUnauthenticated, err)
	}
	account, session, err := service.resolver.ResolveAndTouch(
		ctx,
		digest,
		now,
		SessionIdleLifetime,
		SessionTouchWindow,
	)
	if err != nil {
		return VerifiedSession{}, classifyResolveError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return VerifiedSession{}, wrapOperationError(ErrOperationCanceled, err)
	}
	if account.Validate() != nil || session.Validate() != nil ||
		account.ID() != session.AccountID() || !sameTokenDigest(digest, session.TokenDigest()) {
		return VerifiedSession{}, wrapOperationError(ErrAuthenticationUnavailable, ErrStoredIdentityInvalid)
	}
	if account.Status() != identity.AccountStatusEnabled ||
		account.AuthenticationEpoch() != session.AuthenticationEpoch() {
		return VerifiedSession{}, wrapOperationError(ErrUnauthenticated, ErrSessionInactive)
	}
	active, activeErr := session.ActiveAt(now)
	if activeErr != nil {
		return VerifiedSession{}, wrapOperationError(ErrAuthenticationUnavailable, activeErr)
	}
	if !active {
		return VerifiedSession{}, wrapOperationError(ErrUnauthenticated, ErrSessionInactive)
	}
	principal, err := principalFromAccount(account)
	if err != nil {
		return VerifiedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
	}
	verified, err := newVerifiedSession(principal, session)
	if err != nil {
		return VerifiedSession{}, wrapOperationError(ErrAuthenticationUnavailable, err)
	}
	return verified, nil
}

func classifyResolveError(ctx context.Context, err error) error {
	if canceled := canceledOperationError(ctx, err); canceled != nil {
		return canceled
	}
	if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrSessionInactive) {
		return wrapOperationError(ErrUnauthenticated, err)
	}
	return wrapOperationError(ErrAuthenticationUnavailable, err)
}
