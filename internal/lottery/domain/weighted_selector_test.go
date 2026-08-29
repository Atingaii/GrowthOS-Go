package domain

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

func TestWeightedSelectorMapsEveryTicketToCanonicalAwardRange(t *testing.T) {
	t.Parallel()

	strategy := mustStrategy(t, []Award{
		mustAward(t, 30, "Third", 5, AwardOutcomeReward),
		mustAward(t, 10, "First", 2, AwardOutcomeReward),
		mustAward(t, 20, "Second", 3, AwardOutcomeNoReward),
	})
	wantByTicket := []AwardID{10, 10, 20, 20, 20, 30, 30, 30, 30, 30}

	for ticket, want := range wantByTicket {
		ticket := uint64(ticket)
		t.Run(fmt.Sprintf("ticket_%d", ticket), func(t *testing.T) {
			t.Parallel()

			source := &recordingSource{value: ticket}
			selector := mustSelector(t, source)
			award, err := selector.Select(strategy)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if award.ID() != want {
				t.Fatalf("Select() award id = %d, want %d for ticket %d", award.ID(), want, ticket)
			}
			if source.calls != 1 || len(source.uppers) != 1 || source.uppers[0] != 10 {
				t.Fatalf("source calls/uppers = %d/%v, want 1/[10]", source.calls, source.uppers)
			}
		})
	}
}

func TestWeightedSelectorShortCircuitsDeterministicStrategy(t *testing.T) {
	t.Parallel()

	for _, weight := range []Weight{1, 400, Weight(math.MaxUint64)} {
		weight := weight
		t.Run(fmt.Sprintf("weight_%d", weight), func(t *testing.T) {
			t.Parallel()

			strategy := mustStrategy(t, []Award{
				mustAward(t, 1, "Guaranteed", weight, AwardOutcomeReward),
			})
			source := &recordingSource{err: errors.New("must not be observed")}

			award, err := mustSelector(t, source).Select(strategy)
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if award.ID() != 1 {
				t.Fatalf("Select() award id = %d, want 1", award.ID())
			}
			if source.calls != 0 {
				t.Fatalf("source calls = %d, want 0 for a deterministic strategy", source.calls)
			}
		})
	}
}

func TestWeightedSelectorTreatsNoRewardAsSuccessfulOutcome(t *testing.T) {
	t.Parallel()

	strategy := mustStrategy(t, []Award{
		mustAward(t, 1, "Reward", 1, AwardOutcomeReward),
		mustAward(t, 2, "Try again", 1, AwardOutcomeNoReward),
	})
	selector := mustSelector(t, &recordingSource{value: 1})

	award, err := selector.Select(strategy)
	if err != nil {
		t.Fatalf("Select() unexpected error: %v", err)
	}
	if award.ID() != 2 || award.HasReward() {
		t.Fatalf("Select() award = %#v, want successful no_reward award 2", award)
	}
}

func TestWeightedSelectorSupportsMaximumUint64Range(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		awards    []Award
		ticket    uint64
		wantID    AwardID
		wantUpper uint64
	}{
		{
			name: "multiple awards sum to maximum and reach first bucket",
			awards: []Award{
				mustAward(t, 1, "First", 1, AwardOutcomeReward),
				mustAward(t, 2, "Rest", Weight(math.MaxUint64-1), AwardOutcomeNoReward),
			},
			ticket:    0,
			wantID:    1,
			wantUpper: math.MaxUint64,
		},
		{
			name: "multiple awards sum to maximum and reach final bucket",
			awards: []Award{
				mustAward(t, 1, "First", 1, AwardOutcomeReward),
				mustAward(t, 2, "Rest", Weight(math.MaxUint64-1), AwardOutcomeNoReward),
			},
			ticket:    math.MaxUint64 - 1,
			wantID:    2,
			wantUpper: math.MaxUint64,
		},
		{
			name: "three awards reach end of first maximum-range bucket",
			awards: []Award{
				mustAward(t, 1, "Most", Weight(math.MaxUint64-2), AwardOutcomeReward),
				mustAward(t, 2, "Second", 1, AwardOutcomeNoReward),
				mustAward(t, 3, "Third", 1, AwardOutcomeReward),
			},
			ticket:    math.MaxUint64 - 3,
			wantID:    1,
			wantUpper: math.MaxUint64,
		},
		{
			name: "three awards cross into penultimate bucket",
			awards: []Award{
				mustAward(t, 1, "Most", Weight(math.MaxUint64-2), AwardOutcomeReward),
				mustAward(t, 2, "Second", 1, AwardOutcomeNoReward),
				mustAward(t, 3, "Third", 1, AwardOutcomeReward),
			},
			ticket:    math.MaxUint64 - 2,
			wantID:    2,
			wantUpper: math.MaxUint64,
		},
		{
			name: "three awards reach final bucket",
			awards: []Award{
				mustAward(t, 1, "Most", Weight(math.MaxUint64-2), AwardOutcomeReward),
				mustAward(t, 2, "Second", 1, AwardOutcomeNoReward),
				mustAward(t, 3, "Third", 1, AwardOutcomeReward),
			},
			ticket:    math.MaxUint64 - 1,
			wantID:    3,
			wantUpper: math.MaxUint64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := &recordingSource{value: tt.ticket}
			award, err := mustSelector(t, source).Select(mustStrategy(t, tt.awards))
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if award.ID() != tt.wantID {
				t.Fatalf("Select() award id = %d, want %d", award.ID(), tt.wantID)
			}
			if len(source.uppers) != 1 || source.uppers[0] != tt.wantUpper {
				t.Fatalf("source uppers = %v, want [%d]", source.uppers, tt.wantUpper)
			}
		})
	}
}

