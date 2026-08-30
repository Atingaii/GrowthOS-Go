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
