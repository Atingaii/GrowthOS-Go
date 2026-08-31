package domain

import (
	"errors"
	"testing"
)

func TestPrincipalClosedKindsAndShape(t *testing.T) {
	t.Parallel()

	identifier := mustPrincipalID(t, "operator-7")
	for _, kind := range []PrincipalKind{
		PrincipalKindHuman,
		PrincipalKindService,
		PrincipalKindAgent,
	} {
		principal, err := NewPrincipal(kind, identifier)
		if err != nil {
			t.Fatalf("new %s principal: %v", kind, err)
		}
		if principal.Kind() != kind || principal.ID() != identifier {
			t.Fatalf("principal getters = %q/%q", principal.Kind(), principal.ID())
		}
	}

	invalid := []Principal{
		{},
		{kind: "anonymous", id: identifier},
		{kind: PrincipalKindHuman},
		{kind: PrincipalKindService, id: "UPPER"},
	}
	for _, principal := range invalid {
		if err := principal.Validate(); !errors.Is(err, ErrPrincipalInvalid) {
			t.Fatalf("validate %#v: got %v", principal, err)
		}
	}
	if PrincipalKind("root").Valid() || PrincipalKind("").Valid() {
		t.Fatal("unknown principal kind became valid")
	}
}

func TestResourceConstructorsPreserveExactFacts(t *testing.T) {
	t.Parallel()

	tenantID := mustTenantID(t, "tenant-1")
	owner := mustPrincipal(t, PrincipalKindHuman, "member-9")
	resourceID := mustResourceID(t, "activity-42")

	collection, err := NewCollectionResource(ResourceTypeMarketingActivity, tenantID)
	if err != nil {
		t.Fatalf("new collection: %v", err)
	}
	if collection.Kind() != ResourceKindCollection || collection.Type() != ResourceTypeMarketingActivity {
		t.Fatalf("collection identity = %#v", collection)
	}
	if _, exists := collection.ID(); exists {
		t.Fatal("collection exposed object id")
	}
	if got, exists := collection.TenantID(); !exists || got != tenantID {
		t.Fatalf("collection tenant = %q/%v", got, exists)
	}
	if _, exists := collection.Owner(); exists {
		t.Fatal("collection exposed owner")
	}

	object, err := NewObjectResource(
		ResourceTypeMarketingActivity,
		resourceID,
		tenantID,
		owner,
	)
	if err != nil {
		t.Fatalf("new object: %v", err)
	}
	if got, exists := object.ID(); !exists || got != resourceID {
		t.Fatalf("object id = %q/%v", got, exists)
	}
	if got, exists := object.TenantID(); !exists || got != tenantID {
		t.Fatalf("object tenant = %q/%v", got, exists)
	}
	if got, exists := object.Owner(); !exists || got != owner {
		t.Fatalf("object owner = %#v/%v", got, exists)
	}

	systemObject, err := NewObjectResource(
		ResourceTypeGovernancePolicy,
		mustResourceID(t, "growthos-access"),
		"",
		Principal{},
	)
	if err != nil {
		t.Fatalf("new system object: %v", err)
	}
	if _, exists := systemObject.TenantID(); exists {
		t.Fatal("system object exposed tenant")
	}
	if _, exists := systemObject.Owner(); exists {
		t.Fatal("system object exposed owner")
	}
}

func TestResourceRejectsMixedAndUnknownRepresentations(t *testing.T) {
	t.Parallel()

	invalid := []Resource{
		{},
		{kind: ResourceKindCollection, typeName: "unknown.resource"},
		{
			kind:     ResourceKindCollection,
			typeName: ResourceTypeMarketingActivity,
			id:       "activity-1",
		},
		{
			kind:     ResourceKindCollection,
			typeName: ResourceTypeMarketingActivity,
			owner:    mustPrincipal(t, PrincipalKindHuman, "member-1"),
		},
		{kind: ResourceKindObject, typeName: ResourceTypeMarketingActivity},
		{
			kind:     ResourceKindObject,
			typeName: ResourceTypeMarketingActivity,
			id:       "activity-1",
			tenantID: "TENANT",
		},
		{
			kind:     ResourceKindObject,
			typeName: ResourceTypeMarketingActivity,
			id:       "activity-1",
			owner:    Principal{kind: PrincipalKindHuman},
		},
	}
	for _, resource := range invalid {
		if err := resource.Validate(); !errors.Is(err, ErrResourceInvalid) {
			t.Fatalf("validate %#v: got %v", resource, err)
		}
	}
}

