package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNewStrategy(t *testing.T) {
	t.Parallel()

	reward := mustAward(t, 1, "100 points", 400, AwardOutcomeReward)
	miss := mustAward(t, 2, "Try again", 600, AwardOutcomeNoReward)

	tests := []struct {
		name         string
		id           StrategyID
		strategyName string
		awards       []Award
		wantTotal    uint64
		wantErr      error
	}{
		{
			name:         "valid",
			id:           7,
			strategyName: " New user wheel ",
			awards:       []Award{reward, miss},
			wantTotal:    1000,
		},
		{
			name:         "name at rune limit",
			id:           8,
			strategyName: strings.Repeat("新", MaxStrategyNameRunes),
			awards:       []Award{reward, miss},
			wantTotal:    1000,
		},
		{
			name:         "single award is a valid deterministic strategy",
			id:           9,
			strategyName: "Deterministic wheel",
			awards:       []Award{reward},
			wantTotal:    400,
		},
		{
			name:         "weights need no fixed denominator",
			id:           10,
			strategyName: "Ratio wheel",
			awards: []Award{
				mustAward(t, 10, "A", 2, AwardOutcomeReward),
				mustAward(t, 11, "B", 5, AwardOutcomeNoReward),
			},
			wantTotal: 7,
		},
		{
			name:         "maximum total does not overflow",
			id:           11,
			strategyName: "Maximum wheel",
			awards: []Award{
				mustAward(t, 12, "Largest", Weight(math.MaxUint64), AwardOutcomeReward),
			},
			wantTotal: math.MaxUint64,
		},
		{
			name:         "award names need not be unique",
			id:           12,
			strategyName: "Same labels",
			awards: []Award{
				mustAward(t, 13, "Mystery box", 1, AwardOutcomeReward),
				mustAward(t, 14, "Mystery box", 1, AwardOutcomeReward),
			},
			wantTotal: 2,
		},
		{
			name:         "all intentional misses remain a valid configuration",
			id:           13,
			strategyName: "Maintenance fallback",
			awards: []Award{
				mustAward(t, 15, "Try later", 1, AwardOutcomeNoReward),
				mustAward(t, 16, "Come back tomorrow", 2, AwardOutcomeNoReward),
			},
			wantTotal: 3,
		},
		{
			name:         "missing id",
			strategyName: "New user wheel",
			awards:       []Award{reward},
			wantErr:      ErrStrategyIDRequired,
		},
		{
			name:         "blank name",
			id:           7,
			strategyName: "\n\t",
			awards:       []Award{reward},
			wantErr:      ErrStrategyNameRequired,
		},
		{
			name:         "invalid UTF-8 name",
			id:           7,
			strategyName: string([]byte{0xff}),
			awards:       []Award{reward},
			wantErr:      ErrStrategyNameInvalid,
		},
		{
			name:         "control character in name",
			id:           7,
			strategyName: "New\nuser wheel",
			awards:       []Award{reward},
			wantErr:      ErrStrategyNameInvalid,
		},
		{
			name:         "name too long",
			id:           7,
			strategyName: strings.Repeat("新", MaxStrategyNameRunes+1),
			awards:       []Award{reward},
			wantErr:      ErrStrategyNameTooLong,
		},
		{
			name:         "nil awards",
			id:           7,
			strategyName: "New user wheel",
			wantErr:      ErrStrategyAwardsRequired,
		},
		{
			name:         "zero value award",
			id:           7,
			strategyName: "New user wheel",
			awards:       []Award{{}},
			wantErr:      ErrAwardIDRequired,
		},
		{
			name:         "duplicate award id",
			id:           7,
			strategyName: "New user wheel",
			awards: []Award{
				reward,
				mustAward(t, reward.ID(), "Duplicate", 1, AwardOutcomeReward),
			},
			wantErr: ErrDuplicateAwardID,
		},
		{
			name:         "weight sum overflow",
			id:           7,
			strategyName: "New user wheel",
			awards: []Award{
				mustAward(t, 10, "Largest", Weight(math.MaxUint64), AwardOutcomeReward),
				mustAward(t, 11, "Overflow", 1, AwardOutcomeNoReward),
			},
			wantErr: ErrTotalWeightOverflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			strategy, err := NewStrategy(tt.id, tt.strategyName, tt.awards)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewStrategy() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewStrategy() unexpected error: %v", err)
			}
			if strategy.ID() != tt.id {
				t.Errorf("ID() = %d, want %d", strategy.ID(), tt.id)
			}
			if strategy.Name() != strings.TrimSpace(tt.strategyName) {
				t.Errorf("Name() = %q, want %q", strategy.Name(), strings.TrimSpace(tt.strategyName))
			}
			if strategy.TotalWeight() != tt.wantTotal {
				t.Errorf("TotalWeight() = %d, want %d", strategy.TotalWeight(), tt.wantTotal)
			}
		})
	}
}

