package application

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestAdmissionPolicyFreezesV1Parameters(t *testing.T) {
	t.Parallel()

	policy := V1AdmissionPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if policy.ObservationWindow() != 15*time.Minute ||
		policy.InitialBackoff() != 30*time.Second ||
		policy.MaximumBackoff() != 15*time.Minute {
		t.Fatalf("unexpected duration policy: %#v", policy)
	}
	checks := []struct {
		dimension identity.ThrottleDimension
		want      uint32
		valid     bool
	}{
		{identity.ThrottleDimensionLogin, 5, true},
		{identity.ThrottleDimensionSource, 30, true},
		{"unknown", 0, false},
	}
	for _, check := range checks {
		got, valid := policy.FailureThreshold(check.dimension)
		if got != check.want || valid != check.valid {
			t.Fatalf("threshold %q = %d/%v, want %d/%v", check.dimension, got, valid, check.want, check.valid)
		}
	}

	for _, mutation := range []func(*AdmissionPolicy){
		func(value *AdmissionPolicy) { value.observationWindow++ },
		func(value *AdmissionPolicy) { value.loginThreshold++ },
		func(value *AdmissionPolicy) { value.sourceThreshold++ },
		func(value *AdmissionPolicy) { value.initialBackoff++ },
		func(value *AdmissionPolicy) { value.maximumBackoff++ },
	} {
		changed := policy
		mutation(&changed)
		if changed.Validate() == nil {
			t.Fatalf("mutated policy validated: %#v", changed)
		}
	}
}

// TestAdmissionPortV1RecoveryContract is the executable adapter contract for
// the decision that must be made while both authority rows are locked. The
// MySQL adapter owns persistence and must run this table against its real
// implementation as well.
func TestAdmissionPortV1RecoveryContract(t *testing.T) {
	t.Parallel()

	now := applicationTestNow
	threshold := LoginFailureThreshold
	cases := []struct {
		name        string
		failure     uint32
		inflight    uint32
		blockedTill time.Time
		want        bool
	}{
		{name: "below threshold has room", failure: threshold - 2, inflight: 1, want: true},
		{name: "below threshold aggregate full", failure: threshold - 1, inflight: 1, want: false},
		{name: "threshold active backoff", failure: threshold, blockedTill: now.Add(time.Second), want: false},
		{name: "threshold expired backoff single probe", failure: threshold, blockedTill: now, want: true},
		{name: "threshold expired backoff existing probe", failure: threshold, inflight: 1, blockedTill: now, want: false},
		{name: "above threshold expired backoff single probe", failure: threshold + 2, blockedTill: now.Add(-time.Microsecond), want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := contractAllowsAdmission(
				testCase.failure,
				testCase.inflight,
				threshold,
				testCase.blockedTill,
				now,
			)
			if got != testCase.want {
				t.Fatalf("allowed=%v, want %v", got, testCase.want)
			}
		})
	}
}

func contractAllowsAdmission(
	failure uint32,
	inflight uint32,
	threshold uint32,
	blockedUntil time.Time,
	now time.Time,
) bool {
	if failure < threshold {
		return uint64(failure)+uint64(inflight) < uint64(threshold)
	}
	if !blockedUntil.IsZero() && now.Before(blockedUntil) {
		return false
	}
	return inflight == 0
}

func TestAdmissionRequestAndReceiptAreExactAndRedacted(t *testing.T) {
	t.Parallel()

	loginDigest := mustApplicationThrottleDigest(t, 0x61)
	sourceDigest := mustApplicationThrottleDigest(t, 0x62)
	request, err := NewAdmissionRequest(
		loginDigest,
		sourceDigest,
		applicationTestNow,
		applicationTestNow.Add(MaximumAdmissionLease),
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	loginEpoch, _ := identity.NewAdmissionEpoch(10)
	sourceEpoch, _ := identity.NewAdmissionEpoch(20)
	grant, err := NewAdmissionGrant(loginEpoch, sourceEpoch, request.Deadline())
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	receipt, err := newAdmissionReceipt(request, grant)
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if receipt.LoginEpoch() != loginEpoch || receipt.SourceEpoch() != sourceEpoch ||
		receipt.Deadline() != request.Deadline() {
		t.Fatalf("receipt lost exact evidence: %#v", receipt)
	}
	if got := request.Policy(); got != V1AdmissionPolicy() {
		t.Fatalf("request policy = %#v", got)
	}

	for name, value := range map[string]any{
		"request": request,
		"grant":   grant,
		"receipt": receipt,
	} {
		t.Run(name, func(t *testing.T) {
			assertRedactedValue(t, value, "616161", "626262")
		})
	}

	encoded, err := json.Marshal(receipt)
	if err != nil || string(encoded) != `"identity admission receipt (redacted)"` {
		t.Fatalf("receipt JSON = %s, %v", encoded, err)
	}
	if fmt.Sprintf("%#v", receipt) != redactedAdmissionReceipt {
		t.Fatalf("GoString was not redacted: %#v", receipt)
	}
	if receipt.LogValue().Kind() != slog.KindString {
		t.Fatal("receipt LogValue must be a single redacted string")
	}
}

func TestAdmissionRejectsLeaseAndEpochDrift(t *testing.T) {
	t.Parallel()

	loginDigest := mustApplicationThrottleDigest(t, 1)
	sourceDigest := mustApplicationThrottleDigest(t, 2)
	invalidDeadlines := []time.Time{
		applicationTestNow,
		applicationTestNow.Add(MaximumAdmissionLease + time.Microsecond),
		applicationTestNow.Add(time.Second + time.Nanosecond),
	}
	for _, deadline := range invalidDeadlines {
		request, err := NewAdmissionRequest(loginDigest, sourceDigest, applicationTestNow, deadline)
		if err == nil || request != (AdmissionRequest{}) {
			t.Fatalf("invalid deadline %v = %#v, %v", deadline, request, err)
		}
	}

	request, err := NewAdmissionRequest(
		loginDigest,
		sourceDigest,
		applicationTestNow,
		applicationTestNow.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	epoch, _ := identity.NewAdmissionEpoch(1)
	wrongDeadlineGrant, err := NewAdmissionGrant(
		epoch,
		epoch,
		request.Deadline().Add(time.Microsecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt, receiptErr := newAdmissionReceipt(request, wrongDeadlineGrant); receiptErr == nil ||
		receipt != (AdmissionReceipt{}) {
		t.Fatalf("mismatched grant forged receipt: %#v, %v", receipt, receiptErr)
	}

	for _, outcome := range []AdmissionFinalOutcome{
		AdmissionFinalOutcomeSuccess,
		AdmissionFinalOutcomeFailure,
		AdmissionFinalOutcomeNeutral,
	} {
		if !outcome.Valid() {
			t.Fatalf("valid final outcome rejected: %q", outcome)
		}
	}
	if AdmissionFinalOutcome("retry").Valid() {
		t.Fatal("open-ended final outcome accepted")
	}
}

func assertRedactedValue(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	rendered := []string{fmt.Sprint(value), fmt.Sprintf("%#v", value)}
	if encoded, err := json.Marshal(value); err == nil {
		rendered = append(rendered, string(encoded))
	} else {
		t.Fatalf("marshal redacted value: %v", err)
	}
	for _, item := range rendered {
		for _, secret := range forbidden {
			if strings.Contains(item, secret) {
				t.Fatalf("rendered value leaked %q: %s", secret, item)
			}
		}
		if !strings.Contains(item, "redacted") {
			t.Fatalf("rendered value lacks explicit redaction marker: %s", item)
		}
	}
}
