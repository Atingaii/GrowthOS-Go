package application

import (
	"encoding/json"
	"log/slog"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

const (
	AuthenticationObservationWindow = 15 * time.Minute
	LoginFailureThreshold           = uint32(5)
	SourceFailureThreshold          = uint32(30)
	AuthenticationInitialBackoff    = 30 * time.Second
	AuthenticationMaximumBackoff    = 15 * time.Minute
)

// AdmissionPolicy is the frozen v1 policy carried across the application port
// so an adapter cannot silently apply a different threshold or recovery rule.
type AdmissionPolicy struct {
	observationWindow time.Duration
	loginThreshold    uint32
	sourceThreshold   uint32
	initialBackoff    time.Duration
	maximumBackoff    time.Duration
}

func V1AdmissionPolicy() AdmissionPolicy {
	return AdmissionPolicy{
		observationWindow: AuthenticationObservationWindow,
		loginThreshold:    LoginFailureThreshold,
		sourceThreshold:   SourceFailureThreshold,
		initialBackoff:    AuthenticationInitialBackoff,
		maximumBackoff:    AuthenticationMaximumBackoff,
	}
}

func (policy AdmissionPolicy) Validate() error {
	if policy != V1AdmissionPolicy() {
		return ErrInvalidArgument
	}
	return nil
}

func (policy AdmissionPolicy) ObservationWindow() time.Duration {
	return policy.observationWindow
}

func (policy AdmissionPolicy) FailureThreshold(
	dimension identity.ThrottleDimension,
) (uint32, bool) {
	switch dimension {
	case identity.ThrottleDimensionLogin:
		return policy.loginThreshold, true
	case identity.ThrottleDimensionSource:
		return policy.sourceThreshold, true
	default:
		return 0, false
	}
}

func (policy AdmissionPolicy) InitialBackoff() time.Duration { return policy.initialBackoff }

func (policy AdmissionPolicy) MaximumBackoff() time.Duration { return policy.maximumBackoff }

// AdmissionRequest asks the persistence adapter to reserve both dimensions in
// one fixed-order transaction.
type AdmissionRequest struct {
	loginDigest  identity.ThrottleDigest
	sourceDigest identity.ThrottleDigest
	admittedAt   time.Time
	deadline     time.Time
	policy       AdmissionPolicy
}

// NewAdmissionRequest constructs the exact two-dimensional request.
func NewAdmissionRequest(
	loginDigest identity.ThrottleDigest,
	sourceDigest identity.ThrottleDigest,
	admittedAt time.Time,
	deadline time.Time,
) (AdmissionRequest, error) {
	request := AdmissionRequest{
		loginDigest:  loginDigest,
		sourceDigest: sourceDigest,
		admittedAt:   admittedAt,
		deadline:     deadline,
		policy:       V1AdmissionPolicy(),
	}
	if request.Validate() != nil {
		return AdmissionRequest{}, ErrInvalidArgument
	}
	return request, nil
}

func (request AdmissionRequest) Validate() error {
	if request.loginDigest.Validate() != nil || request.sourceDigest.Validate() != nil ||
		request.admittedAt.IsZero() || request.deadline.IsZero() ||
		request.admittedAt != canonicalInstant(request.admittedAt) ||
		request.deadline != canonicalInstant(request.deadline) ||
		!request.admittedAt.Before(request.deadline) ||
		request.deadline.Sub(request.admittedAt) > MaximumAdmissionLease ||
		request.policy.Validate() != nil {
		return ErrInvalidArgument
	}
	return nil
}

const redactedAdmissionRequest = "identity admission request (redacted)"

func (AdmissionRequest) String() string   { return redactedAdmissionRequest }
func (AdmissionRequest) GoString() string { return redactedAdmissionRequest }
func (AdmissionRequest) LogValue() slog.Value {
	return slog.StringValue(redactedAdmissionRequest)
}
func (AdmissionRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedAdmissionRequest)
}

func (request AdmissionRequest) LoginDigest() identity.ThrottleDigest {
	return request.loginDigest
}

func (request AdmissionRequest) SourceDigest() identity.ThrottleDigest {
	return request.sourceDigest
}

func (request AdmissionRequest) AdmittedAt() time.Time { return request.admittedAt }

func (request AdmissionRequest) Deadline() time.Time { return request.deadline }

func (request AdmissionRequest) Policy() AdmissionPolicy { return request.policy }

// AdmissionGrant is validated adapter evidence from one successful atomic
// reservation. Application turns it into a private AdmissionReceipt before
// executing password work.
type AdmissionGrant struct {
	loginEpoch  identity.AdmissionEpoch
	sourceEpoch identity.AdmissionEpoch
	deadline    time.Time
}

