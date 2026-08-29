package randomsource

import (
	cryptorand "crypto/rand"
	"errors"
	"io"
	"math/big"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

var (
	// ErrSourceNotConfigured means the adapter has no entropy reader.
	ErrSourceNotConfigured = errors.New("lottery random source: not configured")
	// ErrUpperBoundRequired means a caller requested the empty interval [0,0).
	ErrUpperBoundRequired = errors.New("lottery random source: upper bound must be positive")
	// ErrEntropyUnavailable means the operating-system entropy boundary failed.
	ErrEntropyUnavailable = errors.New("lottery random source: entropy unavailable")
)

var _ domain.BoundedRandomSource = (*CryptoSource)(nil)

// CryptoSource uses crypto/rand.Int to generate a uniform value in [0, upper).
// Cryptographic unpredictability is appropriate for value-bearing selection,
// but does not by itself provide auditability, replay, or proof of fairness.
type CryptoSource struct {
	reader io.Reader
}

// NewCryptoSource uses the process-wide cryptographic Reader. The standard
// Reader is safe for concurrent use.
func NewCryptoSource() *CryptoSource {
	return &CryptoSource{reader: cryptorand.Reader}
}

func newCryptoSource(reader io.Reader) *CryptoSource {
	return &CryptoSource{reader: reader}
}

// Uint64N returns a uniform value in [0, upper) without modulo bias. SetUint64
// and crypto/rand.Int preserve the complete uint64 range, including MaxUint64.
func (s *CryptoSource) Uint64N(upper uint64) (uint64, error) {
	if s == nil || s.reader == nil {
		return 0, newSourceError(ErrSourceNotConfigured, nil)
	}
	if upper == 0 {
		return 0, newSourceError(ErrUpperBoundRequired, nil)
	}

	maximum := new(big.Int).SetUint64(upper)
	value, err := cryptorand.Int(s.reader, maximum)
	if err != nil {
		return 0, newSourceError(ErrEntropyUnavailable, err)
	}
	if value.Sign() < 0 || !value.IsUint64() || value.Uint64() >= upper {
		return 0, newSourceError(ErrEntropyUnavailable, errors.New("crypto/rand returned an out-of-range value"))
	}
	return value.Uint64(), nil
}

// SourceError exposes only a stable class while retaining a diagnostic cause.
type SourceError struct {
	class error
	cause error
}

func newSourceError(class, cause error) *SourceError {
	if !knownSourceError(class) {
		class = ErrEntropyUnavailable
	}
	return &SourceError{class: class, cause: cause}
}

func (e *SourceError) Error() string {
	if e == nil || !knownSourceError(e.class) {
		return ErrEntropyUnavailable.Error()
	}
	return e.class.Error()
}

func (e *SourceError) Is(target error) bool {
	if e == nil || !knownSourceError(e.class) {
		return target == ErrEntropyUnavailable
	}
	return target == e.class
}

func (e *SourceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func knownSourceError(class error) bool {
	return class == ErrSourceNotConfigured ||
		class == ErrUpperBoundRequired ||
		class == ErrEntropyUnavailable
}
