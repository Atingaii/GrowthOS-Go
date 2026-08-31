package domain

import "fmt"

const (
	// MaxOpaqueIdentifierBytes bounds identifiers copied into policy snapshots
	// and decision evidence. External identity adapters must map their native
	// identifiers into this canonical internal form.
	MaxOpaqueIdentifierBytes = 128
)

// PrincipalID identifies a security principal within its PrincipalKind.
type PrincipalID string

// ResourceID identifies one protected object. A collection has no ResourceID.
type ResourceID string

// TenantID is an opaque authorization-scope key. Its presence does not claim
// that GrowthOS already provides tenant lifecycle or storage isolation.
type TenantID string

// RoleBindingID identifies one immutable principal-role-scope association.
type RoleBindingID string

// PolicyID identifies one logical access-control policy across revisions.
type PolicyID string

// AuditReference is a bounded opaque evaluation or correlation key. It is not
// a session token, trace, credential, or durable audit event identity.
type AuditReference string

// NewPrincipalID constructs a canonical internal principal identifier.
func NewPrincipalID(value string) (PrincipalID, error) {
	identifier := PrincipalID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

// Validate checks the canonical bounded lowercase ASCII token grammar.
func (identifier PrincipalID) Validate() error {
	return validateIdentifier("principal id", string(identifier))
}

// String returns the opaque identifier without resolving an identity record.
func (identifier PrincipalID) String() string { return string(identifier) }

// NewResourceID constructs a canonical protected-object identifier.
func NewResourceID(value string) (ResourceID, error) {
	identifier := ResourceID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

// Validate checks the canonical bounded lowercase ASCII token grammar.
func (identifier ResourceID) Validate() error {
	return validateIdentifier("resource id", string(identifier))
}

// String returns the opaque identifier.
func (identifier ResourceID) String() string { return string(identifier) }

// NewTenantID constructs a canonical internal tenant-scope identifier.
func NewTenantID(value string) (TenantID, error) {
	identifier := TenantID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

// Validate checks the canonical bounded lowercase ASCII token grammar.
func (identifier TenantID) Validate() error {
	return validateIdentifier("tenant id", string(identifier))
}

// String returns the opaque identifier.
func (identifier TenantID) String() string { return string(identifier) }

// NewRoleBindingID constructs a canonical policy-binding identifier.
func NewRoleBindingID(value string) (RoleBindingID, error) {
	identifier := RoleBindingID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

// Validate checks the canonical bounded lowercase ASCII token grammar.
func (identifier RoleBindingID) Validate() error {
	return validateIdentifier("role binding id", string(identifier))
}

// String returns the opaque identifier.
func (identifier RoleBindingID) String() string { return string(identifier) }

// NewPolicyID constructs a canonical logical-policy identifier.
func NewPolicyID(value string) (PolicyID, error) {
	identifier := PolicyID(value)
	if err := identifier.Validate(); err != nil {
		return "", err
	}
	return identifier, nil
}

// Validate checks the canonical bounded lowercase ASCII token grammar.
func (identifier PolicyID) Validate() error {
	return validateIdentifier("policy id", string(identifier))
}

// String returns the opaque identifier.
func (identifier PolicyID) String() string { return string(identifier) }

// NewAuditReference constructs a canonical pure-domain correlation key.
func NewAuditReference(value string) (AuditReference, error) {
	reference := AuditReference(value)
	if err := reference.Validate(); err != nil {
		return "", err
	}
	return reference, nil
}

// Validate checks the canonical bounded lowercase ASCII token grammar.
func (reference AuditReference) Validate() error {
	return validateIdentifier("audit reference", string(reference))
}

// String returns the opaque reference.
func (reference AuditReference) String() string { return string(reference) }

// validateIdentifier accepts
// [a-z0-9](?:[a-z0-9._:-]{0,126}[a-z0-9])? and rejects aliases that would
// require trimming, case folding, Unicode normalization, or path handling.
func validateIdentifier(label, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is required", ErrIdentifierInvalid, label)
	}
	if len(value) > MaxOpaqueIdentifierBytes {
		return fmt.Errorf(
			"%w: %s exceeds %d bytes",
			ErrIdentifierInvalid,
			label,
			MaxOpaqueIdentifierBytes,
		)
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
		return fmt.Errorf("%w: %s is not canonical lowercase ASCII", ErrIdentifierInvalid, label)
	}
	return nil
}

func isLowerASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}
