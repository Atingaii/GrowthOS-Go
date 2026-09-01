package passwordhash

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	CurrentMemoryKiB     uint32 = 19456
	CurrentIterations    uint32 = 2
	CurrentParallelism   uint8  = 1
	SaltBytes                   = 16
	OutputBytes          uint32 = 32
	MaximumEnvelopeBytes        = 256

	MinimumLegacyMemoryKiB   uint32 = 8192
	MaximumLegacyMemoryKiB   uint32 = 65536
	MinimumLegacyIterations  uint32 = 1
	MaximumLegacyIterations  uint32 = 4
	MinimumLegacyParallelism uint8  = 1
	MaximumLegacyParallelism uint8  = 4
)

var phcBase64 = base64.RawStdEncoding.Strict()

// Envelope is a validated PHC envelope. Formatting is redacted; Encoded is an
// explicit trust-boundary operation for persistence.
type Envelope struct {
	encoded string
}

// ParseEnvelope validates the complete PHC grammar before any Argon2 work.
func ParseEnvelope(encoded string) (Envelope, error) {
	parsed, err := parseEnvelope(encoded)
	if err != nil {
		return Envelope{}, err
	}
	zero(parsed.salt)
	zero(parsed.output)
	return Envelope{encoded: encoded}, nil
}

// Encoded returns the persistence representation. Callers must never log it.
func (e Envelope) Encoded() string { return e.encoded }

func (e Envelope) String() string { return "passwordhash.Envelope{[REDACTED]}" }

func (e Envelope) GoString() string { return e.String() }

func (e Envelope) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

type parameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func (p parameters) current() bool {
	return p.memory == CurrentMemoryKiB &&
		p.iterations == CurrentIterations &&
		p.parallelism == CurrentParallelism
}

type parsedEnvelope struct {
	parameters parameters
	salt       []byte
	output     []byte
}

func parseEnvelope(encoded string) (parsedEnvelope, error) {
	if encoded == "" || len(encoded) > MaximumEnvelopeBytes {
		return parsedEnvelope{}, ErrInvalidEnvelope
	}

	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return parsedEnvelope{}, ErrInvalidEnvelope
	}

	params, err := parseParameters(parts[3])
	if err != nil {
		return parsedEnvelope{}, ErrInvalidEnvelope
	}
	salt, err := decodeCanonicalBase64(parts[4], SaltBytes)
	if err != nil {
		return parsedEnvelope{}, ErrInvalidEnvelope
	}
	output, err := decodeCanonicalBase64(parts[5], int(OutputBytes))
	if err != nil {
		return parsedEnvelope{}, ErrInvalidEnvelope
	}

	return parsedEnvelope{parameters: params, salt: salt, output: output}, nil
}

func parseParameters(encoded string) (parameters, error) {
	parts := strings.Split(encoded, ",")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "m=") ||
		!strings.HasPrefix(parts[1], "t=") || !strings.HasPrefix(parts[2], "p=") {
		return parameters{}, ErrInvalidEnvelope
	}

	memory, err := parseCanonicalUint(strings.TrimPrefix(parts[0], "m="), 32)
	if err != nil || memory < uint64(MinimumLegacyMemoryKiB) || memory > uint64(MaximumLegacyMemoryKiB) {
		return parameters{}, ErrInvalidEnvelope
	}
	iterations, err := parseCanonicalUint(strings.TrimPrefix(parts[1], "t="), 32)
	if err != nil || iterations < uint64(MinimumLegacyIterations) || iterations > uint64(MaximumLegacyIterations) {
		return parameters{}, ErrInvalidEnvelope
	}
	parallelism, err := parseCanonicalUint(strings.TrimPrefix(parts[2], "p="), 8)
	if err != nil || parallelism < uint64(MinimumLegacyParallelism) || parallelism > uint64(MaximumLegacyParallelism) {
		return parameters{}, ErrInvalidEnvelope
	}
	if memory < 8*parallelism {
		return parameters{}, ErrInvalidEnvelope
	}

	return parameters{
		memory:      uint32(memory),
		iterations:  uint32(iterations),
		parallelism: uint8(parallelism),
	}, nil
}

func parseCanonicalUint(encoded string, bits int) (uint64, error) {
	if encoded == "" || (len(encoded) > 1 && encoded[0] == '0') {
		return 0, ErrInvalidEnvelope
	}
	for index := range encoded {
		if encoded[index] < '0' || encoded[index] > '9' {
			return 0, ErrInvalidEnvelope
		}
	}
	value, err := strconv.ParseUint(encoded, 10, bits)
	if err != nil {
		return 0, ErrInvalidEnvelope
	}
	return value, nil
}

func decodeCanonicalBase64(encoded string, expectedLength int) ([]byte, error) {
	if encoded == "" || strings.ContainsRune(encoded, '=') {
		return nil, ErrInvalidEnvelope
	}
	decoded, err := phcBase64.DecodeString(encoded)
	if err != nil || len(decoded) != expectedLength {
		return nil, ErrInvalidEnvelope
	}
	if phcBase64.EncodeToString(decoded) != encoded {
		return nil, ErrInvalidEnvelope
	}
	return decoded, nil
}

func encodeEnvelope(params parameters, salt, output []byte) (string, error) {
	if len(salt) != SaltBytes || len(output) != int(OutputBytes) {
		return "", ErrInvalidEnvelope
	}
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.memory,
		params.iterations,
		params.parallelism,
		phcBase64.EncodeToString(salt),
		phcBase64.EncodeToString(output),
	)
	if len(encoded) > MaximumEnvelopeBytes {
		return "", ErrInvalidEnvelope
	}
	return encoded, nil
}
