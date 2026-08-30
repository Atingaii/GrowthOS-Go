package domain

import "errors"

var (
	// ErrMembershipSubjectRefRequired means no Lottery-side membership lookup
	// reference was supplied. A valid value still proves no caller identity.
	ErrMembershipSubjectRefRequired = errors.New("lottery: membership subject reference is required")
	// ErrMembershipTierFactInvalid means the authority snapshot is structurally
	// incomplete or outside the accepted v1 tier vocabulary.
	ErrMembershipTierFactInvalid = errors.New("lottery: membership tier fact is invalid")
	// ErrMembershipRoutingPolicyInvalid means the concrete code-owned policy is
	// missing a safe revision or a required Strategy target.
	ErrMembershipRoutingPolicyInvalid = errors.New("lottery: membership routing policy is invalid")
	// ErrMembershipRoutingEvaluationInvalid means no trustworthy route can be
	// formed from the supplied policy, fact, or logical evaluation instant.
	ErrMembershipRoutingEvaluationInvalid = errors.New("lottery: membership routing evaluation is invalid")
	// ErrMembershipTierFactFromFuture means the source snapshot was formed after
	// the controlled server evaluation instant.
	ErrMembershipTierFactFromFuture = errors.New("lottery: membership tier fact is from the future")
)
