package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewAward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          AwardID
		displayName string
		weight      Weight
		outcome     AwardOutcome
		wantErr     error
	}{
		{
			name:        "reward",
			id:          1,
			displayName: " 100 points ",
			weight:      400,
			outcome:     AwardOutcomeReward,
		},
		{
			name:        "intentional miss",
			id:          2,
			displayName: "Try again",
			weight:      249,
			outcome:     AwardOutcomeNoReward,
		},
		{
			name:        "name at rune limit",
			id:          3,
			displayName: strings.Repeat("奖", MaxAwardNameRunes),
			weight:      1,
			outcome:     AwardOutcomeReward,
		},
		{
			name:        "missing id",
			displayName: "100 points",
			weight:      1,
			outcome:     AwardOutcomeReward,
			wantErr:     ErrAwardIDRequired,
		},
		{
			name:        "blank name",
			id:          1,
			displayName: " \t\n ",
			weight:      1,
			outcome:     AwardOutcomeReward,
			wantErr:     ErrAwardNameRequired,
		},
		{
			name:        "invalid UTF-8 name",
			id:          1,
			displayName: string([]byte{0xff}),
			weight:      1,
			outcome:     AwardOutcomeReward,
			wantErr:     ErrAwardNameInvalid,
		},
		{
			name:        "control character in name",
			id:          1,
			displayName: "100\npoints",
			weight:      1,
			outcome:     AwardOutcomeReward,
			wantErr:     ErrAwardNameInvalid,
		},
		{
			name:        "name too long",
			id:          1,
			displayName: strings.Repeat("奖", MaxAwardNameRunes+1),
			weight:      1,
			outcome:     AwardOutcomeReward,
			wantErr:     ErrAwardNameTooLong,
		},
		{
			name:        "zero weight",
			id:          1,
			displayName: "100 points",
			outcome:     AwardOutcomeReward,
			wantErr:     ErrAwardWeightRequired,
		},
		{
			name:        "unknown outcome",
			id:          1,
			displayName: "100 points",
			weight:      1,
			outcome:     "coupon",
			wantErr:     ErrAwardOutcomeInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			award, err := NewAward(tt.id, tt.displayName, tt.weight, tt.outcome)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("NewAward() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewAward() unexpected error: %v", err)
			}
			if award.ID() != tt.id {
				t.Errorf("ID() = %d, want %d", award.ID(), tt.id)
			}
			if award.Name() != strings.TrimSpace(tt.displayName) {
				t.Errorf("Name() = %q, want %q", award.Name(), strings.TrimSpace(tt.displayName))
			}
			if award.Weight() != tt.weight {
				t.Errorf("Weight() = %d, want %d", award.Weight(), tt.weight)
			}
			if award.Outcome() != tt.outcome {
				t.Errorf("Outcome() = %q, want %q", award.Outcome(), tt.outcome)
			}
			if award.HasReward() != (tt.outcome == AwardOutcomeReward) {
				t.Errorf("HasReward() = %t, outcome = %q", award.HasReward(), tt.outcome)
			}
		})
	}
}

func TestRestoreAwardRejectsNonCanonicalStoredName(t *testing.T) {
	t.Parallel()

	tests := []string{
		" reward",
		"reward ",
		"\u00a0reward",
		"reward\u00a0",
		"\u3000reward",
		"reward\u3000",
	}
	for _, storedName := range tests {
		storedName := storedName
		t.Run(storedName, func(t *testing.T) {
			t.Parallel()

			_, err := RestoreAward(1, storedName, 1, AwardOutcomeReward)
			if !errors.Is(err, ErrAwardNameInvalid) {
				t.Fatalf("RestoreAward() error = %v, want errors.Is(_, %v)", err, ErrAwardNameInvalid)
			}
		})
	}
}

func TestRestoreAwardPreservesCanonicalUnicode(t *testing.T) {
	t.Parallel()

	storedName := "Cafe e\u0301"
	award, err := RestoreAward(1, storedName, 1, AwardOutcomeReward)
	if err != nil {
		t.Fatalf("RestoreAward() unexpected error: %v", err)
	}
	if award.Name() != storedName {
		t.Fatalf("RestoreAward() name = %q, want byte-preserved %q", award.Name(), storedName)
	}
}
