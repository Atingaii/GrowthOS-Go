package domain

import (
	"cmp"
	"fmt"
	"slices"
)

const (
	// MaxRoleBindingsPerPolicy bounds principal-role-scope associations evaluated
	// synchronously from one immutable snapshot.
	MaxRoleBindingsPerPolicy = 1024
)

// PolicyRevision is a non-zero snapshot correlation value. It is not a content
// hash and pure construction cannot prove global (PolicyID, Revision) uniqueness.
type PolicyRevision uint64

// PolicyIdentity identifies one exact logical policy snapshot.
type PolicyIdentity struct {
	id       PolicyID
	revision PolicyRevision
}

// NewPolicyIdentity constructs an exact policy snapshot reference.
func NewPolicyIdentity(id PolicyID, revision PolicyRevision) (PolicyIdentity, error) {
	identity := PolicyIdentity{id: id, revision: revision}
	if err := identity.Validate(); err != nil {
		return PolicyIdentity{}, err
	}
	return identity, nil
}

// Validate rejects zero and non-canonical policy identities.
func (identity PolicyIdentity) Validate() error {
	if err := identity.id.Validate(); err != nil {
		return fmt.Errorf("%w: id: %w", ErrPolicyIdentityInvalid, err)
	}
	if identity.revision == 0 {
		return fmt.Errorf(
			"%w: %w: revision is required",
			ErrPolicyIdentityInvalid,
			ErrPolicyRevisionInvalid,
		)
	}
	return nil
}

// ID returns the logical policy identifier.
func (identity PolicyIdentity) ID() PolicyID { return identity.id }

// Revision returns the exact non-zero snapshot correlation value.
func (identity PolicyIdentity) Revision() PolicyRevision { return identity.revision }

// Policy is a bounded, immutable, canonically ordered access-control snapshot.
type Policy struct {
	identity PolicyIdentity
	roles    []Role
	bindings []RoleBinding
}

