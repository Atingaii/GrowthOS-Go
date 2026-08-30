package domain

import "fmt"

const maxMembershipRoutingPolicyRevisionBytes = 256

// MembershipRoutingPolicyRevision identifies one immutable Lottery routing
// policy. It is independent of fact, Strategy, schema, and application versions.
type MembershipRoutingPolicyRevision string

// MembershipStrategyRoutingPolicy is the first concrete membership route. A
// premium fact selects the override; a standard fact selects the required
// baseline default. Equal targets are permitted because branches remain useful
// evidence even while a rollout converges on one Strategy.
type MembershipStrategyRoutingPolicy struct {
	revision        MembershipRoutingPolicyRevision
	premiumTarget   StrategyID
	baselineDefault StrategyID
}

// NewMembershipStrategyRoutingPolicy constructs the code-owned v1 policy.
func NewMembershipStrategyRoutingPolicy(
	revision string,
	premiumTarget StrategyID,
	baselineDefault StrategyID,
) (MembershipStrategyRoutingPolicy, error) {
	policy := MembershipStrategyRoutingPolicy{
		revision:        MembershipRoutingPolicyRevision(revision),
		premiumTarget:   premiumTarget,
		baselineDefault: baselineDefault,
	}
	if err := policy.Validate(); err != nil {
		return MembershipStrategyRoutingPolicy{}, err
	}
	return policy, nil
}

// Validate rejects missing revisions or zero routing targets.
func (policy MembershipStrategyRoutingPolicy) Validate() error {
	if err := validateMembershipMetadataToken(
		string(policy.revision),
		maxMembershipRoutingPolicyRevisionBytes,
	); err != nil {
		return fmt.Errorf("%w: revision %v", ErrMembershipRoutingPolicyInvalid, err)
	}
	if policy.premiumTarget == 0 {
		return fmt.Errorf("%w: premium target is required", ErrMembershipRoutingPolicyInvalid)
	}
	if policy.baselineDefault == 0 {
		return fmt.Errorf("%w: baseline default is required", ErrMembershipRoutingPolicyInvalid)
	}
	return nil
}

// Revision returns the immutable policy revision.
func (policy MembershipStrategyRoutingPolicy) Revision() MembershipRoutingPolicyRevision {
	return policy.revision
}

// PremiumTarget returns the explicit premium Strategy target.
func (policy MembershipStrategyRoutingPolicy) PremiumTarget() StrategyID {
	return policy.premiumTarget
}

// BaselineDefault returns the required Strategy target for confirmed standard.
func (policy MembershipStrategyRoutingPolicy) BaselineDefault() StrategyID {
	return policy.baselineDefault
}
