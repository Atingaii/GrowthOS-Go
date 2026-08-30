package domain

import (
	"fmt"
	"time"
)

const maxPolicyRevisionBytes = 256

// RuleCode is a stable machine identifier for one concrete Participation rule.
// It is independent of policy, fact, schema, Strategy, and application versions.
type RuleCode string

// PolicyRevision identifies the immutable policy snapshot used for a decision.
type PolicyRevision string

// RuleSetRevision identifies one immutable ordered composition of concrete
// Participation prerequisites. It is independent of every policy revision,
// fact revision, configuration schema, and application build.
type RuleSetRevision string

// NewRuleSetRevision constructs a stable revision for one ordered prerequisite
// plan. Lesson 26 keeps the plan in code; this value does not imply a persisted
// or dynamically configurable ruleset.
func NewRuleSetRevision(revision string) (RuleSetRevision, error) {
	ruleSetRevision := RuleSetRevision(revision)
	if err := ruleSetRevision.Validate(); err != nil {
		return "", err
	}
	return ruleSetRevision, nil
}

// Validate rejects a zero, non-canonical, non-printing, or oversized revision.
func (revision RuleSetRevision) Validate() error {
	if err := validateMetadataToken(string(revision), maxPolicyRevisionBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrRuleSetRevisionInvalid, err)
	}
	return nil
}

// String returns the opaque revision token without changing its semantics.
func (revision RuleSetRevision) String() string { return string(revision) }

// NewUserRuleCode is intentionally concrete. Lesson 25 does not introduce a
// generic Rule interface for its only implemented eligibility rule.
const NewUserRuleCode RuleCode = "participation.new_user.registered_on_or_after"

// NewUserPolicy defines the inclusive registration lower bound for the first
// concrete eligibility requirement.
type NewUserPolicy struct {
	revision            PolicyRevision
	registeredAtOrAfter time.Time
}

// NewNewUserPolicy constructs a versioned inclusive registration-cutoff policy.
func NewNewUserPolicy(
	revision string,
	registeredAtOrAfter time.Time,
) (NewUserPolicy, error) {
	policy := NewUserPolicy{
		revision:            PolicyRevision(revision),
		registeredAtOrAfter: canonicalInstant(registeredAtOrAfter),
	}
	if err := policy.Validate(); err != nil {
		return NewUserPolicy{}, err
	}
	return policy, nil
}

// Validate rejects zero-value or corrupt policies at any adapter boundary.
func (policy NewUserPolicy) Validate() error {
	if err := validateMetadataToken(string(policy.revision), maxPolicyRevisionBytes); err != nil {
		return fmt.Errorf("%w: revision %v", ErrNewUserPolicyInvalid, err)
	}
	if policy.registeredAtOrAfter.IsZero() {
		return fmt.Errorf("%w: registration cutoff is required", ErrNewUserPolicyInvalid)
	}
	return nil
}

// Revision returns the policy snapshot revision.
func (policy NewUserPolicy) Revision() PolicyRevision { return policy.revision }

// RegisteredAtOrAfter returns the inclusive canonical UTC cutoff.
func (policy NewUserPolicy) RegisteredAtOrAfter() time.Time {
	return policy.registeredAtOrAfter
}
