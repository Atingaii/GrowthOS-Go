package application

import (
	"encoding/json"
	"log/slog"
	"time"
	"unicode/utf8"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

const (
	SessionTokenBytes              = 32
	SessionReferenceEntropyBytes   = 16
	OperationReferenceEntropyBytes = 16
	MaximumIssueAttempts           = 3
	SessionIdleLifetime            = 15 * time.Minute
	SessionAbsoluteLifetime        = 8 * time.Hour
	SessionTouchWindow             = time.Minute
	MaximumAdmissionLease          = 3 * time.Second
	AdmissionFinalizeTimeout       = 250 * time.Millisecond
)

const (
	redactedVerifiedSession = "identity verified session (redacted)"
	redactedIssuedSession   = "identity issued session (redacted)"
	redactedLoginCommand    = "identity login command (redacted)"
)

// VerifiedSession is a trusted application output. Its private fields ensure a
// transport cannot assemble one from caller-controlled principal/session data.
type VerifiedSession struct {
	principalID       identity.PrincipalID
	sessionReference  identity.SessionRef
	authenticatedAt   time.Time
	idleExpiresAt     time.Time
	absoluteExpiresAt time.Time
}

func newVerifiedSession(
	principalID identity.PrincipalID,
	session identity.Session,
) (VerifiedSession, error) {
	verified := VerifiedSession{
		principalID:       principalID,
		sessionReference:  session.Reference(),
		authenticatedAt:   session.IssuedAt(),
		idleExpiresAt:     session.IdleExpiresAt(),
		absoluteExpiresAt: session.AbsoluteExpiresAt(),
	}
	if verified.Validate() != nil {
		return VerifiedSession{}, ErrAuthenticationUnavailable
	}
	return verified, nil
}

func (verified VerifiedSession) Validate() error {
	if verified.principalID.Validate() != nil ||
		verified.sessionReference.Validate() != nil ||
		verified.authenticatedAt.IsZero() ||
		verified.idleExpiresAt.IsZero() ||
		verified.absoluteExpiresAt.IsZero() ||
		verified.authenticatedAt != canonicalInstant(verified.authenticatedAt) ||
		verified.idleExpiresAt != canonicalInstant(verified.idleExpiresAt) ||
		verified.absoluteExpiresAt != canonicalInstant(verified.absoluteExpiresAt) ||
		!verified.authenticatedAt.Before(verified.idleExpiresAt) ||
		verified.absoluteExpiresAt.Before(verified.idleExpiresAt) {
		return ErrAuthenticationUnavailable
	}
	return nil
}

// PrincipalID is the authentication-layer subject identifier. Converting it
// into a Governance Principal belongs to the later authorization integration
// chapter, so real-session authentication does not import the policy kernel.
func (verified VerifiedSession) PrincipalID() identity.PrincipalID { return verified.principalID }

func (verified VerifiedSession) SessionReference() identity.SessionRef {
	return verified.sessionReference
}

func (verified VerifiedSession) AuthenticatedAt() time.Time {
	return verified.authenticatedAt
}

func (verified VerifiedSession) IdleExpiresAt() time.Time { return verified.idleExpiresAt }

func (verified VerifiedSession) AbsoluteExpiresAt() time.Time {
	return verified.absoluteExpiresAt
}

func (VerifiedSession) String() string   { return redactedVerifiedSession }
func (VerifiedSession) GoString() string { return redactedVerifiedSession }
func (VerifiedSession) LogValue() slog.Value {
	return slog.StringValue(redactedVerifiedSession)
}
func (VerifiedSession) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedVerifiedSession)
}

// IssuedSession adds the one raw token that may be copied into Set-Cookie only
// after a confirmed issue COMMIT.
type IssuedSession struct {
	verified VerifiedSession
	rawToken [SessionTokenBytes]byte
}

func newIssuedSession(
	verified VerifiedSession,
	rawToken []byte,
) (IssuedSession, error) {
	if verified.Validate() != nil || len(rawToken) != SessionTokenBytes || allZero(rawToken) {
		return IssuedSession{}, ErrAuthenticationUnavailable
	}
	issued := IssuedSession{verified: verified}
	copy(issued.rawToken[:], rawToken)
	return issued, nil
}

