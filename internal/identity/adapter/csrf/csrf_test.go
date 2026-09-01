package csrf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

var csrfTestNow = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

func TestIssueAndVerifySessionBoundToken(t *testing.T) {
	keyring := mustKeyring(t, nil, bytes.NewReader(bytes.Repeat([]byte{0x31}, NonceBytes)))
	digest := mustTokenDigest(t, 0x41)
	token, err := keyring.Issue(digest)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != Version || parts[1] != "active_1" ||
		len(parts[2]) != encodedFieldBytes || len(parts[3]) != encodedFieldBytes {
		t.Fatalf("Issue() token shape = %q", token)
	}
	if token != "v1.active_1.MTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTE.gxfhIy_ZO5fPd9RiqCfVotx1D9k6UtmCfbidc6nGB20" {
		t.Fatalf("Issue() token = %q, want stable vector", token)
	}
	if err := keyring.Verify(token, digest, csrfTestNow); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if err := keyring.Verify(token, mustTokenDigest(t, 0x42), csrfTestNow); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Verify(other session) error = %v", err)
	}

	tampered := token[:len(token)-1] + "A"
	if tampered == token {
		tampered = token[:len(token)-1] + "B"
	}
	if err := keyring.Verify(tampered, digest, csrfTestNow); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Verify(tampered) error = %v", err)
	}
}

func TestPreviousKeyHasAbsoluteExclusiveWindow(t *testing.T) {
	previousKey := mustKey(t, "previous_1", 0x22)
	previous, err := NewPreviousKey(previousKey, csrfTestNow.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	previousIssuer, err := NewKeyring(
		previousKey,
		nil,
		bytes.NewReader(bytes.Repeat([]byte{0x51}, NonceBytes)),
		csrfTestNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := mustTokenDigest(t, 0x61)
	token, err := previousIssuer.Issue(digest)
	if err != nil {
		t.Fatal(err)
	}

	rotated := mustKeyring(t, &previous, bytes.NewReader(bytes.Repeat([]byte{0x52}, NonceBytes)))
	if err := rotated.Verify(token, digest, csrfTestNow.Add(time.Hour)); err != nil {
		t.Fatalf("Verify(previous within window) error = %v", err)
	}
	if err := rotated.Verify(token, digest, previous.AcceptUntil()); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Verify(previous at deadline) error = %v", err)
	}
	newToken, err := rotated.Issue(digest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newToken, "v1.active_1.") {
		t.Fatalf("rotated Issue() used non-active key: %q", newToken)
	}
}

func TestKeyringRejectsUnsafeConfiguration(t *testing.T) {
	validMaterial := bytes.Repeat([]byte{0x11}, KeyBytes)
	for _, test := range []struct {
		id       string
		material []byte
	}{
		{id: "", material: validMaterial},
		{id: "contains.dot", material: validMaterial},
		{id: strings.Repeat("a", MaximumKeyIDBytes+1), material: validMaterial},
		{id: "active", material: nil},
		{id: "active", material: make([]byte, KeyBytes)},
	} {
		if _, err := NewKey(test.id, test.material); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("NewKey(%q,%d) error = %v", test.id, len(test.material), err)
		}
	}
	active := mustKey(t, "active_1", 0x11)
	if _, err := NewKeyring(active, nil, nil, csrfTestNow); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewKeyring(nil entropy) error = %v", err)
	}
	var typedNilEntropy *bytes.Reader
	if _, err := NewKeyring(active, nil, typedNilEntropy, csrfTestNow); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewKeyring(typed nil entropy) error = %v", err)
	}
	if _, err := NewKeyring(active, nil, bytes.NewReader(nil), time.Time{}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewKeyring(zero time) error = %v", err)
	}
	if _, err := NewKeyring(active, nil, bytes.NewReader(nil), csrfTestNow.Add(time.Nanosecond)); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewKeyring(noncanonical time) error = %v", err)
	}
	if _, err := NewPreviousKey(mustKey(t, "previous", 0x22), csrfTestNow.Add(time.Nanosecond)); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewPreviousKey(noncanonical time) error = %v", err)
	}
	for _, until := range []time.Time{
		csrfTestNow,
		csrfTestNow.Add(MaximumPreviousVerifyTime + time.Microsecond),
	} {
		previous, err := NewPreviousKey(mustKey(t, "previous", 0x22), until)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := NewKeyring(active, &previous, bytes.NewReader(nil), csrfTestNow); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("NewKeyring(previous until %v) error = %v", until, err)
		}
	}
	sameIDPrevious, err := NewPreviousKey(mustKey(t, "active_1", 0x22), csrfTestNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewKeyring(active, &sameIDPrevious, bytes.NewReader(nil), csrfTestNow); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewKeyring(duplicate id) error = %v", err)
	}
}

