package domain

import "errors"

var (
	// ErrParticipantRefRequired means no subject lookup reference was supplied.
	// A valid reference is still not proof of authentication or authorization.
	ErrParticipantRefRequired = errors.New("participation: participant reference is required")
	// ErrRegistrationFactInvalid means the authoritative fact snapshot is
	// incomplete or internally inconsistent. It is not an ineligible decision.
	ErrRegistrationFactInvalid = errors.New("participation: registration fact is invalid")
	// ErrNewUserPolicyInvalid means the concrete registration-cutoff policy is
	// incomplete or cannot be evaluated safely.
	ErrNewUserPolicyInvalid = errors.New("participation: new-user policy is invalid")
	// ErrEligibilityEvaluationInvalid means the evaluator did not receive a
	// complete policy, fact snapshot, or controlled evaluation instant.
	ErrEligibilityEvaluationInvalid = errors.New("participation: eligibility evaluation is invalid")
	// ErrEligibilityFactFromFuture means a source claimed a registration or
	// observation instant after the controlled server evaluation instant.
	ErrEligibilityFactFromFuture = errors.New("participation: registration fact is from the future")
)
