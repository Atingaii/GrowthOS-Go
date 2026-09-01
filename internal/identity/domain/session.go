package domain

import (
	"fmt"
	"time"
)

// SessionRevokeReason is the closed reason vocabulary for a server-side
// session that may no longer establish caller identity.
type SessionRevokeReason string

const (
	SessionRevokeReasonLogout                     SessionRevokeReason = "logout"
	SessionRevokeReasonConcurrencyLimit           SessionRevokeReason = "concurrency_limit"
	SessionRevokeReasonAuthenticationEpochChanged SessionRevokeReason = "authentication_epoch_changed"
	SessionRevokeReasonAccountDisabled            SessionRevokeReason = "account_disabled"
	SessionRevokeReasonSecurityResponse           SessionRevokeReason = "security_response"
)

func (reason SessionRevokeReason) Valid() bool {
	switch reason {
	case SessionRevokeReasonLogout,
		SessionRevokeReasonConcurrencyLimit,
		SessionRevokeReasonAuthenticationEpochChanged,
		SessionRevokeReasonAccountDisabled,
		SessionRevokeReasonSecurityResponse:
		return true
	default:
		return false
	}
}

// Session is an immutable server-side identity snapshot. The bearer material
// is represented only by its nonzero fixed digest.
type Session struct {
	reference           SessionRef
	issueOperationRef   OperationRef
	accountID           AccountID
	tokenDigest         TokenDigest
	authenticationEpoch AuthenticationEpoch
	issuedAt            time.Time
	lastSeenAt          time.Time
	idleExpiresAt       time.Time
	absoluteExpiresAt   time.Time
	revokedAt           time.Time
	revokeReason        SessionRevokeReason
	revokeOperationRef  OperationRef
}

func NewSession(
	reference SessionRef,
	issueOperationRef OperationRef,
	accountID AccountID,
	tokenDigest TokenDigest,
	authenticationEpoch AuthenticationEpoch,
	issuedAt time.Time,
	idleExpiresAt time.Time,
	absoluteExpiresAt time.Time,
) (Session, error) {
	return RestoreSession(
		reference,
		issueOperationRef,
		accountID,
		tokenDigest,
		authenticationEpoch,
		issuedAt,
		issuedAt,
		idleExpiresAt,
		absoluteExpiresAt,
		time.Time{},
		"",
		"",
	)
}

