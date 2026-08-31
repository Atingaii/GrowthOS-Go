package domain

import "testing"

func TestExplicitDenyThreatMatrix(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	tenantA := mustTenantID(t, "tenant-a")
	tenantB := mustTenantID(t, "tenant-b")
	resourceID := mustResourceID(t, "activity-1")
	resource := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		tenantA,
		principal,
	)
	tenantAScope, err := NewTenantScope(tenantA)
	if err != nil {
		t.Fatalf("new tenant a scope: %v", err)
	}
	tenantBScope, err := NewTenantScope(tenantB)
	if err != nil {
		t.Fatalf("new tenant b scope: %v", err)
	}
	exactScope := mustResourceScope(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		tenantA,
	)
	ownedScope := mustOwnedScope(t, tenantA)

	tests := []struct {
		name        string
		bindings    []RoleBinding
		wantAllowed bool
		wantReason  DecisionReason
		wantMatches int
	}{
		{
			name: "matching allow and nonmatching deny",
			bindings: []RoleBinding{
				mustRoleBinding(t, "allow-a", principal, RoleMarketingOperator, tenantAScope, BindingEffectAllow),
				mustRoleBinding(t, "deny-b", principal, RoleMarketingOperator, tenantBScope, BindingEffectDeny),
			},
			wantAllowed: true,
			wantReason:  DecisionReasonExplicitAllow,
			wantMatches: 1,
		},
		{
			name: "different role deny overrides allow",
			bindings: []RoleBinding{
				mustRoleBinding(t, "allow-marketing", principal, RoleMarketingOperator, tenantAScope, BindingEffectAllow),
				mustRoleBinding(t, "deny-auditor", principal, RoleSecurityAuditor, tenantAScope, BindingEffectDeny),
			},
			wantReason:  DecisionReasonExplicitDenyOverrodeAllow,
			wantMatches: 2,
		},
		{
			name: "system deny overrides exact allow",
			bindings: []RoleBinding{
				mustRoleBinding(t, "allow-exact", principal, RoleMarketingOperator, exactScope, BindingEffectAllow),
				mustRoleBinding(t, "deny-system", principal, RoleMarketingOperator, NewSystemScope(), BindingEffectDeny),
			},
			wantReason:  DecisionReasonExplicitDenyOverrodeAllow,
			wantMatches: 2,
		},
		{
			name: "system deny overrides owned allow",
			bindings: []RoleBinding{
				mustRoleBinding(t, "allow-owned", principal, RoleMarketingOperator, ownedScope, BindingEffectAllow),
				mustRoleBinding(t, "deny-system-owned", principal, RoleMarketingOperator, NewSystemScope(), BindingEffectDeny),
			},
			wantReason:  DecisionReasonExplicitDenyOverrodeAllow,
			wantMatches: 2,
		},
		{
			name: "same scope allow and deny",
			bindings: []RoleBinding{
				mustRoleBinding(t, "allow-same", principal, RoleMarketingOperator, tenantAScope, BindingEffectAllow),
				mustRoleBinding(t, "deny-same", principal, RoleMarketingOperator, tenantAScope, BindingEffectDeny),
			},
			wantReason:  DecisionReasonExplicitDenyOverrodeAllow,
			wantMatches: 2,
		},
		{
			name: "deny without target permission does not poison allow",
			bindings: []RoleBinding{
				mustRoleBinding(t, "allow-publish", principal, RoleMarketingOperator, tenantAScope, BindingEffectAllow),
				mustRoleBinding(t, "deny-member", principal, RoleGrowthMember, tenantAScope, BindingEffectDeny),
			},
			wantAllowed: true,
			wantReason:  DecisionReasonExplicitAllow,
			wantMatches: 1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			policy := mustBaselinePolicy(t, "deny-matrix", 1, test.bindings)
			decision, err := policy.Evaluate(mustAuthorizationRequest(
				t,
				principal,
				resource,
				ActionRead,
				"deny-matrix-evaluation",
			))
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if decision.Allowed() != test.wantAllowed || decision.Reason() != test.wantReason {
				t.Fatalf("decision = %#v, want allowed=%v reason=%q", decision, test.wantAllowed, test.wantReason)
			}
			if got := len(decision.Matches()); got != test.wantMatches {
				t.Fatalf("match count = %d, want %d", got, test.wantMatches)
			}
		})
	}
}
