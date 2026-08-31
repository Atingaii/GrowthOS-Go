package domain

import (
	"cmp"
	"fmt"
	"slices"
)

const (
	// MaxRolesPerPolicy bounds the immutable role catalog of one policy snapshot.
	MaxRolesPerPolicy = 64
	// MaxPermissionsPerRole bounds exact capabilities expanded during evaluation.
	MaxPermissionsPerRole = 64
)

// RoleID is the closed v1 responsibility vocabulary. A role identifier is not
// a frontend workspace name, display label, group, or authentication claim.
type RoleID string

const (
	RolePlatformAdministrator RoleID = "platform_administrator"
	RoleMarketingOperator     RoleID = "marketing_operator"
	RoleLotteryDesigner       RoleID = "lottery_designer"
	RoleSecurityAuditor       RoleID = "security_auditor"
	RoleGrowthMember          RoleID = "growth_member"
)

// Valid reports whether role belongs to the reviewed v1 template catalog.
func (roleID RoleID) Valid() bool {
	switch roleID {
	case RolePlatformAdministrator,
		RoleMarketingOperator,
		RoleLotteryDesigner,
		RoleSecurityAuditor,
		RoleGrowthMember:
		return true
	default:
		return false
	}
}

// Role is one immutable, canonically ordered subset of its reviewed template
// capability ceiling. A policy revision may reduce a role but cannot silently
// grant that role a capability outside the template.
type Role struct {
	id          RoleID
	permissions []Permission
}

// NewRole constructs a canonical immutable role. An empty GrowthMember role is
// valid and intentionally carries no current operator-resource capability.
func NewRole(id RoleID, permissions []Permission) (Role, error) {
	ownedPermissions := append([]Permission(nil), permissions...)
	slices.SortFunc(ownedPermissions, comparePermission)
	role := Role{id: id, permissions: ownedPermissions}
	if err := role.Validate(); err != nil {
		return Role{}, err
	}
	return role, nil
}

// Validate rejects unknown roles, table-external capabilities, duplicates,
// oversized collections, and non-canonical internal order.
func (role Role) Validate() error {
	if !role.id.Valid() {
		return fmt.Errorf("%w: %w: id %q", ErrRoleInvalid, ErrRoleUnsupported, role.id)
	}
	if len(role.permissions) > MaxPermissionsPerRole {
		return fmt.Errorf(
			"%w: %w: got %d, maximum %d",
			ErrRoleInvalid,
			ErrRolePermissionLimit,
			len(role.permissions),
			MaxPermissionsPerRole,
		)
	}
	if !slices.IsSortedFunc(role.permissions, comparePermission) {
		return fmt.Errorf("%w: permissions are not canonical", ErrRoleInvalid)
	}
	var previous Permission
	for index, permission := range role.permissions {
		if err := permission.Validate(); err != nil {
			return fmt.Errorf("%w: permission %d: %w", ErrRoleInvalid, index, err)
		}
		if !roleTemplateContains(role.id, permission) {
			return fmt.Errorf(
				"%w: role %q exceeds its capability template with %s:%s:%s",
				ErrRoleInvalid,
				role.id,
				permission.resourceKind,
				permission.resourceType,
				permission.action,
			)
		}
		if index > 0 && permission == previous {
			return fmt.Errorf(
				"%w: %w: %s:%s:%s",
				ErrRoleInvalid,
				ErrRolePermissionDuplicate,
				permission.resourceKind,
				permission.resourceType,
				permission.action,
			)
		}
		previous = permission
	}
	return nil
}

// ID returns the stable responsibility identifier.
func (role Role) ID() RoleID { return role.id }

// Permissions returns a defensive copy in canonical order.
func (role Role) Permissions() []Permission {
	return append([]Permission(nil), role.permissions...)
}

func (role Role) clone() Role {
	return Role{id: role.id, permissions: role.Permissions()}
}

