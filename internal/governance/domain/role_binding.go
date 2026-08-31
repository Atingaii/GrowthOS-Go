package domain

import "fmt"

// BindingEffect is the closed outcome contribution of a matching role binding.
// Deny is an explicit scoped restriction, not a technical evaluation failure.
type BindingEffect string

const (
	BindingEffectAllow BindingEffect = "allow"
	BindingEffectDeny  BindingEffect = "deny"
)

// Valid reports whether effect belongs to the v1 closed vocabulary.
func (effect BindingEffect) Valid() bool {
	return effect == BindingEffectAllow || effect == BindingEffectDeny
}

// RoleBinding associates one Principal with one Role and Scope. Effect makes a
// narrow explicit restriction capable of overriding a broader grant without
// changing the role's reviewed capability ceiling.
type RoleBinding struct {
	id        RoleBindingID
	principal Principal
	roleID    RoleID
	scope     Scope
	effect    BindingEffect
}

// NewRoleBinding constructs one immutable principal-role-scope association.
func NewRoleBinding(
	id RoleBindingID,
	principal Principal,
	roleID RoleID,
	scope Scope,
	effect BindingEffect,
) (RoleBinding, error) {
	binding := RoleBinding{
		id:        id,
		principal: principal,
		roleID:    roleID,
		scope:     scope,
		effect:    effect,
	}
	if err := binding.Validate(); err != nil {
		return RoleBinding{}, err
	}
	return binding, nil
}

// Validate rejects zero, unsupported, and partially forged bindings. Role
// existence in a particular snapshot is checked by Policy.
func (binding RoleBinding) Validate() error {
	if err := binding.id.Validate(); err != nil {
		return fmt.Errorf("%w: id: %w", ErrRoleBindingInvalid, err)
	}
	if err := binding.principal.Validate(); err != nil {
		return fmt.Errorf("%w: principal: %w", ErrRoleBindingInvalid, err)
	}
	if !binding.roleID.Valid() {
		return fmt.Errorf(
			"%w: %w: role %q",
			ErrRoleBindingInvalid,
			ErrRoleUnsupported,
			binding.roleID,
		)
	}
	if err := binding.scope.Validate(); err != nil {
		return fmt.Errorf("%w: scope: %w", ErrRoleBindingInvalid, err)
	}
	if !binding.effect.Valid() {
		return fmt.Errorf(
			"%w: %w: effect %q",
			ErrRoleBindingInvalid,
			ErrBindingEffectUnsupported,
			binding.effect,
		)
	}
	return nil
}

// ID returns the immutable binding identity.
func (binding RoleBinding) ID() RoleBindingID { return binding.id }

// Principal returns the exact bound security subject.
func (binding RoleBinding) Principal() Principal { return binding.principal }

// RoleID returns the reviewed role template identifier.
func (binding RoleBinding) RoleID() RoleID { return binding.roleID }

// Scope returns the exact data-range constraint.
func (binding RoleBinding) Scope() Scope { return binding.scope }

// Effect returns allow or deny.
func (binding RoleBinding) Effect() BindingEffect { return binding.effect }

type semanticRoleBinding struct {
	principal Principal
	roleID    RoleID
	scope     Scope
	effect    BindingEffect
}

func (binding RoleBinding) semanticIdentity() semanticRoleBinding {
	return semanticRoleBinding{
		principal: binding.principal,
		roleID:    binding.roleID,
		scope:     binding.scope,
		effect:    binding.effect,
	}
}
