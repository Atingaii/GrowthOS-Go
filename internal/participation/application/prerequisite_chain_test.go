package application

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/participation/domain"
)

func TestEligibilityPrerequisiteChainExecutesFixedOrderAndShortCircuits(t *testing.T) {
	asOf := applicationTestInstant()
	newUserPolicy := applicationTestPolicy(t, asOf.Add(-24*time.Hour))
	riskPolicy := riskAdmissionTestPolicy(t)
	ruleSetRevision := prerequisiteTestRuleSetRevision(t)
	maxAge := 15 * time.Minute
	tests := []struct {
		name             string
		registrationFact domain.RegistrationFactSnapshot
		riskFact         domain.RiskScreeningFactSnapshot
		wantOutcome      domain.EligibilityOutcome
		wantReason       domain.ReasonCode
		wantSteps        int
		wantRiskCalls    int
	}{
		{
			name:             "both prerequisites pass",
			registrationFact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-5*time.Minute)),
			riskFact:         riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-2*time.Minute)),
			wantOutcome:      domain.EligibilityOutcomeEligible,
			wantReason:       domain.ReasonAllPrerequisitesSatisfied,
			wantSteps:        2,
			wantRiskCalls:    1,
		},
		{
			name:             "new user rejection skips risk authority",
			registrationFact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour-time.Nanosecond), asOf.Add(-5*time.Minute)),
			riskFact:         riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-2*time.Minute)),
			wantOutcome:      domain.EligibilityOutcomeIneligible,
			wantReason:       domain.ReasonRegistrationBeforeCutoff,
			wantSteps:        1,
		},
		{
			name:             "risk rejection terminates second step",
			registrationFact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-5*time.Minute)),
			riskFact:         riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionBlocked, asOf.Add(-2*time.Minute)),
			wantOutcome:      domain.EligibilityOutcomeIneligible,
			wantReason:       domain.ReasonRiskScreeningBlocked,
			wantSteps:        2,
			wantRiskCalls:    1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registrationReader := &chainRegistrationReader{fact: test.registrationFact}
			riskReader := &chainRiskReader{fact: test.riskFact}
			clock := &chainClock{now: asOf}
			chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, maxAge, maxAge)

			evaluation, err := chain.Evaluate(
				context.Background(),
				42,
				ruleSetRevision,
				newUserPolicy,
				riskPolicy,
			)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if evaluation.Outcome() != test.wantOutcome || evaluation.ReasonCode() != test.wantReason {
				t.Fatalf("evaluation = outcome %q reason %q", evaluation.Outcome(), evaluation.ReasonCode())
			}
			if !evaluation.Confirmed() {
				t.Fatal("nil-error business evaluation is not marked confirmed")
			}
			if evaluation.RuleSetRevision() != ruleSetRevision || !evaluation.EvaluatedAt().Equal(asOf) {
				t.Fatal("aggregate lost its ruleset revision or shared as-of")
			}
			steps := evaluation.Steps()
			if len(steps) != test.wantSteps {
				t.Fatalf("trace length = %d, want %d", len(steps), test.wantSteps)
			}
			if steps[0].RuleCode() != domain.NewUserRuleCode || !steps[0].EvaluatedAt().Equal(asOf) {
				t.Fatal("first trace step is not the new-user decision at shared as-of")
			}
			if steps[0].PolicyRevision() != newUserPolicy.Revision() ||
				steps[0].FactSource() != test.registrationFact.Source() ||
				steps[0].FactRevision() != test.registrationFact.Revision() {
				t.Fatal("first trace step lost bounded policy or registration provenance")
			}
			if len(steps) == 2 && (steps[1].RuleCode() != domain.RiskAdmissionRuleCode || !steps[1].EvaluatedAt().Equal(asOf)) {
				t.Fatal("second trace step is not the risk decision at shared as-of")
			}
			if len(steps) == 2 && (steps[1].PolicyRevision() != riskPolicy.Revision() ||
				steps[1].FactSource() != test.riskFact.Source() ||
				steps[1].FactRevision() != test.riskFact.Revision()) {
				t.Fatal("second trace step lost bounded policy or risk provenance")
			}
			if registrationReader.Calls() != 1 || riskReader.Calls() != test.wantRiskCalls || clock.Calls() != 1 {
				t.Fatalf(
					"calls = registration %d risk %d clock %d; want 1/%d/1",
					registrationReader.Calls(),
					riskReader.Calls(),
					clock.Calls(),
					test.wantRiskCalls,
				)
			}
		})
	}
}

