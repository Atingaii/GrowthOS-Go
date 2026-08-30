package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

func TestNewUserEligibilityServiceReturnsConfirmedBoundaryDecisions(t *testing.T) {
	cutoff := applicationTestInstant()
	evaluatedAt := cutoff.Add(2 * time.Hour)
	policy := applicationTestPolicy(t, cutoff)
	tests := []struct {
		name         string
		registeredAt time.Time
		outcome      domain.EligibilityOutcome
		reason       domain.ReasonCode
	}{
		{name: "before cutoff", registeredAt: cutoff.Add(-time.Nanosecond), outcome: domain.EligibilityOutcomeIneligible, reason: domain.ReasonRegistrationBeforeCutoff},
		{name: "at cutoff", registeredAt: cutoff, outcome: domain.EligibilityOutcomeEligible, reason: domain.ReasonRegistrationOnOrAfterCutoff},
		{name: "after cutoff", registeredAt: cutoff.Add(time.Nanosecond), outcome: domain.EligibilityOutcomeEligible, reason: domain.ReasonRegistrationOnOrAfterCutoff},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact := applicationTestFact(t, 42, test.registeredAt, evaluatedAt.Add(-time.Minute))
			reader := &factReaderStub{fact: fact}
			clock := &clockStub{now: evaluatedAt}
			service := applicationTestService(t, reader, clock, time.Hour)

			decision, err := service.Evaluate(context.Background(), 42, policy)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if decision.Outcome() != test.outcome || decision.ReasonCode() != test.reason {
				t.Fatalf("decision = outcome %q reason %q", decision.Outcome(), decision.ReasonCode())
			}
			if reader.calls != 1 || reader.ref != 42 || clock.calls != 1 {
				t.Fatalf("dependencies = reader calls/ref %d/%d, clock %d", reader.calls, reader.ref, clock.calls)
			}
		})
	}
}

func TestNewUserEligibilityServiceEnforcesFreshnessBeforeBusinessDecision(t *testing.T) {
	cutoff := applicationTestInstant()
	evaluatedAt := cutoff.Add(24 * time.Hour)
	policy := applicationTestPolicy(t, cutoff)
	maxAge := 15 * time.Minute
	tests := []struct {
		name        string
		observedAt  time.Time
		wantError   error
		wantOutcome domain.EligibilityOutcome
	}{
		{name: "exact maximum age remains valid", observedAt: evaluatedAt.Add(-maxAge), wantOutcome: domain.EligibilityOutcomeIneligible},
		{name: "one nanosecond stale has no decision", observedAt: evaluatedAt.Add(-maxAge - time.Nanosecond), wantError: ErrRegistrationFactStale},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// This old registration would become a confirmed business rejection if
			// freshness were accidentally checked after the cutoff decision.
			fact := applicationTestFact(t, 42, cutoff.Add(-24*time.Hour), test.observedAt)
			reader := &factReaderStub{fact: fact}
			clock := &clockStub{now: evaluatedAt}
			service := applicationTestService(t, reader, clock, maxAge)

			decision, err := service.Evaluate(context.Background(), 42, policy)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) || decision != (domain.NewUserEligibilityDecision{}) {
					t.Fatalf("Evaluate() = %#v, %v; want zero and %v", decision, err, test.wantError)
				}
			} else if err != nil || decision.Outcome() != test.wantOutcome {
				t.Fatalf("Evaluate() = %#v, %v; want %q", decision, err, test.wantOutcome)
			}
			if reader.calls != 1 || clock.calls != 1 {
				t.Fatalf("calls = reader %d, clock %d", reader.calls, clock.calls)
			}
		})
	}
}

