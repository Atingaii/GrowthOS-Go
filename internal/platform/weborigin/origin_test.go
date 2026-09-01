package weborigin

import (
	"errors"
	"testing"
)

func TestParseExactAcceptsCanonicalBrowserOrigins(t *testing.T) {
	for _, value := range []string{
		"http://127.0.0.1",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"https://growth.example",
		"https://api-1.growth.example:8443",
		"https://xn--fsqu00a.example",
	} {
		origin, err := ParseExact(value)
		if err != nil || origin.Validate() != nil || origin.String() != value {
			t.Fatalf("ParseExact(%q) = %#v, %v", value, origin, err)
		}
	}
}

func TestParseExactRejectsNormalizationAndAuthorityAmbiguity(t *testing.T) {
	for _, value := range []string{
		"", " https://growth.example", "ftp://growth.example",
		"HTTPS://growth.example", "https://GROWTH.example",
		"http://127.0.0.1:80", "https://growth.example:443",
		"http://127.0.0.1:08080", "http://127.000.0.1",
		"http://127.1", "http://0x7f.0.0.1", "http://[0:0:0:0:0:0:0:1]",
		"http://[::ffff:127.0.0.1]", "http://[fe80::1%25en0]",
		"https://growth.example.", "https://growth.123", "https://growth_example",
		"https://用户.example", "https://user@growth.example",
		"https://growth.example/", "https://growth.example?x=1",
		"https://growth.example#fragment",
	} {
		if _, err := ParseExact(value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ParseExact(%q) error = %v", value, err)
		}
	}
}

func TestOriginLoopbackClassificationIsNumericAndExplicit(t *testing.T) {
	for value, want := range map[string]bool{
		"http://127.0.0.1:8080":  true,
		"http://[::1]:8080":      true,
		"https://growth.example": false,
		"https://localhost":      false,
	} {
		origin, err := ParseExact(value)
		if err != nil || origin.IsLoopback() != want {
			t.Fatalf("ParseExact(%q) loopback = %v, error = %v", value, origin.IsLoopback(), err)
		}
	}
}
