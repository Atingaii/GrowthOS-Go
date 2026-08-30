package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

func TestEvaluateRiskScreeningFactAtUsesSourceAssessmentFreshness(t *testing.T) {
	evaluatedAt := applicationTestInstant()
	policy := riskAdmissionTestPolicy(t)
	maxAge := 15 * time.Minute
	tests := []struct {
		name        string
		disposition domain.RiskScreeningDisposition
		assessedAt  time.Time
		wantOutcome domain.EligibilityOutcome
		wantReason  domain.ReasonCode
		wantError   error
	}{
		{
			name:        "passed at exact freshness boundary",
			disposition: domain.RiskScreeningDispositionPassed,
			assessedAt:  evaluatedAt.Add(-maxAge),
			wantOutcome: domain.EligibilityOutcomeEligible,
			wantReason:  domain.ReasonRiskScreeningPassed,
		},
		{
			name:        "blocked at exact freshness boundary",
			disposition: domain.RiskScreeningDispositionBlocked,
			assessedAt:  evaluatedAt.Add(-maxAge),
			wantOutcome: domain.EligibilityOutcomeIneligible,
			wantReason:  domain.ReasonRiskScreeningBlocked,
		},
		{
			name:        "one nanosecond stale",
			disposition: domain.RiskScreeningDispositionPassed,
			assessedAt:  evaluatedAt.Add(-maxAge - time.Nanosecond),
			wantError:   ErrRiskScreeningFactStale,
		},
		{
			name:        "one nanosecond after shared as-of",
			disposition: domain.RiskScreeningDispositionPassed,
			assessedAt:  evaluatedAt.Add(time.Nanosecond),
			wantError:   ErrRiskScreeningFactInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact := riskAdmissionTestFact(t, 42, test.disposition, test.assessedAt)
			decision, err := evaluateRiskScreeningFactAt(
				context.Background(),
				42,
				policy,
				fact,
				evaluationInstant{value: evaluatedAt},
				maxAge,
			)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) || decision != (domain.RiskAdmissionDecision{}) {
					t.Fatalf("evaluateRiskScreeningFactAt() = %#v, %v; want zero and %v", decision, err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("evaluateRiskScreeningFactAt() error = %v", err)
			}
			if decision.Outcome() != test.wantOutcome || decision.ReasonCode() != test.wantReason {
				t.Fatalf("decision = outcome %q reason %q", decision.Outcome(), decision.ReasonCode())
			}
			if !decision.EvaluatedAt().Equal(evaluatedAt) || decision.RuleCode() != domain.RiskAdmissionRuleCode {
				t.Fatal("decision did not retain the controlled rule identity and shared as-of")
			}
		})
	}
}

