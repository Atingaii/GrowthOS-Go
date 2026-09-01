package passwordhash

import (
	"bytes"
	"context"
	"crypto/subtle"
	"io"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	MinimumEnrollmentCodePoints = 12
	MinimumLoginCodePoints      = 1
	MaximumPasswordCodePoints   = 128
	MaximumPasswordBytes        = 512
)

var currentParameters = parameters{
	memory:      CurrentMemoryKiB,
	iterations:  CurrentIterations,
	parallelism: CurrentParallelism,
}

// Verification is deliberately redacted when formatted. NeedsRehash is only
// meaningful when Matched is true and must not be exposed in an HTTP response.
type Verification struct {
	matched     bool
	needsRehash bool
}

// Matched reports whether the supplied credential matched the envelope.
func (v Verification) Matched() bool { return v.matched }

// NeedsRehash reports whether a successful match used a non-current accepted
// profile. It is always false when Matched is false.
func (v Verification) NeedsRehash() bool { return v.needsRehash }

func (v Verification) String() string { return "passwordhash.Verification{[REDACTED]}" }

func (v Verification) GoString() string { return v.String() }

func (v Verification) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// Hasher is safe for concurrent use. All Hasher values built by New share one
// process gate, so repeated construction cannot multiply Argon2 concurrency.
type Hasher struct {
	gate           *workGate
	acquireTimeout time.Duration
	entropy        io.Reader
	entropyMu      sync.Mutex
	dummy          parsedEnvelope
}

// New constructs a password adapter. The first successful constructor fixes
// the process-wide concurrency limit; later constructors must request the same
// limit and share the same gate.
func New(config Config) (*Hasher, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	gate, err := sharedWorkGate(normalized.maxConcurrent)
	if err != nil {
		return nil, err
	}
	dummy, err := fixedDummyEnvelope()
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	return &Hasher{
		gate:           gate,
		acquireTimeout: normalized.acquireTimeout,
		entropy:        normalized.entropy,
		dummy:          dummy,
	}, nil
}

func (h *Hasher) String() string { return "passwordhash.Hasher{[REDACTED]}" }

func (h *Hasher) GoString() string { return h.String() }

func (h *Hasher) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// HashEnrollment validates the stricter bootstrap policy and creates a current
// Argon2id PHC envelope with an injected cryptographic entropy source.
func (h *Hasher) HashEnrollment(ctx context.Context, password []byte) (Envelope, error) {
	if h == nil || h.gate == nil || h.entropy == nil {
		return Envelope{}, ErrInvalidConfiguration
	}
	passwordCopy := bytes.Clone(password)
	defer zero(passwordCopy)
	if err := validatePassword(passwordCopy, MinimumEnrollmentCodePoints); err != nil {
		return Envelope{}, err
	}
	if err := h.gate.acquire(ctx, h.acquireTimeout); err != nil {
		return Envelope{}, err
	}
	defer h.gate.release()

	if err := contextError(ctx); err != nil {
		return Envelope{}, err
	}
	salt := make([]byte, SaltBytes)
	defer zero(salt)
	h.entropyMu.Lock()
	_, entropyErr := io.ReadFull(h.entropy, salt)
	h.entropyMu.Unlock()
	if entropyErr != nil || allZeroBytes(salt) {
		return Envelope{}, ErrEntropyUnavailable
	}
	output := derive(passwordCopy, salt, currentParameters)
	defer zero(output)
	if err := contextError(ctx); err != nil {
		return Envelope{}, err
	}
	encoded, err := encodeEnvelope(currentParameters, salt, output)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{encoded: encoded}, nil
}

// VerifyLogin strictly parses a persisted envelope and performs bounded
// verification. A mismatch is a zero Verification with no error.
func (h *Hasher) VerifyLogin(
	ctx context.Context,
	password []byte,
	encodedEnvelope string,
) (Verification, error) {
	if h == nil || h.gate == nil {
		return Verification{}, ErrInvalidConfiguration
	}
	passwordCopy := bytes.Clone(password)
	defer zero(passwordCopy)
	if err := validatePassword(passwordCopy, MinimumLoginCodePoints); err != nil {
		return Verification{}, err
	}
	parsed, err := parseEnvelope(encodedEnvelope)
	if err != nil {
		return Verification{}, err
	}
	defer zero(parsed.salt)
	defer zero(parsed.output)
	matched, err := h.verifyParsed(ctx, passwordCopy, parsed)
	if err != nil || !matched {
		return Verification{}, err
	}
	return Verification{matched: true, needsRehash: !parsed.parameters.current()}, nil
}

// VerifyUnknownLogin performs the fixed current-profile dummy path for an
// unknown account. It intentionally discards the comparison result.
func (h *Hasher) VerifyUnknownLogin(ctx context.Context, password []byte) error {
	if h == nil || h.gate == nil {
		return ErrInvalidConfiguration
	}
	passwordCopy := bytes.Clone(password)
	defer zero(passwordCopy)
	if err := validatePassword(passwordCopy, MinimumLoginCodePoints); err != nil {
		return err
	}
	_, err := h.verifyParsed(ctx, passwordCopy, h.dummy)
	return err
}

func (h *Hasher) verifyParsed(
	ctx context.Context,
	password []byte,
	parsed parsedEnvelope,
) (bool, error) {
	if err := h.gate.acquire(ctx, h.acquireTimeout); err != nil {
		return false, err
	}
	defer h.gate.release()
	if err := contextError(ctx); err != nil {
		return false, err
	}

	actual := derive(password, parsed.salt, parsed.parameters)
	defer zero(actual)
	if err := contextError(ctx); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(actual, parsed.output) == 1, nil
}

func validatePassword(password []byte, minimumCodePoints int) error {
	if !utf8.Valid(password) || len(password) > MaximumPasswordBytes {
		return ErrPasswordRejected
	}
	count := utf8.RuneCount(password)
	if count < minimumCodePoints || count > MaximumPasswordCodePoints {
		return ErrPasswordRejected
	}
	return nil
}

func derive(password, salt []byte, params parameters) []byte {
	return argon2.IDKey(
		password,
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		OutputBytes,
	)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidConfiguration
	}
	if err := ctx.Err(); err != nil {
		return hashingUnavailable(err)
	}
	return nil
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func allZeroBytes(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}

var (
	dummyOnce  sync.Once
	dummyValue parsedEnvelope
	dummyError error
)

func fixedDummyEnvelope() (parsedEnvelope, error) {
	dummyOnce.Do(func() {
		salt := []byte("GrowthOS-dummy-1")
		password := []byte("GrowthOS fixed dummy credential v1")
		output := derive(password, salt, currentParameters)
		encoded, err := encodeEnvelope(currentParameters, salt, output)
		zero(password)
		zero(output)
		if err != nil {
			dummyError = err
			return
		}
		dummyValue, dummyError = parseEnvelope(encoded)
	})
	return dummyValue, dummyError
}
