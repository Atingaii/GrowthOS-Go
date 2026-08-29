package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestEphemeralSelectionLoadsRequestedStrategyAndReturnsConfiguredAward(t *testing.T) {
	strategy := selectionTestStrategy(t, 42, []domain.Award{
		selectionTestAward(t, 20, "Try again", 3, domain.AwardOutcomeNoReward),
		selectionTestAward(t, 10, "Coffee", 1, domain.AwardOutcomeReward),
	})
	reader := &selectionReaderStub{strategy: strategy}
	selector := &awardSelectorStub{award: strategy.Awards()[1]}
	service, err := NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	if err := service.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	ctx := context.WithValue(context.Background(), selectionContextKey{}, "request-context")
	got, err := service.Select(ctx, strategy.ID())
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if reader.ctx != ctx || reader.id != strategy.ID() {
		t.Fatalf("reader call = context %v, id %d", reader.ctx, reader.id)
	}
	if selector.strategy.ID() != strategy.ID() {
		t.Fatalf("selector strategy = %d, want %d", selector.strategy.ID(), strategy.ID())
	}
	if got.Strategy.ID() != strategy.ID() || !sameAward(got.Award, strategy.Awards()[1]) {
		t.Fatalf("selection = %#v, want requested Strategy and configured Award", got)
	}
}

func TestEphemeralSelectionRejectsMissingAndTypedNilDependencies(t *testing.T) {
	strategy := selectionTestStrategy(t, 1, []domain.Award{
		selectionTestAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	validReader := &selectionReaderStub{strategy: strategy}
	validSelector := &awardSelectorStub{award: strategy.Awards()[0]}
	var typedNilReader *selectionReaderStub
	var typedNilSelector *awardSelectorStub

	tests := []struct {
		name     string
		reader   StrategyReader
		selector AwardSelector
	}{
		{name: "nil reader", selector: validSelector},
		{name: "typed nil reader", reader: typedNilReader, selector: validSelector},
		{name: "nil selector", reader: validReader},
		{name: "typed nil selector", reader: validReader, selector: typedNilSelector},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewEphemeralSelectionService(test.reader, test.selector)
			if !errors.Is(err, ErrSelectionNotConfigured) || service != nil {
				t.Fatalf("constructor = %#v, %v; want nil, not configured", service, err)
			}
		})
	}

	var nilService *EphemeralSelectionService
	if err := nilService.Validate(); !errors.Is(err, ErrSelectionNotConfigured) {
		t.Fatalf("nil service Validate() error = %v, want not configured", err)
	}
	if _, err := nilService.Select(context.Background(), 1); !errors.Is(err, ErrSelectionNotConfigured) {
		t.Fatalf("nil service error = %v, want not configured", err)
	}
	brokenService := &EphemeralSelectionService{strategies: typedNilReader, selector: validSelector}
	if err := brokenService.Validate(); !errors.Is(err, ErrSelectionNotConfigured) {
		t.Fatalf("broken service Validate() error = %v, want not configured", err)
	}
	if _, err := brokenService.Select(context.Background(), 1); !errors.Is(err, ErrSelectionNotConfigured) {
		t.Fatalf("typed-nil field error = %v, want not configured", err)
	}
}

func TestEphemeralSelectionMakesObservedCancellationWinDependencyErrorRace(t *testing.T) {
	strategy := selectionTestStrategy(t, 7, []domain.Award{
		selectionTestAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})

	readerContext, cancelReader := context.WithCancel(context.Background())
	readerFailure := errors.New("reader completed after cancellation")
	reader := &selectionReaderStub{err: readerFailure, afterRead: cancelReader}
	selector := &awardSelectorStub{}
	service, err := NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct reader-race service: %v", err)
	}
	if _, err := service.Select(readerContext, strategy.ID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("reader race error = %v, want canceled", err)
	}
	if selector.calls != 0 {
		t.Fatalf("selector calls = %d, want zero after reader cancellation", selector.calls)
	}

	selectorContext, cancelSelector := context.WithCancel(context.Background())
	selectorFailure := errors.New("selector completed after cancellation")
	reader = &selectionReaderStub{strategy: strategy}
	selector = &awardSelectorStub{err: selectorFailure, afterSelect: cancelSelector}
	service, err = NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct selector-race service: %v", err)
	}
	if _, err := service.Select(selectorContext, strategy.ID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("selector race error = %v, want canceled", err)
	}
}