const redactedAdmissionGrant = "identity admission grant (redacted)"

func (AdmissionGrant) String() string   { return redactedAdmissionGrant }
func (AdmissionGrant) GoString() string { return redactedAdmissionGrant }
func (AdmissionGrant) LogValue() slog.Value {
	return slog.StringValue(redactedAdmissionGrant)
}
func (AdmissionGrant) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedAdmissionGrant)
}

// NewAdmissionGrant is the strict restoration boundary for a persistence
// adapter. It does not itself make a grant trustworthy; only a configured port
// result consumed inside Login can do that.
func NewAdmissionGrant(
	loginEpoch identity.AdmissionEpoch,
	sourceEpoch identity.AdmissionEpoch,
	deadline time.Time,
) (AdmissionGrant, error) {
	grant := AdmissionGrant{
		loginEpoch:  loginEpoch,
		sourceEpoch: sourceEpoch,
		deadline:    deadline,
	}
	if grant.Validate() != nil {
		return AdmissionGrant{}, ErrInvalidArgument
	}
	return grant, nil
}

func (grant AdmissionGrant) Validate() error {
	if grant.loginEpoch.Validate() != nil || grant.sourceEpoch.Validate() != nil ||
		grant.deadline.IsZero() || grant.deadline != canonicalInstant(grant.deadline) {
		return ErrInvalidArgument
	}
	return nil
}

// AdmissionReceipt cannot be constructed by adapters or transports. It binds
// the exact keys, epochs, and lease deadline returned during Login.
type AdmissionReceipt struct {
	loginDigest  identity.ThrottleDigest
	sourceDigest identity.ThrottleDigest
	loginEpoch   identity.AdmissionEpoch
	sourceEpoch  identity.AdmissionEpoch
	deadline     time.Time
}

const redactedAdmissionReceipt = "identity admission receipt (redacted)"

func (AdmissionReceipt) String() string   { return redactedAdmissionReceipt }
func (AdmissionReceipt) GoString() string { return redactedAdmissionReceipt }
func (AdmissionReceipt) LogValue() slog.Value {
	return slog.StringValue(redactedAdmissionReceipt)
}
func (AdmissionReceipt) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedAdmissionReceipt)
}

func newAdmissionReceipt(
	request AdmissionRequest,
	grant AdmissionGrant,
) (AdmissionReceipt, error) {
	if request.Validate() != nil || grant.Validate() != nil ||
		grant.deadline != request.deadline {
		return AdmissionReceipt{}, ErrAuthenticationUnavailable
	}
	receipt := AdmissionReceipt{
		loginDigest:  request.loginDigest,
		sourceDigest: request.sourceDigest,
		loginEpoch:   grant.loginEpoch,
		sourceEpoch:  grant.sourceEpoch,
		deadline:     grant.deadline,
	}
	if receipt.Validate() != nil {
		return AdmissionReceipt{}, ErrAuthenticationUnavailable
	}
	return receipt, nil
}

func (receipt AdmissionReceipt) Validate() error {
	if receipt.loginDigest.Validate() != nil || receipt.sourceDigest.Validate() != nil ||
		receipt.loginEpoch.Validate() != nil || receipt.sourceEpoch.Validate() != nil ||
		receipt.deadline.IsZero() || receipt.deadline != canonicalInstant(receipt.deadline) {
		return ErrInvalidArgument
	}
	return nil
}

func (receipt AdmissionReceipt) LoginDigest() identity.ThrottleDigest {
	return receipt.loginDigest
}

func (receipt AdmissionReceipt) SourceDigest() identity.ThrottleDigest {
	return receipt.sourceDigest
}

func (receipt AdmissionReceipt) LoginEpoch() identity.AdmissionEpoch {
	return receipt.loginEpoch
}

func (receipt AdmissionReceipt) SourceEpoch() identity.AdmissionEpoch {
	return receipt.sourceEpoch
}

func (receipt AdmissionReceipt) Deadline() time.Time { return receipt.deadline }

// AdmissionFinalOutcome is the sole finalize vocabulary.
type AdmissionFinalOutcome string

const (
	AdmissionFinalOutcomeSuccess AdmissionFinalOutcome = "success"
	AdmissionFinalOutcomeFailure AdmissionFinalOutcome = "failure"
	AdmissionFinalOutcomeNeutral AdmissionFinalOutcome = "neutral"
)

func (outcome AdmissionFinalOutcome) Valid() bool {
	switch outcome {
	case AdmissionFinalOutcomeSuccess,
		AdmissionFinalOutcomeFailure,
		AdmissionFinalOutcomeNeutral:
		return true
	default:
		return false
	}
}
