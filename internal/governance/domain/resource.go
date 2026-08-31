package domain

import "fmt"

// ResourceKind distinguishes collection actions from object-level actions.
type ResourceKind string

const (
	ResourceKindCollection ResourceKind = "collection"
	ResourceKindObject     ResourceKind = "object"
)

// Valid reports whether kind belongs to the v1 closed vocabulary.
func (kind ResourceKind) Valid() bool {
	return kind == ResourceKindCollection || kind == ResourceKindObject
}

// Resource is the immutable target of an authorization request. Tenant and
// owner are facts for scope matching; construction cannot prove their source.
type Resource struct {
	kind     ResourceKind
	typeName ResourceType
	id       ResourceID
	tenantID TenantID
	owner    Principal
}

// NewCollectionResource creates a collection target. tenantID may be empty for
// a system collection; an empty tenant never behaves as a global wildcard.
func NewCollectionResource(resourceType ResourceType, tenantID TenantID) (Resource, error) {
	resource := Resource{
		kind:     ResourceKindCollection,
		typeName: resourceType,
		tenantID: tenantID,
	}
	if err := resource.Validate(); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

// NewObjectResource creates an exact object target. tenantID and owner may be
// absent only when the authoritative resource has no such fact.
func NewObjectResource(
	resourceType ResourceType,
	id ResourceID,
	tenantID TenantID,
	owner Principal,
) (Resource, error) {
	resource := Resource{
		kind:     ResourceKindObject,
		typeName: resourceType,
		id:       id,
		tenantID: tenantID,
		owner:    owner,
	}
	if err := resource.Validate(); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

// Validate rejects unknown types and mixed collection/object representations.
func (resource Resource) Validate() error {
	if !resource.kind.Valid() {
		return fmt.Errorf("%w: kind %q", ErrResourceInvalid, resource.kind)
	}
	if !resource.typeName.Valid() {
		return fmt.Errorf(
			"%w: %w: type %q",
			ErrResourceInvalid,
			ErrResourceTypeUnsupported,
			resource.typeName,
		)
	}
	if resource.tenantID != "" {
		if err := resource.tenantID.Validate(); err != nil {
			return fmt.Errorf("%w: tenant: %w", ErrResourceInvalid, err)
		}
	}
	switch resource.kind {
	case ResourceKindCollection:
		if resource.id != "" || !resource.owner.isZero() {
			return fmt.Errorf("%w: collection cannot carry object id or owner", ErrResourceInvalid)
		}
	case ResourceKindObject:
		if err := resource.id.Validate(); err != nil {
			return fmt.Errorf("%w: object id: %w", ErrResourceInvalid, err)
		}
		if !resource.owner.isZero() {
			if err := resource.owner.Validate(); err != nil {
				return fmt.Errorf("%w: owner: %w", ErrResourceInvalid, err)
			}
		}
	}
	return nil
}

// Kind returns collection or object.
func (resource Resource) Kind() ResourceKind { return resource.kind }

// Type returns the protected business resource type.
func (resource Resource) Type() ResourceType { return resource.typeName }

// ID returns an exact object identifier and true only for object resources.
func (resource Resource) ID() (ResourceID, bool) {
	return resource.id, resource.kind == ResourceKindObject
}

// TenantID returns the server-supplied tenant fact when present.
func (resource Resource) TenantID() (TenantID, bool) {
	return resource.tenantID, resource.tenantID != ""
}

// Owner returns the server-supplied owner fact when present.
func (resource Resource) Owner() (Principal, bool) {
	return resource.owner, !resource.owner.isZero()
}