func TestEvaluateRiskScreeningFactAtRejectsInvalidInputsWithoutDecision(t *testing.T) {
	evaluatedAt := applicationTestInstant()
	policy := riskAdmissionTestPolicy(t)
	validFact := riskAdmissionTestFact(
		t,
		42,
		domain.RiskScreeningDispositionPassed,
		evaluatedAt.Add(-time.Minute),
	)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name       string
		ctx        context.Context
		ref        domain.ParticipantRef
		policy     domain.RiskAdmissionPolicy
		fact       domain.RiskScreeningFactSnapshot
		instant    evaluationInstant
		maxFactAge time.Duration
		want       error
	}{
		{name: "zero fact", ctx: context.Background(), ref: 42, policy: policy, instant: evaluationInstant{value: evaluatedAt}, maxFactAge: time.Hour, want: ErrRiskScreeningFactInvalid},
		{name: "different participant", ctx: context.Background(), ref: 99, policy: policy, fact: validFact, instant: evaluationInstant{value: evaluatedAt}, maxFactAge: time.Hour, want: ErrRiskScreeningFactInvalid},
		{name: "zero instant", ctx: context.Background(), ref: 42, policy: policy, fact: validFact, maxFactAge: time.Hour, want: ErrEligibilityClockInvalid},
		{name: "non canonical instant", ctx: context.Background(), ref: 42, policy: policy, fact: validFact, instant: evaluationInstant{value: evaluatedAt.In(time.FixedZone("UTC+8", 8*60*60))}, maxFactAge: time.Hour, want: ErrEligibilityClockInvalid},
		{name: "zero maximum age", ctx: context.Background(), ref: 42, policy: policy, fact: validFact, instant: evaluationInstant{value: evaluatedAt}, want: ErrPrerequisiteChainNotConfigured},
		{name: "negative maximum age", ctx: context.Background(), ref: 42, policy: policy, fact: validFact, instant: evaluationInstant{value: evaluatedAt}, maxFactAge: -time.Nanosecond, want: ErrPrerequisiteChainNotConfigured},
		{name: "canceled before evaluator", ctx: canceled, ref: 42, policy: policy, fact: validFact, instant: evaluationInstant{value: evaluatedAt}, maxFactAge: time.Hour, want: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := evaluateRiskScreeningFactAt(
				test.ctx,
				test.ref,
				test.policy,
				test.fact,
				test.instant,
				test.maxFactAge,
			)
			if !errors.Is(err, test.want) || decision != (domain.RiskAdmissionDecision{}) {
				t.Fatalf("evaluateRiskScreeningFactAt() = %#v, %v; want zero and %v", decision, err, test.want)
			}
		})
	}
}

func TestReadRiskScreeningFactClassifiesFailuresSafely(t *testing.T) {
	base := applicationTestInstant()
	secret := errors.New("secret risk endpoint and subject detail")
	secretDeadline := fmt.Errorf("secret risk provider detail: %w", context.DeadlineExceeded)
	tests := []struct {
		name      string
		readerErr error
		wantClass error
		wantCause error
	}{
		{name: "not found", readerErr: WrapRiskScreeningFactReadError(ErrRiskScreeningFactNotFound, secret), wantClass: ErrRiskScreeningFactNotFound, wantCause: secret},
		{name: "unavailable", readerErr: WrapRiskScreeningFactReadError(ErrRiskScreeningFactUnavailable, secret), wantClass: ErrRiskScreeningFactUnavailable, wantCause: secret},
		{name: "classified failure", readerErr: WrapRiskScreeningFactReadError(ErrRiskScreeningFactReadFailure, secret), wantClass: ErrRiskScreeningFactReadFailure, wantCause: secret},
		{name: "unknown failure", readerErr: secret, wantClass: ErrRiskScreeningFactReadFailure, wantCause: secret},
		{name: "provider deadline", readerErr: context.DeadlineExceeded, wantClass: ErrRiskScreeningFactUnavailable, wantCause: context.DeadlineExceeded},
		{name: "wrapped provider deadline", readerErr: secretDeadline, wantClass: ErrRiskScreeningFactUnavailable, wantCause: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &riskFactReaderStub{
				fact: riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, base),
				err:  test.readerErr,
			}
			fact, err := readRiskScreeningFact(context.Background(), reader, 42)
			if fact != (domain.RiskScreeningFactSnapshot{}) || !errors.Is(err, test.wantClass) {
				t.Fatalf("readRiskScreeningFact() = %#v, %v; want zero and %v", fact, err, test.wantClass)
			}
			var readError *RiskScreeningFactReadError
			if !errors.As(err, &readError) {
				t.Fatalf("error type = %T, want *RiskScreeningFactReadError", err)
			}
			if !riskDiagnosticContains(readError, test.wantCause) {
				t.Fatalf("explicit diagnostic cause does not contain %v", test.wantCause)
			}
			if errors.Is(err, secret) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("diagnostic cause leaked into the public errors.Is tree")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("safe error rendered diagnostic cause: %q", err.Error())
			}
			if reader.calls != 1 || reader.ref != 42 {
				t.Fatalf("reader calls/ref = %d/%d", reader.calls, reader.ref)
			}
		})
	}
}

