package domain

import (
	"fmt"
	"time"
)

// MaxThrottleAggregateCount makes failure-plus-inflight arithmetic explicit.
// Persistence adapters must reject a row whose aggregate cannot fit the
// frozen unsigned 32-bit storage budget.
const MaxThrottleAggregateCount uint64 = 1<<32 - 1

// ThrottleDimension is the closed admission-key dimension. The digest hides
// the login or trusted-source value from this pure state model.
type ThrottleDimension string

const (
	ThrottleDimensionLogin  ThrottleDimension = "login"
	ThrottleDimensionSource ThrottleDimension = "source"
)

func (dimension ThrottleDimension) Valid() bool {
	switch dimension {
	case ThrottleDimensionLogin, ThrottleDimensionSource:
		return true
	default:
		return false
	}
}

// AdmissionEpoch fences a recovered batch of authentication reservations.
// It is nonzero and monotonic; exhaustion fails closed rather than wrapping.
type AdmissionEpoch uint64

func NewAdmissionEpoch(value uint64) (AdmissionEpoch, error) {
	epoch := AdmissionEpoch(value)
	if err := epoch.Validate(); err != nil {
		return 0, err
	}
	return epoch, nil
}

func (epoch AdmissionEpoch) Validate() error {
	if epoch == 0 {
		return ErrAdmissionEpochInvalid
	}
	return nil
}

// ThrottleState is one immutable bounded-window failure and pre-verification
// reservation snapshot. Reservations are aggregated in the same authority
// row; AdmissionEpoch fences a crashed/expired batch from a newer batch.
type ThrottleState struct {
	dimension         ThrottleDimension
	digest            ThrottleDigest
	windowStartedAt   time.Time
	windowExpiresAt   time.Time
	failureCount      uint32
	inflightCount     uint32
	admissionEpoch    AdmissionEpoch
	inflightExpiresAt time.Time
	blockedUntil      time.Time
}

func NewThrottleState(
	dimension ThrottleDimension,
	digest ThrottleDigest,
	windowStartedAt time.Time,
	windowExpiresAt time.Time,
	failureCount uint32,
	inflightCount uint32,
	admissionEpoch AdmissionEpoch,
	inflightExpiresAt time.Time,
	blockedUntil time.Time,
) (ThrottleState, error) {
	state := ThrottleState{
		dimension:         dimension,
		digest:            digest,
		windowStartedAt:   windowStartedAt,
		windowExpiresAt:   windowExpiresAt,
		failureCount:      failureCount,
		inflightCount:     inflightCount,
		admissionEpoch:    admissionEpoch,
		inflightExpiresAt: inflightExpiresAt,
		blockedUntil:      blockedUntil,
	}
	if err := state.Validate(); err != nil {
		return ThrottleState{}, err
	}
	return state, nil
}

func (state ThrottleState) Validate() error {
	if !state.dimension.Valid() {
		return fmt.Errorf(
			"%w: %w: %q",
			ErrThrottleStateInvalid,
			ErrThrottleDimensionUnsupported,
			state.dimension,
		)
	}
	if err := state.digest.Validate(); err != nil {
		return fmt.Errorf("%w: digest: %w", ErrThrottleStateInvalid, err)
	}
	if err := validateCanonicalTime("window start", state.windowStartedAt, ErrThrottleTimeInvalid); err != nil {
		return fmt.Errorf("%w: %w", ErrThrottleStateInvalid, err)
	}
	if err := validateCanonicalTime("window expiry", state.windowExpiresAt, ErrThrottleTimeInvalid); err != nil {
		return fmt.Errorf("%w: %w", ErrThrottleStateInvalid, err)
	}
	if !state.windowStartedAt.Before(state.windowExpiresAt) {
		return fmt.Errorf("%w: %w: window start must precede expiry", ErrThrottleStateInvalid, ErrThrottleTimeInvalid)
	}
	if err := state.admissionEpoch.Validate(); err != nil {
		return fmt.Errorf("%w: admission epoch: %w", ErrThrottleStateInvalid, err)
	}
	if uint64(state.failureCount)+uint64(state.inflightCount) > MaxThrottleAggregateCount {
		return fmt.Errorf(
			"%w: %w: failure plus inflight exceeds %d",
			ErrThrottleStateInvalid,
			ErrThrottleCountInvalid,
			MaxThrottleAggregateCount,
		)
	}

	if state.inflightCount == 0 {
		if !state.inflightExpiresAt.IsZero() {
			return fmt.Errorf("%w: %w: expiry without inflight", ErrThrottleStateInvalid, ErrThrottleCountInvalid)
		}
	} else {
		if err := validateCanonicalTime("inflight expiry", state.inflightExpiresAt, ErrThrottleTimeInvalid); err != nil {
			return fmt.Errorf("%w: %w", ErrThrottleStateInvalid, err)
		}
		if !state.windowStartedAt.Before(state.inflightExpiresAt) {
			return fmt.Errorf("%w: %w: inflight expiry must follow window start", ErrThrottleStateInvalid, ErrThrottleTimeInvalid)
		}
	}

	if state.blockedUntil.IsZero() {
		return nil
	}
	if err := validateCanonicalTime("blocked until", state.blockedUntil, ErrThrottleTimeInvalid); err != nil {
		return fmt.Errorf("%w: %w", ErrThrottleStateInvalid, err)
	}
	if state.failureCount == 0 {
		return fmt.Errorf("%w: blocked state requires a failure", ErrThrottleStateInvalid)
	}
	if !state.windowStartedAt.Before(state.blockedUntil) {
		return fmt.Errorf("%w: %w: block must follow window start", ErrThrottleStateInvalid, ErrThrottleTimeInvalid)
	}
	if state.windowExpiresAt.Before(state.blockedUntil) {
		return fmt.Errorf("%w: %w: block exceeds observation window", ErrThrottleStateInvalid, ErrThrottleTimeInvalid)
	}
	return nil
}

