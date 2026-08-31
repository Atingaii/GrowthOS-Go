package domain

import (
	"errors"
	"testing"
)

func TestCapabilityCatalogAcceptsOnlyRegisteredPairs(t *testing.T) {
	t.Parallel()

	valid := []struct {
		resourceKind ResourceKind
		resourceType ResourceType
		action       Action
	}{
		{ResourceKindCollection, ResourceTypeMarketingActivity, ActionCreate},
		{ResourceKindCollection, ResourceTypeMarketingActivity, ActionRead},
		{ResourceKindObject, ResourceTypeMarketingActivity, ActionRead},
		{ResourceKindObject, ResourceTypeMarketingActivity, ActionPublish},
		{ResourceKindObject, ResourceTypeMarketingActivity, ActionRollback},
		{ResourceKindObject, ResourceTypeMarketingActivity, ActionRetire},
		{ResourceKindCollection, ResourceTypeLotteryStrategy, ActionCreate},
		{ResourceKindCollection, ResourceTypeLotteryStrategy, ActionRead},
		{ResourceKindObject, ResourceTypeLotteryStrategy, ActionRead},
		{ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionCreate},
		{ResourceKindCollection, ResourceTypeLotteryRoutingGraph, ActionRead},
		{ResourceKindObject, ResourceTypeLotteryRoutingGraph, ActionRead},
		{ResourceKindCollection, ResourceTypeGovernancePolicy, ActionRead},
		{ResourceKindObject, ResourceTypeGovernancePolicy, ActionRead},
		{ResourceKindObject, ResourceTypeGovernancePolicy, ActionChange},
		{ResourceKindCollection, ResourceTypeGovernanceAudit, ActionRead},
	}
	for _, capability := range valid {
		if err := ValidateCapability(
			capability.resourceKind,
			capability.resourceType,
			capability.action,
		); err != nil {
			t.Fatalf(
				"validate %s:%s:%s: %v",
				capability.resourceKind,
				capability.resourceType,
				capability.action,
				err,
			)
		}
	}

	invalid := []struct {
		resourceKind   ResourceKind
		resourceType   ResourceType
		action         Action
		classification error
	}{
		{"", ResourceTypeMarketingActivity, ActionRead, ErrCapabilityUnsupported},
		{ResourceKindObject, "", ActionRead, ErrResourceTypeUnsupported},
		{ResourceKindObject, "marketing.*", ActionRead, ErrResourceTypeUnsupported},
		{ResourceKindObject, ResourceTypeMarketingActivity, "*", ErrActionUnsupported},
		{ResourceKindCollection, ResourceTypeMarketingActivity, ActionPublish, ErrCapabilityUnsupported},
		{ResourceKindObject, ResourceTypeMarketingActivity, ActionCreate, ErrCapabilityUnsupported},
		{ResourceKindObject, ResourceTypeLotteryStrategy, ActionPublish, ErrCapabilityUnsupported},
		{ResourceKindObject, ResourceTypeGovernanceAudit, ActionRead, ErrCapabilityUnsupported},
		{ResourceKindCollection, ResourceTypeGovernancePolicy, ActionChange, ErrCapabilityUnsupported},
	}
	for _, capability := range invalid {
		err := ValidateCapability(
			capability.resourceKind,
			capability.resourceType,
			capability.action,
		)
		if !errors.Is(err, ErrCapabilityUnsupported) ||
			!errors.Is(err, capability.classification) {
			t.Fatalf(
				"validate invalid %s:%s:%s: got %v, want capability and %v",
				capability.resourceKind,
				capability.resourceType,
				capability.action,
				err,
				capability.classification,
			)
		}
	}
}

func TestClosedCapabilityEnums(t *testing.T) {
	t.Parallel()

	for _, resourceType := range []ResourceType{
		ResourceTypeMarketingActivity,
		ResourceTypeLotteryStrategy,
		ResourceTypeLotteryRoutingGraph,
		ResourceTypeGovernancePolicy,
		ResourceTypeGovernanceAudit,
	} {
		if !resourceType.Valid() {
			t.Fatalf("known resource type %q is invalid", resourceType)
		}
	}
	if ResourceType("admin").Valid() || ResourceType("*").Valid() || ResourceType("").Valid() {
		t.Fatal("unknown resource type became valid")
	}

	for _, action := range []Action{
		ActionCreate,
		ActionRead,
		ActionPublish,
		ActionRollback,
		ActionRetire,
		ActionChange,
	} {
		if !action.Valid() {
			t.Fatalf("known action %q is invalid", action)
		}
	}
	if Action("admin").Valid() || Action("*").Valid() || Action("").Valid() {
		t.Fatal("unknown action became valid")
	}
}