func TestScopeMatchingIsExactAndFailClosed(t *testing.T) {
	t.Parallel()

	principal := mustPrincipal(t, PrincipalKindHuman, "operator-1")
	otherPrincipal := mustPrincipal(t, PrincipalKindHuman, "operator-2")
	sameIDService := mustPrincipal(t, PrincipalKindService, "operator-1")
	tenantA := mustTenantID(t, "tenant-a")
	tenantB := mustTenantID(t, "tenant-b")
	resourceID := mustResourceID(t, "activity-1")

	ownedTenantA := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		tenantA,
		principal,
	)
	otherOwnedTenantA := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		tenantA,
		otherPrincipal,
	)
	sameIDKindMismatch := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		tenantA,
		sameIDService,
	)
	tenantBObject := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		tenantB,
		principal,
	)
	missingFacts := mustObjectResource(
		t,
		ResourceTypeMarketingActivity,
		resourceID,
		"",
		Principal{},
	)
	tenantACollection, err := NewCollectionResource(ResourceTypeMarketingActivity, tenantA)
	if err != nil {
		t.Fatalf("new tenant collection: %v", err)
	}

	tenantScope, err := NewTenantScope(tenantA)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}
	exactScope, err := NewResourceScope(
		ResourceTypeMarketingActivity,
		resourceID,
		tenantA,
	)
	if err != nil {
		t.Fatalf("new exact scope: %v", err)
	}
	systemExactScope, err := NewResourceScope(
		ResourceTypeMarketingActivity,
		resourceID,
		"",
	)
	if err != nil {
		t.Fatalf("new system exact scope: %v", err)
	}

	tests := []struct {
		name      string
		scope     Scope
		resource  Resource
		principal Principal
		want      bool
	}{
		{"system object", NewSystemScope(), ownedTenantA, principal, true},
		{"system collection", NewSystemScope(), tenantACollection, principal, true},
		{"tenant object", tenantScope, ownedTenantA, principal, true},
		{"tenant collection", tenantScope, tenantACollection, principal, true},
		{"tenant mismatch", tenantScope, tenantBObject, principal, false},
		{"tenant fact missing", tenantScope, missingFacts, principal, false},
		{"owned exact owner", mustOwnedScope(t, tenantA), ownedTenantA, principal, true},
		{"owned other owner", mustOwnedScope(t, tenantA), otherOwnedTenantA, principal, false},
		{"owned same id different principal kind", mustOwnedScope(t, tenantA), sameIDKindMismatch, principal, false},
		{"owned tenant mismatch", mustOwnedScope(t, tenantA), tenantBObject, principal, false},
		{"owned collection", mustOwnedScope(t, tenantA), tenantACollection, principal, false},
		{"owned fact missing", mustOwnedScope(t, tenantA), missingFacts, principal, false},
		{"resource exact", exactScope, ownedTenantA, principal, true},
		{"resource tenant mismatch", exactScope, tenantBObject, principal, false},
		{"tenant resource scope rejects missing tenant", exactScope, missingFacts, principal, false},
		{"system resource scope rejects present tenant", systemExactScope, ownedTenantA, principal, false},
		{"system resource exact", systemExactScope, missingFacts, principal, true},
		{"resource collection", exactScope, tenantACollection, principal, false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.scope.matches(test.principal, test.resource); got != test.want {
				t.Fatalf("matches = %v, want %v", got, test.want)
			}
		})
	}
}

