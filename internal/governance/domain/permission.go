package domain

import "fmt"

// Permission is one exact kind/type/action capability. It carries no wildcard,
// scope, principal, or allow/deny effect; those belong to RoleBinding.
type Permission struct {
	resourceKind ResourceKind
	resourceType ResourceType
	action       Action
}

// NewPermission constructs one registered exact capability.
func NewPermission(
	resourceKind ResourceKind,
	resourceType ResourceType,
	action Action,
) (Permission, error) {
	permission := Permission{
		resourceKind: resourceKind,
		resourceType: resourceType,
		action:       action,
	}
	if err := permission.Validate(); err != nil {
		return Permission{}, err
	}
	return permission, nil
}

// Validate rechecks the closed capability catalog.
func (permission Permission) Validate() error {
	if err := ValidateCapability(
		permission.resourceKind,
		permission.resourceType,
		permission.action,
	); err != nil {
		return fmt.Errorf("%w: %w", ErrPermissionInvalid, err)
	}
	return nil
}

// ResourceKind returns collection or object.
func (permission Permission) ResourceKind() ResourceKind { return permission.resourceKind }

// ResourceType returns the protected business resource type.
func (permission Permission) ResourceType() ResourceType { return permission.resourceType }

// Action returns the exact business verb.
func (permission Permission) Action() Action { return permission.action }

func (permission Permission) matches(resource Resource, action Action) bool {
	return permission.resourceKind == resource.kind &&
		permission.resourceType == resource.typeName &&
		permission.action == action
}
