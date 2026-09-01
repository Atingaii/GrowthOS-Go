package domain

import "fmt"

const (
	// MaxCanonicalIdentifierBytes matches the Governance PrincipalID ceiling.
	MaxCanonicalIdentifierBytes = 128
	// MinLoginNameBytes and MaxLoginNameBytes encode
	// [a-z][a-z0-9._-]{2,63} without normalization.
	MinLoginNameBytes = 3
	MaxLoginNameBytes = 64
)

// AccountID is the stable local workforce-account identity.
type AccountID string

// LoginName is the human-entered local-provider lookup key. It is deliberately
// distinct from AccountID and PrincipalID.
type LoginName string

// PrincipalID is wire-compatible with Governance PrincipalID while keeping the
// Identity domain independent from the authorization package.
type PrincipalID string

// SessionRef is a non-secret server-side session correlation identity. It is
// never the opaque bearer value carried by a client.
type SessionRef string

// OperationRef is a server-generated, non-secret correlation identity used to
// reconcile an authority write whose commit response may be unknown. It is
// never accepted as caller-selected identity or credential material.
type OperationRef string

func NewAccountID(value string) (AccountID, error) {
	identifier := AccountID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

func (identifier AccountID) Validate() error {
	if err := validateCanonicalIdentifier(string(identifier)); err != nil {
		return fmt.Errorf("%w: %v", ErrAccountIDInvalid, err)
	}
	return nil
}

func (identifier AccountID) String() string { return string(identifier) }

func NewLoginName(value string) (LoginName, error) {
	name := LoginName(value)
	if err := name.Validate(); err != nil {
		return "", err
	}
	return name, nil
}

func (name LoginName) Validate() error {
	value := string(name)
	if len(value) < MinLoginNameBytes || len(value) > MaxLoginNameBytes {
		return fmt.Errorf(
			"%w: length must be between %d and %d bytes",
			ErrLoginNameInvalid,
			MinLoginNameBytes,
			MaxLoginNameBytes,
		)
	}
	if !isLowerASCIILetter(value[0]) {
		return fmt.Errorf("%w: first byte must be a lowercase ASCII letter", ErrLoginNameInvalid)
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if isLowerASCIILetterOrDigit(character) {
			continue
		}
		switch character {
		case '.', '_', '-':
			continue
		default:
			return fmt.Errorf("%w: byte %d is outside the canonical grammar", ErrLoginNameInvalid, index)
		}
	}
	return nil
}

func (name LoginName) String() string { return string(name) }

func NewPrincipalID(value string) (PrincipalID, error) {
	identifier := PrincipalID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

func (identifier PrincipalID) Validate() error {
	if err := validateCanonicalIdentifier(string(identifier)); err != nil {
		return fmt.Errorf("%w: %v", ErrPrincipalIDInvalid, err)
	}
	return nil
}

func (identifier PrincipalID) String() string { return string(identifier) }

func NewSessionRef(value string) (SessionRef, error) {
	reference := SessionRef(value)
	if err := reference.Validate(); err != nil {
		return "", err
	}
	return reference, nil
}

func (reference SessionRef) Validate() error {
	if err := validateCanonicalIdentifier(string(reference)); err != nil {
		return fmt.Errorf("%w: %v", ErrSessionRefInvalid, err)
	}
	return nil
}

func (reference SessionRef) String() string { return string(reference) }

func NewOperationRef(value string) (OperationRef, error) {
	reference := OperationRef(value)
	if err := reference.Validate(); err != nil {
		return "", err
	}
	return reference, nil
}

func (reference OperationRef) Validate() error {
	if err := validateCanonicalIdentifier(string(reference)); err != nil {
		return fmt.Errorf("%w: %v", ErrOperationRefInvalid, err)
	}
	return nil
}

func (reference OperationRef) String() string { return string(reference) }

// validateCanonicalIdentifier mirrors Governance's reviewed grammar:
// [a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])?.
func validateCanonicalIdentifier(value string) error {
	if value == "" {
		return fmt.Errorf("value is required")
	}
	if len(value) > MaxCanonicalIdentifierBytes {
		return fmt.Errorf("value exceeds %d bytes", MaxCanonicalIdentifierBytes)
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isLowerASCIILetterOrDigit(character) {
			continue
		}
		if index > 0 && index < len(value)-1 {
			switch character {
			case '.', '_', ':', '-':
				continue
			}
		}
		return fmt.Errorf("value is not canonical lowercase ASCII")
	}
	return nil
}

func isLowerASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z'
}

func isLowerASCIILetterOrDigit(value byte) bool {
	return isLowerASCIILetter(value) || value >= '0' && value <= '9'
}