func TestReadRiskScreeningFactMakesCallerCancellationWin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &riskFactReaderStub{
		err:       errors.New("dependency failure after caller cancellation"),
		afterRead: cancel,
	}

	fact, err := readRiskScreeningFact(ctx, reader, 42)
	if !errors.Is(err, context.Canceled) || fact != (domain.RiskScreeningFactSnapshot{}) {
		t.Fatalf("readRiskScreeningFact() = %#v, %v; want zero and canceled", fact, err)
	}
}

func TestRiskScreeningFactReadErrorHasOnePublicClassAndExplicitCause(t *testing.T) {
	secret := errors.New("secret diagnostic")
	err := WrapRiskScreeningFactReadError(ErrRiskScreeningFactUnavailable, secret)
	if !errors.Is(err, ErrRiskScreeningFactUnavailable) {
		t.Fatal("risk wrapper lost its stable class")
	}
	if errors.Is(err, secret) || err.Cause() != secret {
		t.Fatal("risk diagnostic cause was not isolated behind Cause()")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("risk wrapper rendered its diagnostic cause")
	}

	unknown := WrapRiskScreeningFactReadError(errors.New("unknown class"), secret)
	if !errors.Is(unknown, ErrRiskScreeningFactReadFailure) || errors.Is(unknown, ErrRiskScreeningFactUnavailable) {
		t.Fatal("unknown risk class did not collapse to exactly read failure")
	}
	var zero RiskScreeningFactReadError
	if !errors.Is(&zero, ErrRiskScreeningFactReadFailure) || zero.Cause() != nil {
		t.Fatal("zero risk wrapper is not fail-closed")
	}
	var typedNil *RiskScreeningFactReadError
	if typedNil.Error() != ErrRiskScreeningFactReadFailure.Error() ||
		!errors.Is(typedNil, ErrRiskScreeningFactReadFailure) ||
		typedNil.Cause() != nil {
		t.Fatal("typed-nil risk wrapper is not fail-closed")
	}
}

type riskFactReaderStub struct {
	fact      domain.RiskScreeningFactSnapshot
	err       error
	afterRead func()
	calls     int
	ref       domain.ParticipantRef
}

func (reader *riskFactReaderStub) FindRiskScreeningFact(
	_ context.Context,
	participantRef domain.ParticipantRef,
) (domain.RiskScreeningFactSnapshot, error) {
	reader.calls++
	reader.ref = participantRef
	if reader.afterRead != nil {
		reader.afterRead()
	}
	return reader.fact, reader.err
}

func riskAdmissionTestPolicy(t *testing.T) domain.RiskAdmissionPolicy {
	t.Helper()
	policy, err := domain.NewRiskAdmissionPolicy("risk-admission-policy-v1")
	if err != nil {
		t.Fatalf("construct risk policy: %v", err)
	}
	return policy
}

func riskAdmissionTestFact(
	t *testing.T,
	participantRef domain.ParticipantRef,
	disposition domain.RiskScreeningDisposition,
	assessedAt time.Time,
) domain.RiskScreeningFactSnapshot {
	t.Helper()
	fact, err := domain.NewRiskScreeningFactSnapshot(
		participantRef,
		disposition,
		assessedAt,
		"risk-screening-service",
		"risk-screening:9001",
	)
	if err != nil {
		t.Fatalf("construct risk fact: %v", err)
	}
	return fact
}

func riskDiagnosticContains(readError *RiskScreeningFactReadError, target error) bool {
	for readError != nil {
		cause := readError.Cause()
		if errors.Is(cause, target) {
			return true
		}
		nested, ok := cause.(*RiskScreeningFactReadError)
		if !ok {
			return false
		}
		readError = nested
	}
	return false
}
