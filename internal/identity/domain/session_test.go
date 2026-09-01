package domain

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSessionExactActivityBoundaries(t *testing.T) {
	t.Parallel()

	issuedAt := canonicalTime(2026, 9, 1, 1, 0, 0, 123456000)
	idleExpiry := issuedAt.Add(15 * time.Minute)
	absoluteExpiry := issuedAt.Add(8 * time.Hour)
	session := mustSession(t, issuedAt, idleExpiry, absoluteExpiry)

	checks := []struct {
		name   string
		now    time.Time
		active bool
	}{
		{name: "at issue", now: issuedAt, active: true},
		{name: "before idle", now: idleExpiry.Add(-time.Microsecond), active: true},
		{name: "at idle", now: idleExpiry, active: false},
		{name: "after idle", now: idleExpiry.Add(time.Microsecond), active: false},
		{name: "at absolute", now: absoluteExpiry, active: false},
	}
	for _, check := range checks {
		active, err := session.ActiveAt(check.now)
		if err != nil || active != check.active {
			t.Fatalf("%s: active=%v err=%v, want %v", check.name, active, err, check.active)
		}
	}
	if active, err := session.ActiveAt(issuedAt.Add(-time.Microsecond)); active || !errors.Is(err, ErrSessionEvaluationTimeInvalid) {
		t.Fatalf("before issue = %v, %v", active, err)
	}
	if active, err := session.ActiveAt(time.Time{}); active ||
		!errors.Is(err, ErrSessionEvaluationTimeInvalid) {
		t.Fatalf("zero evaluation = %v, %v", active, err)
	}

	// Touch may clamp idle to absolute, so equality is a valid stored shape and
	// remains inactive at the exact shared deadline.
	equalExpiry := mustSession(t, issuedAt, absoluteExpiry, absoluteExpiry)
	if active, err := equalExpiry.ActiveAt(absoluteExpiry); err != nil || active {
		t.Fatalf("equal deadline activity = %v, %v", active, err)
	}
}

