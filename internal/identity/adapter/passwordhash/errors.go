package passwordhash

import (
	"context"
	"errors"
)

var (
	// ErrInvalidConfiguration reports a rejected adapter configuration without
	// exposing injected dependencies.
	ErrInvalidConfiguration = errors.New("password hashing configuration is invalid")
	// ErrPasswordRejected reports that a password is outside the bounded input
	// contract. It intentionally does not identify the failed bound.
	ErrPasswordRejected = errors.New("password is outside the accepted bounds")
	// ErrInvalidEnvelope reports malformed, non-canonical, or unsupported PHC
	// data. It never includes the envelope itself.
	ErrInvalidEnvelope = errors.New("password envelope is invalid")
	// ErrHashingUnavailable classifies cancellation and semaphore admission
	// failures without disclosing whether an account exists.
	ErrHashingUnavailable = errors.New("password hashing is unavailable")
	// ErrEntropyUnavailable reports that enrollment could not obtain a salt. The
	// underlying reader error is deliberately not exposed.
	ErrEntropyUnavailable = errors.New("password hashing entropy is unavailable")
)

type classifiedError struct {
	class        error
	contextClass error
}

func (e classifiedError) Error() string { return e.class.Error() }

// Is exposes only stable public classes and the two non-sensitive context
// classes. It deliberately does not provide Unwrap, so a future dependency
// error cannot leak through an ordinary error chain.
func (e classifiedError) Is(target error) bool {
	return target == e.class || (e.contextClass != nil && target == e.contextClass)
}

func hashingUnavailable(cause error) error {
	var contextClass error
	switch cause {
	case context.Canceled:
		contextClass = context.Canceled
	case context.DeadlineExceeded:
		contextClass = context.DeadlineExceeded
	}
	return classifiedError{class: ErrHashingUnavailable, contextClass: contextClass}
}
