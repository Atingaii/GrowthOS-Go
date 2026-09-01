package application

import (
	"encoding/json"
	"log/slog"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

// SessionCommitOperation is the closed security-state write vocabulary.
type SessionCommitOperation string

const (
	SessionCommitOperationIssue  SessionCommitOperation = "issue"
	SessionCommitOperationRevoke SessionCommitOperation = "revoke"
)

// SessionCommitReceipt contains no raw bearer token. A committed issue receipt
// is diagnostic evidence for an unusable orphan; it cannot recover or reissue
// the token whose response was rejected.
type SessionCommitReceipt struct {
	operation SessionCommitOperation
	before    identity.Session
	after     identity.Session
	hasBefore bool
}

const redactedSessionCommitReceipt = "identity session commit receipt (redacted)"

func (SessionCommitReceipt) String() string   { return redactedSessionCommitReceipt }
func (SessionCommitReceipt) GoString() string { return redactedSessionCommitReceipt }
func (SessionCommitReceipt) LogValue() slog.Value {
	return slog.StringValue(redactedSessionCommitReceipt)
}
func (SessionCommitReceipt) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedSessionCommitReceipt)
}

func newIssueCommitReceipt(after identity.Session) (SessionCommitReceipt, error) {
	receipt := SessionCommitReceipt{
		operation: SessionCommitOperationIssue,
		after:     after,
	}
	if receipt.Validate() != nil {
		return SessionCommitReceipt{}, ErrAuthenticationUnavailable
	}
	return receipt, nil
}

func newRevokeCommitReceipt(
	before identity.Session,
	after identity.Session,
) (SessionCommitReceipt, error) {
	receipt := SessionCommitReceipt{
		operation: SessionCommitOperationRevoke,
		before:    before,
		after:     after,
		hasBefore: true,
	}
	if receipt.Validate() != nil {
		return SessionCommitReceipt{}, ErrAuthenticationUnavailable
	}
	return receipt, nil
}

func (receipt SessionCommitReceipt) Operation() SessionCommitOperation {
	return receipt.operation
}

func (receipt SessionCommitReceipt) Before() (identity.Session, bool) {
	if !receipt.hasBefore {
		return identity.Session{}, false
	}
	return receipt.before, true
}

func (receipt SessionCommitReceipt) After() identity.Session { return receipt.after }

func (receipt SessionCommitReceipt) Validate() error {
	if receipt.after.Validate() != nil {
		return ErrInvalidArgument
	}
	switch receipt.operation {
	case SessionCommitOperationIssue:
		if receipt.hasBefore || receipt.before != (identity.Session{}) {
			return ErrInvalidArgument
		}
		if receipt.after.LastSeenAt() != receipt.after.IssuedAt() ||
			receipt.after.IdleExpiresAt() != receipt.after.IssuedAt().Add(SessionIdleLifetime) ||
			receipt.after.AbsoluteExpiresAt() != receipt.after.IssuedAt().Add(SessionAbsoluteLifetime) {
			return ErrInvalidArgument
		}
		if _, _, _, revoked := receipt.after.Revocation(); revoked {
			return ErrInvalidArgument
		}
		return nil
	case SessionCommitOperationRevoke:
		if !receipt.hasBefore || receipt.before.Validate() != nil ||
			!sameSessionIdentity(receipt.before, receipt.after) ||
			receipt.before.IdleExpiresAt() != receipt.after.IdleExpiresAt() {
			return ErrInvalidArgument
		}
		if _, _, _, revoked := receipt.before.Revocation(); revoked {
			return ErrInvalidArgument
		}
		_, reason, operationRef, revoked := receipt.after.Revocation()
		if !revoked || reason != identity.SessionRevokeReasonLogout ||
			operationRef.Validate() != nil {
			return ErrInvalidArgument
		}
		return nil
	default:
		return ErrInvalidArgument
	}
}

// SessionCommitObservation is an exact authoritative read-back or confirmed
// absence. Its fields are private to prevent a transport from selecting a mode
// independently.
type SessionCommitObservation struct {
	session identity.Session
	found   bool
	valid   bool
}

const redactedSessionCommitObservation = "identity session commit observation (redacted)"

func (SessionCommitObservation) String() string   { return redactedSessionCommitObservation }
func (SessionCommitObservation) GoString() string { return redactedSessionCommitObservation }
func (SessionCommitObservation) LogValue() slog.Value {
	return slog.StringValue(redactedSessionCommitObservation)
}
func (SessionCommitObservation) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedSessionCommitObservation)
}

func ObserveSessionCommitState(session identity.Session) SessionCommitObservation {
	return SessionCommitObservation{session: session, found: true, valid: session.Validate() == nil}
}

func ObserveSessionCommitAbsence() SessionCommitObservation {
	return SessionCommitObservation{valid: true}
}

type SessionCommitReconciliation string

const (
	SessionCommitReconciliationCommitted     SessionCommitReconciliation = "committed"
	SessionCommitReconciliationNotCommitted  SessionCommitReconciliation = "not_committed"
	SessionCommitReconciliationIndeterminate SessionCommitReconciliation = "indeterminate"
)

// ReconcileSessionCommit is pure and never recommends replay or token
// delivery. Invalid/mismatched observations fail closed as indeterminate.
func ReconcileSessionCommit(
	receipt SessionCommitReceipt,
	observation SessionCommitObservation,
) SessionCommitReconciliation {
	if receipt.Validate() != nil || !observation.valid ||
		(observation.found && observation.session.Validate() != nil) {
		return SessionCommitReconciliationIndeterminate
	}
	switch receipt.operation {
	case SessionCommitOperationIssue:
		if !observation.found {
			return SessionCommitReconciliationNotCommitted
		}
		if sameSession(observation.session, receipt.after) {
			return SessionCommitReconciliationCommitted
		}
		return SessionCommitReconciliationIndeterminate
	case SessionCommitOperationRevoke:
		if !observation.found {
			return SessionCommitReconciliationIndeterminate
		}
		if sameSession(observation.session, receipt.after) {
			return SessionCommitReconciliationCommitted
		}
		if sameSession(observation.session, receipt.before) {
			return SessionCommitReconciliationNotCommitted
		}
		return SessionCommitReconciliationIndeterminate
	default:
		return SessionCommitReconciliationIndeterminate
	}
}

func sameSessionIdentity(left, right identity.Session) bool {
	return left.Reference() == right.Reference() &&
		left.IssueOperationRef() == right.IssueOperationRef() &&
		left.AccountID() == right.AccountID() &&
		sameTokenDigest(left.TokenDigest(), right.TokenDigest()) &&
		left.AuthenticationEpoch() == right.AuthenticationEpoch() &&
		left.IssuedAt() == right.IssuedAt() &&
		left.LastSeenAt() == right.LastSeenAt() &&
		left.AbsoluteExpiresAt() == right.AbsoluteExpiresAt()
}

func sameSession(left, right identity.Session) bool {
	if !sameSessionIdentity(left, right) ||
		left.IdleExpiresAt() != right.IdleExpiresAt() {
		return false
	}
	leftAt, leftReason, leftOperation, leftRevoked := left.Revocation()
	rightAt, rightReason, rightOperation, rightRevoked := right.Revocation()
	return leftRevoked == rightRevoked && leftAt == rightAt && leftReason == rightReason &&
		leftOperation == rightOperation
}
