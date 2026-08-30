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
	// ErrRiskScreeningFactInvalid means the authoritative risk snapshot is
	// incomplete or internally inconsistent. It is not a blocked decision.
	ErrRiskScreeningFactInvalid = errors.New("participation: risk screening fact is invalid")
	// ErrRiskAdmissionPolicyInvalid means the concrete risk-admission policy is
	// incomplete and cannot be evaluated safely.
	ErrRiskAdmissionPolicyInvalid = errors.New("participation: risk admission policy is invalid")
	// ErrRiskAdmissionEvaluationInvalid means the evaluator did not receive a
	// complete policy, fact snapshot, or controlled evaluation instant.
	ErrRiskAdmissionEvaluationInvalid = errors.New("participation: risk admission evaluation is invalid")
	// ErrRiskScreeningFactFromFuture means a source claims it formed a risk
	// screening disposition after the controlled evaluation instant.
	ErrRiskScreeningFactFromFuture = errors.New("participation: risk screening fact is from the future")
	// ErrRuleSetRevisionInvalid means a linear prerequisite plan lacks a stable,
	// bounded revision identity. It is distinct from each rule policy revision.
	ErrRuleSetRevisionInvalid = errors.New("participation: rule-set revision is invalid")
)