func TestEligibilityPrerequisiteChainUsesOneCanonicalAsOfAndOwnsTrace(t *testing.T) {
	asOfUTC := applicationTestInstant()
	asOfShanghai := asOfUTC.In(time.FixedZone("UTC+8", 8*60*60))
	clock := &chainClock{now: asOfShanghai}
	registrationReader := &chainRegistrationReader{
		fact: applicationTestFact(t, 42, asOfUTC.Add(-24*time.Hour), asOfUTC.Add(-time.Minute)),
		afterRead: func() {
			clock.SetNow(asOfUTC.Add(12 * time.Hour))
		},
	}
	riskReader := &chainRiskReader{
		fact: riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOfUTC.Add(-time.Minute)),
	}
	chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, time.Hour, time.Hour)

	evaluation, err := chain.Evaluate(
		context.Background(),
		42,
		prerequisiteTestRuleSetRevision(t),
		applicationTestPolicy(t, asOfUTC.Add(-24*time.Hour)),
		riskAdmissionTestPolicy(t),
	)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if clock.Calls() != 1 {
		t.Fatalf("clock calls = %d, want exactly one", clock.Calls())
	}
	if evaluation.EvaluatedAt().Location() != time.UTC || !evaluation.EvaluatedAt().Equal(asOfUTC) {
		t.Fatalf("aggregate evaluated-at = %v, want canonical %v", evaluation.EvaluatedAt(), asOfUTC)
	}
	steps := evaluation.Steps()
	if len(steps) != 2 || !steps[0].EvaluatedAt().Equal(asOfUTC) || !steps[1].EvaluatedAt().Equal(asOfUTC) {
		t.Fatal("steps did not share the one captured as-of")
	}
	originalFirst := steps[0]
	steps[0] = EligibilityTraceStep{}
	reloaded := evaluation.Steps()
	if len(reloaded) != 2 || !reflect.DeepEqual(reloaded[0], originalFirst) {
		t.Fatal("mutating Steps() result rewrote stored trace evidence")
	}
}

func TestEligibilityPrerequisiteChainTechnicalFailuresReturnZeroAggregate(t *testing.T) {
	asOf := applicationTestInstant()
	maxAge := 10 * time.Minute
	validRegistration := applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-time.Minute))
	validRisk := riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute))
	tests := []struct {
		name                 string
		registrationFact     domain.RegistrationFactSnapshot
		registrationError    error
		riskFact             domain.RiskScreeningFactSnapshot
		riskError            error
		want                 error
		wantRegistrationCall int
		wantRiskCalls        int
	}{
		{name: "registration fact plus not-found error still fails", registrationFact: validRegistration, registrationError: ErrRegistrationFactNotFound, riskFact: validRisk, want: ErrRegistrationFactNotFound, wantRegistrationCall: 1},
		{name: "registration stale", registrationFact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-maxAge-time.Nanosecond)), riskFact: validRisk, want: ErrRegistrationFactStale, wantRegistrationCall: 1},
		{name: "registration after as-of", registrationFact: applicationTestFact(t, 42, asOf, asOf.Add(time.Nanosecond)), riskFact: validRisk, want: ErrRegistrationFactInvalid, wantRegistrationCall: 1},
		{name: "risk fact plus unavailable error still fails", registrationFact: validRegistration, riskFact: validRisk, riskError: ErrRiskScreeningFactUnavailable, want: ErrRiskScreeningFactUnavailable, wantRegistrationCall: 1, wantRiskCalls: 1},
		{name: "risk stale", registrationFact: validRegistration, riskFact: riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-maxAge-time.Nanosecond)), want: ErrRiskScreeningFactStale, wantRegistrationCall: 1, wantRiskCalls: 1},
		{name: "risk after as-of", registrationFact: validRegistration, riskFact: riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(time.Nanosecond)), want: ErrRiskScreeningFactInvalid, wantRegistrationCall: 1, wantRiskCalls: 1},
		{name: "risk participant mismatch", registrationFact: validRegistration, riskFact: riskAdmissionTestFact(t, 99, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute)), want: ErrRiskScreeningFactInvalid, wantRegistrationCall: 1, wantRiskCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registrationReader := &chainRegistrationReader{fact: test.registrationFact, err: test.registrationError}
			riskReader := &chainRiskReader{fact: test.riskFact, err: test.riskError}
			clock := &chainClock{now: asOf}
			chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, maxAge, maxAge)

			evaluation, err := chain.Evaluate(
				context.Background(),
				42,
				prerequisiteTestRuleSetRevision(t),
				applicationTestPolicy(t, asOf.Add(-24*time.Hour)),
				riskAdmissionTestPolicy(t),
			)
			if !errors.Is(err, test.want) {
				t.Fatalf("Evaluate() error = %v, want %v", err, test.want)
			}
			assertZeroPrerequisiteEvaluation(t, evaluation)
			if registrationReader.Calls() != test.wantRegistrationCall || riskReader.Calls() != test.wantRiskCalls || clock.Calls() != 1 {
				t.Fatalf(
					"calls = registration %d risk %d clock %d; want %d/%d/1",
					registrationReader.Calls(),
					riskReader.Calls(),
					clock.Calls(),
					test.wantRegistrationCall,
					test.wantRiskCalls,
				)
			}
		})
	}
}

