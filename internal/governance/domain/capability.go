package domain

import "fmt"

// ResourceType names a protected business resource kind rather than a URL,
// table, frontend workspace, or infrastructure account.
type ResourceType string

const (
	ResourceTypeMarketingActivity   ResourceType = "marketing.activity"
	ResourceTypeLotteryStrategy     ResourceType = "lottery.strategy"
	ResourceTypeLotteryRoutingGraph ResourceType = "lottery.routing_graph"
	ResourceTypeGovernancePolicy    ResourceType = "governance.policy"
	ResourceTypeGovernanceAudit     ResourceType = "governance.audit"
)

// Valid reports whether resourceType belongs to the reviewed v1 catalog.
func (resourceType ResourceType) Valid() bool {
	switch resourceType {
	case ResourceTypeMarketingActivity,
		ResourceTypeLotteryStrategy,
		ResourceTypeLotteryRoutingGraph,
		ResourceTypeGovernancePolicy,
		ResourceTypeGovernanceAudit:
		return true
	default:
		return false
	}
}

// Action is a precise business verb. It is not inferred from an HTTP method.
type Action string

const (
	ActionCreate   Action = "create"
	ActionRead     Action = "read"
	ActionPublish  Action = "publish"
	ActionRollback Action = "rollback"
	ActionRetire   Action = "retire"
	ActionChange   Action = "change"
)

// Valid reports whether action belongs to the reviewed v1 vocabulary.
func (action Action) Valid() bool {
	switch action {
	case ActionCreate, ActionRead, ActionPublish, ActionRollback, ActionRetire, ActionChange:
		return true
	default:
		return false
	}
}

// ValidateCapability rejects kind/type/action tuples that are individually
// known but not meaningful together. The catalog has no wildcard or prefix
// matching.
func ValidateCapability(resourceKind ResourceKind, resourceType ResourceType, action Action) error {
	if !resourceKind.Valid() {
		return fmt.Errorf(
			"%w: resource kind %q",
			ErrCapabilityUnsupported,
			resourceKind,
		)
	}
	if !resourceType.Valid() {
		return fmt.Errorf(
			"%w: %w: resource type %q",
			ErrCapabilityUnsupported,
			ErrResourceTypeUnsupported,
			resourceType,
		)
	}
	if !action.Valid() {
		return fmt.Errorf(
			"%w: %w: action %q",
			ErrCapabilityUnsupported,
			ErrActionUnsupported,
			action,
		)
	}

	valid := false
	switch resourceType {
	case ResourceTypeMarketingActivity:
		valid = resourceKind == ResourceKindCollection &&
			(action == ActionCreate || action == ActionRead) ||
			resourceKind == ResourceKindObject &&
				(action == ActionRead || action == ActionPublish ||
					action == ActionRollback || action == ActionRetire)
	case ResourceTypeLotteryStrategy, ResourceTypeLotteryRoutingGraph:
		valid = resourceKind == ResourceKindCollection &&
			(action == ActionCreate || action == ActionRead) ||
			resourceKind == ResourceKindObject && action == ActionRead
	case ResourceTypeGovernancePolicy:
		valid = resourceKind == ResourceKindCollection && action == ActionRead ||
			resourceKind == ResourceKindObject &&
				(action == ActionRead || action == ActionChange)
	case ResourceTypeGovernanceAudit:
		valid = resourceKind == ResourceKindCollection && action == ActionRead
	}
	if !valid {
		return fmt.Errorf(
			"%w: %s:%s:%s is not registered",
			ErrCapabilityUnsupported,
			resourceKind,
			resourceType,
			action,
		)
	}
	return nil
}