func RestoreSession(
	reference SessionRef,
	issueOperationRef OperationRef,
	accountID AccountID,
	tokenDigest TokenDigest,
	authenticationEpoch AuthenticationEpoch,
	issuedAt time.Time,
	lastSeenAt time.Time,
	idleExpiresAt time.Time,
	absoluteExpiresAt time.Time,
	revokedAt time.Time,
	revokeReason SessionRevokeReason,
	revokeOperationRef OperationRef,
) (Session, error) {
	session := Session{
		reference:           reference,
		issueOperationRef:   issueOperationRef,
		accountID:           accountID,
		tokenDigest:         tokenDigest,
		authenticationEpoch: authenticationEpoch,
		issuedAt:            issuedAt,
		lastSeenAt:          lastSeenAt,
		idleExpiresAt:       idleExpiresAt,
		absoluteExpiresAt:   absoluteExpiresAt,
		revokedAt:           revokedAt,
		revokeReason:        revokeReason,
		revokeOperationRef:  revokeOperationRef,
	}
	if err := session.Validate(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (session Session) Validate() error {
	if err := session.reference.Validate(); err != nil {
		return fmt.Errorf("%w: reference: %w", ErrSessionInvalid, err)
	}
	if err := session.issueOperationRef.Validate(); err != nil {
		return fmt.Errorf("%w: issue operation reference: %w", ErrSessionInvalid, err)
	}
	if err := session.accountID.Validate(); err != nil {
		return fmt.Errorf("%w: account id: %w", ErrSessionInvalid, err)
	}
	if err := session.tokenDigest.Validate(); err != nil {
		return fmt.Errorf("%w: digest: %w", ErrSessionInvalid, err)
	}
	if err := session.authenticationEpoch.Validate(); err != nil {
		return fmt.Errorf("%w: authentication epoch: %w", ErrSessionInvalid, err)
	}
	for _, item := range []struct {
		label string
		value time.Time
	}{
		{label: "issued at", value: session.issuedAt},
		{label: "last seen at", value: session.lastSeenAt},
		{label: "idle expiry", value: session.idleExpiresAt},
		{label: "absolute expiry", value: session.absoluteExpiresAt},
	} {
		if err := validateCanonicalTime(item.label, item.value, ErrSessionTimeInvalid); err != nil {
			return fmt.Errorf("%w: %w", ErrSessionInvalid, err)
		}
	}
	if session.lastSeenAt.Before(session.issuedAt) {
		return fmt.Errorf("%w: %w: last seen precedes issue", ErrSessionInvalid, ErrSessionTimeInvalid)
	}
	if !session.lastSeenAt.Before(session.idleExpiresAt) {
		return fmt.Errorf("%w: %w: last seen must precede idle expiry", ErrSessionInvalid, ErrSessionTimeInvalid)
	}
	if session.absoluteExpiresAt.Before(session.idleExpiresAt) {
		return fmt.Errorf("%w: %w: idle expiry exceeds absolute expiry", ErrSessionInvalid, ErrSessionTimeInvalid)
	}

	if session.revokedAt.IsZero() {
		if session.revokeReason != "" || session.revokeOperationRef != "" {
			return fmt.Errorf("%w: %w: partial revocation", ErrSessionInvalid, ErrSessionRevocationInvalid)
		}
		return nil
	}
	if err := validateCanonicalTime("revoked at", session.revokedAt, ErrSessionRevocationInvalid); err != nil {
		return fmt.Errorf("%w: %w", ErrSessionInvalid, err)
	}
	if session.revokedAt.Before(session.issuedAt) {
		return fmt.Errorf("%w: %w: revocation precedes issue", ErrSessionInvalid, ErrSessionRevocationInvalid)
	}
	if session.revokedAt.Before(session.lastSeenAt) {
		return fmt.Errorf("%w: %w: revocation precedes last seen", ErrSessionInvalid, ErrSessionRevocationInvalid)
	}
	if !session.revokeReason.Valid() {
		return fmt.Errorf(
			"%w: %w: %q",
			ErrSessionInvalid,
			ErrSessionRevokeReasonUnsupported,
			session.revokeReason,
		)
	}
	if err := session.revokeOperationRef.Validate(); err != nil {
		return fmt.Errorf("%w: %w: %v", ErrSessionInvalid, ErrSessionRevocationInvalid, err)
	}
	return nil
}

func (session Session) Reference() SessionRef { return session.reference }

func (session Session) IssueOperationRef() OperationRef { return session.issueOperationRef }

func (session Session) AccountID() AccountID { return session.accountID }

func (session Session) TokenDigest() TokenDigest { return session.tokenDigest }

func (session Session) AuthenticationEpoch() AuthenticationEpoch {
	return session.authenticationEpoch
}

func (session Session) IssuedAt() time.Time { return session.issuedAt }

func (session Session) LastSeenAt() time.Time { return session.lastSeenAt }

func (session Session) IdleExpiresAt() time.Time { return session.idleExpiresAt }

func (session Session) AbsoluteExpiresAt() time.Time { return session.absoluteExpiresAt }

func (session Session) Revocation() (time.Time, SessionRevokeReason, OperationRef, bool) {
	if session.revokedAt.IsZero() {
		return time.Time{}, "", "", false
	}
	return session.revokedAt, session.revokeReason, session.revokeOperationRef, true
}

// ActiveAt evaluates the exact instant with closed expiry boundaries: now at
// or after either expiry is inactive. A time before issue is a technical error.
func (session Session) ActiveAt(now time.Time) (bool, error) {
	if err := session.Validate(); err != nil {
		return false, err
	}
	if err := validateCanonicalTime("evaluated at", now, ErrSessionEvaluationTimeInvalid); err != nil {
		return false, err
	}
	if now.Before(session.issuedAt) {
		return false, fmt.Errorf("%w: evaluated at precedes issue", ErrSessionEvaluationTimeInvalid)
	}
	if now.Before(session.lastSeenAt) {
		return false, fmt.Errorf("%w: evaluated at precedes last seen", ErrSessionEvaluationTimeInvalid)
	}
	if !session.revokedAt.IsZero() {
		return false, nil
	}
	if !now.Before(session.idleExpiresAt) || !now.Before(session.absoluteExpiresAt) {
		return false, nil
	}
	return true, nil
}

// Touch returns a new immutable activity snapshot. The requested lifetime is
// positive and microsecond-aligned; the idle deadline is extended, never
// shortened, and is clamped to the fixed absolute deadline. An expired or
// revoked session cannot be revived.
func (session Session) Touch(at time.Time, idleLifetime time.Duration) (Session, error) {
	if err := session.Validate(); err != nil {
		return Session{}, err
	}
	if idleLifetime <= 0 || idleLifetime%time.Microsecond != 0 {
		return Session{}, fmt.Errorf(
			"%w: idle lifetime must be positive and microsecond-aligned",
			ErrSessionTouchInvalid,
		)
	}
	active, err := session.ActiveAt(at)
	if err != nil {
		return Session{}, fmt.Errorf("%w: %w", ErrSessionTouchInvalid, err)
	}
	if !active {
		return Session{}, ErrSessionInactive
	}

	candidate := at.Add(idleLifetime)
	if err := validateCanonicalTime("candidate idle expiry", candidate, ErrSessionTouchInvalid); err != nil {
		return Session{}, err
	}
	if candidate.Before(at) {
		return Session{}, fmt.Errorf("%w: idle expiry overflow", ErrSessionTouchInvalid)
	}
	if session.absoluteExpiresAt.Before(candidate) {
		candidate = session.absoluteExpiresAt
	}
	if candidate.Before(session.idleExpiresAt) {
		candidate = session.idleExpiresAt
	}

	return RestoreSession(
		session.reference,
		session.issueOperationRef,
		session.accountID,
		session.tokenDigest,
		session.authenticationEpoch,
		session.issuedAt,
		at,
		candidate,
		session.absoluteExpiresAt,
		session.revokedAt,
		session.revokeReason,
		session.revokeOperationRef,
	)
}

// Revoke returns a new immutable snapshot and never mutates the receiver.
func (session Session) Revoke(
	at time.Time,
	reason SessionRevokeReason,
	operationRef OperationRef,
) (Session, error) {
	if err := session.Validate(); err != nil {
		return Session{}, err
	}
	if !session.revokedAt.IsZero() {
		return Session{}, ErrSessionAlreadyRevoked
	}
	if err := validateCanonicalTime("revoked at", at, ErrSessionRevocationInvalid); err != nil {
		return Session{}, err
	}
	if at.Before(session.issuedAt) {
		return Session{}, fmt.Errorf("%w: revocation precedes issue", ErrSessionRevocationInvalid)
	}
	if !reason.Valid() {
		return Session{}, fmt.Errorf("%w: %q", ErrSessionRevokeReasonUnsupported, reason)
	}
	if err := operationRef.Validate(); err != nil {
		return Session{}, fmt.Errorf("%w: %v", ErrSessionRevocationInvalid, err)
	}
	return RestoreSession(
		session.reference,
		session.issueOperationRef,
		session.accountID,
		session.tokenDigest,
		session.authenticationEpoch,
		session.issuedAt,
		session.lastSeenAt,
		session.idleExpiresAt,
		session.absoluteExpiresAt,
		at,
		reason,
		operationRef,
	)
}
