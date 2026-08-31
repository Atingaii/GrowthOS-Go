package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestRoleBindingPreservesExactPrincipalRoleScopeAndEffect(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	tenantID := mustTenantID(t, "tenant-a")
	scope, err := NewTenantScope(tenantID)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}
	id := mustRoleBindingID(t, "binding-1")
	binding, err := NewRoleBinding(
		id,
		principal,
		RoleMarketingOperator,
		scope,
		BindingEffectAllow,
	)
	if err != nil {
		t.Fatalf("new role binding: %v", err)
	}
	if binding.ID() != id ||
		binding.Principal() != principal ||
		binding.RoleID() != RoleMarketingOperator ||
		binding.Scope() != scope ||
		binding.Effect() != BindingEffectAllow {
		t.Fatalf("binding getters = %#v", binding)
	}

	deny, err := NewRoleBinding(
		mustRoleBindingID(t, "binding-2"),
		principal,
		RoleMarketingOperator,
		mustResourceScope(
			t,
			ResourceTypeMarketingActivity,
			mustResourceID(t, "activity-1"),
			tenantID,
		),
		BindingEffectDeny,
	)
	if err != nil {
		t.Fatalf("new deny binding: %v", err)
	}
	if deny.Effect() != BindingEffectDeny {
		t.Fatalf("deny effect = %q", deny.Effect())
	}
}

func TestRoleBindingRejectsZeroPartialAndUnsupportedState(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	system := NewSystemScope()
	id := mustRoleBindingID(t, "binding-1")
	invalid := []RoleBinding{
		{},
		{
			id:        id,
			principal: principal,
			roleID:    "admin",
			scope:     system,
			effect:    BindingEffectAllow,
		},
		{
			id:        id,
			principal: principal,
			roleID:    RoleMarketingOperator,
			effect:    BindingEffectAllow,
		},
		{
			id:        id,
			principal: principal,
			roleID:    RoleMarketingOperator,
			scope:     system,
			effect:    "permit",
		},
		{
			id:     id,
			roleID: RoleMarketingOperator,
			scope:  system,
			effect: BindingEffectAllow,
		},
	}
	for _, binding := range invalid {
		if err := binding.Validate(); !errors.Is(err, ErrRoleBindingInvalid) {
			t.Fatalf("validate %#v: got %v", binding, err)
		}
	}
	if !BindingEffectAllow.Valid() || !BindingEffectDeny.Valid() {
		t.Fatal("known binding effect invalid")
	}
	if BindingEffect("permit").Valid() || BindingEffect("").Valid() {
		t.Fatal("unknown binding effect became valid")
	}
	_, err := NewRoleBinding(
		id,
		principal,
		RoleMarketingOperator,
		system,
		BindingEffect("permit"),
	)
	if !errors.Is(err, ErrRoleBindingInvalid) ||
		!errors.Is(err, ErrBindingEffectUnsupported) {
		t.Fatalf("unsupported effect error = %v", err)
	}
}

func TestPolicyIdentityIsExactNonZeroCorrelation(t *testing.T) {
	t.Parallel()

	policyID := mustPolicyID(t, "growthos-access")
	identity, err := NewPolicyIdentity(policyID, 7)
	if err != nil {
		t.Fatalf("new policy identity: %v", err)
	}
	if identity.ID() != policyID || identity.Revision() != 7 {
		t.Fatalf("identity getters = %q/%d", identity.ID(), identity.Revision())
	}

	if _, err := NewPolicyIdentity("", 7); !errors.Is(err, ErrPolicyIdentityInvalid) {
		t.Fatalf("empty policy id: %v", err)
	}
	if _, err := NewPolicyIdentity(policyID, 0); !errors.Is(err, ErrPolicyRevisionInvalid) {
		t.Fatalf("zero revision: %v", err)
	}
}

