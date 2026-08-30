package application

import (
	"errors"
	"strings"
	"testing"
)

func TestRegistrationFactReadErrorPreservesClassAndCauseWithoutRenderingCause(t *testing.T) {
	secretCause := errors.New("secret upstream address and user payload")
	err := WrapRegistrationFactReadError(ErrRegistrationFactUnavailable, secretCause)

	if !errors.Is(err, ErrRegistrationFactUnavailable) || !errors.Is(err, secretCause) {
		t.Fatal("fact read error did not preserve its class and trusted cause")
	}
	if err.Error() != ErrRegistrationFactUnavailable.Error() || strings.Contains(err.Error(), "secret") {
		t.Fatalf("Error() = %q, want only safe class", err.Error())
	}
}

func TestRegistrationFactReadErrorFailsClosedForUnknownAndZeroClasses(t *testing.T) {
	unknown := WrapRegistrationFactReadError(errors.New("unreviewed"), nil)
	if !errors.Is(unknown, ErrRegistrationFactReadFailure) {
		t.Fatal("unknown class did not fail closed as read failure")
	}

	var zero RegistrationFactReadError
	if zero.Error() != ErrRegistrationFactReadFailure.Error() || !errors.Is(&zero, ErrRegistrationFactReadFailure) {
		t.Fatal("zero fact read error is not consistently classified")
	}
	var typedNil *RegistrationFactReadError
	if typedNil.Error() != ErrRegistrationFactReadFailure.Error() || !errors.Is(typedNil, ErrRegistrationFactReadFailure) {
		t.Fatal("typed-nil fact read error is not consistently classified")
	}
}