func (state ThrottleState) Dimension() ThrottleDimension { return state.dimension }

func (state ThrottleState) Digest() ThrottleDigest { return state.digest }

func (state ThrottleState) WindowStartedAt() time.Time { return state.windowStartedAt }

func (state ThrottleState) WindowExpiresAt() time.Time { return state.windowExpiresAt }

func (state ThrottleState) FailureCount() uint32 { return state.failureCount }

func (state ThrottleState) InflightCount() uint32 { return state.inflightCount }

func (state ThrottleState) AdmissionEpoch() AdmissionEpoch { return state.admissionEpoch }

func (state ThrottleState) InflightExpiresAt() (time.Time, bool) {
	if state.inflightExpiresAt.IsZero() {
		return time.Time{}, false
	}
	return state.inflightExpiresAt, true
}

func (state ThrottleState) BlockedUntil() (time.Time, bool) {
	if state.blockedUntil.IsZero() {
		return time.Time{}, false
	}
	return state.blockedUntil, true
}

// WindowActiveAt uses an exclusive upper boundary.
func (state ThrottleState) WindowActiveAt(now time.Time) (bool, error) {
	if err := state.Validate(); err != nil {
		return false, err
	}
	if err := state.validateEvaluationTime(now); err != nil {
		return false, err
	}
	return now.Before(state.windowExpiresAt), nil
}

// BlockedAt uses an exclusive blocked-until boundary.
func (state ThrottleState) BlockedAt(now time.Time) (bool, error) {
	if err := state.Validate(); err != nil {
		return false, err
	}
	if err := state.validateEvaluationTime(now); err != nil {
		return false, err
	}
	return !state.blockedUntil.IsZero() && now.Before(state.blockedUntil), nil
}

// InflightExpiredAt uses a closed deadline boundary: at the exact expiry the
// reservation batch is expired and may be recovered.
func (state ThrottleState) InflightExpiredAt(now time.Time) (bool, error) {
	if err := state.Validate(); err != nil {
		return false, err
	}
	if err := state.validateEvaluationTime(now); err != nil {
		return false, err
	}
	return state.inflightCount > 0 && !now.Before(state.inflightExpiresAt), nil
}

// RecoverExpiredInflight returns a new fenced snapshot after an expired batch
// is reclaimed. The epoch advances exactly once for a nonempty expired batch;
// an old receipt therefore cannot decrement a later batch. A non-expired or
// empty batch is returned unchanged with recovered=false.
func (state ThrottleState) RecoverExpiredInflight(now time.Time) (ThrottleState, bool, error) {
	if err := state.Validate(); err != nil {
		return ThrottleState{}, false, err
	}
	expired, err := state.InflightExpiredAt(now)
	if err != nil {
		return ThrottleState{}, false, err
	}
	if !expired {
		return state, false, nil
	}
	if state.admissionEpoch == AdmissionEpoch(^uint64(0)) {
		return ThrottleState{}, false, ErrAdmissionEpochExhausted
	}

	recovered := state
	recovered.inflightCount = 0
	recovered.inflightExpiresAt = time.Time{}
	recovered.admissionEpoch++
	if err := recovered.Validate(); err != nil {
		return ThrottleState{}, false, err
	}
	return recovered, true, nil
}

func (state ThrottleState) validateEvaluationTime(now time.Time) error {
	if err := validateCanonicalTime("evaluated at", now, ErrThrottleEvaluationTimeInvalid); err != nil {
		return err
	}
	if now.Before(state.windowStartedAt) {
		return fmt.Errorf("%w: evaluated at precedes window", ErrThrottleEvaluationTimeInvalid)
	}
	return nil
}