func TestNewBaselinePolicyCanonicalizesAndResolvesRoles(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	bindings := []RoleBinding{
		mustRoleBinding(
			t,
			"binding-z",
			principal,
			RoleMarketingOperator,
			NewSystemScope(),
			BindingEffectAllow,
		),
		mustRoleBinding(
			t,
			"binding-a",
			principal,
			RoleSecurityAuditor,
			NewSystemScope(),
			BindingEffectAllow,
		),
	}
	identity := mustPolicyIdentity(t, "growthos-access", 1)
	policy, err := NewBaselinePolicy(identity, bindings)
	if err != nil {
		t.Fatalf("new baseline policy: %v", err)
	}
	if policy.Identity() != identity {
		t.Fatalf("policy identity = %#v", policy.Identity())
	}
	if got := policy.Roles(); len(got) != 5 {
		t.Fatalf("role count = %d", len(got))
	}
	gotBindings := policy.RoleBindings()
	if len(gotBindings) != 2 ||
		gotBindings[0].ID() != "binding-a" ||
		gotBindings[1].ID() != "binding-z" {
		t.Fatalf("canonical bindings = %#v", gotBindings)
	}
	role, exists := policy.role(RoleMarketingOperator)
	if !exists || role.ID() != RoleMarketingOperator {
		t.Fatalf("resolve role = %#v/%v", role, exists)
	}
	if _, exists := policy.role("admin"); exists {
		t.Fatal("resolved unknown role")
	}
}

func TestPolicyAllowsDenyRestrictionAlongsideAllowGrant(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	tenantID := mustTenantID(t, "tenant-a")
	tenantScope, err := NewTenantScope(tenantID)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}
	bindings := []RoleBinding{
		mustRoleBinding(
			t,
			"allow-tenant",
			principal,
			RoleMarketingOperator,
			tenantScope,
			BindingEffectAllow,
		),
		mustRoleBinding(
			t,
			"deny-activity",
			principal,
			RoleMarketingOperator,
			mustResourceScope(
				t,
				ResourceTypeMarketingActivity,
				mustResourceID(t, "activity-9"),
				tenantID,
			),
			BindingEffectDeny,
		),
	}
	if _, err := NewBaselinePolicy(mustPolicyIdentity(t, "growthos-access", 2), bindings); err != nil {
		t.Fatalf("allow plus narrower deny policy: %v", err)
	}
}

func TestPolicyRejectsDuplicateDanglingAndSemanticBindings(t *testing.T) {
	t.Parallel()

	identity := mustPolicyIdentity(t, "growthos-access", 1)
	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	allow := mustRoleBinding(
		t,
		"binding-1",
		principal,
		RoleMarketingOperator,
		NewSystemScope(),
		BindingEffectAllow,
	)

	tests := []struct {
		name           string
		roles          []Role
		bindings       []RoleBinding
		classification error
	}{
		{
			name:           "duplicate role",
			roles:          []Role{findBaselineRole(t, RoleMarketingOperator), findBaselineRole(t, RoleMarketingOperator)},
			classification: ErrPolicyRoleDuplicate,
		},
		{
			name:           "duplicate binding id",
			roles:          BaselineRoles(),
			bindings:       []RoleBinding{allow, allow},
			classification: ErrPolicyBindingDuplicate,
		},
		{
			name:  "semantic duplicate",
			roles: BaselineRoles(),
			bindings: []RoleBinding{
				allow,
				mustRoleBinding(
					t,
					"binding-2",
					principal,
					RoleMarketingOperator,
					NewSystemScope(),
					BindingEffectAllow,
				),
			},
			classification: ErrPolicyBindingConflict,
		},
		{
			name:           "dangling role",
			roles:          []Role{findBaselineRole(t, RoleSecurityAuditor)},
			bindings:       []RoleBinding{allow},
			classification: ErrPolicyBindingRoleMissing,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewPolicy(identity, test.roles, test.bindings)
			if !errors.Is(err, ErrPolicyInvalid) || !errors.Is(err, test.classification) {
				t.Fatalf("new policy: got %v, want policy and %v", err, test.classification)
			}
		})
	}
}

func TestPolicyCapacityLimitsFailBeforeDeepCopy(t *testing.T) {
	t.Parallel()

	identity := mustPolicyIdentity(t, "growthos-access", 1)
	roles := make([]Role, MaxRolesPerPolicy+1)
	if _, err := NewPolicy(identity, roles, nil); !errors.Is(err, ErrPolicyRoleLimit) {
		t.Fatalf("oversized roles: %v", err)
	}

	bindings := make([]RoleBinding, MaxRoleBindingsPerPolicy+1)
	if _, err := NewPolicy(identity, nil, bindings); !errors.Is(err, ErrPolicyBindingLimit) {
		t.Fatalf("oversized bindings: %v", err)
	}
}