func TestEligibilityPrerequisiteChainRejectsInvalidClockBeforeReaders(t *testing.T) {
	asOf := applicationTestInstant()
	registrationReader := &chainRegistrationReader{
		fact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-time.Minute)),
	}
	riskReader := &chainRiskReader{
		fact: riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute)),
	}
	clock := &chainClock{}
	chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, time.Hour, time.Hour)

	evaluation, err := chain.Evaluate(
		context.Background(),
		42,
		prerequisiteTestRuleSetRevision(t),
		applicationTestPolicy(t, asOf.Add(-24*time.Hour)),
		riskAdmissionTestPolicy(t),
	)
	if !errors.Is(err, ErrEligibilityClockInvalid) {
		t.Fatalf("Evaluate() error = %v, want invalid clock", err)
	}
	assertZeroPrerequisiteEvaluation(t, evaluation)
	if clock.Calls() != 1 || registrationReader.Calls() != 0 || riskReader.Calls() != 0 {
		t.Fatalf(
			"calls = clock %d registration %d risk %d; want 1/0/0",
			clock.Calls(), registrationReader.Calls(), riskReader.Calls(),
		)
	}
}

func TestEligibilityPrerequisiteChainDoesNotRecaptureClockWhileRiskReaderBlocks(t *testing.T) {
	asOf := applicationTestInstant()
	started := make(chan struct{})
	release := make(chan struct{})
	registrationReader := &chainRegistrationReader{
		fact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-time.Minute)),
	}
	riskReader := &blockingChainRiskReader{
		fact:    riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute)),
		started: started,
		release: release,
	}
	clock := &chainClock{now: asOf}
	chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, time.Hour, time.Hour)
	ruleSet := prerequisiteTestRuleSetRevision(t)
	newUserPolicy := applicationTestPolicy(t, asOf.Add(-24*time.Hour))
	riskPolicy := riskAdmissionTestPolicy(t)
	type evaluationResult struct {
		evaluation PrerequisiteEvaluation
		err        error
	}
	result := make(chan evaluationResult, 1)
	go func() {
		evaluation, err := chain.Evaluate(
			context.Background(),
			42,
			ruleSet,
			newUserPolicy,
			riskPolicy,
		)
		result <- evaluationResult{evaluation: evaluation, err: err}
	}()

	select {
	case <-started:
	case early := <-result:
		t.Fatalf("Evaluate() returned before risk reader blocked: %#v, %v", early.evaluation, early.err)
	case <-time.After(2 * time.Second):
		t.Fatal("Evaluate() did not reach the blocking risk reader")
	}
	if clock.Calls() != 1 || registrationReader.Calls() != 1 || riskReader.Calls() != 1 {
		t.Fatalf(
			"calls while risk blocked = clock %d registration %d risk %d; want 1/1/1",
			clock.Calls(), registrationReader.Calls(), riskReader.Calls(),
		)
	}
	clock.SetNow(asOf.Add(12 * time.Hour))
	close(release)
	var got evaluationResult
	select {
	case got = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("Evaluate() did not complete after releasing risk reader")
	}
	if got.err != nil {
		t.Fatalf("Evaluate() error = %v", got.err)
	}
	if !got.evaluation.Confirmed() || !got.evaluation.EvaluatedAt().Equal(asOf) || clock.Calls() != 1 {
		t.Fatal("blocking risk read caused the chain-wide as-of to drift or be recaptured")
	}
	for _, step := range got.evaluation.Steps() {
		if !step.EvaluatedAt().Equal(asOf) {
			t.Fatalf("step %q evaluated-at = %v, want %v", step.RuleCode(), step.EvaluatedAt(), asOf)
		}
	}
}