func TestNewUserEligibilityServiceRejectsInvalidFactsWithoutBusinessDecision(t *testing.T) {
	cutoff := applicationTestInstant()
	evaluatedAt := cutoff.Add(time.Hour)
	policy := applicationTestPolicy(t, cutoff)
	tests := []struct {
		name string
		fact domain.RegistrationFactSnapshot
	}{
		{name: "zero fact"},
		{name: "different participant", fact: applicationTestFact(t, 99, cutoff.Add(-time.Hour), cutoff)},
		{name: "future registration", fact: applicationTestFact(t, 42, evaluatedAt.Add(time.Minute), evaluatedAt.Add(time.Minute))},
		{name: "future observation", fact: applicationTestFact(t, 42, cutoff, evaluatedAt.Add(time.Minute))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &factReaderStub{fact: test.fact}
			clock := &clockStub{now: evaluatedAt}
			service := applicationTestService(t, reader, clock, time.Hour)
			decision, err := service.Evaluate(context.Background(), 42, policy)
			if !errors.Is(err, ErrRegistrationFactInvalid) || decision != (domain.NewUserEligibilityDecision{}) {
				t.Fatalf("Evaluate() = %#v, %v; want zero invalid fact", decision, err)
			}
			if clock.calls != 1 {
				t.Fatalf("clock calls = %d, want one after successful read", clock.calls)
			}
		})
	}
}

func TestNewUserEligibilityServiceClassifiesFactReadFailuresWithoutCallingClock(t *testing.T) {
	cutoff := applicationTestInstant()
	policy := applicationTestPolicy(t, cutoff)
	secret := errors.New("secret SQL and upstream detail")
	secretProviderDeadline := fmt.Errorf("secret provider detail: %w", context.DeadlineExceeded)
	tests := []struct {
		name      string
		readerErr error
		want      error
		wantCause bool
	}{
		{name: "not found", readerErr: WrapRegistrationFactReadError(ErrRegistrationFactNotFound, secret), want: ErrRegistrationFactNotFound, wantCause: true},
		{name: "unavailable", readerErr: WrapRegistrationFactReadError(ErrRegistrationFactUnavailable, secret), want: ErrRegistrationFactUnavailable, wantCause: true},
		{name: "classified read failure", readerErr: WrapRegistrationFactReadError(ErrRegistrationFactReadFailure, secret), want: ErrRegistrationFactReadFailure, wantCause: true},
		{name: "unknown provider failure", readerErr: secret, want: ErrRegistrationFactReadFailure, wantCause: true},
		{name: "provider deadline without caller expiry", readerErr: context.DeadlineExceeded, want: ErrRegistrationFactUnavailable},
		{name: "wrapped provider deadline is safely rendered", readerErr: secretProviderDeadline, want: ErrRegistrationFactUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &factReaderStub{
				fact: applicationTestFact(t, 42, cutoff, cutoff),
				err:  test.readerErr,
			}
			clock := &clockStub{now: cutoff}
			service := applicationTestService(t, reader, clock, time.Hour)
			decision, err := service.Evaluate(context.Background(), 42, policy)
			if !errors.Is(err, test.want) || decision != (domain.NewUserEligibilityDecision{}) {
				t.Fatalf("Evaluate() = %#v, %v; want zero and %v", decision, err, test.want)
			}
			var readError *RegistrationFactReadError
			if !errors.As(err, &readError) {
				t.Fatalf("error type = %T, want *RegistrationFactReadError", err)
			}
			if test.wantCause && !registrationDiagnosticContains(readError, secret) {
				t.Fatal("safe application error lost the explicit diagnostic cause")
			}
			if errors.Is(err, secret) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("diagnostic cause leaked into the public errors.Is tree")
			}
			if errors.Is(test.readerErr, context.DeadlineExceeded) && !errors.Is(readError.Cause(), context.DeadlineExceeded) {
				t.Fatal("provider timeout classification lost its explicit diagnostic cause")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("public error rendered secret cause: %q", err.Error())
			}
			if clock.calls != 0 {
				t.Fatalf("clock calls = %d, want zero after reader error", clock.calls)
			}
		})
	}
}

