package domain

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestThrottleStateExactWindowBlockAndInflightBoundaries(t *testing.T) {
	t.Parallel()

	startedAt := canonicalTime(2026, 9, 1, 2, 0, 0, 0)
	expiresAt := startedAt.Add(10 * time.Minute)
	blockedUntil := startedAt.Add(5 * time.Minute)
	inflightExpiry := startedAt.Add(3 * time.Second)
	state := mustThrottleState(
		t,
		ThrottleDimensionLogin,
		startedAt,
		expiresAt,
		5,
		2,
		7,
		inflightExpiry,
		blockedUntil,
	)
	checks := []struct {
		name            string
		now             time.Time
		windowActive    bool
		blocked         bool
		inflightExpired bool
	}{
		{name: "start", now: startedAt, windowActive: true, blocked: true, inflightExpired: false},
		{name: "before inflight end", now: inflightExpiry.Add(-time.Microsecond), windowActive: true, blocked: true, inflightExpired: false},
		{name: "at inflight end", now: inflightExpiry, windowActive: true, blocked: true, inflightExpired: true},
		{name: "at block end", now: blockedUntil, windowActive: true, blocked: false, inflightExpired: true},
		{name: "at window end", now: expiresAt, windowActive: false, blocked: false, inflightExpired: true},
	}
	for _, check := range checks {
		windowActive, windowErr := state.WindowActiveAt(check.now)
		blocked, blockedErr := state.BlockedAt(check.now)
		inflightExpired, inflightErr := state.InflightExpiredAt(check.now)
		if windowErr != nil || blockedErr != nil || inflightErr != nil ||
			windowActive != check.windowActive || blocked != check.blocked ||
			inflightExpired != check.inflightExpired {
			t.Fatalf(
				"%s: window=%v/%v blocked=%v/%v inflight=%v/%v",
				check.name,
				windowActive,
				windowErr,
				blocked,
				blockedErr,
				inflightExpired,
				inflightErr,
			)
		}
	}
	if _, err := state.WindowActiveAt(startedAt.Add(-time.Microsecond)); !errors.Is(err, ErrThrottleEvaluationTimeInvalid) {
		t.Fatalf("before window error = %v", err)
	}
}

func TestThrottleExpiredInflightRecoveryFencesOldBatch(t *testing.T) {
	t.Parallel()

	startedAt := canonicalTime(2026, 9, 1, 2, 0, 0, 0)
	inflightExpiry := startedAt.Add(3 * time.Second)
	state := mustThrottleState(
		t,
		ThrottleDimensionSource,
		startedAt,
		startedAt.Add(15*time.Minute),
		4,
		2,
		11,
		inflightExpiry,
		time.Time{},
	)

	unchanged, recovered, err := state.RecoverExpiredInflight(inflightExpiry.Add(-time.Microsecond))
	if err != nil || recovered || unchanged != state {
		t.Fatalf("early recovery = %#v/%v/%v", unchanged, recovered, err)
	}
	reclaimed, recovered, err := state.RecoverExpiredInflight(inflightExpiry)
	if err != nil || !recovered {
		t.Fatalf("deadline recovery = %#v/%v/%v", reclaimed, recovered, err)
	}
	if reclaimed.InflightCount() != 0 || reclaimed.AdmissionEpoch() != 12 {
		t.Fatalf("reclaimed count/epoch = %d/%d", reclaimed.InflightCount(), reclaimed.AdmissionEpoch())
	}
	if _, exists := reclaimed.InflightExpiresAt(); exists {
		t.Fatal("reclaimed snapshot retained inflight expiry")
	}
	if state.InflightCount() != 2 || state.AdmissionEpoch() != 11 {
		t.Fatal("recovery mutated original")
	}
	second, recovered, err := reclaimed.RecoverExpiredInflight(inflightExpiry)
	if err != nil || recovered || second != reclaimed {
		t.Fatalf("second recovery = %#v/%v/%v", second, recovered, err)
	}

	exhausted := withThrottleMutation(state, func(value *ThrottleState) {
		value.admissionEpoch = AdmissionEpoch(^uint64(0))
	})
	result, recovered, err := exhausted.RecoverExpiredInflight(inflightExpiry)
	if !errors.Is(err, ErrAdmissionEpochExhausted) || recovered || result != (ThrottleState{}) {
		t.Fatalf("exhausted recovery = %#v/%v/%v", result, recovered, err)
	}
}

