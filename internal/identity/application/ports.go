package application

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

// CredentialReader resolves one canonical local login name. A missing account
// is reported as ErrAccountNotFound; corrupt storage never masquerades as
// missing.
type CredentialReader interface {
	FindByLogin(context.Context, identity.LoginName) (identity.WorkforceAccount, error)
}

// PasswordVerifier performs the bounded Argon2id work. Unknown accounts use a
// separately configured, valid dummy envelope through VerifyUnknownLogin.
type PasswordVerifier interface {
	VerifyLogin(
		ctx context.Context,
		password []byte,
		encodedEnvelope string,
	) (PasswordVerification, error)
	VerifyUnknownLogin(ctx context.Context, password []byte) error
}

// EntropyReader matches crypto/rand.Read-compatible sources while remaining
// deterministic in contract tests. Services serialize their own reads; an
// implementation shared across distinct service instances must additionally
// be safe for concurrent use.
type EntropyReader interface {
	Read([]byte) (int, error)
}

// AdmissionController owns the two-row persistent login/source reservation.
// Begin must not retain a transaction or connection after it returns. For each
// dimension it admits normally while failure+inflight is below the threshold.
// At/above threshold it rejects during active backoff; once backoff expires it
// may grant exactly one probe only when inflight is zero. An expired inflight
// batch is fenced by incrementing the domain AdmissionEpoch before reuse.
type AdmissionController interface {
	BeginAdmission(
		context.Context,
		AdmissionRequest,
	) (AdmissionGrant, error)
	FinalizeAdmission(
		context.Context,
		AdmissionReceipt,
		AdmissionFinalOutcome,
	) error
}

// SessionIssuer atomically rechecks the account snapshot, enforces the active
// session cap, optionally revokes an incoming-session hint, and inserts the new
// session. It must classify a COMMIT acknowledgement loss distinctly.
type SessionIssuer interface {
	IssueSession(context.Context, SessionIssueAttempt) error
}

// SessionResolver returns one strictly restored account/session pair and
// performs the authoritative 60-second-window touch when required. Any touch
// failure returns zero values and an error.
type SessionResolver interface {
	ResolveAndTouch(
		ctx context.Context,
		digest identity.TokenDigest,
		now time.Time,
		idleLifetime time.Duration,
		touchWindow time.Duration,
	) (identity.WorkforceAccount, identity.Session, error)
}

// SessionRevocationReader reads the exact account/session state used to plan a
// current-session revocation. It never touches or silently substitutes rows.
type SessionRevocationReader interface {
	FindForRevocation(
		context.Context,
		identity.TokenDigest,
	) (identity.WorkforceAccount, identity.Session, error)
}

// SessionRevoker conditionally writes one exact immutable revoke transition.
type SessionRevoker interface {
	RevokeSession(context.Context, SessionRevokeAttempt) error
}

// SessionCommitObserver performs a bounded authoritative read used only after
// an explicit commit-outcome receipt. It cannot recover a bearer token.
type SessionCommitObserver interface {
	ObserveSessionCommit(
		context.Context,
		SessionCommitReceipt,
	) (SessionCommitObservation, error)
}

// PasswordVerification is a small validated result. A rehash recommendation
// is meaningful only after a match and is never part of an HTTP response.
type PasswordVerification struct {
	matched     bool
	needsRehash bool
}

// NewPasswordVerification constructs the result returned by a trusted hash
// adapter.
func NewPasswordVerification(matched, needsRehash bool) (PasswordVerification, error) {
	if needsRehash && !matched {
		return PasswordVerification{}, ErrInvalidArgument
	}
	return PasswordVerification{matched: matched, needsRehash: needsRehash}, nil
}

// Matched reports whether the supplied credential matched.
func (verification PasswordVerification) Matched() bool { return verification.matched }

// NeedsRehash reports an internal profile-upgrade signal.
func (verification PasswordVerification) NeedsRehash() bool {
	return verification.needsRehash
}

// Validate rejects partial or forged combinations.
func (verification PasswordVerification) Validate() error {
	if verification.needsRehash && !verification.matched {
		return ErrInvalidArgument
	}
	return nil
}

// SessionIssueAttempt contains the exact candidate and account snapshot the
// repository must recheck under the account row lock.
type SessionIssueAttempt struct {
	account              identity.WorkforceAccount
	session              identity.Session
	previousTokenDigest  identity.TokenDigest
	hasPreviousTokenHint bool
}

const redactedSessionIssueAttempt = "identity session issue attempt (redacted)"

func (SessionIssueAttempt) String() string   { return redactedSessionIssueAttempt }
func (SessionIssueAttempt) GoString() string { return redactedSessionIssueAttempt }
func (SessionIssueAttempt) LogValue() slog.Value {
	return slog.StringValue(redactedSessionIssueAttempt)
}
func (SessionIssueAttempt) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedSessionIssueAttempt)
}