// NewPolicy constructs and deep-copies one exact snapshot. Collections are
// sorted canonically; scalar values are never trimmed or case folded.
func NewPolicy(
	identity PolicyIdentity,
	roles []Role,
	bindings []RoleBinding,
) (Policy, error) {
	if err := identity.Validate(); err != nil {
		return Policy{}, fmt.Errorf("%w: identity: %w", ErrPolicyInvalid, err)
	}
	if len(roles) > MaxRolesPerPolicy {
		return Policy{}, fmt.Errorf(
			"%w: %w: got %d, maximum %d",
			ErrPolicyInvalid,
			ErrPolicyRoleLimit,
			len(roles),
			MaxRolesPerPolicy,
		)
	}
	if len(bindings) > MaxRoleBindingsPerPolicy {
		return Policy{}, fmt.Errorf(
			"%w: %w: got %d, maximum %d",
			ErrPolicyInvalid,
			ErrPolicyBindingLimit,
			len(bindings),
			MaxRoleBindingsPerPolicy,
		)
	}
	ownedRoles := make([]Role, len(roles))
	for index, role := range roles {
		ownedRoles[index] = role.clone()
	}
	slices.SortFunc(ownedRoles, compareRole)
	ownedBindings := append([]RoleBinding(nil), bindings...)
	slices.SortFunc(ownedBindings, compareRoleBinding)

	policy := Policy{
		identity: identity,
		roles:    ownedRoles,
		bindings: ownedBindings,
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// NewBaselinePolicy constructs a snapshot with the full reviewed v1 role
// templates and caller-supplied scoped bindings.
func NewBaselinePolicy(
	identity PolicyIdentity,
	bindings []RoleBinding,
) (Policy, error) {
	return NewPolicy(identity, BaselineRoles(), bindings)
}

// Validate rejects corrupt identities, non-canonical order, duplicate or
// semantically repeated bindings, duplicate roles, and dangling role references.
func (policy Policy) Validate() error {
	if err := policy.identity.Validate(); err != nil {
		return fmt.Errorf("%w: identity: %w", ErrPolicyInvalid, err)
	}
	if len(policy.roles) > MaxRolesPerPolicy {
		return fmt.Errorf(
			"%w: %w: got %d, maximum %d",
			ErrPolicyInvalid,
			ErrPolicyRoleLimit,
			len(policy.roles),
			MaxRolesPerPolicy,
		)
	}
	if len(policy.bindings) > MaxRoleBindingsPerPolicy {
		return fmt.Errorf(
			"%w: %w: got %d, maximum %d",
			ErrPolicyInvalid,
			ErrPolicyBindingLimit,
			len(policy.bindings),
			MaxRoleBindingsPerPolicy,
		)
	}
	if !slices.IsSortedFunc(policy.roles, compareRole) {
		return fmt.Errorf("%w: roles are not canonical", ErrPolicyInvalid)
	}
	if !slices.IsSortedFunc(policy.bindings, compareRoleBinding) {
		return fmt.Errorf("%w: bindings are not canonical", ErrPolicyInvalid)
	}

	rolesByID := make(map[RoleID]Role, len(policy.roles))
	for index, role := range policy.roles {
		if err := role.Validate(); err != nil {
			return fmt.Errorf("%w: role %d: %w", ErrPolicyInvalid, index, err)
		}
		if _, exists := rolesByID[role.id]; exists {
			return fmt.Errorf(
				"%w: %w: role %q",
				ErrPolicyInvalid,
				ErrPolicyRoleDuplicate,
				role.id,
			)
		}
		rolesByID[role.id] = role
	}

	bindingIDs := make(map[RoleBindingID]struct{}, len(policy.bindings))
	semanticBindings := make(map[semanticRoleBinding]RoleBindingID, len(policy.bindings))
	for index, binding := range policy.bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("%w: binding %d: %w", ErrPolicyInvalid, index, err)
		}
		if _, exists := bindingIDs[binding.id]; exists {
			return fmt.Errorf(
				"%w: %w: binding %q",
				ErrPolicyInvalid,
				ErrPolicyBindingDuplicate,
				binding.id,
			)
		}
		bindingIDs[binding.id] = struct{}{}
		semanticIdentity := binding.semanticIdentity()
		if previousID, exists := semanticBindings[semanticIdentity]; exists {
			return fmt.Errorf(
				"%w: %w: bindings %q and %q",
				ErrPolicyInvalid,
				ErrPolicyBindingConflict,
				previousID,
				binding.id,
			)
		}
		semanticBindings[semanticIdentity] = binding.id
		if _, exists := rolesByID[binding.roleID]; !exists {
			return fmt.Errorf(
				"%w: %w: binding %q references role %q",
				ErrPolicyInvalid,
				ErrPolicyBindingRoleMissing,
				binding.id,
				binding.roleID,
			)
		}
	}
	return nil
}

// Identity returns the exact policy snapshot reference.
func (policy Policy) Identity() PolicyIdentity { return policy.identity }

// Roles returns a defensive deep copy in canonical RoleID order.
func (policy Policy) Roles() []Role {
	roles := make([]Role, len(policy.roles))
	for index, role := range policy.roles {
		roles[index] = role.clone()
	}
	return roles
}

// RoleBindings returns a defensive copy in canonical BindingID order.
func (policy Policy) RoleBindings() []RoleBinding {
	return append([]RoleBinding(nil), policy.bindings...)
}

func (policy Policy) role(roleID RoleID) (Role, bool) {
	index, found := slices.BinarySearchFunc(policy.roles, Role{id: roleID}, compareRole)
	if !found {
		return Role{}, false
	}
	return policy.roles[index], true
}

func compareRole(left, right Role) int {
	return cmp.Compare(left.id, right.id)
}

func compareRoleBinding(left, right RoleBinding) int {
	return cmp.Compare(left.id, right.id)
}