func TestIssueRejectsEntropyFailureAndConcurrentReadsAreSerialized(t *testing.T) {
	keyring := mustKeyring(t, nil, io.LimitReader(bytes.NewReader(nil), 0))
	if _, err := keyring.Issue(mustTokenDigest(t, 0x71)); !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("Issue(short entropy) error = %v", err)
	}
	zeroKeyring := mustKeyring(t, nil, bytes.NewReader(make([]byte, NonceBytes)))
	if _, err := zeroKeyring.Issue(mustTokenDigest(t, 0x72)); !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("Issue(zero entropy) error = %v", err)
	}

	reader := &overlapRejectingReader{}
	concurrent := mustKeyring(t, nil, reader)
	digest := mustTokenDigest(t, 0x73)
	var wait sync.WaitGroup
	errorsFound := make(chan error, 16)
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := concurrent.Issue(digest)
			errorsFound <- err
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent Issue() error = %v", err)
		}
	}
}

func TestVerifyStrictParserAndRedactedFormatting(t *testing.T) {
	keyring := mustKeyring(t, nil, bytes.NewReader(bytes.Repeat([]byte{0x44}, NonceBytes)))
	digest := mustTokenDigest(t, 0x81)
	token, err := keyring.Issue(digest)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{
		"", "v1", token + ".extra", strings.Replace(token, "v1.", "v2.", 1),
		strings.Replace(token, "active_1", "unknown", 1),
		strings.Replace(token, ".", "..", 1), token + "=",
	}
	for _, value := range invalid {
		if err := keyring.Verify(value, digest, csrfTestNow); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("Verify(%q) error = %v", value, err)
		}
	}
	if err := keyring.Verify(token, digest, csrfTestNow.Add(time.Nanosecond)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Verify(noncanonical time) error = %v", err)
	}
	var nilKeyring *Keyring
	if _, err := nilKeyring.Issue(digest); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("nil Issue() error = %v", err)
	}
	if err := nilKeyring.Verify(token, digest, csrfTestNow); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("nil Verify() error = %v", err)
	}

	encoded, err := json.Marshal(keyring)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		fmt.Sprint(keyring), fmt.Sprintf("%#v", keyring), keyring.LogValue().String(),
		slog.AnyValue(keyring).Resolve().String(), string(encoded),
		fmt.Sprint(keyring.active), fmt.Sprintf("%#v", keyring.active),
	} {
		if !strings.Contains(rendered, "REDACTED") || strings.Contains(rendered, "1111") {
			t.Fatalf("unsafe keyring rendering %q", rendered)
		}
	}
}

func mustKeyring(t *testing.T, previous *PreviousKey, entropy io.Reader) *Keyring {
	t.Helper()
	keyring, err := NewKeyring(mustKey(t, "active_1", 0x11), previous, entropy, csrfTestNow)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func mustKey(t *testing.T, id string, fill byte) Key {
	t.Helper()
	key, err := NewKey(id, bytes.Repeat([]byte{fill}, KeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustTokenDigest(t *testing.T, fill byte) identity.TokenDigest {
	t.Helper()
	digest, err := identity.NewTokenDigest(bytes.Repeat([]byte{fill}, identity.DigestBytes))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

type overlapRejectingReader struct {
	mu     sync.Mutex
	active bool
	next   byte
}

func (reader *overlapRejectingReader) Read(destination []byte) (int, error) {
	reader.mu.Lock()
	if reader.active {
		reader.mu.Unlock()
		return 0, errors.New("overlapping entropy read")
	}
	reader.active = true
	reader.next++
	fill := reader.next
	reader.mu.Unlock()

	for index := range destination {
		destination[index] = fill
	}

	reader.mu.Lock()
	reader.active = false
	reader.mu.Unlock()
	return len(destination), nil
}
