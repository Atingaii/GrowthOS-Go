package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestPermissionIsExactAcrossResourceKindTypeAndAction(t *testing.T) {
	t.Parallel()

	permission, err := NewPermission(
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionPublish,
	)
	if err != nil {
		t.Fatalf("new permission: %v", err)
	}
	if permission.ResourceKind() != ResourceKindObject ||
		permission.ResourceType() != ResourceTypeMarketingActivity ||
		permission.Action() != ActionPublish {
		t.Fatalf("permission getters = %#v", permission)
	}

	object := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		mustResourceID(t, "activity-1"),
		"",
		Principal{},
	)
	collection, err := NewCollectionResource(ResourceTypeMarketingActivity, "")
	if err != nil {
		t.Fatalf("new collection: %v", err)
	}
	if !permission.matches(object, ActionPublish) {
		t.Fatal("exact object publish did not match")
	}
	if permission.matches(collection, ActionPublish) {
		t.Fatal("object publish permission matched collection")
	}
	if permission.matches(object, ActionRead) {
		t.Fatal("publish permission matched read")
	}

	invalid := []Permission{
		{},
		{resourceKind: ResourceKindCollection, resourceType: ResourceTypeMarketingActivity, action: ActionPublish},
		{resourceKind: ResourceKindObject, resourceType: ResourceTypeMarketingActivity, action: ActionCreate},
		{resourceKind: ResourceKindObject, resourceType: "marketing.*", action: ActionRead},
	}
	for _, candidate := range invalid {
		if err := candidate.Validate(); !errors.Is(err, ErrPermissionInvalid) {
			t.Fatalf("validate %#v: got %v", candidate, err)
		}
	}
}

func TestStrategySimulatePermissionIsExactToStrategyObject(t *testing.T) {
	t.Parallel()

	permission := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeLotteryStrategy,
		ActionSimulate,
	)
	strategy := mustObjectResource(
		t,
		ResourceTypeLotteryStrategy,
		mustResourceID(t, "strategy-1"),
		"",
		Principal{},
	)
	strategyCollection, err := NewCollectionResource(ResourceTypeLotteryStrategy, "")
	if err != nil {
		t.Fatalf("new strategy collection: %v", err)
	}
	routingGraph := mustObjectResource(
		t,
		ResourceTypeLotteryRoutingGraph,
		mustResourceID(t, "routing-graph-1"),
		"",
		Principal{},
	)

	if !permission.matches(strategy, ActionSimulate) {
		t.Fatal("exact strategy object simulate did not match")
	}
	if permission.matches(strategy, ActionRead) {
		t.Fatal("strategy simulate permission matched read")
	}
	if permission.matches(strategyCollection, ActionSimulate) {
		t.Fatal("strategy object simulate permission matched collection")
	}
	if permission.matches(routingGraph, ActionSimulate) {
		t.Fatal("strategy simulate permission matched routing graph")
	}
}

func TestBaselineRolesAreClosedCanonicalCapabilityCeilings(t *testing.T) {
	t.Parallel()

	wantCounts := map[RoleID]int{
		RolePlatformAdministrator: 17,
		RoleMarketingOperator:     10,
		RoleLotteryDesigner:       9,
		RoleSecurityAuditor:       9,
		RoleGrowthMember:          0,
	}
	roles := BaselineRoles()
	if len(roles) != len(wantCounts) {
		t.Fatalf("role count = %d, want %d", len(roles), len(wantCounts))
	}
	if !slices.IsSortedFunc(roles, func(left, right Role) int {
		if left.ID() < right.ID() {
			return -1
		}
		if left.ID() > right.ID() {
			return 1
		}
		return 0
	}) {
		t.Fatal("baseline roles are not canonical")
	}

	for _, role := range roles {
		if err := role.Validate(); err != nil {
			t.Fatalf("validate role %q: %v", role.ID(), err)
		}
		if got := len(role.Permissions()); got != wantCounts[role.ID()] {
			t.Fatalf("role %q permission count = %d, want %d", role.ID(), got, wantCounts[role.ID()])
		}
	}

	administrator := findBaselineRole(t, RolePlatformAdministrator)
	if !slices.Contains(
		administrator.Permissions(),
		mustPermission(t, ResourceKindObject, ResourceTypeGovernancePolicy, ActionChange),
	) {
		t.Fatal("platform administrator lacks exact policy change capability")
	}
	marketing := findBaselineRole(t, RoleMarketingOperator)
	if slices.Contains(
		marketing.Permissions(),
		mustPermission(t, ResourceKindObject, ResourceTypeGovernancePolicy, ActionChange),
	) {
		t.Fatal("marketing operator gained policy change capability")
	}
	strategySimulate := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeLotteryStrategy,
		ActionSimulate,
	)
	if !slices.Contains(administrator.Permissions(), strategySimulate) {
		t.Fatal("platform administrator lacks exact strategy simulate capability")
	}
	if !slices.Contains(
		findBaselineRole(t, RoleLotteryDesigner).Permissions(),
		strategySimulate,
	) {
		t.Fatal("lottery designer lacks exact strategy simulate capability")
	}
	for _, roleID := range []RoleID{
		RoleMarketingOperator,
		RoleSecurityAuditor,
		RoleGrowthMember,
	} {
		if slices.Contains(findBaselineRole(t, roleID).Permissions(), strategySimulate) {
			t.Fatalf("role %q gained exact strategy simulate capability", roleID)
		}
	}
	member := findBaselineRole(t, RoleGrowthMember)
	if len(member.Permissions()) != 0 {
		t.Fatal("growth member gained operator-resource permission")
	}
}

