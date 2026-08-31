package domain

import "errors"

var (
	// Value-object failures.
	ErrIdentifierInvalid           = errors.New("governance identifier invalid")
	ErrPrincipalInvalid            = errors.New("governance principal invalid")
	ErrPrincipalKindUnsupported    = errors.New("governance principal kind unsupported")
	ErrResourceInvalid             = errors.New("governance resource invalid")
	ErrResourceTypeUnsupported     = errors.New("governance resource type unsupported")
	ErrActionUnsupported           = errors.New("governance action unsupported")
	ErrCapabilityUnsupported       = errors.New("governance capability unsupported")
	ErrScopeInvalid                = errors.New("governance scope invalid")
	ErrScopeKindUnsupported        = errors.New("governance scope kind unsupported")
	ErrPermissionInvalid           = errors.New("governance permission invalid")
	ErrPermissionEffectUnsupported = errors.New("governance permission effect unsupported")
	ErrRoleInvalid                 = errors.New("governance role invalid")
	ErrRoleUnsupported             = errors.New("governance role unsupported")
	ErrRolePermissionDuplicate     = errors.New("governance role permission duplicate")
	ErrRolePermissionLimit         = errors.New("governance role permission limit exceeded")
	ErrAuditContextInvalid         = errors.New("governance audit context invalid")

	// Policy snapshot failures.
	ErrRoleBindingInvalid       = errors.New("governance role binding invalid")
	ErrPolicyInvalid            = errors.New("governance policy invalid")
	ErrPolicyIdentityInvalid    = errors.New("governance policy identity invalid")
	ErrPolicyRevisionInvalid    = errors.New("governance policy revision invalid")
	ErrPolicyRoleDuplicate      = errors.New("governance policy role duplicate")
	ErrPolicyBindingDuplicate   = errors.New("governance policy binding duplicate")
	ErrPolicyBindingConflict    = errors.New("governance policy binding conflict")
	ErrPolicyBindingRoleMissing = errors.New("governance policy binding role missing")
	ErrPolicyRoleLimit          = errors.New("governance policy role limit exceeded")
	ErrPolicyBindingLimit       = errors.New("governance policy binding limit exceeded")

	// Evaluation failures. A failure in this group always accompanies a zero
	// Decision; a confirmed deny is not an error.
	ErrAuthorizationRequestInvalid    = errors.New("governance authorization request invalid")
	ErrAuthorizationEvaluationInvalid = errors.New("governance authorization evaluation invalid")
	ErrDecisionInvalid                = errors.New("governance decision invalid")
)
