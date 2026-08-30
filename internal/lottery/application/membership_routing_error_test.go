package application

import (
	"errors"
	"strings"
	"testing"
)

func TestMembershipTierFactReadErrorPreservesSafeClassAndExplicitCause(t *testing.T) {
	secretCause := errors.New("secret upstream address and membership payload")
	tests := []struct {
		name  string
		class error
	}{
		{name: "not found", class: ErrMembershipTierFactNotFound},
		{name: "unavailable", class: ErrMembershipTierFactUnavailable},
		{name: "read failure", class: ErrMembershipTierFactReadFailure},
		{name: "invalid provider payload", class: ErrMembershipTierFactInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := WrapMembershipTierFactReadError(test.class, secretCause)
			if !errors.Is(err, test.class) {
				t.Fatalf("errors.Is(%v) = false", test.class)
			}
			if errors.Is(err, secretCause) || errors.Unwrap(err) != nil {
				t.Fatal("diagnostic cause leaked into the public error chain")
			}
			if err.Cause() != secretCause {
				t.Fatal("explicit Cause() lost the provider diagnostic")
			}
			if err.Error() != test.class.Error() || strings.Contains(err.Error(), "secret") {
				t.Fatalf("Error() = %q, want only safe class", err.Error())
			}
		})
	}
}

func TestMembershipTierFactReadErrorFailsClosedForUnknownZeroAndTypedNil(t *testing.T) {
	unknown := WrapMembershipTierFactReadError(errors.New("unreviewed class"), nil)
	if !errors.Is(unknown, ErrMembershipTierFactReadFailure) ||
		errors.Is(unknown, ErrMembershipTierFactNotFound) ||
		errors.Is(unknown, ErrMembershipTierFactUnavailable) {
		t.Fatal("unknown class did not collapse to exactly read failure")
	}

	var zero MembershipTierFactReadError
	if zero.Error() != ErrMembershipTierFactReadFailure.Error() ||
		!errors.Is(&zero, ErrMembershipTierFactReadFailure) ||
		zero.Cause() != nil {
		t.Fatal("zero read error is not consistently fail-closed")
	}

	var typedNil *MembershipTierFactReadError
	if typedNil.Error() != ErrMembershipTierFactReadFailure.Error() ||
		!errors.Is(typedNil, ErrMembershipTierFactReadFailure) ||
		typedNil.Cause() != nil {
		t.Fatal("typed-nil read error is not consistently fail-closed")
	}
}

func TestMembershipTierFactReadErrorExposesExactlyOneSemanticClass(t *testing.T) {
	err := WrapMembershipTierFactReadError(
		ErrMembershipTierFactUnavailable,
		ErrMembershipTierFactNotFound,
	)

	if !errors.Is(err, ErrMembershipTierFactUnavailable) {
		t.Fatal("wrapper lost its declared semantic class")
	}
	if errors.Is(err, ErrMembershipTierFactNotFound) ||
		errors.Is(err, ErrMembershipTierFactReadFailure) {
		t.Fatal("diagnostic cause leaked a second semantic class into errors.Is")
	}
	if err.Cause() != ErrMembershipTierFactNotFound {
		t.Fatal("trusted diagnostic cause was not retained explicitly")
	}
}

func TestMembershipTierFactReadErrorKeepsDomainPayloadErrorOnlyInCause(t *testing.T) {
	rawDomainError := errors.New("stand-in domain payload contract error")
	err := WrapMembershipTierFactReadError(
		ErrMembershipTierFactInvalid,
		rawDomainError,
	)

	if !errors.Is(err, ErrMembershipTierFactInvalid) {
		t.Fatal("wrapper lost the safe application invalid-fact class")
	}
	if errors.Is(err, rawDomainError) ||
		errors.Is(err, ErrMembershipTierFactNotFound) ||
		errors.Is(err, ErrMembershipTierFactUnavailable) ||
		errors.Is(err, ErrMembershipTierFactReadFailure) {
		t.Fatal("invalid-fact wrapper exposed more than one public semantic class")
	}
	if err.Cause() != rawDomainError {
		t.Fatal("raw provider payload error was not retained in Cause()")
	}
}
