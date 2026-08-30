package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegistrationFactSnapshotCanonicalizesAndExposesMinimumFact(t *testing.T) {
	registeredAt := time.Date(2026, 8, 30, 16, 0, 0, 123456000, time.FixedZone("UTC+8", 8*60*60))
	observedAt := registeredAt.Add(30 * time.Minute)

	fact, err := NewRegistrationFactSnapshot(42, registeredAt, observedAt, "account-directory", "user-event:9001")
	if err != nil {
		t.Fatalf("construct registration fact: %v", err)
	}
	if err := fact.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if fact.ParticipantRef() != 42 || fact.Source() != "account-directory" || fact.Revision() != "user-event:9001" {
		t.Fatalf("fact metadata = ref %d, source %q, revision %q", fact.ParticipantRef(), fact.Source(), fact.Revision())
	}
	if fact.RegisteredAt().Location() != time.UTC || fact.ObservedAt().Location() != time.UTC {
		t.Fatal("fact instants were not canonicalized to UTC")
	}
	if !fact.RegisteredAt().Equal(registeredAt) || !fact.ObservedAt().Equal(observedAt) {
		t.Fatal("canonicalization changed the represented instant")
	}
}

func TestRegistrationFactSnapshotRejectsIncompleteOrInconsistentFacts(t *testing.T) {
	base := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		participant  ParticipantRef
		registeredAt time.Time
		observedAt   time.Time
		source       string
		revision     string
		want         error
	}{
		{name: "zero participant", registeredAt: base, observedAt: base, source: "directory", revision: "1", want: ErrParticipantRefRequired},
		{name: "zero registered-at", participant: 1, observedAt: base, source: "directory", revision: "1", want: ErrRegistrationFactInvalid},
		{name: "zero observed-at", participant: 1, registeredAt: base, source: "directory", revision: "1", want: ErrRegistrationFactInvalid},
		{name: "registered after observed", participant: 1, registeredAt: base.Add(time.Nanosecond), observedAt: base, source: "directory", revision: "1", want: ErrRegistrationFactInvalid},
		{name: "empty source", participant: 1, registeredAt: base, observedAt: base, revision: "1", want: ErrRegistrationFactInvalid},
		{name: "trimmed source", participant: 1, registeredAt: base, observedAt: base, source: " directory", revision: "1", want: ErrRegistrationFactInvalid},
		{name: "control source", participant: 1, registeredAt: base, observedAt: base, source: "dir\nector", revision: "1", want: ErrRegistrationFactInvalid},
		{name: "format source", participant: 1, registeredAt: base, observedAt: base, source: "dir\u202Eector", revision: "1", want: ErrRegistrationFactInvalid},
		{name: "long source", participant: 1, registeredAt: base, observedAt: base, source: strings.Repeat("s", maxFactSourceBytes+1), revision: "1", want: ErrRegistrationFactInvalid},
		{name: "empty revision", participant: 1, registeredAt: base, observedAt: base, source: "directory", want: ErrRegistrationFactInvalid},
		{name: "trimmed revision", participant: 1, registeredAt: base, observedAt: base, source: "directory", revision: "1 ", want: ErrRegistrationFactInvalid},
		{name: "zero-width revision", participant: 1, registeredAt: base, observedAt: base, source: "directory", revision: "v\u200B1", want: ErrRegistrationFactInvalid},
		{name: "long revision", participant: 1, registeredAt: base, observedAt: base, source: "directory", revision: strings.Repeat("r", maxFactRevisionBytes+1), want: ErrRegistrationFactInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fact, err := NewRegistrationFactSnapshot(
				test.participant,
				test.registeredAt,
				test.observedAt,
				test.source,
				test.revision,
			)
			if !errors.Is(err, test.want) || fact != (RegistrationFactSnapshot{}) {
				t.Fatalf("constructor = %#v, %v; want zero and %v", fact, err, test.want)
			}
		})
	}

	if err := (RegistrationFactSnapshot{}).Validate(); !errors.Is(err, ErrParticipantRefRequired) {
		t.Fatalf("zero fact Validate() error = %v, want participant required", err)
	}
}
