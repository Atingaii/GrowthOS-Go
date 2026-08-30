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
	// ErrRiskScreeningFactNotFound means no authoritative screening snapshot was
	// available. Absence is not proof that the participant passed screening.
	ErrRiskScreeningFactNotFound = errors.New("participation eligibility: risk screening fact not found")
	// ErrRiskScreeningFactUnavailable means the controlled risk authority could
	// not answer. Safety-critical screening fails closed.
	ErrRiskScreeningFactUnavailable = errors.New("participation eligibility: risk screening fact unavailable")
	// ErrRiskScreeningFactReadFailure covers an unclassified risk provider error.
	ErrRiskScreeningFactReadFailure = errors.New("participation eligibility: risk screening fact read failed")
	// ErrRiskScreeningFactInvalid means a provider returned a zero, future,
	// unknown-disposition, or different-participant snapshot.
	ErrRiskScreeningFactInvalid = errors.New("participation eligibility: risk screening fact is invalid")
	// ErrRiskScreeningFactStale means the source-owned assessment instant fell
	// outside the configured freshness window.
	ErrRiskScreeningFactStale = errors.New("participation eligibility: risk screening fact is stale")
	// ErrPrerequisiteChainInvalidArgument reports an invalid participant, policy,
	// ruleset revision, or nil context before the chain performs any I/O.
	ErrPrerequisiteChainInvalidArgument = errors.New("participation prerequisite chain: invalid argument")
	// ErrPrerequisiteChainNotConfigured reports a missing reader, clock, or
	// non-positive freshness bound.
	ErrPrerequisiteChainNotConfigured = errors.New("participation prerequisite chain: not configured")
	// ErrPrerequisiteStepInvalid means a concrete step broke the chain contract.
	ErrPrerequisiteStepInvalid = errors.New("participation prerequisite chain: step result is invalid")
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

// Cause keeps the diagnostic error available to explicitly trusted code while
// excluding it from the errors.Is tree. This preserves exactly one public
// semantic class even if an adapter accidentally supplies another sentinel.
func (e *RegistrationFactReadError) Cause() error {
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

// RiskScreeningFactReadError retains a trusted cause while rendering and
// matching only one reviewed semantic class.
type RiskScreeningFactReadError struct {
	class error
	cause error
}

// WrapRiskScreeningFactReadError builds a safe risk provider error. Unknown
// classes collapse to the fail-closed read-failure class.
func WrapRiskScreeningFactReadError(class, cause error) *RiskScreeningFactReadError {
	if !knownRiskScreeningFactReadClass(class) {
		class = ErrRiskScreeningFactReadFailure
	}
	return &RiskScreeningFactReadError{class: class, cause: cause}
}

func (e *RiskScreeningFactReadError) Error() string {
	if e == nil || !knownRiskScreeningFactReadClass(e.class) {
		return ErrRiskScreeningFactReadFailure.Error()
	}
	return e.class.Error()
}

// Is exposes exactly the reviewed semantic class to errors.Is.
func (e *RiskScreeningFactReadError) Is(target error) bool {
	if e == nil || !knownRiskScreeningFactReadClass(e.class) {
		return target == ErrRiskScreeningFactReadFailure
	}
	return target == e.class
}

// Cause retains an opaque diagnostic cause outside the errors.Is tree.
func (e *RiskScreeningFactReadError) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownRiskScreeningFactReadClass(class error) bool {
	return class == ErrRiskScreeningFactNotFound ||
		class == ErrRiskScreeningFactUnavailable ||
		class == ErrRiskScreeningFactReadFailure
}