func TestEligibilityPrerequisiteChainMakesCancellationWinAtEveryBoundary(t *testing.T) {
	asOf := applicationTestInstant()
	validRegistration := applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-time.Minute))
	validRisk := riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute))
	tests := []struct {
		name                  string
		prepare               func() (context.Context, *chainRegistrationReader, *chainRiskReader, *chainClock)
		wantClockCalls        int
		wantRegistrationCalls int
		wantRiskCalls         int
	}{
		{
			name: "pre-canceled",
			prepare: func() (context.Context, *chainRegistrationReader, *chainRiskReader, *chainClock) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, &chainRegistrationReader{fact: validRegistration}, &chainRiskReader{fact: validRisk}, &chainClock{now: asOf}
			},
		},
		{
			name: "canceled by clock",
			prepare: func() (context.Context, *chainRegistrationReader, *chainRiskReader, *chainClock) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, &chainRegistrationReader{fact: validRegistration}, &chainRiskReader{fact: validRisk}, &chainClock{now: asOf, afterNow: cancel}
			},
			wantClockCalls: 1,
		},
		{
			name: "canceled by registration reader",
			prepare: func() (context.Context, *chainRegistrationReader, *chainRiskReader, *chainClock) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, &chainRegistrationReader{fact: validRegistration, afterRead: cancel}, &chainRiskReader{fact: validRisk}, &chainClock{now: asOf}
			},
			wantClockCalls:        1,
			wantRegistrationCalls: 1,
		},
		{
			name: "canceled by risk reader",
			prepare: func() (context.Context, *chainRegistrationReader, *chainRiskReader, *chainClock) {
				ctx, cancel := context.WithCancel(context.Background())
				return ctx, &chainRegistrationReader{fact: validRegistration}, &chainRiskReader{fact: validRisk, afterRead: cancel}, &chainClock{now: asOf}
			},
			wantClockCalls:        1,
			wantRegistrationCalls: 1,
			wantRiskCalls:         1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, registrationReader, riskReader, clock := test.prepare()
			chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, time.Hour, time.Hour)
			evaluation, err := chain.Evaluate(
				ctx,
				42,
				prerequisiteTestRuleSetRevision(t),
				applicationTestPolicy(t, asOf.Add(-24*time.Hour)),
				riskAdmissionTestPolicy(t),
			)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Evaluate() error = %v, want canceled", err)
			}
			assertZeroPrerequisiteEvaluation(t, evaluation)
			if clock.Calls() != test.wantClockCalls ||
				registrationReader.Calls() != test.wantRegistrationCalls ||
				riskReader.Calls() != test.wantRiskCalls {
				t.Fatalf(
					"calls = clock %d registration %d risk %d; want %d/%d/%d",
					clock.Calls(), registrationReader.Calls(), riskReader.Calls(),
					test.wantClockCalls, test.wantRegistrationCalls, test.wantRiskCalls,
				)
			}
		})
	}
}

