package passwordhash

import (
	"errors"
	"strings"
	"testing"
)

func TestParseEnvelopeRejectsNonCanonicalOrUnsafePHC(t *testing.T) {
	t.Parallel()

	salt := phcBase64.EncodeToString(make([]byte, SaltBytes))
	output := phcBase64.EncodeToString(make([]byte, OutputBytes))
	valid := "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$" + output
	if _, err := ParseEnvelope(valid); err != nil {
		t.Fatalf("ParseEnvelope(valid) error = %v", err)
	}

	cases := map[string]string{
		"empty":                          "",
		"too long":                       strings.Repeat("x", MaximumEnvelopeBytes+1),
		"algorithm":                      strings.Replace(valid, "argon2id", "argon2i", 1),
		"version":                        strings.Replace(valid, "v=19", "v=16", 1),
		"missing version":                strings.Replace(valid, "$v=19", "", 1),
		"parameter order":                strings.Replace(valid, "m=19456,t=2,p=1", "t=2,m=19456,p=1", 1),
		"unknown parameter":              strings.Replace(valid, "p=1", "q=1", 1),
		"duplicate parameter":            strings.Replace(valid, "p=1", "p=1,p=1", 1),
		"leading zero":                   strings.Replace(valid, "m=19456", "m=019456", 1),
		"memory below hard minimum":      strings.Replace(valid, "m=19456", "m=8191", 1),
		"memory above hard maximum":      strings.Replace(valid, "m=19456", "m=65537", 1),
		"iterations below hard minimum":  strings.Replace(valid, "t=2", "t=0", 1),
		"iterations above hard maximum":  strings.Replace(valid, "t=2", "t=5", 1),
		"parallelism below hard minimum": strings.Replace(valid, "p=1", "p=0", 1),
		"parallelism above hard maximum": strings.Replace(valid, "p=1", "p=5", 1),
		"numeric sign":                   strings.Replace(valid, "t=2", "t=+2", 1),
		"numeric overflow":               strings.Replace(valid, "m=19456", "m=18446744073709551616", 1),
		"padded salt":                    strings.Replace(valid, salt, salt+"=", 1),
		"padded output":                  valid + "=",
		"short salt":                     strings.Replace(valid, salt, phcBase64.EncodeToString(make([]byte, SaltBytes-1)), 1),
		"short output":                   strings.Replace(valid, output, phcBase64.EncodeToString(make([]byte, OutputBytes-1)), 1),
		"invalid base64":                 strings.Replace(valid, salt, strings.Repeat("*", len(salt)), 1),
		"extra field":                    valid + "$extra",
		"trailing whitespace":            valid + " ",
		"embedded nul":                   strings.Replace(valid, "argon2id", "argon2id\x00", 1),
	}

	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := ParseEnvelope(encoded)
			if result != (Envelope{}) {
				t.Fatalf("ParseEnvelope() returned non-zero result")
			}
			if !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("ParseEnvelope() error = %v, want ErrInvalidEnvelope", err)
			}
			if encoded != "" && strings.Contains(err.Error(), encoded) {
				t.Fatalf("ParseEnvelope() error exposed input")
			}
		})
	}
}

func TestParseEnvelopeAcceptsHardBoundedLegacyProfiles(t *testing.T) {
	t.Parallel()

	salt := phcBase64.EncodeToString(make([]byte, SaltBytes))
	output := phcBase64.EncodeToString(make([]byte, OutputBytes))
	profiles := []string{
		"m=8192,t=1,p=1",
		"m=65536,t=4,p=4",
		"m=19456,t=2,p=1",
	}
	for _, profile := range profiles {
		encoded := "$argon2id$v=19$" + profile + "$" + salt + "$" + output
		if _, err := ParseEnvelope(encoded); err != nil {
			t.Fatalf("ParseEnvelope(%s) error = %v", profile, err)
		}
	}
}