func TestWeightedSelectorFailsClosed(t *testing.T) {
	t.Parallel()

	strategy := mustStrategy(t, []Award{
		mustAward(t, 1, "First", 3, AwardOutcomeReward),
		mustAward(t, 2, "Second", 4, AwardOutcomeNoReward),
	})
	sourceFailure := errors.New("entropy device unavailable with sensitive details")

	tests := []struct {
		name      string
		selector  *WeightedSelector
		strategy  Strategy
		wantErr   error
		wantCause error
		wantCalls int
	}{
		{
			name:      "zero selector",
			strategy:  strategy,
			wantErr:   ErrSelectorNotConfigured,
			wantCalls: 0,
		},
		{
			name:      "zero strategy",
			selector:  mustSelector(t, &recordingSource{}),
			wantErr:   ErrSelectionStrategyInvalid,
			wantCause: ErrStrategyAwardsRequired,
			wantCalls: 0,
		},
		{
			name:      "random source failure",
			selector:  mustSelector(t, &recordingSource{err: sourceFailure}),
			strategy:  strategy,
			wantErr:   ErrRandomSourceFailure,
			wantCause: sourceFailure,
			wantCalls: 1,
		},
		{
			name:      "random source returns upper bound",
			selector:  mustSelector(t, &recordingSource{value: 7}),
			strategy:  strategy,
			wantErr:   ErrRandomSourceContractViolation,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			award, err := tt.selector.Select(tt.strategy)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Select() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
			if tt.wantCause != nil && !errors.Is(err, tt.wantCause) {
				t.Fatalf("Select() error = %v, want wrapped cause %v", err, tt.wantCause)
			}
			if award != (Award{}) {
				t.Fatalf("Select() award = %#v, want zero Award on failure", award)
			}
			if tt.selector != nil {
				source, ok := tt.selector.source.(*recordingSource)
				if !ok {
					t.Fatal("test selector source has unexpected type")
				}
				if source.calls != tt.wantCalls {
					t.Fatalf("source calls = %d, want %d", source.calls, tt.wantCalls)
				}
			}
			if err != nil && err.Error() != tt.wantErr.Error() {
				t.Fatalf("Select() public error = %q, want safe class %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewWeightedSelectorRejectsNilSource(t *testing.T) {
	t.Parallel()

	selector, err := NewWeightedSelector(nil)
	if !errors.Is(err, ErrSelectorNotConfigured) {
		t.Fatalf("NewWeightedSelector() error = %v, want errors.Is(_, %v)", err, ErrSelectorNotConfigured)
	}
	if selector != nil {
		t.Fatalf("NewWeightedSelector() = %#v, want nil", selector)
	}
}

func TestNewWeightedSelectorRejectsTypedNilSource(t *testing.T) {
	t.Parallel()

	var source *recordingSource
	selector, err := NewWeightedSelector(source)
	if !errors.Is(err, ErrSelectorNotConfigured) {
		t.Fatalf("NewWeightedSelector() error = %v, want errors.Is(_, %v)", err, ErrSelectorNotConfigured)
	}
	if selector != nil {
		t.Fatalf("NewWeightedSelector() = %#v, want nil", selector)
	}
}

func TestWeightedSelectorFailsClosedOnInternalMappingHole(t *testing.T) {
	t.Parallel()

	strategy := Strategy{
		id:   1,
		name: "Invalid internal strategy",
		awards: []Award{
			mustAward(t, 1, "First real bucket", 1, AwardOutcomeReward),
			mustAward(t, 2, "Second real bucket", 1, AwardOutcomeNoReward),
		},
		totalWeight: 3,
	}
	award, err := mustSelector(t, &recordingSource{value: 2}).Select(strategy)
	if !errors.Is(err, ErrSelectionInvariantViolation) {
		t.Fatalf("Select() error = %v, want errors.Is(_, %v)", err, ErrSelectionInvariantViolation)
	}
	if award != (Award{}) {
		t.Fatalf("Select() award = %#v, want zero Award", award)
	}
	if err.Error() != ErrSelectionInvariantViolation.Error() {
		t.Fatalf("Select() public error leaked internal mapping details: %q", err)
	}
}

func TestWeightedSelectorFailsClosedOnInvalidDeterministicMapping(t *testing.T) {
	t.Parallel()

	source := &recordingSource{}
	strategy := Strategy{
		id:          1,
		name:        "Invalid internal deterministic strategy",
		awards:      []Award{mustAward(t, 1, "Only bucket", 1, AwardOutcomeReward)},
		totalWeight: 2,
	}
	award, err := mustSelector(t, source).Select(strategy)
	if !errors.Is(err, ErrSelectionInvariantViolation) {
		t.Fatalf("Select() error = %v, want errors.Is(_, %v)", err, ErrSelectionInvariantViolation)
	}
	if award != (Award{}) {
		t.Fatalf("Select() award = %#v, want zero Award", award)
	}
	if source.calls != 0 {
		t.Fatalf("source calls = %d, want 0 before rejecting invalid deterministic mapping", source.calls)
	}
}

func TestWeightedSelectorConcurrentWithSafeSource(t *testing.T) {
	t.Parallel()

	strategy := mustStrategy(t, []Award{
		mustAward(t, 1, "First", 1, AwardOutcomeReward),
		mustAward(t, 2, "Second", 1, AwardOutcomeNoReward),
	})
	selector := mustSelector(t, constantSource{value: 1})
	const (
		workers    = 32
		iterations = 128
	)
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range iterations {
				award, err := selector.Select(strategy)
				if err != nil {
					errorsFound <- err
					return
				}
				if award.ID() != 2 {
					errorsFound <- fmt.Errorf("award id = %d, want 2", award.ID())
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Select() failed: %v", err)
	}
}

func TestWeightedSelectorExactCountsMatchRelativeWeights(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		weights []Weight
	}{
		{name: "one to three", weights: []Weight{1, 3}},
		{name: "scaled one hundred to three hundred", weights: []Weight{100, 300}},
		{name: "three candidates", weights: []Weight{2, 3, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			awards := make([]Award, len(tt.weights))
			var total uint64
			for index, weight := range tt.weights {
				awards[index] = mustAward(t, AwardID(index+1), "Award", weight, AwardOutcomeReward)
				total += uint64(weight)
			}
			strategy := mustStrategy(t, awards)
			counts := make(map[AwardID]uint64, len(awards))
			for ticket := uint64(0); ticket < total; ticket++ {
				award, err := mustSelector(t, &recordingSource{value: ticket}).Select(strategy)
				if err != nil {
					t.Fatalf("Select(ticket=%d) unexpected error: %v", ticket, err)
				}
				counts[award.ID()]++
			}
			for index, weight := range tt.weights {
				id := AwardID(index + 1)
				if counts[id] != uint64(weight) {
					t.Fatalf("award %d count = %d, want weight %d", id, counts[id], weight)
				}
			}
		})
	}
}

func BenchmarkWeightedSelectorWorstCase(b *testing.B) {
	for _, size := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("awards_%d", size), func(b *testing.B) {
			awards := make([]Award, size)
			for index := range awards {
				awards[index] = mustAward(b, AwardID(index+1), "Award", 1, AwardOutcomeReward)
			}
			strategy := mustStrategy(b, awards)
			selector := mustSelector(b, constantSource{value: uint64(size - 1)})

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				award, err := selector.Select(strategy)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkAward = award
			}
		})
	}
}

var benchmarkAward Award

type recordingSource struct {
	value  uint64
	err    error
	calls  int
	uppers []uint64
}

func (s *recordingSource) Uint64N(upper uint64) (uint64, error) {
	s.calls++
	s.uppers = append(s.uppers, upper)
	return s.value, s.err
}

type constantSource struct{ value uint64 }

func (s constantSource) Uint64N(uint64) (uint64, error) { return s.value, nil }

type testingTB interface {
	Helper()
	Fatalf(string, ...any)
}

func mustStrategy(tb testingTB, awards []Award) Strategy {
	tb.Helper()
	strategy, err := NewStrategy(1, "Weighted strategy", awards)
	if err != nil {
		tb.Fatalf("NewStrategy() unexpected error: %v", err)
	}
	return strategy
}

func mustSelector(tb testingTB, source BoundedRandomSource) *WeightedSelector {
	tb.Helper()
	selector, err := NewWeightedSelector(source)
	if err != nil {
		tb.Fatalf("NewWeightedSelector() unexpected error: %v", err)
	}
	return selector
}