func TestNewUserEligibilityServiceRejectsInvalidArgumentsAndDependenciesEarly(t *testing.T) {
	cutoff := applicationTestInstant()
	policy := applicationTestPolicy(t, cutoff)
	fact := applicationTestFact(t, 42, cutoff, cutoff)
	reader := &factReaderStub{fact: fact}
	clock := &clockStub{now: cutoff}
	service := applicationTestService(t, reader, clock, time.Hour)

	tests := []struct {
		name   string
		ctx    context.Context
		ref    domain.ParticipantRef
		policy domain.NewUserPolicy
	}{
		{name: "nil context", ref: 42, policy: policy},
		{name: "zero participant", ctx: context.Background(), policy: policy},
		{name: "zero policy", ctx: context.Background(), ref: 42},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := service.Evaluate(test.ctx, test.ref, test.policy)
			if !errors.Is(err, ErrEligibilityInvalidArgument) || decision != (domain.NewUserEligibilityDecision{}) {
				t.Fatalf("Evaluate() = %#v, %v; want invalid argument", decision, err)
			}
		})
	}
	if reader.calls != 0 || clock.calls != 0 {
		t.Fatal("invalid arguments reached a dependency")
	}

	var typedNilReader *factReaderStub
	var typedNilClock *clockStub
	var typedNilClockFunc ClockFunc
	dependencyTests := []struct {
		name   string
		reader RegistrationFactReader
		clock  Clock
		age    time.Duration
	}{
		{name: "nil reader", clock: clock, age: time.Minute},
		{name: "typed nil reader", reader: typedNilReader, clock: clock, age: time.Minute},
		{name: "nil clock", reader: reader, age: time.Minute},
		{name: "typed nil clock", reader: reader, clock: typedNilClock, age: time.Minute},
		{name: "typed nil clock function", reader: reader, clock: typedNilClockFunc, age: time.Minute},
		{name: "zero maximum age", reader: reader, clock: clock},
		{name: "negative maximum age", reader: reader, clock: clock, age: -time.Nanosecond},
	}
	for _, test := range dependencyTests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NewNewUserEligibilityService(test.reader, test.clock, test.age)
			if !errors.Is(err, ErrEligibilityNotConfigured) || got != nil {
				t.Fatalf("constructor = %#v, %v; want nil not configured", got, err)
			}
		})
	}

	var nilService *NewUserEligibilityService
	if err := nilService.Validate(); !errors.Is(err, ErrEligibilityNotConfigured) {
		t.Fatalf("nil Validate() error = %v", err)
	}
	if _, err := nilService.Evaluate(context.Background(), 42, policy); !errors.Is(err, ErrEligibilityNotConfigured) {
		t.Fatalf("nil service Evaluate() error = %v", err)
	}

	manuallyConstructedServices := []struct {
		name    string
		service *NewUserEligibilityService
	}{
		{name: "zero value", service: &NewUserEligibilityService{}},
		{
			name: "missing maximum age",
			service: &NewUserEligibilityService{
				facts: reader,
				clock: clock,
			},
		},
	}
	for _, test := range manuallyConstructedServices {
		t.Run("manually constructed "+test.name, func(t *testing.T) {
			if err := test.service.Validate(); !errors.Is(err, ErrEligibilityNotConfigured) {
				t.Fatalf("Validate() error = %v; want not configured", err)
			}
			decision, err := test.service.Evaluate(context.Background(), 42, policy)
			if !errors.Is(err, ErrEligibilityNotConfigured) || decision != (domain.NewUserEligibilityDecision{}) {
				t.Fatalf("Evaluate() = %#v, %v; want zero decision and not configured", decision, err)
			}
		})
	}
}