func TestStrategyOwnsAwardCollection(t *testing.T) {
	t.Parallel()

	first := mustAward(t, 1, "100 points", 4, AwardOutcomeReward)
	second := mustAward(t, 2, "Try again", 6, AwardOutcomeNoReward)
	input := []Award{first, second}

	strategy, err := NewStrategy(7, "New user wheel", input)
	if err != nil {
		t.Fatalf("NewStrategy() unexpected error: %v", err)
	}

	input[0] = second
	if got := strategy.Awards()[0].ID(); got != first.ID() {
		t.Fatalf("constructor retained caller slice: first award id = %d, want %d", got, first.ID())
	}

	returned := strategy.Awards()
	returned[0] = second
	if got := strategy.Awards()[0].ID(); got != first.ID() {
		t.Fatalf("Awards() exposed aggregate slice: first award id = %d, want %d", got, first.ID())
	}
}

func TestStrategyCanonicalizesAwardOrder(t *testing.T) {
	t.Parallel()

	third := mustAward(t, 30, "Third", 3, AwardOutcomeReward)
	first := mustAward(t, 10, "First", 1, AwardOutcomeReward)
	second := mustAward(t, 20, "Second", 2, AwardOutcomeNoReward)

	strategy, err := NewStrategy(7, "Canonical wheel", []Award{third, first, second})
	if err != nil {
		t.Fatalf("NewStrategy() unexpected error: %v", err)
	}

	got := strategy.Awards()
	want := []AwardID{10, 20, 30}
	for index, award := range got {
		if award.ID() != want[index] {
			t.Fatalf("Awards()[%d].ID() = %d, want %d", index, award.ID(), want[index])
		}
	}
}

func TestStrategyAwardLookup(t *testing.T) {
	t.Parallel()

	reward := mustAward(t, 1, "100 points", 4, AwardOutcomeReward)
	miss := mustAward(t, 2, "Try again", 6, AwardOutcomeNoReward)
	strategy, err := NewStrategy(7, "New user wheel", []Award{reward, miss})
	if err != nil {
		t.Fatalf("NewStrategy() unexpected error: %v", err)
	}

	got, ok := strategy.Award(miss.ID())
	if !ok {
		t.Fatal("Award() did not find configured id")
	}
	if got != miss {
		t.Fatalf("Award() = %#v, want %#v", got, miss)
	}

	if _, ok := strategy.Award(999); ok {
		t.Fatal("Award() found an unconfigured id")
	}
}

func TestRestoreStrategyRejectsNonCanonicalStoredName(t *testing.T) {
	t.Parallel()

	award := mustAward(t, 1, "Reward", 1, AwardOutcomeReward)
	for _, storedName := range []string{
		" wheel",
		"wheel ",
		"\u00a0wheel",
		"wheel\u00a0",
		"\u3000wheel",
		"wheel\u3000",
	} {
		storedName := storedName
		t.Run(storedName, func(t *testing.T) {
			t.Parallel()

			_, err := RestoreStrategy(1, storedName, []Award{award})
			if !errors.Is(err, ErrStrategyNameInvalid) {
				t.Fatalf("RestoreStrategy() error = %v, want errors.Is(_, %v)", err, ErrStrategyNameInvalid)
			}
		})
	}
}

func mustAward(t testingTB, id AwardID, name string, weight Weight, outcome AwardOutcome) Award {
	t.Helper()

	award, err := NewAward(id, name, weight, outcome)
	if err != nil {
		t.Fatalf("NewAward() unexpected error: %v", err)
	}
	return award
}
