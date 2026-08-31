package domain

import "fmt"

// PrincipalKind is the closed class of actor represented in an authorization
// request. No kind is an authentication mechanism or implicit superuser.
type PrincipalKind string

const (
	PrincipalKindHuman   PrincipalKind = "human"
	PrincipalKindService PrincipalKind = "service"
	PrincipalKindAgent   PrincipalKind = "agent"
)

// Valid reports whether kind belongs to the v1 closed vocabulary.
func (kind PrincipalKind) Valid() bool {
	switch kind {
	case PrincipalKindHuman, PrincipalKindService, PrincipalKindAgent:
		return true
	default:
		return false
	}
}

// Principal is the minimum immutable authorization subject reference. Its
// successful construction proves only canonical shape, never caller identity.
type Principal struct {
	kind PrincipalKind
	id   PrincipalID
}

// NewPrincipal constructs a canonical subject reference.
func NewPrincipal(kind PrincipalKind, id PrincipalID) (Principal, error) {
	principal := Principal{kind: kind, id: id}
	if err := principal.Validate(); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

// Validate rejects zero, unsupported, and partially forged principal values.
func (principal Principal) Validate() error {
	if !principal.kind.Valid() {
		return fmt.Errorf(
			"%w: %w: kind %q",
			ErrPrincipalInvalid,
			ErrPrincipalKindUnsupported,
			principal.kind,
		)
	}
	if err := principal.id.Validate(); err != nil {
		return fmt.Errorf("%w: id: %w", ErrPrincipalInvalid, err)
	}
	return nil
}

// Kind returns human, service, or agent.
func (principal Principal) Kind() PrincipalKind { return principal.kind }

// ID returns the opaque internal subject identifier.
func (principal Principal) ID() PrincipalID { return principal.id }

func (principal Principal) isZero() bool {
	return principal.kind == "" && principal.id == ""
}