func (issued IssuedSession) Validate() error {
	if issued.verified.Validate() != nil || allZero(issued.rawToken[:]) {
		return ErrAuthenticationUnavailable
	}
	return nil
}

func (issued IssuedSession) VerifiedSession() VerifiedSession { return issued.verified }

// RawToken returns a defensive copy. Callers must clear it after constructing
// the Cookie value and must never log or serialize it.
func (issued IssuedSession) RawToken() []byte {
	cloned := make([]byte, len(issued.rawToken))
	copy(cloned, issued.rawToken[:])
	return cloned
}

func (IssuedSession) String() string   { return redactedIssuedSession }
func (IssuedSession) GoString() string { return redactedIssuedSession }
func (IssuedSession) LogValue() slog.Value {
	return slog.StringValue(redactedIssuedSession)
}
func (IssuedSession) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedIssuedSession)
}

// LoginCommand is strict, single-use input assembled by the HTTP adapter after
// request framing, Origin, and source-trust validation. Login consumes and
// clears its private password and prior-token buffers on every return path.
type LoginCommand struct {
	loginName     identity.LoginName
	password      []byte
	loginDigest   identity.ThrottleDigest
	sourceDigest  identity.ThrottleDigest
	previousToken []byte
}

// NewLoginCommand validates the password without trimming, case-folding,
// normalizing, or truncating it. A previous token is an optional revoke hint;
// it is never a candidate for the newly issued token.
func NewLoginCommand(
	loginName identity.LoginName,
	password []byte,
	loginDigest identity.ThrottleDigest,
	sourceDigest identity.ThrottleDigest,
	previousToken []byte,
) (LoginCommand, error) {
	command := LoginCommand{
		loginName:     loginName,
		password:      cloneSecret(password),
		loginDigest:   loginDigest,
		sourceDigest:  sourceDigest,
		previousToken: cloneSecret(previousToken),
	}
	if command.Validate() != nil {
		zeroSecret(command.password)
		zeroSecret(command.previousToken)
		return LoginCommand{}, ErrInvalidArgument
	}
	return command, nil
}

func (command LoginCommand) Validate() error {
	if command.loginName.Validate() != nil || command.loginDigest.Validate() != nil ||
		command.sourceDigest.Validate() != nil || !validLoginPassword(command.password) ||
		(len(command.previousToken) != 0 &&
			(len(command.previousToken) != SessionTokenBytes || allZero(command.previousToken))) {
		return ErrInvalidArgument
	}
	return nil
}

func (command LoginCommand) LoginName() identity.LoginName { return command.loginName }

// Password returns a defensive copy for the one configured verifier call.
func (command LoginCommand) Password() []byte { return cloneSecret(command.password) }

func (LoginCommand) String() string   { return redactedLoginCommand }
func (LoginCommand) GoString() string { return redactedLoginCommand }
func (LoginCommand) LogValue() slog.Value {
	return slog.StringValue(redactedLoginCommand)
}
func (LoginCommand) MarshalJSON() ([]byte, error) { return json.Marshal(redactedLoginCommand) }

func validLoginPassword(password []byte) bool {
	if len(password) == 0 || len(password) > 512 || !utf8.Valid(password) {
		return false
	}
	runeCount := utf8.RuneCount(password)
	return runeCount >= 1 && runeCount <= 128
}

func cloneSecret(value []byte) []byte {
	if value == nil {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}

func zeroSecret(value []byte) { clear(value) }

func allZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}

func principalIDFromAccount(account identity.WorkforceAccount) (identity.PrincipalID, error) {
	if account.Validate() != nil || account.Status() != identity.AccountStatusEnabled {
		return "", ErrAuthenticationUnavailable
	}
	principalID := account.PrincipalID()
	if principalID.Validate() != nil {
		return "", ErrAuthenticationUnavailable
	}
	return principalID, nil
}
