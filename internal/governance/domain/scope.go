package domain

import "fmt"

// ScopeKind is the closed v1 data-range vocabulary for role bindings.
type ScopeKind string

const (
	ScopeKindSystem   ScopeKind = "system"
	ScopeKindTenant   ScopeKind = "tenant"
	ScopeKindOwned    ScopeKind = "owned"
	ScopeKindResource ScopeKind = "resource"
)

// Valid reports whether kind belongs to the v1 closed vocabulary.
func (kind ScopeKind) Valid() bool {
	switch kind {
	case ScopeKindSystem, ScopeKindTenant, ScopeKindOwned, ScopeKindResource:
		return true
	default:
		return false
	}
}

// Scope is a closed immutable union. Empty fields never mean wildcard.
type Scope struct {
	kind         ScopeKind
	tenantID     TenantID
	resourceType ResourceType
	resourceID   ResourceID
}

// NewSystemScope creates an explicit cross-resource binding range. Exact
// permissions are still required; this is not a superuser bypass.
func NewSystemScope() Scope { return Scope{kind: ScopeKindSystem} }

// NewTenantScope creates a binding range for one exact tenant.
func NewTenantScope(tenantID TenantID) (Scope, error) {
	scope := Scope{kind: ScopeKindTenant, tenantID: tenantID}
	if err := scope.Validate(); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// NewOwnedScope creates one tenant-qualified object range whose owner must
// equal the evaluated Principal. It never crosses tenants or matches missing
// tenant/owner facts.
func NewOwnedScope(tenantID TenantID) (Scope, error) {
	scope := Scope{kind: ScopeKindOwned, tenantID: tenantID}
	if err := scope.Validate(); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// NewResourceScope creates an exact object binding. tenantID may be absent for
// a system object; tenant presence must match the request target exactly.
func NewResourceScope(
	resourceType ResourceType,
	resourceID ResourceID,
	tenantID TenantID,
) (Scope, error) {
	scope := Scope{
		kind:         ScopeKindResource,
		tenantID:     tenantID,
		resourceType: resourceType,
		resourceID:   resourceID,
	}
	if err := scope.Validate(); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// Validate rejects unsupported kinds and mixed union representations.
func (scope Scope) Validate() error {
	if !scope.kind.Valid() {
		return fmt.Errorf(
			"%w: %w: kind %q",
			ErrScopeInvalid,
			ErrScopeKindUnsupported,
			scope.kind,
		)
	}
	switch scope.kind {
	case ScopeKindSystem:
		if scope.tenantID != "" || scope.resourceType != "" || scope.resourceID != "" {
			return fmt.Errorf("%w: %s scope carries foreign fields", ErrScopeInvalid, scope.kind)
		}
	case ScopeKindTenant, ScopeKindOwned:
		if err := scope.tenantID.Validate(); err != nil {
			return fmt.Errorf("%w: tenant: %w", ErrScopeInvalid, err)
		}
		if scope.resourceType != "" || scope.resourceID != "" {
			return fmt.Errorf("%w: tenant scope carries resource fields", ErrScopeInvalid)
		}
	case ScopeKindResource:
		if !scope.resourceType.Valid() {
			return fmt.Errorf(
				"%w: %w: resource type %q",
				ErrScopeInvalid,
				ErrResourceTypeUnsupported,
				scope.resourceType,
			)
		}
		if err := scope.resourceID.Validate(); err != nil {
			return fmt.Errorf("%w: resource id: %w", ErrScopeInvalid, err)
		}
		if scope.tenantID != "" {
			if err := scope.tenantID.Validate(); err != nil {
				return fmt.Errorf("%w: tenant: %w", ErrScopeInvalid, err)
			}
		}
	}
	return nil
}

// Kind returns system, tenant, owned, or resource.
func (scope Scope) Kind() ScopeKind { return scope.kind }

// TenantID returns the exact tenant carried by tenant or resource scope.
func (scope Scope) TenantID() (TenantID, bool) {
	return scope.tenantID, scope.tenantID != ""
}

// Resource returns the exact object identity and true for resource scope.
func (scope Scope) Resource() (ResourceType, ResourceID, bool) {
	return scope.resourceType, scope.resourceID, scope.kind == ScopeKindResource
}

func (scope Scope) matches(principal Principal, resource Resource) bool {
	switch scope.kind {
	case ScopeKindSystem:
		return true
	case ScopeKindTenant:
		tenantID, exists := resource.TenantID()
		return exists && tenantID == scope.tenantID
	case ScopeKindOwned:
		tenantID, tenantExists := resource.TenantID()
		owner, exists := resource.Owner()
		return resource.kind == ResourceKindObject && tenantExists &&
			tenantID == scope.tenantID && exists && owner == principal
	case ScopeKindResource:
		if resource.kind != ResourceKindObject ||
			resource.typeName != scope.resourceType ||
			resource.id != scope.resourceID {
			return false
		}
		return resource.tenantID == scope.tenantID
	default:
		return false
	}
}
