package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxActivityNameRunes is the character-level persistence and display bound.
	MaxActivityNameRunes = 128
	// MaxForeignRevisionBytes bounds Lottery-owned correlation tokens copied into
	// Marketing publications. The token is not claimed to be a content hash.
	MaxForeignRevisionBytes = 128
	// MaxEvidenceReferenceBytes bounds opaque approval and retirement evidence
	// references. Evidence payloads and caller identity are intentionally absent.
	MaxEvidenceReferenceBytes = 128
)

// ActivityName is the canonical, user-facing Activity label.
type ActivityName string

// NewActivityName trims surrounding Unicode whitespace and rejects control
// characters, invalid UTF-8, empty values, and values beyond the rune budget.
func NewActivityName(value string) (ActivityName, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("%w: value is not valid UTF-8", ErrActivityNameInvalid)
	}
	value = strings.TrimSpace(value)
	name := ActivityName(value)
	if err := name.Validate(); err != nil {
		return "", err
	}
	return name, nil
}

// Validate rejects non-canonical persisted names rather than silently changing
// them during aggregate restoration.
func (name ActivityName) Validate() error {
	value := string(name)
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: value is not valid UTF-8", ErrActivityNameInvalid)
	}
	if value == "" {
		return fmt.Errorf("%w: value is required", ErrActivityNameInvalid)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%w: value is not canonical", ErrActivityNameInvalid)
	}
	if utf8.RuneCountInString(value) > MaxActivityNameRunes {
		return fmt.Errorf("%w: exceeds %d runes", ErrActivityNameInvalid, MaxActivityNameRunes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: contains a control character", ErrActivityNameInvalid)
		}
	}
	return nil
}

// String returns the canonical Activity label.
func (name ActivityName) String() string { return string(name) }

// EvidenceReference is a bounded opaque pointer to approval or retirement
// evidence owned outside this pure domain slice. It is not proof of identity,
// authorization, or a two-person approval workflow.
type EvidenceReference string

// NewEvidenceReference constructs a canonical v1 evidence token.
func NewEvidenceReference(value string) (EvidenceReference, error) {
	reference := EvidenceReference(value)
	if err := reference.Validate(); err != nil {
		return "", err
	}
	return reference, nil
}

// Validate checks the bounded ASCII token grammar
// [A-Za-z0-9][A-Za-z0-9._:/-]{0,127}.
func (reference EvidenceReference) Validate() error {
	if err := validateASCIIToken(string(reference), MaxEvidenceReferenceBytes, true); err != nil {
		return fmt.Errorf("%w: %v", ErrEvidenceReferenceInvalid, err)
	}
	return nil
}

// String returns the opaque evidence token without resolving its payload.
func (reference EvidenceReference) String() string { return string(reference) }

func validateRevisionToken(value string) error {
	return validateASCIIToken(value, MaxForeignRevisionBytes, false)
}

func validateASCIIToken(value string, maximumBytes int, allowSlash bool) error {
	if value == "" {
		return fmt.Errorf("value is required")
	}
	if len(value) > maximumBytes {
		return fmt.Errorf("exceeds %d bytes", maximumBytes)
	}
	for index, character := range []byte(value) {
		if isASCIILetterOrDigit(character) {
			continue
		}
		if index > 0 {
			switch character {
			case '.', '_', ':', '-':
				continue
			case '/':
				if allowSlash {
					continue
				}
			}
		}
		return fmt.Errorf("does not match the v1 ASCII token grammar")
	}
	return nil
}

func isASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}

func canonicalUTCInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0).Truncate(time.Microsecond)
}

func validateCanonicalUTCInstant(value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("value is required")
	}
	if value != canonicalUTCInstant(value) {
		return fmt.Errorf("value is not canonical UTC microsecond precision")
	}
	return nil
}