func TestThrottleStateGettersAndDigestAreImmutable(t *testing.T) {
	t.Parallel()

	startedAt := canonicalTime(2026, 9, 1, 2, 0, 0, 0)
	digestInput := digestBytes(4)
	digest, err := NewThrottleDigest(digestInput)
	if err != nil {
		t.Fatalf("new digest: %v", err)
	}
	state, err := NewThrottleState(
		ThrottleDimensionSource,
		digest,
		startedAt,
		startedAt.Add(time.Minute),
		0,
		0,
		mustAdmissionEpoch(t, 1),
		time.Time{},
		time.Time{},
	)
	if err != nil {
		t.Fatalf("new state: %v", err)
	}
	digestInput[0] = 9
	first := state.Digest().Bytes()
	first[0] = 8
	if second := state.Digest().Bytes(); second[0] != 4 {
		t.Fatalf("digest mutation changed state: %x", second)
	}
	if state.Dimension() != ThrottleDimensionSource || state.WindowStartedAt() != startedAt ||
		state.WindowExpiresAt() != startedAt.Add(time.Minute) || state.FailureCount() != 0 ||
		state.InflightCount() != 0 || state.AdmissionEpoch() != 1 {
		t.Fatalf("unexpected getters: %#v", state)
	}
	if _, exists := state.BlockedUntil(); exists {
		t.Fatal("unblocked state exposed blocked-until")
	}
	if _, exists := state.InflightExpiresAt(); exists {
		t.Fatal("empty inflight state exposed expiry")
	}
}

func TestThrottleStateRejectsUnknownPartialAndOverflowShape(t *testing.T) {
	t.Parallel()

	startedAt := canonicalTime(2026, 9, 1, 2, 0, 0, 0)
	valid := ThrottleState{
		dimension:       ThrottleDimensionLogin,
		digest:          mustThrottleDigest(t, 3),
		windowStartedAt: startedAt,
		windowExpiresAt: startedAt.Add(time.Minute),
		failureCount:    1,
		admissionEpoch:  1,
	}
	invalid := []ThrottleState{
		{},
		withThrottleMutation(valid, func(state *ThrottleState) { state.dimension = "device" }),
		withThrottleMutation(valid, func(state *ThrottleState) { state.digest = ThrottleDigest{} }),
		withThrottleMutation(valid, func(state *ThrottleState) { state.windowStartedAt = time.Time{} }),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.windowExpiresAt = state.windowStartedAt
		}),
		withThrottleMutation(valid, func(state *ThrottleState) { state.admissionEpoch = 0 }),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.inflightCount = 1
			state.inflightExpiresAt = time.Time{}
		}),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.inflightCount = 0
			state.inflightExpiresAt = state.windowStartedAt.Add(time.Second)
		}),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.inflightCount = 1
			state.inflightExpiresAt = state.windowStartedAt
		}),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.failureCount = ^uint32(0)
			state.inflightCount = 1
			state.inflightExpiresAt = state.windowStartedAt.Add(time.Second)
		}),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.failureCount = 0
			state.blockedUntil = state.windowStartedAt.Add(time.Second)
		}),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.blockedUntil = state.windowStartedAt
		}),
		withThrottleMutation(valid, func(state *ThrottleState) {
			state.blockedUntil = state.windowExpiresAt.Add(time.Microsecond)
		}),
	}
	for _, state := range invalid {
		if err := state.Validate(); !errors.Is(err, ErrThrottleStateInvalid) {
			t.Fatalf("validate invalid state %#v: %v", state, err)
		}
	}

	state, err := NewThrottleState(
		"device",
		valid.digest,
		valid.windowStartedAt,
		valid.windowExpiresAt,
		valid.failureCount,
		0,
		1,
		time.Time{},
		time.Time{},
	)
	if !errors.Is(err, ErrThrottleDimensionUnsupported) || state != (ThrottleState{}) {
		t.Fatalf("unknown dimension construction = %#v, %v", state, err)
	}
}