// BaselineRoles returns the full reviewed capability ceiling for all v1 roles
// in canonical RoleID order. A caller cannot mutate future results.
func BaselineRoles() []Role {
	roleIDs := []RoleID{
		RolePlatformAdministrator,
		RoleMarketingOperator,
		RoleLotteryDesigner,
		RoleSecurityAuditor,
		RoleGrowthMember,
	}
	slices.Sort(roleIDs)
	roles := make([]Role, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		role, err := NewRole(roleID, roleTemplatePermissions(roleID))
		if err != nil {
			panic(fmt.Sprintf("invalid built-in role %q: %v", roleID, err))
		}
		roles = append(roles, role)
	}
	return roles
}

func roleTemplateContains(roleID RoleID, candidate Permission) bool {
	return slices.Contains(roleTemplatePermissions(roleID), candidate)
}

func roleTemplatePermissions(roleID RoleID) []Permission {
	activityCreateCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeMarketingActivity,
		ActionCreate,
	)
	activityReadCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	activityReadObject := catalogPermission(
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	activityPublish := catalogPermission(
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionPublish,
	)
	activityRollback := catalogPermission(
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRollback,
	)
	activityRetire := catalogPermission(
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRetire,
	)
	strategyCreateCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeLotteryStrategy,
		ActionCreate,
	)
	strategyReadCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeLotteryStrategy,
		ActionRead,
	)
	strategyReadObject := catalogPermission(
		ResourceKindObject,
		ResourceTypeLotteryStrategy,
		ActionRead,
	)
	graphCreateCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeLotteryRoutingGraph,
		ActionCreate,
	)
	graphReadCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeLotteryRoutingGraph,
		ActionRead,
	)
	graphReadObject := catalogPermission(
		ResourceKindObject,
		ResourceTypeLotteryRoutingGraph,
		ActionRead,
	)
	policyReadCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeGovernancePolicy,
		ActionRead,
	)
	policyReadObject := catalogPermission(
		ResourceKindObject,
		ResourceTypeGovernancePolicy,
		ActionRead,
	)
	policyChangeObject := catalogPermission(
		ResourceKindObject,
		ResourceTypeGovernancePolicy,
		ActionChange,
	)
	auditReadCollection := catalogPermission(
		ResourceKindCollection,
		ResourceTypeGovernanceAudit,
		ActionRead,
	)

	switch roleID {
	case RolePlatformAdministrator:
		return []Permission{
			activityCreateCollection,
			activityReadCollection,
			activityReadObject,
			activityPublish,
			activityRollback,
			activityRetire,
			strategyCreateCollection,
			strategyReadCollection,
			strategyReadObject,
			graphCreateCollection,
			graphReadCollection,
			graphReadObject,
			policyReadCollection,
			policyReadObject,
			policyChangeObject,
			auditReadCollection,
		}
	case RoleMarketingOperator:
		return []Permission{
			activityCreateCollection,
			activityReadCollection,
			activityReadObject,
			activityPublish,
			activityRollback,
			activityRetire,
			strategyReadCollection,
			strategyReadObject,
			graphReadCollection,
			graphReadObject,
		}
	case RoleLotteryDesigner:
		return []Permission{
			activityReadCollection,
			activityReadObject,
			strategyCreateCollection,
			strategyReadCollection,
			strategyReadObject,
			graphCreateCollection,
			graphReadCollection,
			graphReadObject,
		}
	case RoleSecurityAuditor:
		return []Permission{
			activityReadCollection,
			activityReadObject,
			strategyReadCollection,
			strategyReadObject,
			graphReadCollection,
			graphReadObject,
			policyReadCollection,
			policyReadObject,
			auditReadCollection,
		}
	case RoleGrowthMember:
		return nil
	default:
		return nil
	}
}

func catalogPermission(
	resourceKind ResourceKind,
	resourceType ResourceType,
	action Action,
) Permission {
	return Permission{
		resourceKind: resourceKind,
		resourceType: resourceType,
		action:       action,
	}
}

func comparePermission(left, right Permission) int {
	if comparison := cmp.Compare(left.resourceType, right.resourceType); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(left.resourceKind, right.resourceKind); comparison != 0 {
		return comparison
	}
	return cmp.Compare(left.action, right.action)
}
