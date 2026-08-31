package domain

import (
	"reflect"
	"slices"
	"testing"
)

func FuzzGovernanceIdentifiersNeverNormalizeOrAcceptWildcards(f *testing.F) {
	for _, seed := range []string{
		"",
		"a",
		"operator-1",
		" tenant",
		"TENANT",
		"tenant/*",
		"租户",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		constructors := []func(string) (string, error){
			func(value string) (string, error) {
				identifier, err := NewPrincipalID(value)
				return identifier.String(), err
			},
			func(value string) (string, error) {
				identifier, err := NewResourceID(value)
				return identifier.String(), err
			},
			func(value string) (string, error) {
				identifier, err := NewTenantID(value)
				return identifier.String(), err
			},
			func(value string) (string, error) {
				reference, err := NewAuditReference(value)
				return reference.String(), err
			},
		}
		for _, construct := range constructors {
			value, err := construct(raw)
			if err != nil {
				if value != "" {
					t.Fatalf("failure returned partial value %q", value)
				}
				continue
			}
			if value != raw {
				t.Fatalf("constructor normalized %q into %q", raw, value)
			}
			if raw == "*" || containsWildcardOrSlash(raw) {
				t.Fatalf("constructor accepted wildcard/path value %q", raw)
			}
		}
	})
}

func FuzzPolicyEvaluationDenyPrecedenceAndOrderIndependence(
	f *testing.F,
) {
	f.Add(false, false, byte(0))
	f.Add(true, false, byte(1))
	f.Add(true, true, byte(2))
	f.Fuzz(func(t *testing.T, includeDeny bool, otherTenant bool, actionSelector byte) {
		principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
		tenantA := mustTenantID(t, "tenant-a")
		targetTenant := tenantA
		if otherTenant {
			targetTenant = mustTenantID(t, "tenant-b")
		}
		resourceID := mustResourceID(t, "activity-1")
		tenantScope, err := NewTenantScope(tenantA)
		if err != nil {
			t.Fatalf("new tenant scope: %v", err)
		}
		bindings := []RoleBinding{mustRoleBinding(
			t,
			"z-allow",
			principal,
			RoleMarketingOperator,
			tenantScope,
			BindingEffectAllow,
		)}
		if includeDeny {
			bindings = append(bindings, mustRoleBinding(
				t,
				"a-deny",
				principal,
				RoleMarketingOperator,
				mustResourceScope(
					t,
					ResourceTypeMarketingActivity,
					resourceID,
					tenantA,
				),
				BindingEffectDeny,
			))
		}
		reversed := append([]RoleBinding(nil), bindings...)
		slices.Reverse(reversed)
		left := mustBaselinePolicy(t, "growthos-access", 1, bindings)
		right := mustBaselinePolicy(t, "growthos-access", 1, reversed)

		actions := []Action{ActionRead, ActionPublish, ActionRollback, ActionRetire}
		action := actions[int(actionSelector)%len(actions)]
		request := mustAuthorizationRequest(
			t,
			principal,
			mustObjectResource(
				t,
				ResourceTypeMarketingActivity,
				resourceID,
				targetTenant,
				Principal{},
			),
			action,
			"evaluation-fuzz",
		)
		leftDecision, leftErr := left.Evaluate(request)
		rightDecision, rightErr := right.Evaluate(request)
		if leftErr != nil || rightErr != nil {
			t.Fatalf("evaluate errors = %v/%v", leftErr, rightErr)
		}
		if !reflect.DeepEqual(leftDecision, rightDecision) {
			t.Fatalf("binding order changed decision: %#v / %#v", leftDecision, rightDecision)
		}
		if includeDeny && !otherTenant && leftDecision.Allowed() {
			t.Fatalf("matching deny produced allow: %#v", leftDecision)
		}
		if otherTenant && leftDecision.Allowed() {
			t.Fatalf("tenant mismatch produced allow: %#v", leftDecision)
		}
		if !leftDecision.Confirmed() {
			t.Fatalf("valid policy/request produced unconfirmed decision: %#v", leftDecision)
		}
	})
}

func containsWildcardOrSlash(value string) bool {
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '*', '/', '\\':
			return true
		}
	}
	return false
}