func TestNewRoleAllowsOnlyCanonicalTemplateSubset(t *testing.T) {
	t.Parallel()

	readObject := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	publishObject := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionPublish,
	)
	role, err := NewRole(
		RoleMarketingOperator,
		[]Permission{publishObject, readObject},
	)
	if err != nil {
		t.Fatalf("new subset role: %v", err)
	}
	want := []Permission{readObject, publishObject}
	slices.SortFunc(want, comparePermission)
	if !slices.Equal(role.Permissions(), want) {
		t.Fatalf("canonical permissions = %#v, want %#v", role.Permissions(), want)
	}

	policyChange := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeGovernancePolicy,
		ActionChange,
	)
	if _, err := NewRole(RoleMarketingOperator, []Permission{policyChange}); !errors.Is(err, ErrRoleInvalid) {
		t.Fatalf("marketing role with policy change: %v", err)
	}
	if _, err := NewRole(RoleGrowthMember, []Permission{readObject}); !errors.Is(err, ErrRoleInvalid) {
		t.Fatalf("member role with operator permission: %v", err)
	}
	if _, err := NewRole("admin", nil); !errors.Is(err, ErrRoleUnsupported) {
		t.Fatalf("unknown role: %v", err)
	}
	if _, err := NewRole(RoleMarketingOperator, []Permission{readObject, readObject}); !errors.Is(err, ErrRolePermissionDuplicate) {
		t.Fatalf("duplicate permission: %v", err)
	}

	oversized := make([]Permission, MaxPermissionsPerRole+1)
	for index := range oversized {
		oversized[index] = readObject
	}
	if _, err := NewRole(RoleMarketingOperator, oversized); !errors.Is(err, ErrRolePermissionLimit) {
		t.Fatalf("oversized role: %v", err)
	}
}

func TestRoleDefensiveCopiesCallerAndGetterSlices(t *testing.T) {
	t.Parallel()

	readObject := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	input := []Permission{readObject}
	role, err := NewRole(RoleMarketingOperator, input)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	input[0] = Permission{}
	if got := role.Permissions(); len(got) != 1 || got[0] != readObject {
		t.Fatalf("caller mutation changed role: %#v", got)
	}

	returned := role.Permissions()
	returned[0] = Permission{}
	if got := role.Permissions(); len(got) != 1 || got[0] != readObject {
		t.Fatalf("getter mutation changed role: %#v", got)
	}

	roles := BaselineRoles()
	roles[0] = Role{}
	fresh := BaselineRoles()
	if len(fresh) == 0 || fresh[0].ID() == "" {
		t.Fatal("caller mutation changed baseline role factory")
	}
	permissions := fresh[0].Permissions()
	if len(permissions) > 0 {
		permissions[0] = Permission{}
	}
	if err := BaselineRoles()[0].Validate(); err != nil {
		t.Fatalf("permission getter mutation changed baseline: %v", err)
	}
}

func TestRoleValidateRejectsNonCanonicalAndPartialState(t *testing.T) {
	t.Parallel()

	readObject := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	publishObject := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionPublish,
	)
	canonical := []Permission{readObject, publishObject}
	slices.SortFunc(canonical, comparePermission)
	nonCanonical := append([]Permission(nil), canonical...)
	slices.Reverse(nonCanonical)

	invalid := []Role{
		{},
		{id: RoleMarketingOperator, permissions: nonCanonical},
		{id: RoleMarketingOperator, permissions: []Permission{{}}},
	}
	for _, role := range invalid {
		if err := role.Validate(); !errors.Is(err, ErrRoleInvalid) {
			t.Fatalf("validate %#v: got %v", role, err)
		}
	}
}

func TestRoleIDClosedVocabulary(t *testing.T) {
	t.Parallel()

	for _, roleID := range []RoleID{
		RolePlatformAdministrator,
		RoleMarketingOperator,
		RoleLotteryDesigner,
		RoleSecurityAuditor,
		RoleGrowthMember,
	} {
		if !roleID.Valid() {
			t.Fatalf("known role %q invalid", roleID)
		}
	}
	if RoleID("admin").Valid() || RoleID("super_admin").Valid() || RoleID("").Valid() {
		t.Fatal("unknown role became valid")
	}
}

func mustPermission(
	t *testing.T,
	resourceKind ResourceKind,
	resourceType ResourceType,
	action Action,
) Permission {
	t.Helper()
	permission, err := NewPermission(resourceKind, resourceType, action)
	if err != nil {
		t.Fatalf("new permission %s:%s:%s: %v", resourceKind, resourceType, action, err)
	}
	return permission
}

func findBaselineRole(t *testing.T, roleID RoleID) Role {
	t.Helper()
	for _, role := range BaselineRoles() {
		if role.ID() == roleID {
			return role
		}
	}
	t.Fatalf("baseline role %q missing", roleID)
	return Role{}
}