func TestNewUserEligibilityServiceMakesObservedCancellationWin(t *testing.T) {
	base := applicationTestInstant()
	policy := applicationTestPolicy(t, base)
	fact := applicationTestFact(t, 42, base, base)

	preCanceled, cancelPre := context.WithCancel(context.Background())
	cancelPre()
	reader := &factReaderStub{fact: fact}
	clock := &clockStub{now: base}
	service := applicationTestService(t, reader, clock, time.Hour)
	if decision, err := service.Evaluate(preCanceled, 42, policy); !errors.Is(err, context.Canceled) || decision != (domain.NewUserEligibilityDecision{}) {
		t.Fatalf("pre-canceled Evaluate() = %#v, %v", decision, err)
	}
	if reader.calls != 0 || clock.calls != 0 {
		t.Fatal("pre-canceled call reached dependencies")
	}

	readerContext, cancelReader := context.WithCancel(context.Background())
	reader = &factReaderStub{err: errors.New("dependency completed after cancel"), afterRead: cancelReader}
	clock = &clockStub{now: base}
	service = applicationTestService(t, reader, clock, time.Hour)
	if decision, err := service.Evaluate(readerContext, 42, policy); !errors.Is(err, context.Canceled) || decision != (domain.NewUserEligibilityDecision{}) {
		t.Fatalf("reader cancellation Evaluate() = %#v, %v", decision, err)
	}
	if clock.calls != 0 {
		t.Fatal("reader cancellation reached clock")
	}

	clockContext, cancelClock := context.WithCancel(context.Background())
	reader = &factReaderStub{fact: fact}
	clock = &clockStub{now: base, afterNow: cancelClock}
	service = applicationTestService(t, reader, clock, time.Hour)
	if decision, err := service.Evaluate(clockContext, 42, policy); !errors.Is(err, context.Canceled) || decision != (domain.NewUserEligibilityDecision{}) {
		t.Fatalf("clock cancellation Evaluate() = %#v, %v", decision, err)
	}
	if reader.calls != 1 || clock.calls != 1 {
		t.Fatalf("clock cancellation calls = reader %d clock %d", reader.calls, clock.calls)
	}
}

func TestNewUserEligibilityServiceHandlesBlockingReaderCancellation(t *testing.T) {
	base := applicationTestInstant()
	policy := applicationTestPolicy(t, base)
	started := make(chan struct{})
	release := make(chan struct{})
	reader := &blockingFactReader{
		started: started,
		release: release,
		err:     errors.New("reader failure after release"),
	}
	clock := &clockStub{now: base}
	service := applicationTestService(t, reader, clock, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.Evaluate(ctx, 42, policy)
		result <- err
	}()
	<-started
	cancel()
	close(release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("blocking reader error = %v, want canceled", err)
	}
	if clock.calls != 0 {
		t.Fatal("blocking reader cancellation reached clock")
	}
}

func TestNewUserEligibilityServiceRejectsInvalidClockWithoutDecision(t *testing.T) {
	base := applicationTestInstant()
	reader := &factReaderStub{fact: applicationTestFact(t, 42, base, base)}
	clock := &clockStub{}
	service := applicationTestService(t, reader, clock, time.Hour)
	decision, err := service.Evaluate(context.Background(), 42, applicationTestPolicy(t, base))
	if !errors.Is(err, ErrEligibilityClockInvalid) || decision != (domain.NewUserEligibilityDecision{}) {
		t.Fatalf("Evaluate() = %#v, %v; want zero invalid clock", decision, err)
	}
	if reader.calls != 1 || clock.calls != 1 {
		t.Fatalf("calls = reader %d clock %d", reader.calls, clock.calls)
	}
}

func TestClockFuncAdaptsControlledServerClock(t *testing.T) {
	want := applicationTestInstant()
	clock := ClockFunc(func() time.Time { return want })
	if got := clock.Now(); !got.Equal(want) {
		t.Fatalf("ClockFunc.Now() = %v, want %v", got, want)
	}
}

