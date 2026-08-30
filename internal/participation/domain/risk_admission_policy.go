package domain

import "fmt"

// RiskAdmissionRuleCode is the stable identity of the second concrete
// Participation prerequisite. It maps a risk screening fact to admission; it
// is not an authorization policy or a provider model identity.
const RiskAdmissionRuleCode RuleCode = "participation.risk.screening_admission"

// RiskAdmissionPolicy identifies the immutable Participation policy that
// requires an explicit passed screening disposition. The mapping is fixed in
// v1 rather than made dynamically configurable before a real need exists.
type RiskAdmissionPolicy struct {
	revision PolicyRevision
}

// NewRiskAdmissionPolicy constructs the concrete v1 admission policy.
func NewRiskAdmissionPolicy(revision string) (RiskAdmissionPolicy, error) {
	policy := RiskAdmissionPolicy{revision: PolicyRevision(revision)}
	if err := policy.Validate(); err != nil {
		return RiskAdmissionPolicy{}, err
	}
	return policy, nil
}

// Validate rejects a zero, unsafe, or oversized policy revision.
func (policy RiskAdmissionPolicy) Validate() error {
	if err := validateMetadataToken(string(policy.revision), maxPolicyRevisionBytes); err != nil {
		return fmt.Errorf("%w: revision %v", ErrRiskAdmissionPolicyInvalid, err)
	}
	return nil
}

// Revision returns the exact immutable policy snapshot revision.
func (policy RiskAdmissionPolicy) Revision() PolicyRevision { return policy.revision }