func TestPolicySupportsExplicitDenyAllEmptySnapshot(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(mustPolicyIdentity(t, "deny-all", 1), nil, nil)
	if err != nil {
		t.Fatalf("new empty policy: %v", err)
	}
	if len(policy.Roles()) != 0 || len(policy.RoleBindings()) != 0 {
		t.Fatalf("empty policy contains state: %#v", policy)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("validate empty policy: %v", err)
	}
}

func TestPolicyDefensiveCopiesNestedRolesAndBindings(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	roles := BaselineRoles()
	bindings := []RoleBinding{
		mustRoleBinding(
			t,
			"binding-1",
			principal,
			RoleMarketingOperator,
			NewSystemScope(),
			BindingEffectAllow,
		),
	}
	policy, err := NewPolicy(mustPolicyIdentity(t, "growthos-access", 1), roles, bindings)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}

	roles[0].id = "admin"
	if len(roles[1].permissions) > 0 {
		roles[1].permissions[0] = Permission{}
	}
	bindings[0] = RoleBinding{}
	if err := policy.Validate(); err != nil {
		t.Fatalf("caller mutation changed policy: %v", err)
	}

	returnedRoles := policy.Roles()
	returnedRoles[0].id = "admin"
	if len(returnedRoles[1].permissions) > 0 {
		returnedRoles[1].permissions[0] = Permission{}
	}
	returnedBindings := policy.RoleBindings()
	returnedBindings[0] = RoleBinding{}
	if err := policy.Validate(); err != nil {
		t.Fatalf("getter mutation changed policy: %v", err)
	}
}

func TestPolicyValidateRejectsNonCanonicalAndPartialInternalState(t *testing.T) {
	t.Parallel()

	identity := mustPolicyIdentity(t, "growthos-access", 1)
	roles := BaselineRoles()
	slices.Reverse(roles)
	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	bindings := []RoleBinding{
		mustRoleBinding(
			t,
			"binding-z",
			principal,
			RoleMarketingOperator,
			NewSystemScope(),
			BindingEffectAllow,
		),
		mustRoleBinding(
			t,
			"binding-a",
			principal,
			RoleSecurityAuditor,
			NewSystemScope(),
			BindingEffectAllow,
		),
	}

	invalid := []Policy{
		{},
		{identity: identity, roles: roles},
		{identity: identity, roles: BaselineRoles(), bindings: bindings},
		{identity: identity, roles: []Role{{id: RoleMarketingOperator, permissions: []Permission{{}}}}},
	}
	for index, policy := range invalid {
		if err := policy.Validate(); !errors.Is(err, ErrPolicyInvalid) {
			t.Fatalf("validate invalid %d: got %v", index, err)
		}
	}
}

func mustRoleBindingID(t *testing.T, value string) RoleBindingID {
	t.Helper()
	id, err := NewRoleBindingID(value)
	if err != nil {
		t.Fatalf("new role binding id %q: %v", value, err)
	}
	return id
}

func mustPolicyID(t *testing.T, value string) PolicyID {
	t.Helper()
	id, err := NewPolicyID(value)
	if err != nil {
		t.Fatalf("new policy id %q: %v", value, err)
	}
	return id
}

func mustPolicyIdentity(t *testing.T, value string, revision PolicyRevision) PolicyIdentity {
	t.Helper()
	identity, err := NewPolicyIdentity(mustPolicyID(t, value), revision)
	if err != nil {
		t.Fatalf("new policy identity %q/%d: %v", value, revision, err)
	}
	return identity
}

func mustRoleBinding(
	t *testing.T,
	id string,
	principal Principal,
	roleID RoleID,
	scope Scope,
	effect BindingEffect,
) RoleBinding {
	t.Helper()
	binding, err := NewRoleBinding(
		mustRoleBindingID(t, id),
		principal,
		roleID,
		scope,
		effect,
	)
	if err != nil {
		t.Fatalf("new role binding %q: %v", id, err)
	}
	return binding
}

func mustResourceScope(
	t *testing.T,
	resourceType ResourceType,
	resourceID ResourceID,
	tenantID TenantID,
) Scope {
	t.Helper()
	scope, err := NewResourceScope(resourceType, resourceID, tenantID)
	if err != nil {
		t.Fatalf("new resource scope: %v", err)
	}
	return scope
}
