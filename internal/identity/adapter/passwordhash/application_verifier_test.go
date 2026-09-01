package passwordhash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
)

func TestApplicationVerifierMapsBoundedHashResult(t *testing.T) {
	hasher := newTestHasher(t, bytes.NewReader([]byte("0123456789abcdef")), 1, time.Second)
	verifier, err := NewApplicationVerifier(hasher)
	if err != nil {
		t.Fatalf("NewApplicationVerifier() error = %v", err)
	}

	password := []byte("GrowthOS-password-32")
	callerCopy := bytes.Clone(password)
	envelope, err := hasher.HashEnrollment(context.Background(), password)
	if err != nil {
		t.Fatalf("HashEnrollment() error = %v", err)
	}

	matched, err := verifier.VerifyLogin(context.Background(), password, envelope.Encoded())
	if err != nil {
		t.Fatalf("VerifyLogin(match) error = %v", err)
	}
	if !matched.Matched() || matched.NeedsRehash() {
		t.Fatalf("VerifyLogin(match) = %+v", matched)
	}

	mismatch, err := verifier.VerifyLogin(
		context.Background(),
		[]byte("GrowthOS-password-33"),
		envelope.Encoded(),
	)
	if err != nil {
		t.Fatalf("VerifyLogin(mismatch) error = %v", err)
	}
	if mismatch.Matched() || mismatch.NeedsRehash() {
		t.Fatalf("VerifyLogin(mismatch) = %+v", mismatch)
	}
	if !bytes.Equal(password, callerCopy) {
		t.Fatal("application verifier modified caller-owned password")
	}
	if err := verifier.VerifyUnknownLogin(context.Background(), password); err != nil {
		t.Fatalf("VerifyUnknownLogin() error = %v", err)
	}
}

func TestApplicationVerifierRejectsInvalidCompositionAndPreservesClasses(t *testing.T) {
	if _, err := NewApplicationVerifier(nil); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewApplicationVerifier(nil) error = %v", err)
	}
	if _, err := NewApplicationVerifier(&Hasher{}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewApplicationVerifier(partial) error = %v", err)
	}

	var nilVerifier *ApplicationVerifier
	if _, err := nilVerifier.VerifyLogin(
		context.Background(),
		[]byte("valid password"),
		"invalid envelope",
	); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("nil VerifyLogin() error = %v", err)
	}
	if err := nilVerifier.VerifyUnknownLogin(
		context.Background(),
		[]byte("valid password"),
	); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("nil VerifyUnknownLogin() error = %v", err)
	}

	hasher := newTestHasher(t, bytes.NewReader([]byte("fedcba9876543210")), 1, time.Second)
	verifier, err := NewApplicationVerifier(hasher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.VerifyLogin(
		context.Background(),
		[]byte("valid password"),
		"invalid envelope",
	); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("VerifyLogin(invalid envelope) error = %v", err)
	}
}

func TestApplicationVerifierFormattingIsRedacted(t *testing.T) {
	hasher := newTestHasher(t, bytes.NewReader([]byte("0011223344556677")), 1, time.Second)
	verifier, err := NewApplicationVerifier(hasher)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		fmt.Sprint(verifier),
		fmt.Sprintf("%#v", verifier),
		verifier.LogValue().String(),
		slog.AnyValue(verifier).Resolve().String(),
	} {
		if !strings.Contains(rendered, "REDACTED") {
			t.Fatalf("rendering %q is not redacted", rendered)
		}
	}
}

func TestApplicationVerifierSatisfiesPort(t *testing.T) {
	var _ identityapp.PasswordVerifier = (*ApplicationVerifier)(nil)
}
