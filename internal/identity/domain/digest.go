package domain

import (
	"fmt"
	"log/slog"
)

const DigestBytes = 32

const redactedValue = "<redacted>"

// TokenDigest is the fixed SHA-256-sized lookup digest of an opaque high-
// entropy bearer value. It never contains or reconstructs that bearer value.
type TokenDigest struct {
	value [DigestBytes]byte
}

func NewTokenDigest(value []byte) (TokenDigest, error) {
	var digest TokenDigest
	if len(value) != DigestBytes {
		return TokenDigest{}, fmt.Errorf(
			"%w: got %d bytes, want %d",
			ErrTokenDigestInvalid,
			len(value),
			DigestBytes,
		)
	}
	copy(digest.value[:], value)
	if err := digest.Validate(); err != nil {
		return TokenDigest{}, err
	}
	return digest, nil
}

func (digest TokenDigest) Validate() error {
	if allZero(digest.value[:]) {
		return ErrTokenDigestInvalid
	}
	return nil
}

func (digest TokenDigest) Bytes() []byte { return cloneBytes(digest.value[:]) }

// String, GoString and LogValue prevent accidental disclosure through the
// standard formatting and structured-logging paths. Bytes is the only
// intentional extraction boundary.
func (TokenDigest) String() string { return redactedValue }

func (TokenDigest) GoString() string { return "domain.TokenDigest(" + redactedValue + ")" }

func (TokenDigest) LogValue() slog.Value { return slog.StringValue(redactedValue) }

// ThrottleDigest is a fixed, non-reversible lookup digest for one bounded
// throttle dimension key.
type ThrottleDigest struct {
	value [DigestBytes]byte
}

func NewThrottleDigest(value []byte) (ThrottleDigest, error) {
	var digest ThrottleDigest
	if len(value) != DigestBytes {
		return ThrottleDigest{}, fmt.Errorf(
			"%w: got %d bytes, want %d",
			ErrThrottleDigestInvalid,
			len(value),
			DigestBytes,
		)
	}
	copy(digest.value[:], value)
	if err := digest.Validate(); err != nil {
		return ThrottleDigest{}, err
	}
	return digest, nil
}

func (digest ThrottleDigest) Validate() error {
	if allZero(digest.value[:]) {
		return ErrThrottleDigestInvalid
	}
	return nil
}

func (digest ThrottleDigest) Bytes() []byte { return cloneBytes(digest.value[:]) }

func (ThrottleDigest) String() string { return redactedValue }

func (ThrottleDigest) GoString() string { return "domain.ThrottleDigest(" + redactedValue + ")" }

func (ThrottleDigest) LogValue() slog.Value { return slog.StringValue(redactedValue) }

func allZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
