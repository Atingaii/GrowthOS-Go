package csrf

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

const (
	Version                   = "v1"
	KeyBytes                  = 32
	NonceBytes                = 32
	MACBytes                  = 32
	MinimumKeyIDBytes         = 1
	MaximumKeyIDBytes         = 16
	MaximumPreviousVerifyTime = 8 * time.Hour
	encodedFieldBytes         = 43
	encodedTokenBytes         = 2 + 1 + MaximumKeyIDBytes + 1 + encodedFieldBytes + 1 + encodedFieldBytes
	domainLabel               = "growthos-csrf-v1"
)

var (
	ErrInvalidConfiguration = errors.New("identity csrf configuration is invalid")
	ErrEntropyUnavailable   = errors.New("identity csrf entropy is unavailable")
	ErrTokenInvalid         = errors.New("identity csrf token is invalid")
)

// Key is an immutable, redacted copy of one configured HMAC key.
type Key struct {
	id       string
	material [KeyBytes]byte
}

func NewKey(id string, material []byte) (Key, error) {
	if !validKeyID(id) || len(material) != KeyBytes || allZero(material) {
		return Key{}, ErrInvalidConfiguration
	}
	key := Key{id: id}
	copy(key.material[:], material)
	return key, nil
}

func (key Key) Validate() error {
	if !validKeyID(key.id) || allZero(key.material[:]) {
		return ErrInvalidConfiguration
	}
	return nil
}

func (key Key) ID() string { return key.id }

func (Key) String() string   { return "csrf.Key{[REDACTED]}" }
func (Key) GoString() string { return "csrf.Key{[REDACTED]}" }
func (Key) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
func (Key) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// PreviousKey gives a retired key one absolute, bounded verification window.
// The timestamp is configuration, not token-controlled state.
type PreviousKey struct {
	key         Key
	acceptUntil time.Time
}

func NewPreviousKey(key Key, acceptUntil time.Time) (PreviousKey, error) {
	if key.Validate() != nil || !canonicalNonZeroInstant(acceptUntil) {
		return PreviousKey{}, ErrInvalidConfiguration
	}
	return PreviousKey{key: key, acceptUntil: acceptUntil}, nil
}

func (previous PreviousKey) KeyID() string { return previous.key.id }

func (previous PreviousKey) AcceptUntil() time.Time { return previous.acceptUntil }

func (PreviousKey) String() string   { return "csrf.PreviousKey{[REDACTED]}" }
func (PreviousKey) GoString() string { return "csrf.PreviousKey{[REDACTED]}" }
func (PreviousKey) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
func (PreviousKey) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// Keyring issues only with active and verifies with active plus at most one
// previous key. It is safe for concurrent use.
type Keyring struct {
	active      Key
	previous    PreviousKey
	hasPrevious bool
	entropy     io.Reader
	entropyMu   sync.Mutex
}

// NewKeyring validates the previous-key absolute window against the startup
// instant. Restarts therefore cannot silently restart an eight-hour duration.
func NewKeyring(
	active Key,
	previous *PreviousKey,
	entropy io.Reader,
	configuredAt time.Time,
) (*Keyring, error) {
	if active.Validate() != nil || dependencyIsNil(entropy) ||
		!canonicalNonZeroInstant(configuredAt) {
		return nil, ErrInvalidConfiguration
	}
	keyring := &Keyring{active: active, entropy: entropy}
	if previous != nil {
		if previous.key.Validate() != nil || previous.acceptUntil.IsZero() ||
			previous.key.id == active.id ||
			!configuredAt.Before(previous.acceptUntil) ||
			previous.acceptUntil.After(configuredAt.Add(MaximumPreviousVerifyTime)) {
			return nil, ErrInvalidConfiguration
		}
		keyring.previous = *previous
		keyring.hasPrevious = true
	}
	return keyring, nil
}

