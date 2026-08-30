package application

import "errors"

var (
	// ErrEligibilityInvalidArgument reports a programmer-facing request contract
	// violation such as a nil context, zero reference, or invalid policy value.
	ErrEligibilityInvalidArgument = errors.New("participation eligibility: invalid argument")
	// ErrEligibilityNotConfigured means the concrete service lacks a usable fact
	// reader, controlled clock, or positive freshness limit.
	ErrEligibilityNotConfigured = errors.New("participation eligibility: not configured")
	// ErrEligibilityClockInvalid means the server clock did not provide a usable
	// evaluation instant. It must never become an ineligible business result.
	ErrEligibilityClockInvalid = errors.New("participation eligibility: clock is invalid")
	// ErrRegistrationFactNotFound means the provider could not find the requested
	// subject. Without a stronger provider contract this is inability to decide.
	ErrRegistrationFactNotFound = errors.New("participation eligibility: registration fact not found")
	// ErrRegistrationFactUnavailable means the authoritative provider is
	// temporarily unavailable. The caller must fail closed without claiming a
	// confirmed business rejection.
	ErrRegistrationFactUnavailable = errors.New("participation eligibility: registration fact unavailable")
	// ErrRegistrationFactReadFailure covers an unclassified provider failure.
	ErrRegistrationFactReadFailure = errors.New("participation eligibility: registration fact read failed")
	// ErrRegistrationFactInvalid means the provider returned a corrupt, future,
	// or different-subject snapshot.
	ErrRegistrationFactInvalid = errors.New("participation eligibility: registration fact is invalid")
	// ErrRegistrationFactStale means a structurally valid snapshot exceeded the
	// explicitly configured maximum age.
	ErrRegistrationFactStale = errors.New("participation eligibility: registration fact is stale")
)

// RegistrationFactReadError retains a trusted diagnostic cause while ordinary
// rendering exposes only a reviewed semantic class. Adapters should use this
// wrapper rather than attaching SQL, upstream addresses, or source payloads to
// an error that may cross logging or transport boundaries.
type RegistrationFactReadError struct {
	class error
	cause error
}

// WrapRegistrationFactReadError builds a safe provider error. Unknown classes
// fail closed as ErrRegistrationFactReadFailure.
func WrapRegistrationFactReadError(class, cause error) *RegistrationFactReadError {
	if !knownRegistrationFactReadClass(class) {
		class = ErrRegistrationFactReadFailure
	}
	return &RegistrationFactReadError{class: class, cause: cause}
}

func (e *RegistrationFactReadError) Error() string {
	if e == nil || !knownRegistrationFactReadClass(e.class) {
		return ErrRegistrationFactReadFailure.Error()
	}
	return e.class.Error()
}

// Is makes the stable class usable through errors.Is.
func (e *RegistrationFactReadError) Is(target error) bool {
	if e == nil || !knownRegistrationFactReadClass(e.class) {
		return target == ErrRegistrationFactReadFailure
	}
	return target == e.class
}

// Unwrap keeps the cause available to trusted diagnostic code without
// including it in Error().
func (e *RegistrationFactReadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownRegistrationFactReadClass(class error) bool {
	return class == ErrRegistrationFactNotFound ||
		class == ErrRegistrationFactUnavailable ||
		class == ErrRegistrationFactReadFailure
}