func TestEphemeralSelectionRejectsInvalidArgumentsBeforeDependencies(t *testing.T) {
	reader := &selectionReaderStub{}
	selector := &awardSelectorStub{}
	service, err := NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}

	if _, err := service.Select(nil, 1); !errors.Is(err, ErrSelectionInvalidArgument) {
		t.Fatalf("nil context error = %v, want invalid argument", err)
	}
	if _, err := service.Select(context.Background(), 0); !errors.Is(err, ErrSelectionInvalidArgument) {
		t.Fatalf("zero ID error = %v, want invalid argument", err)
	}
	if reader.calls != 0 || selector.calls != 0 {
		t.Fatalf("dependency calls = reader %d, selector %d; want zero", reader.calls, selector.calls)
	}
}

func TestEphemeralSelectionHonorsCancellationBeforeReadAndBeforeSelect(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	reader := &selectionReaderStub{}
	selector := &awardSelectorStub{}
	service, err := NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	if _, err := service.Select(canceled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled error = %v, want canceled", err)
	}
	if reader.calls != 0 || selector.calls != 0 {
		t.Fatal("pre-canceled call reached a dependency")
	}

	strategy := selectionTestStrategy(t, 2, []domain.Award{
		selectionTestAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	between, cancelBetween := context.WithCancel(context.Background())
	reader = &selectionReaderStub{strategy: strategy, afterRead: cancelBetween}
	selector = &awardSelectorStub{award: strategy.Awards()[0]}
	service, err = NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct between-stage service: %v", err)
	}
	if _, err := service.Select(between, strategy.ID()); !errors.Is(err, context.Canceled) {
		t.Fatalf("between-stage error = %v, want canceled", err)
	}
	if selector.calls != 0 {
		t.Fatalf("selector calls = %d, want zero after cancellation", selector.calls)
	}
}

func TestEphemeralSelectionPropagatesDependencyFailuresWithoutContinuing(t *testing.T) {
	strategy := selectionTestStrategy(t, 7, []domain.Award{
		selectionTestAward(t, 1, "Only", 1, domain.AwardOutcomeReward),
	})
	repositoryFailure := errors.New("reader failed")
	reader := &selectionReaderStub{err: repositoryFailure}
	selector := &awardSelectorStub{}
	service, err := NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}
	if _, err := service.Select(context.Background(), strategy.ID()); !errors.Is(err, repositoryFailure) {
		t.Fatalf("reader error = %v, want original failure", err)
	}
	if selector.calls != 0 {
		t.Fatalf("selector calls = %d, want zero", selector.calls)
	}

	selectorFailure := errors.New("selector failed")
	reader = &selectionReaderStub{strategy: strategy}
	selector = &awardSelectorStub{err: selectorFailure}
	service, err = NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct selector-failure service: %v", err)
	}
	if _, err := service.Select(context.Background(), strategy.ID()); !errors.Is(err, selectorFailure) {
		t.Fatalf("selector error = %v, want original failure", err)
	}
}

