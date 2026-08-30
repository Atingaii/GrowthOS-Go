package domain

import (
	"fmt"
	"time"
)

// MembershipRoutingRuleCode is the stable identity of the first concrete
// Lottery membership-to-Strategy routing rule.
type MembershipRoutingRuleCode string

// MembershipRoutingBranch is the selected outgoing edge of the concrete rule.
type MembershipRoutingBranch string

// MembershipRoutingReasonCode is a stable explanation independent of copy.
type MembershipRoutingReasonCode string

const (
	MembershipStrategyRoutingRuleCode MembershipRoutingRuleCode = "lottery.membership_tier.route_strategy"

	MembershipRoutingBranchPremiumOverride MembershipRoutingBranch = "premium_override"
	MembershipRoutingBranchBaselineDefault MembershipRoutingBranch = "baseline_default"

	MembershipRoutingReasonPremiumStrategy  MembershipRoutingReasonCode = "premium_strategy_selected"
	MembershipRoutingReasonBaselineStrategy MembershipRoutingReasonCode = "baseline_strategy_selected"
)

// MembershipRoutingPathStep records the one concrete branch actually taken.
// It deliberately contains no subject reference, raw source payload, or error.
type MembershipRoutingPathStep struct {
	ruleCode MembershipRoutingRuleCode
	branch   MembershipRoutingBranch
	target   StrategyID
}

// RuleCode returns the concrete decision node identity.
func (step MembershipRoutingPathStep) RuleCode() MembershipRoutingRuleCode {
	return step.ruleCode
}

// Branch returns the selected outgoing edge.
func (step MembershipRoutingPathStep) Branch() MembershipRoutingBranch {
	return step.branch
}

// Target returns the terminal Strategy identity selected by this hop.
func (step MembershipRoutingPathStep) Target() StrategyID { return step.target }

// MembershipStrategyRouteDecision is a confirmed, read-only routing result.
// It is neither eligibility, a loaded Strategy, an Award selection, nor a Draw.
type MembershipStrategyRouteDecision struct {
	target         StrategyID
	ruleCode       MembershipRoutingRuleCode
	branch         MembershipRoutingBranch
	reasonCode     MembershipRoutingReasonCode
	policyRevision MembershipRoutingPolicyRevision
	factSource     MembershipFactSource
	factRevision   MembershipFactRevision
	evaluatedAt    time.Time
	path           []MembershipRoutingPathStep
}

// RouteMembershipStrategy deterministically selects one Strategy identity from
// a validated policy and authoritative membership fact at one controlled time.
func RouteMembershipStrategy(
	policy MembershipStrategyRoutingPolicy,
	fact MembershipTierFactSnapshot,
	evaluatedAt time.Time,
) (MembershipStrategyRouteDecision, error) {
	if err := policy.Validate(); err != nil {
		return MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: %w",
			ErrMembershipRoutingEvaluationInvalid,
			err,
		)
	}
	if err := fact.Validate(); err != nil {
		return MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: %w",
			ErrMembershipRoutingEvaluationInvalid,
			err,
		)
	}
	evaluatedAt = canonicalMembershipInstant(evaluatedAt)
	if evaluatedAt.IsZero() {
		return MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: evaluated-at is required",
			ErrMembershipRoutingEvaluationInvalid,
		)
	}
	if fact.ObservedAt().After(evaluatedAt) {
		return MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: %w",
			ErrMembershipRoutingEvaluationInvalid,
			ErrMembershipTierFactFromFuture,
		)
	}

	var (
		branch MembershipRoutingBranch
		reason MembershipRoutingReasonCode
		target StrategyID
	)
	switch fact.Tier() {
	case MembershipTierStandard:
		branch = MembershipRoutingBranchBaselineDefault
		reason = MembershipRoutingReasonBaselineStrategy
		target = policy.BaselineDefault()
	case MembershipTierPremium:
		branch = MembershipRoutingBranchPremiumOverride
		reason = MembershipRoutingReasonPremiumStrategy
		target = policy.PremiumTarget()
	default:
		return MembershipStrategyRouteDecision{}, fmt.Errorf(
			"%w: membership tier has no explicit branch",
			ErrMembershipRoutingEvaluationInvalid,
		)
	}
	step := MembershipRoutingPathStep{
		ruleCode: MembershipStrategyRoutingRuleCode,
		branch:   branch,
		target:   target,
	}
	return MembershipStrategyRouteDecision{
		target:         target,
		ruleCode:       MembershipStrategyRoutingRuleCode,
		branch:         branch,
		reasonCode:     reason,
		policyRevision: policy.Revision(),
		factSource:     fact.Source(),
		factRevision:   fact.Revision(),
		evaluatedAt:    evaluatedAt,
		path:           []MembershipRoutingPathStep{step},
	}, nil
}

// Confirmed distinguishes a complete business route from a zero error result.
func (decision MembershipStrategyRouteDecision) Confirmed() bool {
	if decision.target == 0 {
		return false
	}
	if decision.ruleCode != MembershipStrategyRoutingRuleCode ||
		decision.policyRevision == "" ||
		decision.factSource == "" ||
		decision.factRevision == "" ||
		decision.evaluatedAt.IsZero() ||
		len(decision.path) != 1 {
		return false
	}
	switch decision.branch {
	case MembershipRoutingBranchPremiumOverride:
		if decision.reasonCode != MembershipRoutingReasonPremiumStrategy {
			return false
		}
	case MembershipRoutingBranchBaselineDefault:
		if decision.reasonCode != MembershipRoutingReasonBaselineStrategy {
			return false
		}
	default:
		return false
	}
	step := decision.path[0]
	return step.ruleCode == decision.ruleCode &&
		step.branch == decision.branch &&
		step.target == decision.target
}

// Target returns the selected Strategy identity without loading the aggregate.
func (decision MembershipStrategyRouteDecision) Target() StrategyID {
	return decision.target
}

// RuleCode returns the stable concrete routing rule identity.
func (decision MembershipStrategyRouteDecision) RuleCode() MembershipRoutingRuleCode {
	return decision.ruleCode
}

// Branch returns the selected premium override or baseline default edge.
func (decision MembershipStrategyRouteDecision) Branch() MembershipRoutingBranch {
	return decision.branch
}

// ReasonCode returns the stable routing explanation.
func (decision MembershipStrategyRouteDecision) ReasonCode() MembershipRoutingReasonCode {
	return decision.reasonCode
}

// PolicyRevision returns the exact routing policy snapshot revision.
func (decision MembershipStrategyRouteDecision) PolicyRevision() MembershipRoutingPolicyRevision {
	return decision.policyRevision
}

// FactSource returns the authority identifier without the source payload.
func (decision MembershipStrategyRouteDecision) FactSource() MembershipFactSource {
	return decision.factSource
}

// FactRevision returns the exact source snapshot revision.
func (decision MembershipStrategyRouteDecision) FactRevision() MembershipFactRevision {
	return decision.factRevision
}

// EvaluatedAt returns the canonical UTC logical evaluation instant.
func (decision MembershipStrategyRouteDecision) EvaluatedAt() time.Time {
	return decision.evaluatedAt
}

// Path returns a copy so callers cannot rewrite branch evidence.
func (decision MembershipStrategyRouteDecision) Path() []MembershipRoutingPathStep {
	return append([]MembershipRoutingPathStep(nil), decision.path...)
}
