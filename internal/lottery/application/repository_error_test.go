package application

import (
	"errors"
	"testing"
)

func TestRepositoryErrorPreservesClassAndCauseWithoutRenderingCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("secret driver detail")
	err := WrapRepositoryError(ErrRepositoryRetryable, cause)

	if !errors.Is(err, ErrRepositoryRetryable) {
		t.Fatal("repository error does not preserve its semantic class")
	}
	if !errors.Is(err, cause) {
		t.Fatal("repository error does not preserve its diagnostic cause")
	}
	if got := err.Error(); got != ErrRepositoryRetryable.Error() {
		t.Fatalf("Error() = %q, want safe class %q", got, ErrRepositoryRetryable.Error())
	}
}

func TestRepositoryErrorFailsClosedForUnknownClass(t *testing.T) {
	t.Parallel()

	err := WrapRepositoryError(errors.New("unreviewed class"), nil)
	if !errors.Is(err, ErrRepositoryFailure) {
		t.Fatal("unknown error class did not fail closed")
	}
}

func TestRepositoryErrorZeroValuesFailClosedConsistently(t *testing.T) {
	t.Parallel()

	var zero RepositoryError
	if zero.Error() != ErrRepositoryFailure.Error() || !errors.Is(&zero, ErrRepositoryFailure) {
		t.Fatal("zero RepositoryError is not consistently classified as repository failure")
	}
	var typedNil *RepositoryError
	if typedNil.Error() != ErrRepositoryFailure.Error() || !errors.Is(typedNil, ErrRepositoryFailure) {
		t.Fatal("typed-nil RepositoryError is not consistently classified as repository failure")
	}
}