func TestScopeConstructorsAndGetters(t *testing.T) {
	t.Parallel()

	system := NewSystemScope()
	if system.Kind() != ScopeKindSystem {
		t.Fatalf("system kind = %q", system.Kind())
	}
	if _, exists := system.TenantID(); exists {
		t.Fatal("system scope exposed tenant")
	}
	if _, _, exists := system.Resource(); exists {
		t.Fatal("system scope exposed resource")
	}

	tenantID := mustTenantID(t, "tenant-7")
	tenant, err := NewTenantScope(tenantID)
	if err != nil {
		t.Fatalf("new tenant scope: %v", err)
	}
	if got, exists := tenant.TenantID(); !exists || got != tenantID {
		t.Fatalf("tenant getter = %q/%v", got, exists)
	}

	owned, err := NewOwnedScope(tenantID)
	if err != nil {
		t.Fatalf("new owned scope: %v", err)
	}
	if owned.Kind() != ScopeKindOwned {
		t.Fatalf("owned kind = %q", owned.Kind())
	}
	if got, exists := owned.TenantID(); !exists || got != tenantID {
		t.Fatalf("owned tenant getter = %q/%v", got, exists)
	}

	resourceID := mustResourceID(t, "activity-3")
	exact, err := NewResourceScope(ResourceTypeMarketingActivity, resourceID, tenantID)
	if err != nil {
		t.Fatalf("new resource scope: %v", err)
	}
	resourceType, gotID, exists := exact.Resource()
	if !exists || resourceType != ResourceTypeMarketingActivity || gotID != resourceID {
		t.Fatalf("resource getter = %q/%q/%v", resourceType, gotID, exists)
	}
	if got, exists := exact.TenantID(); !exists || got != tenantID {
		t.Fatalf("exact tenant getter = %q/%v", got, exists)
	}

	for _, kind := range []ScopeKind{
		ScopeKindSystem,
		ScopeKindTenant,
		ScopeKindOwned,
		ScopeKindResource,
	} {
		if !kind.Valid() {
			t.Fatalf("known kind %q invalid", kind)
		}
	}
	if ScopeKind("*").Valid() || ScopeKind("").Valid() {
		t.Fatal("unknown scope kind became valid")
	}
}

func TestScopeRejectsMixedRepresentations(t *testing.T) {
	t.Parallel()

	invalid := []Scope{
		{},
		{kind: ScopeKindSystem, tenantID: "tenant-1"},
		{kind: ScopeKindOwned},
		{kind: ScopeKindOwned, tenantID: "tenant-1", resourceID: "activity-1"},
		{kind: ScopeKindTenant},
		{kind: ScopeKindTenant, tenantID: "tenant-1", resourceType: ResourceTypeMarketingActivity},
		{kind: ScopeKindResource, resourceType: ResourceTypeMarketingActivity},
		{kind: ScopeKindResource, resourceType: "unknown", resourceID: "activity-1"},
		{
			kind:         ScopeKindResource,
			resourceType: ResourceTypeMarketingActivity,
			resourceID:   "activity-1",
			tenantID:     "TENANT",
		},
	}
	for _, scope := range invalid {
		if err := scope.Validate(); !errors.Is(err, ErrScopeInvalid) {
			t.Fatalf("validate %#v: got %v", scope, err)
		}
	}
}

func mustPrincipalID(t *testing.T, value string) PrincipalID {
	t.Helper()
	identifier, err := NewPrincipalID(value)
	if err != nil {
		t.Fatalf("new principal id %q: %v", value, err)
	}
	return identifier
}

func mustResourceID(t *testing.T, value string) ResourceID {
	t.Helper()
	identifier, err := NewResourceID(value)
	if err != nil {
		t.Fatalf("new resource id %q: %v", value, err)
	}
	return identifier
}

func mustTenantID(t *testing.T, value string) TenantID {
	t.Helper()
	identifier, err := NewTenantID(value)
	if err != nil {
		t.Fatalf("new tenant id %q: %v", value, err)
	}
	return identifier
}

func mustPrincipal(t *testing.T, kind PrincipalKind, id string) Principal {
	t.Helper()
	principal, err := NewPrincipal(kind, mustPrincipalID(t, id))
	if err != nil {
		t.Fatalf("new principal %s/%s: %v", kind, id, err)
	}
	return principal
}

func mustObjectResource(
	t *testing.T,
	resourceType ResourceType,
	id ResourceID,
	tenantID TenantID,
	owner Principal,
) Resource {
	t.Helper()
	resource, err := NewObjectResource(resourceType, id, tenantID, owner)
	if err != nil {
		t.Fatalf("new object resource: %v", err)
	}
	return resource
}

func mustOwnedScope(t *testing.T, tenantID TenantID) Scope {
	t.Helper()
	scope, err := NewOwnedScope(tenantID)
	if err != nil {
		t.Fatalf("new owned scope: %v", err)
	}
	return scope
}