func newSessionIssueAttempt(
	account identity.WorkforceAccount,
	session identity.Session,
	previousTokenDigest identity.TokenDigest,
	hasPreviousTokenHint bool,
) (SessionIssueAttempt, error) {
	attempt := SessionIssueAttempt{
		account:              account,
		session:              session,
		previousTokenDigest:  previousTokenDigest,
		hasPreviousTokenHint: hasPreviousTokenHint,
	}
	if attempt.Validate() != nil {
		return SessionIssueAttempt{}, ErrInvalidArgument
	}
	return attempt, nil
}

func (attempt SessionIssueAttempt) Validate() error {
	if attempt.account.Validate() != nil || attempt.session.Validate() != nil ||
		attempt.account.Status() != identity.AccountStatusEnabled ||
		attempt.account.ID() != attempt.session.AccountID() ||
		attempt.account.AuthenticationEpoch() != attempt.session.AuthenticationEpoch() ||
		attempt.session.LastSeenAt() != attempt.session.IssuedAt() ||
		attempt.session.IdleExpiresAt() != attempt.session.IssuedAt().Add(SessionIdleLifetime) ||
		attempt.session.AbsoluteExpiresAt() != attempt.session.IssuedAt().Add(SessionAbsoluteLifetime) {
		return ErrInvalidArgument
	}
	if _, _, _, revoked := attempt.session.Revocation(); revoked {
		return ErrInvalidArgument
	}
	if attempt.hasPreviousTokenHint {
		if attempt.previousTokenDigest.Validate() != nil {
			return ErrInvalidArgument
		}
	} else if attempt.previousTokenDigest.Validate() == nil {
		return ErrInvalidArgument
	}
	return nil
}

func (attempt SessionIssueAttempt) Account() identity.WorkforceAccount {
	return attempt.account
}

func (attempt SessionIssueAttempt) Session() identity.Session { return attempt.session }

func (attempt SessionIssueAttempt) PreviousTokenDigest() (identity.TokenDigest, bool) {
	if !attempt.hasPreviousTokenHint {
		return identity.TokenDigest{}, false
	}
	return attempt.previousTokenDigest, true
}

// SessionRevokeAttempt is one exact before/after transition.
type SessionRevokeAttempt struct {
	account identity.WorkforceAccount
	before  identity.Session
	after   identity.Session
}

const redactedSessionRevokeAttempt = "identity session revoke attempt (redacted)"

func (SessionRevokeAttempt) String() string   { return redactedSessionRevokeAttempt }
func (SessionRevokeAttempt) GoString() string { return redactedSessionRevokeAttempt }
func (SessionRevokeAttempt) LogValue() slog.Value {
	return slog.StringValue(redactedSessionRevokeAttempt)
}
func (SessionRevokeAttempt) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedSessionRevokeAttempt)
}

func newSessionRevokeAttempt(
	account identity.WorkforceAccount,
	before identity.Session,
	after identity.Session,
) (SessionRevokeAttempt, error) {
	attempt := SessionRevokeAttempt{account: account, before: before, after: after}
	if attempt.Validate() != nil {
		return SessionRevokeAttempt{}, ErrInvalidArgument
	}
	return attempt, nil
}

func (attempt SessionRevokeAttempt) Validate() error {
	if attempt.account.Validate() != nil || attempt.before.Validate() != nil ||
		attempt.after.Validate() != nil ||
		attempt.account.Status() != identity.AccountStatusEnabled ||
		attempt.account.ID() != attempt.before.AccountID() ||
		attempt.account.ID() != attempt.after.AccountID() ||
		attempt.account.AuthenticationEpoch() != attempt.before.AuthenticationEpoch() ||
		attempt.account.AuthenticationEpoch() != attempt.after.AuthenticationEpoch() ||
		!sameSessionIdentity(attempt.before, attempt.after) ||
		attempt.before.IdleExpiresAt() != attempt.after.IdleExpiresAt() {
		return ErrInvalidArgument
	}
	beforeAt, beforeReason, beforeOperation, beforeRevoked := attempt.before.Revocation()
	afterAt, afterReason, afterOperation, afterRevoked := attempt.after.Revocation()
	if beforeRevoked || !beforeAt.IsZero() || beforeReason != "" || !afterRevoked ||
		beforeOperation != "" || afterAt.IsZero() ||
		afterReason != identity.SessionRevokeReasonLogout || afterOperation.Validate() != nil {
		return ErrInvalidArgument
	}
	return nil
}

func (attempt SessionRevokeAttempt) Account() identity.WorkforceAccount {
	return attempt.account
}

func (attempt SessionRevokeAttempt) Before() identity.Session { return attempt.before }

func (attempt SessionRevokeAttempt) After() identity.Session { return attempt.after }

func sameTokenDigest(left, right identity.TokenDigest) bool {
	leftBytes := left.Bytes()
	rightBytes := right.Bytes()
	if len(leftBytes) != len(rightBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}
