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

func TestRepositoryErrorRecognizesStrategyRoutingGraphClassesWithoutConflation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		class    error
		notClass error
	}{
		{
			name:     "graph not found",
			class:    ErrStrategyRoutingGraphNotFound,
			notClass: ErrStrategyNotFound,
		},
		{
			name:     "graph already exists",
			class:    ErrStrategyRoutingGraphAlreadyExists,
			notClass: ErrStrategyAlreadyExists,
		},
		{
			name:     "stored graph invalid",
			class:    ErrStoredStrategyRoutingGraphInvalid,
			notClass: ErrStoredStrategyInvalid,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cause := errors.New("secret graph persistence detail")
			err := WrapRepositoryError(test.class, cause)
			if !errors.Is(err, test.class) {
				t.Fatalf("repository error does not expose class %q", test.class)
			}
			if errors.Is(err, test.notClass) {
				t.Fatalf("graph class %q was conflated with %q", test.class, test.notClass)
			}
			if !errors.Is(err, cause) {
				t.Fatal("repository error lost its diagnostic cause")
			}
			if got := err.Error(); got != test.class.Error() {
				t.Fatalf("Error() = %q, want safe class %q", got, test.class.Error())
			}
			if got := err.Error(); got == cause.Error() {
				t.Fatal("repository error rendered the diagnostic cause")
			}
		})
	}
}

func TestRepositoryErrorRecognizesStrategySnapshotClassesWithoutConflation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		class    error
		notClass error
	}{
		{name: "snapshot not found", class: ErrStrategySnapshotNotFound, notClass: ErrStrategyNotFound},
		{name: "snapshot already exists", class: ErrStrategySnapshotAlreadyExists, notClass: ErrStrategyAlreadyExists},
		{name: "stored snapshot invalid", class: ErrStoredStrategySnapshotInvalid, notClass: ErrStoredStrategyInvalid},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cause := errors.New("secret snapshot storage detail")
			err := WrapRepositoryError(test.class, cause)
			if !errors.Is(err, test.class) || errors.Is(err, test.notClass) {
				t.Fatalf("snapshot class = %v, want %v without %v", err, test.class, test.notClass)
			}
			if !errors.Is(err, cause) {
				t.Fatal("snapshot repository error lost diagnostic cause")
			}
			if got := err.Error(); got != test.class.Error() {
				t.Fatalf("Error() = %q, want safe class %q", got, test.class.Error())
			}
		})
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
