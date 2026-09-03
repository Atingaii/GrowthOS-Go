package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestCapabilityCatalogMatchesReviewedKindTypeActionMatrix(t *testing.T) {
	t.Parallel()

	expected := reviewedExpectedCapabilities(t)
	expectedSet := make(map[Permission]struct{}, len(expected))
	for _, permission := range expected {
		expectedSet[permission] = struct{}{}
	}
	if len(expectedSet) != 17 {
		t.Fatalf("reviewed capability count = %d, want 17", len(expectedSet))
	}

	kinds := []ResourceKind{ResourceKindCollection, ResourceKindObject}
	types := []ResourceType{
		ResourceTypeMarketingActivity,
		ResourceTypeLotteryStrategy,
		ResourceTypeLotteryRoutingGraph,
		ResourceTypeGovernancePolicy,
		ResourceTypeGovernanceAudit,
	}
	actions := []Action{
		ActionCreate,
		ActionRead,
		ActionSimulate,
		ActionPublish,
		ActionRollback,
		ActionRetire,
		ActionChange,
	}

	for _, kind := range kinds {
		for _, resourceType := range types {
			for _, action := range actions {
				candidate := Permission{
					resourceKind: kind,
					resourceType: resourceType,
					action:       action,
				}
				_, wantValid := expectedSet[candidate]
				err := ValidateCapability(kind, resourceType, action)
				if wantValid && err != nil {
					t.Errorf("registered capability %s:%s:%s rejected: %v", kind, resourceType, action, err)
				}
				if !wantValid && !errors.Is(err, ErrCapabilityUnsupported) {
					t.Errorf("table-external capability %s:%s:%s error = %v", kind, resourceType, action, err)
				}
			}
		}
	}
}

func TestBaselineRoleTemplatesMatchIndependentCapabilityCeilings(t *testing.T) {
	t.Parallel()

	expectedByRole := map[RoleID][]Permission{
		RolePlatformAdministrator: reviewedExpectedCapabilities(t),
		RoleMarketingOperator: {
			mustPermission(t, ResourceKindCollection, ResourceTypeMarketingActivity, ActionCreate),
			mustPermission(t, ResourceKindCollection, ResourceTypeMarketingActivity, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionPublish),
			mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRollback),
			mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRetire),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryStrategy, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryStrategy, ActionRead),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryRoutingGraph, ActionRead),
		},
		RoleLotteryDesigner: {
			mustPermission(t, ResourceKindCollection, ResourceTypeMarketingActivity, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRead),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryStrategy, ActionCreate),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryStrategy, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryStrategy, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryStrategy, ActionSimulate),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionCreate),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryRoutingGraph, ActionRead),
		},
		RoleSecurityAuditor: {
			mustPermission(t, ResourceKindCollection, ResourceTypeMarketingActivity, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRead),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryStrategy, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryStrategy, ActionRead),
			mustPermission(t, ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeLotteryRoutingGraph, ActionRead),
			mustPermission(t, ResourceKindCollection, ResourceTypeGovernancePolicy, ActionRead),
			mustPermission(t, ResourceKindObject, ResourceTypeGovernancePolicy, ActionRead),
			mustPermission(t, ResourceKindCollection, ResourceTypeGovernanceAudit, ActionRead),
		},
		RoleGrowthMember: {},
	}

	allCapabilities := reviewedExpectedCapabilities(t)
	for roleID, expected := range expectedByRole {
		slices.SortFunc(expected, comparePermission)
		actual := findBaselineRole(t, roleID).Permissions()
		if !slices.Equal(actual, expected) {
			t.Fatalf("role %q capability ceiling = %#v, want %#v", roleID, actual, expected)
		}
		for _, candidate := range allCapabilities {
			_, inCeiling := slices.BinarySearchFunc(expected, candidate, comparePermission)
			if inCeiling {
				continue
			}
			if _, err := NewRole(roleID, []Permission{candidate}); !errors.Is(err, ErrRoleInvalid) {
				t.Errorf("role %q accepted table-external permission %#v: %v", roleID, candidate, err)
			}
		}
	}
}

func TestReadCapabilityDoesNotCrossCollectionObjectBoundary(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	binding := mustRoleBinding(
		t,
		"system-read",
		principal,
		RoleMarketingOperator,
		NewSystemScope(),
		BindingEffectAllow,
	)
	collectionRead := mustPermission(
		t,
		ResourceKindCollection,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	objectRead := mustPermission(
		t,
		ResourceKindObject,
		ResourceTypeMarketingActivity,
		ActionRead,
	)
	collection, err := NewCollectionResource(ResourceTypeMarketingActivity, "")
	if err != nil {
		t.Fatalf("new collection: %v", err)
	}
	object := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		mustResourceID(t, "activity-1"),
		"",
		Principal{},
	)

	tests := []struct {
		name        string
		permission  Permission
		matching    Resource
		nonMatching Resource
	}{
		{"collection permission", collectionRead, collection, object},
		{"object permission", objectRead, object, collection},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			role, err := NewRole(RoleMarketingOperator, []Permission{test.permission})
			if err != nil {
				t.Fatalf("new constrained role: %v", err)
			}
			policy, err := NewPolicy(
				mustPolicyIdentity(t, "kind-boundary", 1),
				[]Role{role},
				[]RoleBinding{binding},
			)
			if err != nil {
				t.Fatalf("new policy: %v", err)
			}
			matching, err := policy.Evaluate(mustAuthorizationRequest(
				t, principal, test.matching, ActionRead, "matching-read",
			))
			if err != nil || !matching.Allowed() {
				t.Fatalf("matching read = %#v/%v", matching, err)
			}
			nonMatching, err := policy.Evaluate(mustAuthorizationRequest(
				t, principal, test.nonMatching, ActionRead, "nonmatching-read",
			))
			if err != nil {
				t.Fatalf("evaluate nonmatching read: %v", err)
			}
			if nonMatching.Allowed() || nonMatching.Reason() != DecisionReasonNoPermission {
				t.Fatalf("cross-kind read = %#v, want no_permission deny", nonMatching)
			}
		})
	}
}

func reviewedExpectedCapabilities(t *testing.T) []Permission {
	t.Helper()
	return []Permission{
		mustPermission(t, ResourceKindCollection, ResourceTypeMarketingActivity, ActionCreate),
		mustPermission(t, ResourceKindCollection, ResourceTypeMarketingActivity, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionPublish),
		mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRollback),
		mustPermission(t, ResourceKindObject, ResourceTypeMarketingActivity, ActionRetire),
		mustPermission(t, ResourceKindCollection, ResourceTypeLotteryStrategy, ActionCreate),
		mustPermission(t, ResourceKindCollection, ResourceTypeLotteryStrategy, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeLotteryStrategy, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeLotteryStrategy, ActionSimulate),
		mustPermission(t, ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionCreate),
		mustPermission(t, ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeLotteryRoutingGraph, ActionRead),
		mustPermission(t, ResourceKindCollection, ResourceTypeGovernancePolicy, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeGovernancePolicy, ActionRead),
		mustPermission(t, ResourceKindObject, ResourceTypeGovernancePolicy, ActionChange),
		mustPermission(t, ResourceKindCollection, ResourceTypeGovernanceAudit, ActionRead),
	}
}
