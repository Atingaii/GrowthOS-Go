package throttledigest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash"
	"log/slog"
	"net/netip"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

const (
	KeyBytes                    = 32
	MaximumCanonicalSourceBytes = 128
	domainLabel                 = "growthos-auth-throttle-v1"
)

var (
	ErrInvalidConfiguration = errors.New("identity throttle digest configuration is invalid")
	ErrInvalidSubject       = errors.New("identity throttle digest subject is invalid")
)

// Digester is immutable and safe for concurrent use. Its key is copied at the
// constructor boundary and is redacted from every ordinary formatting path.
type Digester struct {
	key [KeyBytes]byte
}

// New requires one dedicated, nonzero 256-bit key. The key must not be reused
// for CSRF, password hashing, bearer tokens, or any other purpose.
func New(key []byte) (*Digester, error) {
	if len(key) != KeyBytes || allZero(key) {
		return nil, ErrInvalidConfiguration
	}
	digester := &Digester{}
	copy(digester.key[:], key)
	return digester, nil
}

func (digester *Digester) String() string {
	return "throttledigest.Digester{[REDACTED]}"
}

func (digester *Digester) GoString() string { return digester.String() }

func (digester *Digester) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}

func (digester *Digester) MarshalJSON() ([]byte, error) {
	return json.Marshal("[REDACTED]")
}

// DigestLogin derives the login dimension from the already canonical Identity
// value. It never normalizes or case-folds caller input.
func (digester *Digester) DigestLogin(
	login identity.LoginName,
) (identity.ThrottleDigest, error) {
	if digester == nil || allZero(digester.key[:]) || login.Validate() != nil {
		return identity.ThrottleDigest{}, ErrInvalidSubject
	}
	return digester.derive(identity.ThrottleDimensionLogin, []byte(login.String()))
}

// DigestSource derives the source dimension from a parsed socket address.
// IPv4-mapped IPv6 is unwrapped so one peer cannot obtain two budgets merely
// by changing the textual representation. Zoned addresses are rejected.
func (digester *Digester) DigestSource(
	address netip.Addr,
) (identity.ThrottleDigest, error) {
	if digester == nil || allZero(digester.key[:]) || !address.IsValid() || address.Zone() != "" {
		return identity.ThrottleDigest{}, ErrInvalidSubject
	}
	canonical := []byte(address.Unmap().String())
	if len(canonical) == 0 || len(canonical) > MaximumCanonicalSourceBytes {
		return identity.ThrottleDigest{}, ErrInvalidSubject
	}
	return digester.derive(identity.ThrottleDimensionSource, canonical)
}

func (digester *Digester) derive(
	dimension identity.ThrottleDimension,
	subject []byte,
) (identity.ThrottleDigest, error) {
	if !dimension.Valid() || len(subject) == 0 || len(subject) > MaximumCanonicalSourceBytes {
		return identity.ThrottleDigest{}, ErrInvalidSubject
	}
	mac := hmac.New(sha256.New, digester.key[:])
	writeLengthPrefixed(mac, []byte(domainLabel))
	writeLengthPrefixed(mac, []byte(dimension))
	writeLengthPrefixed(mac, subject)
	value := mac.Sum(nil)
	defer clear(value)
	digest, err := identity.NewThrottleDigest(value)
	if err != nil {
		return identity.ThrottleDigest{}, ErrInvalidConfiguration
	}
	return digest, nil
}

func writeLengthPrefixed(destination hash.Hash, value []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = destination.Write(length[:])
	_, _ = destination.Write(value)
}

func allZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
