package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestStrategySnapshotIdentityUsesExactBoundedRevision(t *testing.T) {
	t.Parallel()

	boundary := "r" + strings.Repeat("1", MaxStrategyRevisionBytes-1)
	identity, err := NewStrategySnapshotIdentity(41, boundary)
	if err != nil {
		t.Fatalf("NewStrategySnapshotIdentity() error = %v", err)
	}
	if identity.ID() != 41 || identity.Revision() != StrategyRevision(boundary) {
		t.Fatalf("identity = %#v, want exact id/revision", identity)
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name     string
		id       StrategyID
		revision string
		want     error
	}{
		{name: "zero id", revision: "release-v1", want: ErrStrategyIDRequired},
		{name: "empty revision", id: 1, want: ErrStrategySnapshotRevisionInvalid},
		{name: "leading punctuation", id: 1, revision: "-release", want: ErrStrategySnapshotRevisionInvalid},
		{name: "leading whitespace", id: 1, revision: " release", want: ErrStrategySnapshotRevisionInvalid},
		{name: "embedded whitespace", id: 1, revision: "release v1", want: ErrStrategySnapshotRevisionInvalid},
		{name: "slash", id: 1, revision: "release/v1", want: ErrStrategySnapshotRevisionInvalid},
		{name: "non ascii", id: 1, revision: "release-版本", want: ErrStrategySnapshotRevisionInvalid},
		{name: "too long", id: 1, revision: "r" + strings.Repeat("1", MaxStrategyRevisionBytes), want: ErrStrategySnapshotRevisionInvalid},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewStrategySnapshotIdentity(test.id, test.revision)
			if !errors.Is(err, ErrStrategySnapshotIdentityInvalid) || !errors.Is(err, test.want) {
				t.Fatalf("NewStrategySnapshotIdentity() error = %v, want identity invalid and %v", err, test.want)
			}
			if got != (StrategySnapshotIdentity{}) {
				t.Fatalf("invalid identity = %#v, want zero", got)
			}
		})
	}
	if err := (StrategySnapshotIdentity{}).Validate(); !errors.Is(err, ErrStrategySnapshotIdentityInvalid) {
		t.Fatalf("zero identity Validate() error = %v, want identity invalid", err)
	}
}

func TestNewStrategySnapshotBindsCanonicalAggregateToRevision(t *testing.T) {
	t.Parallel()

	strategy := mustSnapshotStrategy(t, 42)
	snapshot, err := NewStrategySnapshot("activity:launch-v1", strategy)
	if err != nil {
		t.Fatalf("NewStrategySnapshot() error = %v", err)
	}
	if snapshot.Identity().ID() != strategy.ID() ||
		snapshot.Identity().Revision() != "activity:launch-v1" ||
		snapshot.SchemaVersion() != StrategySnapshotSchemaVersionV1 {
		t.Fatalf("snapshot envelope = %#v, want exact v1 identity", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	assertSnapshotStrategyEqual(t, snapshot.Strategy(), strategy)
}

func TestStrategySnapshotOwnsCanonicalAwardCollection(t *testing.T) {
	t.Parallel()

	third := mustAward(t, 30, "Third", 3, AwardOutcomeReward)
	first := mustAward(t, 10, "First", 1, AwardOutcomeReward)
	second := mustAward(t, 20, "Second", 2, AwardOutcomeNoReward)
	input := []Award{third, first, second}
	strategy, err := NewStrategy(43, "Versioned wheel", input)
	if err != nil {
		t.Fatalf("NewStrategy() error = %v", err)
	}
	snapshot, err := NewStrategySnapshot("release-v1", strategy)
	if err != nil {
		t.Fatalf("NewStrategySnapshot() error = %v", err)
	}

	input[0] = first
	returned := snapshot.Strategy().Awards()
	returned[0] = third
	got := snapshot.Strategy().Awards()
	if len(got) != 3 || got[0].ID() != 10 || got[1].ID() != 20 || got[2].ID() != 30 {
		t.Fatalf("snapshot awards = %#v, want owned canonical order", got)
	}
}

func TestRestoreStrategySnapshotRejectsUnknownSchemaAndInvalidStoredAggregate(t *testing.T) {
	t.Parallel()

	identity, err := NewStrategySnapshotIdentity(44, "release-v1")
	if err != nil {
		t.Fatalf("NewStrategySnapshotIdentity() error = %v", err)
	}
	validAwards := mustSnapshotStrategy(t, 44).Awards()

	tests := []struct {
		name    string
		id      StrategySnapshotIdentity
		schema  StrategySnapshotSchemaVersion
		nameRaw string
		awards  []Award
		want    error
	}{
		{name: "zero identity", schema: 1, nameRaw: "Stored", awards: validAwards, want: ErrStrategySnapshotIdentityInvalid},
		{name: "zero schema", id: identity, nameRaw: "Stored", awards: validAwards, want: ErrStrategySnapshotSchemaUnsupported},
		{name: "future schema", id: identity, schema: 2, nameRaw: "Stored", awards: validAwards, want: ErrStrategySnapshotSchemaUnsupported},
		{name: "non canonical name", id: identity, schema: 1, nameRaw: " Stored", awards: validAwards, want: ErrStrategyNameInvalid},
		{name: "missing awards", id: identity, schema: 1, nameRaw: "Stored", want: ErrStrategyAwardsRequired},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshot, err := RestoreStrategySnapshot(test.id, test.schema, test.nameRaw, test.awards)
			if !errors.Is(err, ErrStrategySnapshotInvalid) || !errors.Is(err, test.want) {
				t.Fatalf("RestoreStrategySnapshot() error = %v, want snapshot invalid and %v", err, test.want)
			}
			assertZeroStrategySnapshot(t, snapshot)
		})
	}
}