func TestEphemeralSelectionRejectsMismatchedStrategyAndAwardResults(t *testing.T) {
	requested := selectionTestStrategy(t, 10, []domain.Award{
		selectionTestAward(t, 1, "Configured", 2, domain.AwardOutcomeReward),
	})
	wrongStrategy := selectionTestStrategy(t, 11, []domain.Award{
		selectionTestAward(t, 1, "Configured", 2, domain.AwardOutcomeReward),
	})
	tests := []struct {
		name     string
		strategy domain.Strategy
		award    domain.Award
	}{
		{name: "wrong strategy", strategy: wrongStrategy, award: wrongStrategy.Awards()[0]},
		{
			name:     "unknown award ID",
			strategy: requested,
			award:    selectionTestAward(t, 2, "Invented", 1, domain.AwardOutcomeReward),
		},
		{
			name:     "changed award fields",
			strategy: requested,
			award:    selectionTestAward(t, 1, "Changed", 2, domain.AwardOutcomeReward),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &selectionReaderStub{strategy: test.strategy}
			selector := &awardSelectorStub{award: test.award}
			service, err := NewEphemeralSelectionService(reader, selector)
			if err != nil {
				t.Fatalf("construct service: %v", err)
			}
			if _, err := service.Select(context.Background(), requested.ID()); !errors.Is(err, ErrSelectionResultInvalid) {
				t.Fatalf("Select() error = %v, want invalid result", err)
			}
			if test.strategy.ID() != requested.ID() && selector.calls != 0 {
				t.Fatalf("selector calls = %d, want zero for wrong Strategy", selector.calls)
			}
		})
	}
}

func TestEphemeralSelectionSupportsConcurrentCallsWithSafeDependencies(t *testing.T) {
	strategy := selectionTestStrategy(t, 99, []domain.Award{
		selectionTestAward(t, 1, "Only", 1, domain.AwardOutcomeNoReward),
	})
	reader := &concurrentSelectionReader{strategy: strategy}
	selector := &concurrentAwardSelector{award: strategy.Awards()[0]}
	service, err := NewEphemeralSelectionService(reader, selector)
	if err != nil {
		t.Fatalf("construct service: %v", err)
	}

	const workers = 64
	var waitGroup sync.WaitGroup
	errorsSeen := make(chan error, workers)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			selection, err := service.Select(context.Background(), strategy.ID())
			if err == nil && selection.Award.ID() != 1 {
				err = errors.New("unexpected concurrent Award")
			}
			errorsSeen <- err
		}()
	}
	waitGroup.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent Select() error = %v", err)
		}
	}
}

type selectionContextKey struct{}

type selectionReaderStub struct {
	strategy  domain.Strategy
	err       error
	ctx       context.Context
	id        domain.StrategyID
	calls     int
	afterRead context.CancelFunc
}

func (stub *selectionReaderStub) FindByID(ctx context.Context, id domain.StrategyID) (domain.Strategy, error) {
	stub.calls++
	stub.ctx = ctx
	stub.id = id
	if stub.afterRead != nil {
		stub.afterRead()
	}
	return stub.strategy, stub.err
}

type awardSelectorStub struct {
	award       domain.Award
	err         error
	strategy    domain.Strategy
	calls       int
	afterSelect context.CancelFunc
}

func (stub *awardSelectorStub) Select(strategy domain.Strategy) (domain.Award, error) {
	stub.calls++
	stub.strategy = strategy
	if stub.afterSelect != nil {
		stub.afterSelect()
	}
	return stub.award, stub.err
}

type concurrentSelectionReader struct {
	strategy domain.Strategy
}

func (reader *concurrentSelectionReader) FindByID(context.Context, domain.StrategyID) (domain.Strategy, error) {
	return reader.strategy, nil
}

type concurrentAwardSelector struct {
	award domain.Award
}

func (selector *concurrentAwardSelector) Select(domain.Strategy) (domain.Award, error) {
	return selector.award, nil
}

func selectionTestStrategy(t *testing.T, id domain.StrategyID, awards []domain.Award) domain.Strategy {
	t.Helper()
	strategy, err := domain.NewStrategy(id, "Selection test strategy", awards)
	if err != nil {
		t.Fatalf("construct Strategy: %v", err)
	}
	return strategy
}

func selectionTestAward(
	t *testing.T,
	id domain.AwardID,
	name string,
	weight domain.Weight,
	outcome domain.AwardOutcome,
) domain.Award {
	t.Helper()
	award, err := domain.NewAward(id, name, weight, outcome)
	if err != nil {
		t.Fatalf("construct Award: %v", err)
	}
	return award
}
