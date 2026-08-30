package application

import "errors"

var (
	// ErrMembershipRoutingInvalidArgument reports an invalid context, subject
	// reference, or routing policy before any external work begins.
	ErrMembershipRoutingInvalidArgument = errors.New("lottery membership routing: invalid argument")
	// ErrMembershipRoutingNotConfigured reports a missing reader, clock, or
	// positive membership fact freshness bound.
	ErrMembershipRoutingNotConfigured = errors.New("lottery membership routing: not configured")
	// ErrMembershipRoutingClockInvalid reports that the controlled server clock
	// did not provide a usable logical evaluation instant.
	ErrMembershipRoutingClockInvalid = errors.New("lottery membership routing: clock is invalid")
	// ErrMembershipTierFactNotFound means no authoritative membership snapshot
	// was available. Absence cannot be routed through the baseline default.
	ErrMembershipTierFactNotFound = errors.New("lottery membership routing: membership tier fact not found")
	// ErrMembershipTierFactUnavailable means the membership authority could not
	// answer while the caller context remained live.
	ErrMembershipTierFactUnavailable = errors.New("lottery membership routing: membership tier fact unavailable")
	// ErrMembershipTierFactReadFailure covers an unclassified provider failure.
	ErrMembershipTierFactReadFailure = errors.New("lottery membership routing: membership tier fact read failed")
	// ErrMembershipTierFactInvalid reports a zero, unsupported, future, corrupt,
	// or different-subject snapshot returned by the provider.
	ErrMembershipTierFactInvalid = errors.New("lottery membership routing: membership tier fact is invalid")
	// ErrMembershipTierFactStale reports a structurally valid fact older than the
	// configured maximum age at the single evaluation instant.
	ErrMembershipTierFactStale = errors.New("lottery membership routing: membership tier fact is stale")
)

// MembershipTierFactReadError retains an opaque trusted cause while ordinary
// rendering and errors.Is expose exactly one reviewed semantic class. Invalid
// includes a provider payload that an adapter could not map into a domain fact.
type MembershipTierFactReadError struct {
	class error
	cause error
}

// WrapMembershipTierFactReadError constructs a safe adapter-facing error.
// Unknown classes fail closed as ErrMembershipTierFactReadFailure.
func WrapMembershipTierFactReadError(class, cause error) *MembershipTierFactReadError {
	if !knownMembershipTierFactReadClass(class) {
		class = ErrMembershipTierFactReadFailure
	}
	return &MembershipTierFactReadError{class: class, cause: cause}
}

func (e *MembershipTierFactReadError) Error() string {
	if e == nil || !knownMembershipTierFactReadClass(e.class) {
		return ErrMembershipTierFactReadFailure.Error()
	}
	return e.class.Error()
}

// Is exposes only the stable application class, never the provider cause.
func (e *MembershipTierFactReadError) Is(target error) bool {
	if e == nil || !knownMembershipTierFactReadClass(e.class) {
		return target == ErrMembershipTierFactReadFailure
	}
	return target == e.class
}

// Cause returns the provider error only to code that explicitly opts into the
// trusted diagnostic channel. It is intentionally not part of errors.Unwrap.
func (e *MembershipTierFactReadError) Cause() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownMembershipTierFactReadClass(class error) bool {
	return class == ErrMembershipTierFactNotFound ||
		class == ErrMembershipTierFactUnavailable ||
		class == ErrMembershipTierFactReadFailure ||
		class == ErrMembershipTierFactInvalid
}