func TestStrategySnapshotValidateRejectsForgedDerivedAndMixedIdentityState(t *testing.T) {
	t.Parallel()

	valid, err := NewStrategySnapshot("release-v1", mustSnapshotStrategy(t, 45))
	if err != nil {
		t.Fatalf("NewStrategySnapshot() error = %v", err)
	}

	wrongIdentity := valid
	wrongIdentity.identity.id = 46
	if err := wrongIdentity.Validate(); !errors.Is(err, ErrStrategySnapshotIdentityInvalid) {
		t.Fatalf("mixed identity Validate() error = %v, want identity invalid", err)
	}

	wrongWeight := valid
	wrongWeight.strategy.totalWeight++
	if err := wrongWeight.Validate(); !errors.Is(err, ErrStrategySnapshotInvalid) {
		t.Fatalf("forged total Validate() error = %v, want snapshot invalid", err)
	}

	wrongOrder := valid
	wrongOrder.strategy.awards[0], wrongOrder.strategy.awards[1] =
		wrongOrder.strategy.awards[1], wrongOrder.strategy.awards[0]
	if err := wrongOrder.Validate(); !errors.Is(err, ErrStrategySnapshotInvalid) {
		t.Fatalf("forged order Validate() error = %v, want snapshot invalid", err)
	}

	wrongSchema := valid
	wrongSchema.schemaVersion = 2
	if err := wrongSchema.Validate(); !errors.Is(err, ErrStrategySnapshotSchemaUnsupported) {
		t.Fatalf("future schema Validate() error = %v, want schema unsupported", err)
	}

	if err := (StrategySnapshot{}).Validate(); !errors.Is(err, ErrStrategySnapshotInvalid) {
		t.Fatalf("zero snapshot Validate() error = %v, want snapshot invalid", err)
	}
}

func FuzzStrategySnapshotIdentityNeverNormalizesOrReturnsPartial(f *testing.F) {
	f.Add(uint64(1), "release-v1")
	f.Add(uint64(0), "release-v1")
	f.Add(uint64(1), " release-v1")
	f.Add(^uint64(0), "r:2026-08-30")

	f.Fuzz(func(t *testing.T, rawID uint64, revision string) {
		identity, err := NewStrategySnapshotIdentity(StrategyID(rawID), revision)
		if err != nil {
			if identity != (StrategySnapshotIdentity{}) {
				t.Fatalf("failed construction returned partial identity %#v", identity)
			}
			return
		}
		if err := identity.Validate(); err != nil {
			t.Fatalf("successful identity failed validation: %v", err)
		}
		if identity.ID() != StrategyID(rawID) || string(identity.Revision()) != revision {
			t.Fatalf("identity normalized input: %#v from id=%d revision=%q", identity, rawID, revision)
		}
	})
}

func mustSnapshotStrategy(t testingTB, id StrategyID) Strategy {
	t.Helper()

	strategy, err := NewStrategy(id, "Versioned wheel", []Award{
		mustAward(t, 20, "Try again", 6, AwardOutcomeNoReward),
		mustAward(t, 10, "Reward", 4, AwardOutcomeReward),
	})
	if err != nil {
		t.Fatalf("NewStrategy() error = %v", err)
	}
	return strategy
}

func assertSnapshotStrategyEqual(t *testing.T, got, want Strategy) {
	t.Helper()

	if got.ID() != want.ID() || got.Name() != want.Name() || got.TotalWeight() != want.TotalWeight() {
		t.Fatalf("Strategy envelope = %#v, want %#v", got, want)
	}
	if !slicesEqualAwards(got.Awards(), want.Awards()) {
		t.Fatalf("Strategy awards = %#v, want %#v", got.Awards(), want.Awards())
	}
}

func slicesEqualAwards(left, right []Award) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertZeroStrategySnapshot(t *testing.T, snapshot StrategySnapshot) {
	t.Helper()

	if snapshot.Identity() != (StrategySnapshotIdentity{}) ||
		snapshot.SchemaVersion() != 0 ||
		snapshot.Strategy().ID() != 0 ||
		len(snapshot.Strategy().Awards()) != 0 {
		t.Fatalf("operation returned partial Strategy snapshot %#v", snapshot)
	}
}