func TestSessionTouchIsImmutableMonotonicAndAbsoluteBounded(t *testing.T) {
	t.Parallel()

	issuedAt := canonicalTime(2026, 9, 1, 1, 0, 0, 0)
	original := mustSession(t, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(20*time.Minute))
	touchedAt := issuedAt.Add(10 * time.Minute)
	touched, err := original.Touch(touchedAt, 15*time.Minute)
	if err != nil {
		t.Fatalf("touch: %v", err)
	}
	if original.LastSeenAt() != issuedAt || original.IdleExpiresAt() != issuedAt.Add(15*time.Minute) {
		t.Fatal("touch mutated original")
	}
	if touched.LastSeenAt() != touchedAt || touched.IdleExpiresAt() != original.AbsoluteExpiresAt() {
		t.Fatalf("touch shape = last=%v idle=%v", touched.LastSeenAt(), touched.IdleExpiresAt())
	}
	if active, err := touched.ActiveAt(touched.AbsoluteExpiresAt().Add(-time.Microsecond)); err != nil || !active {
		t.Fatalf("before absolute = %v, %v", active, err)
	}
	if active, err := touched.ActiveAt(touched.AbsoluteExpiresAt()); err != nil || active {
		t.Fatalf("at absolute = %v, %v", active, err)
	}

	shortTouch, err := original.Touch(issuedAt.Add(time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("short touch: %v", err)
	}
	if shortTouch.IdleExpiresAt() != original.IdleExpiresAt() {
		t.Fatal("short touch shortened idle expiry")
	}

	for _, lifetime := range []time.Duration{0, -time.Microsecond, time.Nanosecond} {
		result, err := original.Touch(issuedAt.Add(time.Minute), lifetime)
		if !errors.Is(err, ErrSessionTouchInvalid) || result != (Session{}) {
			t.Fatalf("invalid lifetime %v = %#v, %v", lifetime, result, err)
		}
	}
	if result, err := original.Touch(original.IdleExpiresAt(), time.Minute); !errors.Is(err, ErrSessionInactive) || result != (Session{}) {
		t.Fatalf("expired touch = %#v, %v", result, err)
	}

	revoked, err := original.Revoke(
		issuedAt.Add(time.Minute),
		SessionRevokeReasonLogout,
		mustOperationRef(t, "operation-revoke-touch"),
	)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if result, err := revoked.Touch(issuedAt.Add(2*time.Minute), time.Minute); !errors.Is(err, ErrSessionInactive) || result != (Session{}) {
		t.Fatalf("revoked touch = %#v, %v", result, err)
	}
}

func TestSessionRevokeFormsAtomicOptionalUnion(t *testing.T) {
	t.Parallel()

	issuedAt := canonicalTime(2026, 9, 1, 1, 0, 0, 0)
	session := mustSession(t, issuedAt, issuedAt.Add(15*time.Minute), issuedAt.Add(8*time.Hour))
	revokedAt := issuedAt.Add(2 * time.Minute)
	revokeOperation := mustOperationRef(t, "operation-revoke-1")
	revoked, err := session.Revoke(revokedAt, SessionRevokeReasonLogout, revokeOperation)
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, _, exists := session.Revocation(); exists {
		t.Fatal("revoke mutated original session")
	}
	gotAt, gotReason, gotOperation, exists := revoked.Revocation()
	if !exists || gotAt != revokedAt || gotReason != SessionRevokeReasonLogout || gotOperation != revokeOperation {
		t.Fatalf("revocation = %v/%q/%q/%v", gotAt, gotReason, gotOperation, exists)
	}
	if active, err := revoked.ActiveAt(revokedAt); err != nil || active {
		t.Fatalf("revoked activity = %v, %v", active, err)
	}
	if second, err := revoked.Revoke(revokedAt, SessionRevokeReasonLogout, revokeOperation); !errors.Is(err, ErrSessionAlreadyRevoked) || second != (Session{}) {
		t.Fatalf("second revoke = %#v, %v", second, err)
	}
	if invalid, err := session.Revoke(issuedAt.Add(-time.Microsecond), SessionRevokeReasonLogout, revokeOperation); !errors.Is(err, ErrSessionRevocationInvalid) || invalid != (Session{}) {
		t.Fatalf("early revoke = %#v, %v", invalid, err)
	}
	if invalid, err := session.Revoke(revokedAt, "unknown", revokeOperation); !errors.Is(err, ErrSessionRevokeReasonUnsupported) || invalid != (Session{}) {
		t.Fatalf("unknown revoke = %#v, %v", invalid, err)
	}
	if invalid, err := session.Revoke(revokedAt, SessionRevokeReasonLogout, ""); !errors.Is(err, ErrSessionRevocationInvalid) || invalid != (Session{}) {
		t.Fatalf("missing operation = %#v, %v", invalid, err)
	}

	for _, partial := range []Session{
		withSessionMutation(session, func(value *Session) { value.revokeReason = SessionRevokeReasonLogout }),
		withSessionMutation(session, func(value *Session) { value.revokeOperationRef = revokeOperation }),
		withSessionMutation(session, func(value *Session) { value.revokedAt = revokedAt }),
		withSessionMutation(revoked, func(value *Session) { value.revokeReason = "" }),
		withSessionMutation(revoked, func(value *Session) { value.revokeOperationRef = "" }),
	} {
		if err := partial.Validate(); !errors.Is(err, ErrSessionInvalid) {
			t.Fatalf("partial revocation validated: %#v, %v", partial, err)
		}
	}
}

func TestSessionRestoreAndGettersAreExact(t *testing.T) {
	t.Parallel()

	issuedAt := canonicalTime(2026, 9, 1, 1, 2, 3, 456000000)
	session := mustSession(t, issuedAt, issuedAt.Add(time.Minute), issuedAt.Add(time.Hour))
	if session.Reference() != "session-1" || session.IssueOperationRef() != "operation-issue-1" ||
		session.AccountID() != "account-1" || session.AuthenticationEpoch() != 9 ||
		session.IssuedAt() != issuedAt || session.LastSeenAt() != issuedAt ||
		session.IdleExpiresAt() != issuedAt.Add(time.Minute) ||
		session.AbsoluteExpiresAt() != issuedAt.Add(time.Hour) {
		t.Fatalf("unexpected getters: %#v", session)
	}

	first := session.TokenDigest().Bytes()
	first[0] ^= 0xff
	if second := session.TokenDigest().Bytes(); second[0] != 1 {
		t.Fatalf("digest output mutation changed session: %x", second)
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("session validate: %v", err)
	}
}

func TestSessionRejectsInvalidShapeAndTime(t *testing.T) {
	t.Parallel()

	issuedAt := canonicalTime(2026, 9, 1, 1, 0, 0, 0)
	valid := mustSession(t, issuedAt, issuedAt.Add(time.Minute), issuedAt.Add(time.Hour))
	invalid := []Session{
		{},
		withSessionMutation(valid, func(session *Session) { session.reference = "Bad" }),
		withSessionMutation(valid, func(session *Session) { session.issueOperationRef = "" }),
		withSessionMutation(valid, func(session *Session) { session.accountID = "" }),
		withSessionMutation(valid, func(session *Session) { session.tokenDigest = TokenDigest{} }),
		withSessionMutation(valid, func(session *Session) { session.authenticationEpoch = 0 }),
		withSessionMutation(valid, func(session *Session) { session.issuedAt = time.Time{} }),
		withSessionMutation(valid, func(session *Session) { session.issuedAt = time.Now() }),
		withSessionMutation(valid, func(session *Session) {
			session.lastSeenAt = session.issuedAt.Add(-time.Microsecond)
		}),
		withSessionMutation(valid, func(session *Session) {
			session.lastSeenAt = session.idleExpiresAt
		}),
		withSessionMutation(valid, func(session *Session) {
			session.idleExpiresAt = session.absoluteExpiresAt.Add(time.Microsecond)
		}),
	}
	for _, session := range invalid {
		if err := session.Validate(); !errors.Is(err, ErrSessionInvalid) {
			t.Fatalf("validate invalid session %#v: %v", session, err)
		}
	}

	constructed, err := NewSession(
		valid.reference,
		valid.issueOperationRef,
		valid.accountID,
		valid.tokenDigest,
		valid.authenticationEpoch,
		issuedAt,
		issuedAt,
		issuedAt.Add(time.Hour),
	)
	if !errors.Is(err, ErrSessionTimeInvalid) || constructed != (Session{}) {
		t.Fatalf("invalid construction = %#v, %v", constructed, err)
	}

	restored, err := RestoreSession(
		valid.reference,
		valid.issueOperationRef,
		valid.accountID,
		valid.tokenDigest,
		valid.authenticationEpoch,
		issuedAt,
		issuedAt.Add(2*time.Minute),
		issuedAt.Add(time.Minute),
		issuedAt.Add(time.Hour),
		time.Time{},
		"",
		"",
	)
	if !errors.Is(err, ErrSessionTimeInvalid) || restored != (Session{}) {
		t.Fatalf("invalid restore = %#v, %v", restored, err)
	}
}

func TestTokenDigestFixedNonzeroDefensiveAndRedacted(t *testing.T) {
	t.Parallel()

	input := digestBytes(1)
	digest, err := NewTokenDigest(input)
	if err != nil {
		t.Fatalf("new digest: %v", err)
	}
	input[0] = 9
	if got := digest.Bytes(); got[0] != 1 {
		t.Fatalf("input mutation changed digest: %x", got)
	}
	got := digest.Bytes()
	got[0] = 8
	if second := digest.Bytes(); second[0] != 1 {
		t.Fatalf("output mutation changed digest: %x", second)
	}

	secret := bytes.Repeat([]byte{'s'}, DigestBytes)
	secretDigest, err := NewTokenDigest(secret)
	if err != nil {
		t.Fatalf("new secret digest: %v", err)
	}
	for _, formatted := range []string{
		fmt.Sprint(secretDigest),
		fmt.Sprintf("%v", secretDigest),
		fmt.Sprintf("%#v", secretDigest),
	} {
		if strings.Contains(formatted, string(secret)) || !strings.Contains(formatted, redactedValue) {
			t.Fatalf("unsafe digest formatting %q", formatted)
		}
	}
	if formatted := fmt.Sprintf("%x", secretDigest); formatted != fmt.Sprintf("%x", redactedValue) {
		t.Fatalf("hex formatting did not redact: %q", formatted)
	}
	var output bytes.Buffer
	slog.New(slog.NewTextHandler(&output, nil)).Info("session", "digest", secretDigest)
	if logged := output.String(); strings.Contains(logged, string(secret)) || !strings.Contains(logged, redactedValue) {
		t.Fatalf("unsafe digest log %q", logged)
	}

	for _, value := range [][]byte{nil, {}, make([]byte, DigestBytes-1), make([]byte, DigestBytes), make([]byte, DigestBytes+1)} {
		invalid, err := NewTokenDigest(value)
		if !errors.Is(err, ErrTokenDigestInvalid) || invalid != (TokenDigest{}) {
			t.Fatalf("invalid digest len=%d = %#v, %v", len(value), invalid, err)
		}
	}
}

func TestSessionRevokeReasonVocabularyIsClosed(t *testing.T) {
	t.Parallel()

	for _, reason := range []SessionRevokeReason{
		SessionRevokeReasonLogout,
		SessionRevokeReasonConcurrencyLimit,
		SessionRevokeReasonAuthenticationEpochChanged,
		SessionRevokeReasonAccountDisabled,
		SessionRevokeReasonSecurityResponse,
	} {
		if !reason.Valid() {
			t.Fatalf("valid reason %q rejected", reason)
		}
	}
	if SessionRevokeReason("").Valid() || SessionRevokeReason("other").Valid() {
		t.Fatal("unknown revoke reason became valid")
	}
}

func FuzzTokenDigestRequiresExactNonzeroBytes(f *testing.F) {
	f.Add([]byte(nil))
	f.Add(make([]byte, DigestBytes))
	f.Add(digestBytes(1))
	f.Fuzz(func(t *testing.T, value []byte) {
		digest, err := NewTokenDigest(value)
		wantValid := len(value) == DigestBytes && !allZero(value)
		if (err == nil) != wantValid {
			t.Fatalf("len=%d valid=%v err=%v", len(value), wantValid, err)
		}
		if err == nil {
			got := digest.Bytes()
			if len(got) != DigestBytes {
				t.Fatalf("digest length = %d", len(got))
			}
			for index := range got {
				if got[index] != value[index] {
					t.Fatalf("digest changed byte %d", index)
				}
			}
		}
	})
}

func withSessionMutation(session Session, mutate func(*Session)) Session {
	mutate(&session)
	return session
}