func TestEligibilityPrerequisiteChainRejectsInvalidConfigurationBeforeIO(t *testing.T) {
	asOf := applicationTestInstant()
	validRegistration := applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-time.Minute))
	validRisk := riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute))
	validNewPolicy := applicationTestPolicy(t, asOf.Add(-24*time.Hour))
	validRiskPolicy := riskAdmissionTestPolicy(t)
	validRuleSet := prerequisiteTestRuleSetRevision(t)

	var typedNilRegistration *chainRegistrationReader
	var typedNilRisk *chainRiskReader
	var typedNilClock *chainClock
	var typedNilClockFunc ClockFunc
	constructorTests := []struct {
		name            string
		registration    RegistrationFactReader
		risk            RiskScreeningFactReader
		clock           Clock
		registrationAge time.Duration
		riskAge         time.Duration
	}{
		{name: "nil registration", risk: &chainRiskReader{}, clock: &chainClock{now: asOf}, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "typed nil registration", registration: typedNilRegistration, risk: &chainRiskReader{}, clock: &chainClock{now: asOf}, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "nil risk", registration: &chainRegistrationReader{}, clock: &chainClock{now: asOf}, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "typed nil risk", registration: &chainRegistrationReader{}, risk: typedNilRisk, clock: &chainClock{now: asOf}, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "nil clock", registration: &chainRegistrationReader{}, risk: &chainRiskReader{}, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "typed nil clock", registration: &chainRegistrationReader{}, risk: &chainRiskReader{}, clock: typedNilClock, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "typed nil clock function", registration: &chainRegistrationReader{}, risk: &chainRiskReader{}, clock: typedNilClockFunc, registrationAge: time.Minute, riskAge: time.Minute},
		{name: "zero registration age", registration: &chainRegistrationReader{}, risk: &chainRiskReader{}, clock: &chainClock{now: asOf}, riskAge: time.Minute},
		{name: "negative risk age", registration: &chainRegistrationReader{}, risk: &chainRiskReader{}, clock: &chainClock{now: asOf}, registrationAge: time.Minute, riskAge: -time.Nanosecond},
	}
	for _, test := range constructorTests {
		t.Run("constructor "+test.name, func(t *testing.T) {
			chain, err := NewEligibilityPrerequisiteChain(
				test.registration,
				test.risk,
				test.clock,
				test.registrationAge,
				test.riskAge,
			)
			if !errors.Is(err, ErrPrerequisiteChainNotConfigured) || chain != nil {
				t.Fatalf("constructor = %#v, %v; want nil not configured", chain, err)
			}
		})
	}

	var typedNilManualRisk *chainRiskReader
	manualChainTests := []struct {
		name   string
		mutate func(*EligibilityPrerequisiteChain)
	}{
		{
			name: "zero value",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				*chain = EligibilityPrerequisiteChain{}
			},
		},
		{
			name: "only registration reader",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				*chain = EligibilityPrerequisiteChain{
					registrationFacts:      chain.registrationFacts,
					maxRegistrationFactAge: time.Minute,
				}
			},
		},
		{
			name: "only risk reader",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				*chain = EligibilityPrerequisiteChain{
					riskScreeningFacts:  chain.riskScreeningFacts,
					maxRiskScreeningAge: time.Minute,
				}
			},
		},
		{
			name: "missing clock",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				chain.clock = nil
			},
		},
		{
			name: "typed nil risk reader field",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				chain.riskScreeningFacts = typedNilManualRisk
			},
		},
		{
			name: "zero registration age",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				chain.maxRegistrationFactAge = 0
			},
		},
		{
			name: "negative risk age",
			mutate: func(chain *EligibilityPrerequisiteChain) {
				chain.maxRiskScreeningAge = -time.Nanosecond
			},
		},
	}
	for _, test := range manualChainTests {
		t.Run("manual "+test.name, func(t *testing.T) {
			registrationReader := &chainRegistrationReader{fact: validRegistration}
			riskReader := &chainRiskReader{fact: validRisk}
			clock := &chainClock{now: asOf}
			chain := &EligibilityPrerequisiteChain{
				registrationFacts:      registrationReader,
				riskScreeningFacts:     riskReader,
				clock:                  clock,
				maxRegistrationFactAge: time.Minute,
				maxRiskScreeningAge:    time.Minute,
			}
			test.mutate(chain)

			evaluation, err := chain.Evaluate(
				context.Background(),
				42,
				validRuleSet,
				validNewPolicy,
				validRiskPolicy,
			)
			if !errors.Is(err, ErrPrerequisiteChainNotConfigured) {
				t.Fatalf("Evaluate() error = %v, want not configured", err)
			}
			assertZeroPrerequisiteEvaluation(t, evaluation)
			if registrationReader.Calls() != 0 || riskReader.Calls() != 0 || clock.Calls() != 0 {
				t.Fatalf(
					"invalid manual chain reached dependencies: registration %d risk %d clock %d",
					registrationReader.Calls(), riskReader.Calls(), clock.Calls(),
				)
			}
		})
	}

	argumentTests := []struct {
		name       string
		ctx        context.Context
		ref        domain.ParticipantRef
		ruleSet    domain.RuleSetRevision
		newPolicy  domain.NewUserPolicy
		riskPolicy domain.RiskAdmissionPolicy
	}{
		{name: "nil context", ref: 42, ruleSet: validRuleSet, newPolicy: validNewPolicy, riskPolicy: validRiskPolicy},
		{name: "zero participant", ctx: context.Background(), ruleSet: validRuleSet, newPolicy: validNewPolicy, riskPolicy: validRiskPolicy},
		{name: "zero ruleset", ctx: context.Background(), ref: 42, newPolicy: validNewPolicy, riskPolicy: validRiskPolicy},
		{name: "zero new user policy", ctx: context.Background(), ref: 42, ruleSet: validRuleSet, riskPolicy: validRiskPolicy},
		{name: "zero risk policy", ctx: context.Background(), ref: 42, ruleSet: validRuleSet, newPolicy: validNewPolicy},
	}
	for _, test := range argumentTests {
		t.Run("argument "+test.name, func(t *testing.T) {
			registrationReader := &chainRegistrationReader{fact: validRegistration}
			riskReader := &chainRiskReader{fact: validRisk}
			clock := &chainClock{now: asOf}
			chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, time.Hour, time.Hour)
			evaluation, err := chain.Evaluate(test.ctx, test.ref, test.ruleSet, test.newPolicy, test.riskPolicy)
			if !errors.Is(err, ErrPrerequisiteChainInvalidArgument) {
				t.Fatalf("Evaluate() error = %v, want invalid argument", err)
			}
			assertZeroPrerequisiteEvaluation(t, evaluation)
			if registrationReader.Calls() != 0 || riskReader.Calls() != 0 || clock.Calls() != 0 {
				t.Fatal("invalid argument reached a clock or reader")
			}
		})
	}

	var nilChain *EligibilityPrerequisiteChain
	evaluation, err := nilChain.Evaluate(
		context.Background(),
		42,
		validRuleSet,
		validNewPolicy,
		validRiskPolicy,
	)
	if !errors.Is(err, ErrPrerequisiteChainNotConfigured) {
		t.Fatalf("nil chain Evaluate() error = %v", err)
	}
	assertZeroPrerequisiteEvaluation(t, evaluation)
}

