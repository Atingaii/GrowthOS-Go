package application

import (
	"errors"
	"strings"
	"testing"
)

func TestRegistrationFactReadErrorPreservesClassAndCauseWithoutRenderingCause(t *testing.T) {
	secretCause := errors.New("secret upstream address and user payload")
	err := WrapRegistrationFactReadError(ErrRegistrationFactUnavailable, secretCause)

	if !errors.Is(err, ErrRegistrationFactUnavailable) {
		t.Fatal("fact read error did not preserve its stable class")
	}
	if errors.Is(err, secretCause) || err.Cause() != secretCause {
		t.Fatal("diagnostic cause must be explicit and excluded from errors.Is")
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
	if typedNil.Cause() != nil {
		t.Fatal("typed-nil fact read error returned a diagnostic cause")
	}
}

func TestRegistrationFactReadErrorExposesExactlyOneSemanticClass(t *testing.T) {
	err := WrapRegistrationFactReadError(
		ErrRegistrationFactUnavailable,
		ErrRegistrationFactNotFound,
	)

	if !errors.Is(err, ErrRegistrationFactUnavailable) {
		t.Fatal("wrapper lost its declared semantic class")
	}
	if errors.Is(err, ErrRegistrationFactNotFound) || errors.Is(err, ErrRegistrationFactReadFailure) {
		t.Fatal("diagnostic cause leaked a second semantic class into errors.Is")
	}
	if err.Cause() != ErrRegistrationFactNotFound {
		t.Fatal("trusted diagnostic cause was not retained explicitly")
	}
}