func TestNewUserEligibilityServiceSupportsConcurrentReadOnlyEvaluation(t *testing.T) {
	base := applicationTestInstant()
	fact := applicationTestFact(t, 42, base, base.Add(time.Minute))
	reader := &concurrentFactReader{fact: fact}
	clock := concurrentClock{now: base.Add(2 * time.Minute)}
	service := applicationTestService(t, reader, clock, time.Hour)
	policy := applicationTestPolicy(t, base)

	const workers = 64
	var waitGroup sync.WaitGroup
	results := make(chan domain.NewUserEligibilityDecision, workers)
	errorsSeen := make(chan error, workers)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			decision, err := service.Evaluate(context.Background(), 42, policy)
			results <- decision
			errorsSeen <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent Evaluate() error = %v", err)
		}
	}
	var first domain.NewUserEligibilityDecision
	for decision := range results {
		if first == (domain.NewUserEligibilityDecision{}) {
			first = decision
			continue
		}
		if !reflect.DeepEqual(first, decision) {
			t.Fatalf("concurrent decisions differ: %#v vs %#v", first, decision)
		}
	}
	if reader.Calls() != workers {
		t.Fatalf("reader calls = %d, want %d", reader.Calls(), workers)
	}
}

type factReaderStub struct {
	fact      domain.RegistrationFactSnapshot
	err       error
	afterRead func()
	calls     int
	ref       domain.ParticipantRef
}

func (reader *factReaderStub) FindRegistrationFact(
	_ context.Context,
	participantRef domain.ParticipantRef,
) (domain.RegistrationFactSnapshot, error) {
	reader.calls++
	reader.ref = participantRef
	if reader.afterRead != nil {
		reader.afterRead()
	}
	return reader.fact, reader.err
}

type blockingFactReader struct {
	started chan<- struct{}
	release <-chan struct{}
	err     error
}

func (reader *blockingFactReader) FindRegistrationFact(
	context.Context,
	domain.ParticipantRef,
) (domain.RegistrationFactSnapshot, error) {
	close(reader.started)
	<-reader.release
	return domain.RegistrationFactSnapshot{}, reader.err
}

type clockStub struct {
	now      time.Time
	afterNow func()
	calls    int
}

func (clock *clockStub) Now() time.Time {
	clock.calls++
	if clock.afterNow != nil {
		clock.afterNow()
	}
	return clock.now
}

type concurrentFactReader struct {
	mu    sync.Mutex
	fact  domain.RegistrationFactSnapshot
	calls int
}

func (reader *concurrentFactReader) FindRegistrationFact(
	context.Context,
	domain.ParticipantRef,
) (domain.RegistrationFactSnapshot, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.calls++
	return reader.fact, nil
}

func (reader *concurrentFactReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

type concurrentClock struct{ now time.Time }

func (clock concurrentClock) Now() time.Time { return clock.now }

func applicationTestService(
	t *testing.T,
	reader RegistrationFactReader,
	clock Clock,
	maxFactAge time.Duration,
) *NewUserEligibilityService {
	t.Helper()
	service, err := NewNewUserEligibilityService(reader, clock, maxFactAge)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	return service
}

func applicationTestPolicy(t *testing.T, cutoff time.Time) domain.NewUserPolicy {
	t.Helper()
	policy, err := domain.NewNewUserPolicy("new-user-policy-v1", cutoff)
	if err != nil {
		t.Fatalf("construct policy: %v", err)
	}
	return policy
}

func applicationTestFact(
	t *testing.T,
	participantRef domain.ParticipantRef,
	registeredAt time.Time,
	observedAt time.Time,
) domain.RegistrationFactSnapshot {
	t.Helper()
	fact, err := domain.NewRegistrationFactSnapshot(
		participantRef,
		registeredAt,
		observedAt,
		"account-directory",
		"registration-event:9001",
	)
	if err != nil {
		t.Fatalf("construct fact: %v", err)
	}
	return fact
}

func applicationTestInstant() time.Time {
	return time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
}

func registrationDiagnosticContains(readError *RegistrationFactReadError, target error) bool {
	for readError != nil {
		cause := readError.Cause()
		if errors.Is(cause, target) {
			return true
		}
		nested, ok := cause.(*RegistrationFactReadError)
		if !ok {
			return false
		}
		readError = nested
	}
	return false
}
