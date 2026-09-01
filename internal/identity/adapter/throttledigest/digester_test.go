package throttledigest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"testing"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestDigesterUsesStableLengthPrefixedDomainSeparation(t *testing.T) {
	key := make([]byte, KeyBytes)
	for index := range key {
		key[index] = byte(index)
	}
	digester, err := New(key)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	login, err := identity.NewLoginName("alice.ops")
	if err != nil {
		t.Fatal(err)
	}
	loginDigest, err := digester.DigestLogin(login)
	if err != nil {
		t.Fatalf("DigestLogin() error = %v", err)
	}
	if got, want := hex.EncodeToString(loginDigest.Bytes()), "0d1662de1a60febad881659501a1f8b78b7bea45930b293459df23bdfb3eaf44"; got != want {
		t.Fatalf("DigestLogin() = %s, want stable vector %s", got, want)
	}

	sourceDigest, err := digester.DigestSource(netip.MustParseAddr("127.0.0.1"))
	if err != nil {
		t.Fatalf("DigestSource() error = %v", err)
	}
	if bytes.Equal(loginDigest.Bytes(), sourceDigest.Bytes()) {
		t.Fatal("login and source dimensions produced the same digest")
	}
}

func TestDigestSourceCanonicalizesIPv4MappedAddress(t *testing.T) {
	digester := mustDigester(t)
	plain, err := digester.DigestSource(netip.MustParseAddr("192.0.2.10"))
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := digester.DigestSource(netip.MustParseAddr("::ffff:192.0.2.10"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain.Bytes(), mapped.Bytes()) {
		t.Fatal("IPv4 and IPv4-mapped source did not share one budget")
	}
}

func TestDigesterRejectsInvalidConfigurationAndSubjects(t *testing.T) {
	for _, key := range [][]byte{nil, make([]byte, KeyBytes-1), make([]byte, KeyBytes)} {
		if _, err := New(key); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("New(%d bytes) error = %v", len(key), err)
		}
	}
	digester := mustDigester(t)
	if _, err := digester.DigestLogin(""); !errors.Is(err, ErrInvalidSubject) {
		t.Fatalf("DigestLogin(invalid) error = %v", err)
	}
	if _, err := digester.DigestSource(netip.Addr{}); !errors.Is(err, ErrInvalidSubject) {
		t.Fatalf("DigestSource(invalid) error = %v", err)
	}
	if _, err := digester.DigestSource(netip.MustParseAddr("fe80::1%trusted0")); !errors.Is(err, ErrInvalidSubject) {
		t.Fatalf("DigestSource(zoned) error = %v", err)
	}
	var nilDigester *Digester
	if _, err := nilDigester.DigestLogin("alice.ops"); !errors.Is(err, ErrInvalidSubject) {
		t.Fatalf("nil DigestLogin() error = %v", err)
	}
	if _, err := nilDigester.DigestSource(netip.MustParseAddr("127.0.0.1")); !errors.Is(err, ErrInvalidSubject) {
		t.Fatalf("nil DigestSource() error = %v", err)
	}
}

func TestDigesterDoesNotRetainCallerKeyAndFormattingIsRedacted(t *testing.T) {
	key := bytes.Repeat([]byte{0x5a}, KeyBytes)
	digester, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	clear(key)
	login, _ := identity.NewLoginName("alice.ops")
	if _, err := digester.DigestLogin(login); err != nil {
		t.Fatalf("DigestLogin() after caller key clear error = %v", err)
	}
	encoded, err := json.Marshal(digester)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{
		fmt.Sprint(digester),
		fmt.Sprintf("%#v", digester),
		digester.LogValue().String(),
		slog.AnyValue(digester).Resolve().String(),
		string(encoded),
	} {
		if !strings.Contains(rendered, "REDACTED") || strings.Contains(rendered, "5a5a") {
			t.Fatalf("unsafe digester rendering %q", rendered)
		}
	}
}

func mustDigester(t *testing.T) *Digester {
	t.Helper()
	key := bytes.Repeat([]byte{0x7c}, KeyBytes)
	digester, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return digester
}