func TestEligibilityPrerequisiteChainSupportsConcurrentReadOnlyEvaluation(t *testing.T) {
	asOf := applicationTestInstant()
	registrationReader := &chainRegistrationReader{
		fact: applicationTestFact(t, 42, asOf.Add(-24*time.Hour), asOf.Add(-time.Minute)),
	}
	riskReader := &chainRiskReader{
		fact: riskAdmissionTestFact(t, 42, domain.RiskScreeningDispositionPassed, asOf.Add(-time.Minute)),
	}
	clock := &chainClock{now: asOf}
	chain := prerequisiteTestChain(t, registrationReader, riskReader, clock, time.Hour, time.Hour)
	ruleSet := prerequisiteTestRuleSetRevision(t)
	newPolicy := applicationTestPolicy(t, asOf.Add(-24*time.Hour))
	riskPolicy := riskAdmissionTestPolicy(t)

	const workers = 64
	var waitGroup sync.WaitGroup
	results := make(chan PrerequisiteEvaluation, workers)
	errorsSeen := make(chan error, workers)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			evaluation, err := chain.Evaluate(
				context.Background(),
				42,
				ruleSet,
				newPolicy,
				riskPolicy,
			)
			results <- evaluation
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
	var first PrerequisiteEvaluation
	for evaluation := range results {
		if first.Outcome() == "" {
			first = evaluation
			continue
		}
		if !reflect.DeepEqual(first, evaluation) {
			t.Fatalf("concurrent evaluations differ: %#v vs %#v", first, evaluation)
		}
	}
	if registrationReader.Calls() != workers || riskReader.Calls() != workers || clock.Calls() != workers {
		t.Fatalf(
			"concurrent calls = registration %d risk %d clock %d; want %d each",
			registrationReader.Calls(), riskReader.Calls(), clock.Calls(), workers,
		)
	}
}

