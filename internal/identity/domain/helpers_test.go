package domain

import (
	"testing"
	"time"
)

func mustAccountID(t *testing.T, value string) AccountID {
	t.Helper()
	identifier, err := NewAccountID(value)
	if err != nil {
		t.Fatalf("new account id %q: %v", value, err)
	}
	return identifier
}

func mustLoginName(t *testing.T, value string) LoginName {
	t.Helper()
	name, err := NewLoginName(value)
	if err != nil {
		t.Fatalf("new login name %q: %v", value, err)
	}
	return name
}

func mustIdentityPrincipalID(t *testing.T, value string) PrincipalID {
	t.Helper()
	identifier, err := NewPrincipalID(value)
	if err != nil {
		t.Fatalf("new principal id %q: %v", value, err)
	}
	return identifier
}

func mustCredentialVersion(t *testing.T, value uint64) CredentialVersion {
	t.Helper()
	version, err := NewCredentialVersion(value)
	if err != nil {
		t.Fatalf("new credential version %d: %v", value, err)
	}
	return version
}

func mustAuthenticationEpoch(t *testing.T, value uint64) AuthenticationEpoch {
	t.Helper()
	epoch, err := NewAuthenticationEpoch(value)
	if err != nil {
		t.Fatalf("new authentication epoch %d: %v", value, err)
	}
	return epoch
}

func mustPasswordEnvelope(t *testing.T) PasswordEnvelope {
	t.Helper()
	envelope, err := NewPasswordEnvelope([]byte("$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"))
	if err != nil {
		t.Fatalf("new password envelope: %v", err)
	}
	return envelope
}

func mustSession(
	t *testing.T,
	issuedAt time.Time,
	idleExpiresAt time.Time,
	absoluteExpiresAt time.Time,
) Session {
	t.Helper()
	digest, err := NewTokenDigest(digestBytes(1))
	if err != nil {
		t.Fatalf("new token digest: %v", err)
	}
	session, err := NewSession(
		mustSessionRef(t, "session-1"),
		mustOperationRef(t, "operation-issue-1"),
		mustAccountID(t, "account-1"),
		digest,
		mustAuthenticationEpoch(t, 9),
		issuedAt,
		idleExpiresAt,
		absoluteExpiresAt,
	)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	return session
}

func mustOperationRef(t *testing.T, value string) OperationRef {
	t.Helper()
	reference, err := NewOperationRef(value)
	if err != nil {
		t.Fatalf("new operation reference %q: %v", value, err)
	}
	return reference
}

func mustAdmissionEpoch(t *testing.T, value uint64) AdmissionEpoch {
	t.Helper()
	epoch, err := NewAdmissionEpoch(value)
	if err != nil {
		t.Fatalf("new admission epoch %d: %v", value, err)
	}
	return epoch
}

func mustSessionRef(t *testing.T, value string) SessionRef {
	t.Helper()
	reference, err := NewSessionRef(value)
	if err != nil {
		t.Fatalf("new session reference %q: %v", value, err)
	}
	return reference
}

func mustThrottleDigest(t *testing.T, marker byte) ThrottleDigest {
	t.Helper()
	digest, err := NewThrottleDigest(digestBytes(marker))
	if err != nil {
		t.Fatalf("new throttle digest: %v", err)
	}
	return digest
}

func digestBytes(marker byte) []byte {
	value := make([]byte, DigestBytes)
	value[0] = marker
	value[DigestBytes-1] = marker ^ 0xff
	return value
}

func canonicalTime(
	year int,
	month time.Month,
	day int,
	hour int,
	minute int,
	second int,
	nanosecond int,
) time.Time {
	return time.Date(year, month, day, hour, minute, second, nanosecond, time.UTC)
}