func TestThrottleDimensionEpochAndDigestAreClosedBoundedAndRedacted(t *testing.T) {
	t.Parallel()

	if !ThrottleDimensionLogin.Valid() || !ThrottleDimensionSource.Valid() ||
		ThrottleDimension("").Valid() || ThrottleDimension("device").Valid() {
		t.Fatal("throttle dimensions are not closed")
	}
	if epoch, err := NewAdmissionEpoch(1); err != nil || epoch != 1 {
		t.Fatalf("admission epoch = %d, %v", epoch, err)
	}
	if epoch, err := NewAdmissionEpoch(0); !errors.Is(err, ErrAdmissionEpochInvalid) || epoch != 0 {
		t.Fatalf("zero admission epoch = %d, %v", epoch, err)
	}
	for _, value := range [][]byte{nil, {}, make([]byte, DigestBytes-1), make([]byte, DigestBytes), make([]byte, DigestBytes+1)} {
		digest, err := NewThrottleDigest(value)
		if !errors.Is(err, ErrThrottleDigestInvalid) || digest != (ThrottleDigest{}) {
			t.Fatalf("invalid throttle digest len=%d = %#v, %v", len(value), digest, err)
		}
	}

	secret := bytes.Repeat([]byte{'t'}, DigestBytes)
	digest, err := NewThrottleDigest(secret)
	if err != nil {
		t.Fatalf("new digest: %v", err)
	}
	for _, formatted := range []string{fmt.Sprint(digest), fmt.Sprintf("%v", digest), fmt.Sprintf("%#v", digest)} {
		if strings.Contains(formatted, string(secret)) || !strings.Contains(formatted, redactedValue) {
			t.Fatalf("unsafe formatting %q", formatted)
		}
	}
	var output bytes.Buffer
	slog.New(slog.NewTextHandler(&output, nil)).Info("throttle", "digest", digest)
	if logged := output.String(); strings.Contains(logged, string(secret)) || !strings.Contains(logged, redactedValue) {
		t.Fatalf("unsafe structured log %q", logged)
	}
}

func FuzzThrottleDigestRequiresExactNonzeroBytes(f *testing.F) {
	f.Add([]byte(nil))
	f.Add(make([]byte, DigestBytes))
	f.Add(digestBytes(2))
	f.Fuzz(func(t *testing.T, value []byte) {
		digest, err := NewThrottleDigest(value)
		wantValid := len(value) == DigestBytes && !allZero(value)
		if (err == nil) != wantValid {
			t.Fatalf("len=%d valid=%v err=%v", len(value), wantValid, err)
		}
		if err == nil && len(digest.Bytes()) != DigestBytes {
			t.Fatalf("digest length = %d", len(digest.Bytes()))
		}
	})
}

func FuzzThrottleAggregateCountNeverWraps(f *testing.F) {
	f.Add(uint32(0), uint32(0))
	f.Add(^uint32(0), uint32(0))
	f.Add(^uint32(0), uint32(1))
	f.Fuzz(func(t *testing.T, failures uint32, inflight uint32) {
		startedAt := canonicalTime(2026, 9, 1, 2, 0, 0, 0)
		inflightExpiry := time.Time{}
		if inflight > 0 {
			inflightExpiry = startedAt.Add(time.Second)
		}
		state, err := NewThrottleState(
			ThrottleDimensionLogin,
			mustThrottleDigest(t, 9),
			startedAt,
			startedAt.Add(time.Minute),
			failures,
			inflight,
			mustAdmissionEpoch(t, 1),
			inflightExpiry,
			time.Time{},
		)
		wantValid := uint64(failures)+uint64(inflight) <= MaxThrottleAggregateCount
		if (err == nil) != wantValid {
			t.Fatalf("failures=%d inflight=%d valid=%v err=%v state=%#v", failures, inflight, wantValid, err, state)
		}
		if err != nil && state != (ThrottleState{}) {
			t.Fatalf("failed construction returned partial state %#v", state)
		}
	})
}

func mustThrottleState(
	t *testing.T,
	dimension ThrottleDimension,
	startedAt time.Time,
	expiresAt time.Time,
	failureCount uint32,
	inflightCount uint32,
	admissionEpoch uint64,
	inflightExpiresAt time.Time,
	blockedUntil time.Time,
) ThrottleState {
	t.Helper()
	state, err := NewThrottleState(
		dimension,
		mustThrottleDigest(t, 2),
		startedAt,
		expiresAt,
		failureCount,
		inflightCount,
		mustAdmissionEpoch(t, admissionEpoch),
		inflightExpiresAt,
		blockedUntil,
	)
	if err != nil {
		t.Fatalf("new throttle state: %v", err)
	}
	return state
}

func withThrottleMutation(state ThrottleState, mutate func(*ThrottleState)) ThrottleState {
	mutate(&state)
	return state
}