func (*Keyring) String() string   { return "csrf.Keyring{[REDACTED]}" }
func (*Keyring) GoString() string { return "csrf.Keyring{[REDACTED]}" }
func (*Keyring) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
func (*Keyring) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// Issue returns exactly v1.<key_id>.<nonce>.<mac> with raw URL encoding and a
// MAC bound to the authoritative session-token digest.
func (keyring *Keyring) Issue(sessionDigest identity.TokenDigest) (string, error) {
	if keyring == nil || keyring.active.Validate() != nil || keyring.entropy == nil ||
		sessionDigest.Validate() != nil {
		return "", ErrInvalidConfiguration
	}
	nonce := make([]byte, NonceBytes)
	keyring.entropyMu.Lock()
	_, err := io.ReadFull(keyring.entropy, nonce)
	keyring.entropyMu.Unlock()
	if err != nil || allZero(nonce) {
		clear(nonce)
		return "", ErrEntropyUnavailable
	}
	defer clear(nonce)
	mac := calculateMAC(keyring.active, sessionDigest, nonce)
	defer clear(mac)
	return strings.Join([]string{
		Version,
		keyring.active.id,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(mac),
	}, "."), nil
}

// Verify strictly parses and constant-time verifies one token. Unknown,
// expired, malformed, wrong-session and wrong-MAC inputs share one error.
func (keyring *Keyring) Verify(
	rawToken string,
	sessionDigest identity.TokenDigest,
	now time.Time,
) error {
	if keyring == nil || keyring.active.Validate() != nil ||
		sessionDigest.Validate() != nil || !canonicalNonZeroInstant(now) {
		return ErrTokenInvalid
	}
	version, keyID, nonce, suppliedMAC, ok := parseToken(rawToken)
	if !ok {
		return ErrTokenInvalid
	}
	defer clear(nonce)
	defer clear(suppliedMAC)

	selected := keyring.active
	keyAccepted := subtle.ConstantTimeCompare([]byte(keyID), []byte(keyring.active.id)) == 1
	if !keyAccepted && keyring.hasPrevious &&
		subtle.ConstantTimeCompare([]byte(keyID), []byte(keyring.previous.key.id)) == 1 {
		selected = keyring.previous.key
		keyAccepted = now.Before(keyring.previous.acceptUntil)
	}
	expectedMAC := calculateMAC(selected, sessionDigest, nonce)
	defer clear(expectedMAC)
	validMAC := subtle.ConstantTimeCompare(suppliedMAC, expectedMAC) == 1
	validVersion := subtle.ConstantTimeCompare([]byte(version), []byte(Version)) == 1
	if !keyAccepted || !validMAC || !validVersion {
		return ErrTokenInvalid
	}
	return nil
}

func parseToken(rawToken string) (string, string, []byte, []byte, bool) {
	if len(rawToken) == 0 || len(rawToken) > encodedTokenBytes {
		return "", "", nil, nil, false
	}
	parts := strings.Split(rawToken, ".")
	if len(parts) != 4 || parts[0] != Version || !validKeyID(parts[1]) ||
		len(parts[2]) != encodedFieldBytes || len(parts[3]) != encodedFieldBytes {
		return "", "", nil, nil, false
	}
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(parts[2])
	if nonceErr != nil || len(nonce) != NonceBytes ||
		base64.RawURLEncoding.EncodeToString(nonce) != parts[2] || allZero(nonce) {
		clear(nonce)
		return "", "", nil, nil, false
	}
	mac, macErr := base64.RawURLEncoding.DecodeString(parts[3])
	if macErr != nil || len(mac) != MACBytes ||
		base64.RawURLEncoding.EncodeToString(mac) != parts[3] {
		clear(nonce)
		clear(mac)
		return "", "", nil, nil, false
	}
	return parts[0], parts[1], nonce, mac, true
}

func calculateMAC(key Key, sessionDigest identity.TokenDigest, nonce []byte) []byte {
	mac := hmac.New(sha256.New, key.material[:])
	writeLengthPrefixed(mac, []byte(domainLabel))
	writeLengthPrefixed(mac, []byte(key.id))
	digest := sessionDigest.Bytes()
	writeLengthPrefixed(mac, digest)
	clear(digest)
	writeLengthPrefixed(mac, nonce)
	return mac.Sum(nil)
}

func writeLengthPrefixed(destination hash.Hash, value []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	_, _ = destination.Write(length[:])
	_, _ = destination.Write(value)
}

func validKeyID(value string) bool {
	if len(value) < MinimumKeyIDBytes || len(value) > MaximumKeyIDBytes {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func canonicalInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Truncate(time.Microsecond)
}

func canonicalNonZeroInstant(value time.Time) bool {
	return !value.IsZero() && value == canonicalInstant(value)
}

func dependencyIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func allZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
