package randomsource

import (
	"bytes"
	"errors"
	"io"
	"math"
	"sync"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestCryptoSourceRejectsInvalidConfigurationAndBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source *CryptoSource
		upper  uint64
		want   error
	}{
		{name: "nil receiver", upper: 1, want: ErrSourceNotConfigured},
		{name: "nil reader", source: newCryptoSource(nil), upper: 1, want: ErrSourceNotConfigured},
		{name: "zero upper", source: newCryptoSource(zeroReader{}), want: ErrUpperBoundRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.source.Uint64N(tt.upper)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Uint64N() error = %v, want errors.Is(_, %v)", err, tt.want)
			}
			if err.Error() != tt.want.Error() {
				t.Fatalf("Uint64N() public error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestCryptoSourcePreservesEntropyFailureCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("device path and sensitive implementation details")
	source := newCryptoSource(errorReader{err: cause})

	_, err := source.Uint64N(7)
	if !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("Uint64N() error = %v, want errors.Is(_, %v)", err, ErrEntropyUnavailable)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Uint64N() error = %v, want wrapped cause", err)
	}
	if err.Error() != ErrEntropyUnavailable.Error() {
		t.Fatalf("Uint64N() public error leaked cause: %q", err)
	}
}

func TestCryptoSourceSupportsFullUint64UpperBound(t *testing.T) {
	t.Parallel()

	reader := bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe})
	source := newCryptoSource(reader)
	value, err := source.Uint64N(math.MaxUint64)
	if err != nil {
		t.Fatalf("Uint64N() unexpected error: %v", err)
	}
	if value != math.MaxUint64-1 {
		t.Fatalf("Uint64N() = %d, want %d", value, uint64(math.MaxUint64-1))
	}
	if reader.Len() != 0 {
		t.Fatalf("entropy bytes remaining = %d, want 0 after reading all 64 bits", reader.Len())
	}
}

func TestCryptoSourceRejectsOutOfRangeCandidateInsteadOfTakingModulo(t *testing.T) {
	t.Parallel()

	reader := bytes.NewReader([]byte{0x03, 0x02})
	value, err := newCryptoSource(reader).Uint64N(3)
	if err != nil {
		t.Fatalf("Uint64N() unexpected error: %v", err)
	}
	if value != 2 {
		t.Fatalf("Uint64N() = %d, want 2 after rejecting candidate 3", value)
	}
	if reader.Len() != 0 {
		t.Fatalf("entropy bytes remaining = %d, want 0 after rejection and retry", reader.Len())
	}
}

func TestCryptoSourceAlwaysReturnsInsideRequestedRange(t *testing.T) {
	t.Parallel()

	source := NewCryptoSource()
	uppers := []uint64{1, 2, 3, 7, 256, 1<<32 + 1, math.MaxUint64}
	for _, upper := range uppers {
		for range 64 {
			value, err := source.Uint64N(upper)
			if err != nil {
				t.Fatalf("Uint64N(%d) unexpected error: %v", upper, err)
			}
			if value >= upper {
				t.Fatalf("Uint64N(%d) = %d, want value < upper", upper, value)
			}
		}
	}
}

func TestCryptoSourceDefaultReaderIsConcurrentSafe(t *testing.T) {
	t.Parallel()

	const (
		workers    = 32
		iterations = 64
		upper      = uint64(10_000)
	)
	source := NewCryptoSource()
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range iterations {
				value, err := source.Uint64N(upper)
				if err != nil {
					errorsFound <- err
					return
				}
				if value >= upper {
					errorsFound <- errors.New("random value outside requested range")
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent Uint64N() failed: %v", err)
	}
}

func TestCryptoSourceAndWeightedSelectorAreConcurrentSafe(t *testing.T) {
	t.Parallel()

	first, err := domain.NewAward(1, "First", 1, domain.AwardOutcomeReward)
	if err != nil {
		t.Fatalf("NewAward(first) unexpected error: %v", err)
	}
	second, err := domain.NewAward(2, "Second", 1, domain.AwardOutcomeNoReward)
	if err != nil {
		t.Fatalf("NewAward(second) unexpected error: %v", err)
	}
	strategy, err := domain.NewStrategy(1, "Concurrent selection", []domain.Award{first, second})
	if err != nil {
		t.Fatalf("NewStrategy() unexpected error: %v", err)
	}
	selector, err := domain.NewWeightedSelector(NewCryptoSource())
	if err != nil {
		t.Fatalf("NewWeightedSelector() unexpected error: %v", err)
	}

	const (
		workers    = 32
		iterations = 64
	)
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range iterations {
				award, selectErr := selector.Select(strategy)
				if selectErr != nil {
					errorsFound <- selectErr
					return
				}
				if award.ID() != first.ID() && award.ID() != second.ID() {
					errorsFound <- errors.New("selector returned an unconfigured award")
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent production selection failed: %v", err)
	}
}

func BenchmarkCryptoSourceUint64N(b *testing.B) {
	source := NewCryptoSource()
	b.ReportAllocs()
	for b.Loop() {
		value, err := source.Uint64N(10_000)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkValue = value
	}
}

var benchmarkValue uint64

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

var _ io.Reader = zeroReader{}
var _ io.Reader = errorReader{}