type chainRegistrationReader struct {
	mu        sync.Mutex
	fact      domain.RegistrationFactSnapshot
	err       error
	afterRead func()
	calls     int
}

func (reader *chainRegistrationReader) FindRegistrationFact(
	context.Context,
	domain.ParticipantRef,
) (domain.RegistrationFactSnapshot, error) {
	reader.mu.Lock()
	reader.calls++
	fact, err, afterRead := reader.fact, reader.err, reader.afterRead
	reader.mu.Unlock()
	if afterRead != nil {
		afterRead()
	}
	return fact, err
}

func (reader *chainRegistrationReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

type chainRiskReader struct {
	mu        sync.Mutex
	fact      domain.RiskScreeningFactSnapshot
	err       error
	afterRead func()
	calls     int
}

type blockingChainRiskReader struct {
	mu      sync.Mutex
	fact    domain.RiskScreeningFactSnapshot
	started chan<- struct{}
	release <-chan struct{}
	calls   int
}

func (reader *blockingChainRiskReader) FindRiskScreeningFact(
	context.Context,
	domain.ParticipantRef,
) (domain.RiskScreeningFactSnapshot, error) {
	reader.mu.Lock()
	reader.calls++
	fact := reader.fact
	reader.mu.Unlock()
	close(reader.started)
	<-reader.release
	return fact, nil
}

func (reader *blockingChainRiskReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

func (reader *chainRiskReader) FindRiskScreeningFact(
	context.Context,
	domain.ParticipantRef,
) (domain.RiskScreeningFactSnapshot, error) {
	reader.mu.Lock()
	reader.calls++
	fact, err, afterRead := reader.fact, reader.err, reader.afterRead
	reader.mu.Unlock()
	if afterRead != nil {
		afterRead()
	}
	return fact, err
}

func (reader *chainRiskReader) Calls() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.calls
}

type chainClock struct {
	mu       sync.Mutex
	now      time.Time
	afterNow func()
	calls    int
}

func (clock *chainClock) Now() time.Time {
	clock.mu.Lock()
	clock.calls++
	now, afterNow := clock.now, clock.afterNow
	clock.mu.Unlock()
	if afterNow != nil {
		afterNow()
	}
	return now
}

func (clock *chainClock) Calls() int {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.calls
}

func (clock *chainClock) SetNow(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = now
}

func prerequisiteTestChain(
	t *testing.T,
	registrationFacts RegistrationFactReader,
	riskScreeningFacts RiskScreeningFactReader,
	clock Clock,
	maxRegistrationAge time.Duration,
	maxRiskAge time.Duration,
) *EligibilityPrerequisiteChain {
	t.Helper()
	chain, err := NewEligibilityPrerequisiteChain(
		registrationFacts,
		riskScreeningFacts,
		clock,
		maxRegistrationAge,
		maxRiskAge,
	)
	if err != nil {
		t.Fatalf("construct prerequisite chain: %v", err)
	}
	if err := chain.Validate(); err != nil {
		t.Fatalf("chain Validate() error = %v", err)
	}
	return chain
}

func prerequisiteTestRuleSetRevision(t *testing.T) domain.RuleSetRevision {
	t.Helper()
	revision, err := domain.NewRuleSetRevision("participation-prerequisites-v1")
	if err != nil {
		t.Fatalf("construct ruleset revision: %v", err)
	}
	return revision
}

func assertZeroPrerequisiteEvaluation(t *testing.T, evaluation PrerequisiteEvaluation) {
	t.Helper()
	if evaluation.Confirmed() ||
		evaluation.Outcome() != "" ||
		evaluation.ReasonCode() != "" ||
		evaluation.RuleSetRevision() != "" ||
		!evaluation.EvaluatedAt().IsZero() ||
		len(evaluation.Steps()) != 0 {
		t.Fatalf("evaluation = %#v, want zero aggregate", evaluation)
	}
}
