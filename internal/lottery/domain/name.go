package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxStrategyNameRunes is the character-level storage and display contract
	// that the persistence and transport adapters must preserve.
	MaxStrategyNameRunes = 128
	// MaxAwardNameRunes is the character-level storage and display contract that
	// the persistence and transport adapters must preserve.
	MaxAwardNameRunes = 128
)

func normalizeName(
	name string,
	maxRunes int,
	requiredError error,
	invalidError error,
	tooLongError error,
) (string, error) {
	if !utf8.ValidString(name) {
		return "", invalidError
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", requiredError
	}
	if utf8.RuneCountInString(name) > maxRunes {
		return "", tooLongError
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", invalidError
		}
	}

	return name, nil
}

func validateCanonicalName(
	name string,
	maxRunes int,
	requiredError error,
	invalidError error,
	tooLongError error,
) error {
	canonical, err := normalizeName(name, maxRunes, requiredError, invalidError, tooLongError)
	if err != nil {
		return err
	}
	if canonical != name {
		return invalidError
	}
	return nil
}
