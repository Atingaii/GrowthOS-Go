package domain

import (
	"errors"
	"testing"
	"time"
)

func TestDecideActivityGateUsesConfirmedHalfOpenWindowOutcomes(t *testing.T) {
	start := testMarketingInstant().Add(time.Hour)
	end := start.Add(time.Hour)
	publication := mustTestReleasePublication(t, 1, 1, start, end, start.Add(-time.Hour))
	activity := mustTestPublishedActivity(t, 1, 1)
	tests := []struct {
		name   string
		at     time.Time
		status ActivityGateStatus
		allow  bool
	}{
		{name: "before start by microsecond", at: start.Add(-time.Microsecond), status: ActivityGateStatusScheduled},
		{name: "before start by nanosecond", at: start.Add(-time.Nanosecond), status: ActivityGateStatusScheduled},
		{name: "inclusive start", at: start, status: ActivityGateStatusActive, allow: true},
		{name: "inside", at: start.Add(time.Minute), status: ActivityGateStatusActive, allow: true},
		{name: "before end by nanosecond", at: end.Add(-time.Nanosecond), status: ActivityGateStatusActive, allow: true},
		{name: "exclusive end", at: end, status: ActivityGateStatusEnded},
		{name: "after end", at: end.Add(time.Microsecond), status: ActivityGateStatusEnded},
		{name: "same instant local zone", at: start.In(time.FixedZone("UTC+8", 8*60*60)), status: ActivityGateStatusActive, allow: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := DecideActivityGate(activity, publication, test.at)
			if err != nil {
				t.Fatalf("DecideActivityGate: %v", err)
			}
			if decision.Status() != test.status || decision.AllowsParticipation() != test.allow {
				t.Fatalf("decision = %q/%v, want %q/%v", decision.Status(), decision.AllowsParticipation(), test.status, test.allow)
			}
			if decision.ActivityID() != 1 || decision.PublicationVersion() != 1 ||
				decision.EvaluatedAt() != canonicalUTCInstant(test.at) {
				t.Fatalf(
					"trace = %d/%d/%v",
					decision.ActivityID(),
					decision.PublicationVersion(),
					decision.EvaluatedAt(),
				)
			}
			if err := decision.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestDecideActivityGateReturnsConfirmedNotPublishedForDraft(t *testing.T) {
	draft := mustTestActivity(t, 5)
	at := testMarketingInstant().Add(999 * time.Nanosecond)
	decision, err := DecideActivityGate(draft, ActivityPublication{}, at)
	if err != nil {
		t.Fatalf("DecideActivityGate: %v", err)
	}
	if decision.Status() != ActivityGateStatusNotPublished || decision.AllowsParticipation() ||
		decision.ActivityID() != 5 || decision.PublicationVersion() != 0 ||
		decision.EvaluatedAt() != canonicalUTCInstant(at) {
		t.Fatalf("draft decision = %#v", decision)
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestDecideActivityGateRetirementPrecedesWindowAndCanUseRootOnly(t *testing.T) {
	start := testMarketingInstant().Add(time.Hour)
	end := start.Add(time.Hour)
	publication := mustTestReleasePublication(t, 1, 1, start, end, start.Add(-time.Hour))
	published := mustTestPublishedActivity(t, 1, 1)
	retirement, err := PlanRetire(
		published,
		mustTestEvidence(t, "retirement/incident-1"),
		start.Add(-30*time.Minute),
	)
	if err != nil {
		t.Fatalf("PlanRetire: %v", err)
	}
	retired := retirement.Next()
	for _, input := range []ActivityPublication{publication, {}} {
		for _, at := range []time.Time{start.Add(-time.Minute), start, end.Add(time.Minute)} {
			decision, err := DecideActivityGate(retired, input, at)
			if err != nil {
				t.Fatalf("DecideActivityGate(%v): %v", at, err)
			}
			if decision.Status() != ActivityGateStatusRetired || decision.AllowsParticipation() ||
				decision.PublicationVersion() != 1 {
				t.Fatalf("retired decision = %#v", decision)
			}
		}
	}
}

func TestDecideActivityGateReturnsZeroDecisionForTechnicalFailure(t *testing.T) {
	start := testMarketingInstant()
	publication := mustTestReleasePublication(t, 1, 1, start, start.Add(time.Hour), start)
	draft := mustTestActivity(t, 1)
	published := mustTestPublishedActivity(t, 1, 1)
	retirement, err := PlanRetire(published, mustTestEvidence(t, "retirement/1"), start)
	if err != nil {
		t.Fatalf("prepare retired: %v", err)
	}
	corrupt := publication.clone()
	corrupt.schemaVersion = 0
	wrongActivity := publication.clone()
	wrongActivity.activityID = 2
	wrongVersion := publication.clone()
	wrongVersion.version = 2
	tests := []struct {
		name        string
		activity    Activity
		publication ActivityPublication
		at          time.Time
	}{
		{name: "invalid Activity", publication: publication, at: start},
		{name: "draft with publication", activity: draft, publication: publication, at: start},
		{name: "published without publication", activity: published, at: start},
		{name: "corrupt publication", activity: published, publication: corrupt, at: start},
		{name: "wrong Activity publication", activity: published, publication: wrongActivity, at: start},
		{name: "wrong active version", activity: published, publication: wrongVersion, at: start},
		{name: "zero clock", activity: published, publication: publication},
		{name: "draft zero clock", activity: draft},
		{name: "retired wrong publication", activity: retirement.Next(), publication: wrongActivity, at: start},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := DecideActivityGate(test.activity, test.publication, test.at)
			if !errors.Is(err, ErrActivityGateInvalid) {
				t.Fatalf("DecideActivityGate() error = %v", err)
			}
			assertZeroGateDecision(t, decision)
		})
	}
}

func TestActivityGateDecisionRejectsForgedPartialState(t *testing.T) {
	now := testMarketingInstant()
	valid := ActivityGateDecision{
		status:             ActivityGateStatusActive,
		activityID:         1,
		publicationVersion: 1,
		evaluatedAt:        now,
	}
	tests := []struct {
		name   string
		value  ActivityGateDecision
		mutate func(*ActivityGateDecision)
	}{
		{name: "zero"},
		{name: "unknown status", value: valid, mutate: func(value *ActivityGateDecision) { value.status = "paused" }},
		{name: "missing Activity", value: valid, mutate: func(value *ActivityGateDecision) { value.activityID = 0 }},
		{name: "missing publication", value: valid, mutate: func(value *ActivityGateDecision) { value.publicationVersion = 0 }},
		{name: "not published has publication", value: valid, mutate: func(value *ActivityGateDecision) { value.status = ActivityGateStatusNotPublished }},
		{name: "missing clock", value: valid, mutate: func(value *ActivityGateDecision) { value.evaluatedAt = time.Time{} }},
		{name: "noncanonical clock", value: valid, mutate: func(value *ActivityGateDecision) { value.evaluatedAt = now.Add(time.Nanosecond) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := test.value
			if test.mutate != nil {
				test.mutate(&value)
			}
			if err := value.Validate(); !errors.Is(err, ErrActivityGateInvalid) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	notPublished := ActivityGateDecision{
		status:      ActivityGateStatusNotPublished,
		activityID:  1,
		evaluatedAt: now,
	}
	if err := notPublished.Validate(); err != nil {
		t.Fatalf("not-published Validate: %v", err)
	}
}
