package domain

import (
	"cmp"
	"fmt"
	"slices"
)

const (
	// MaxDecisionMatches equals the policy binding bound because one canonical
	// role can contribute at most one exact permission per binding and request.
	MaxDecisionMatches = MaxRoleBindingsPerPolicy
)

// AuthorizationRequest is the complete pure-domain input. Construction checks
// shape and capability registration, not whether its facts came from a trusted
// session or repository; those boundaries are implemented in later lessons.
type AuthorizationRequest struct {
	principal    Principal
	resource     Resource
	action       Action
	auditContext AuditContext
}

// NewAuthorizationRequest constructs one exact access question.
func NewAuthorizationRequest(
	principal Principal,
	resource Resource,
	action Action,
	auditContext AuditContext,
) (AuthorizationRequest, error) {
	request := AuthorizationRequest{
		principal:    principal,
		resource:     resource,
		action:       action,
		auditContext: auditContext,
	}
	if err := request.Validate(); err != nil {
		return AuthorizationRequest{}, err
	}
	return request, nil
}

// Validate rejects zero, partial, unknown, and kind/type/action-inconsistent
// requests before any policy result can be formed.
func (request AuthorizationRequest) Validate() error {
	if err := request.principal.Validate(); err != nil {
		return fmt.Errorf("%w: principal: %w", ErrAuthorizationRequestInvalid, err)
	}
	if err := request.resource.Validate(); err != nil {
		return fmt.Errorf("%w: resource: %w", ErrAuthorizationRequestInvalid, err)
	}
	if err := ValidateCapability(
		request.resource.kind,
		request.resource.typeName,
		request.action,
	); err != nil {
		return fmt.Errorf("%w: capability: %w", ErrAuthorizationRequestInvalid, err)
	}
	if err := request.auditContext.Validate(); err != nil {
		return fmt.Errorf("%w: audit context: %w", ErrAuthorizationRequestInvalid, err)
	}
	return nil
}

// Principal returns the exact authorization subject reference.
func (request AuthorizationRequest) Principal() Principal { return request.principal }

// Resource returns the server-fact target descriptor.
func (request AuthorizationRequest) Resource() Resource { return request.resource }

// Action returns the exact business verb.
func (request AuthorizationRequest) Action() Action { return request.action }

// AuditContext returns minimal evaluation correlation.
func (request AuthorizationRequest) AuditContext() AuditContext { return request.auditContext }

// DecisionOutcome is the closed confirmed result. Technical inability to form a
// result is represented by an error and a zero Decision instead.
type DecisionOutcome string

const (
	DecisionOutcomeAllow DecisionOutcome = "allow"
	DecisionOutcomeDeny  DecisionOutcome = "deny"
)

// Valid reports whether outcome belongs to the confirmed v1 vocabulary.
func (outcome DecisionOutcome) Valid() bool {
	return outcome == DecisionOutcomeAllow || outcome == DecisionOutcomeDeny
}

// DecisionReason is an internal, low-cardinality explanation. It is not a
// client error code and may disclose policy shape if exposed directly.
type DecisionReason string

const (
	DecisionReasonExplicitAllow             DecisionReason = "explicit_allow"
	DecisionReasonExplicitDeny              DecisionReason = "explicit_deny"
	DecisionReasonExplicitDenyOverrodeAllow DecisionReason = "explicit_deny_overrode_allow"
	DecisionReasonNoBinding                 DecisionReason = "no_binding"
	DecisionReasonNoPermission              DecisionReason = "no_permission"
	DecisionReasonScopeMismatch             DecisionReason = "scope_mismatch"
)

// Valid reports whether reason belongs to the v1 internal vocabulary.
func (reason DecisionReason) Valid() bool {
	switch reason {
	case DecisionReasonExplicitAllow,
		DecisionReasonExplicitDeny,
		DecisionReasonExplicitDenyOverrodeAllow,
		DecisionReasonNoBinding,
		DecisionReasonNoPermission,
		DecisionReasonScopeMismatch:
		return true
	default:
		return false
	}
}

// DecisionMatch identifies one binding and exact role capability that matched
// the request. It intentionally omits credentials, arbitrary metadata, policy
// payloads, resource content, and user-facing text.
type DecisionMatch struct {
	bindingID  RoleBindingID
	roleID     RoleID
	effect     BindingEffect
	scopeKind  ScopeKind
	permission Permission
}

// Validate rechecks a bounded evidence item.
func (match DecisionMatch) Validate() error {
	if err := match.bindingID.Validate(); err != nil {
		return fmt.Errorf("%w: binding id: %w", ErrDecisionInvalid, err)
	}
	if !match.roleID.Valid() {
		return fmt.Errorf("%w: role %q", ErrDecisionInvalid, match.roleID)
	}
	if !match.effect.Valid() {
		return fmt.Errorf("%w: effect %q", ErrDecisionInvalid, match.effect)
	}
	if !match.scopeKind.Valid() {
		return fmt.Errorf("%w: scope kind %q", ErrDecisionInvalid, match.scopeKind)
	}
	if err := match.permission.Validate(); err != nil {
		return fmt.Errorf("%w: permission: %w", ErrDecisionInvalid, err)
	}
	return nil
}

// BindingID returns the exact matched role-binding identity.
func (match DecisionMatch) BindingID() RoleBindingID { return match.bindingID }

// RoleID returns the exact matched role identifier.
func (match DecisionMatch) RoleID() RoleID { return match.roleID }

// Effect returns allow or deny.
func (match DecisionMatch) Effect() BindingEffect { return match.effect }

// ScopeKind returns the kind of scope that matched the resource facts.
func (match DecisionMatch) ScopeKind() ScopeKind { return match.scopeKind }

// Permission returns the exact matched capability.
func (match DecisionMatch) Permission() Permission { return match.permission }

// Decision is one immutable confirmed policy result with exact revision and
// request evidence. It is not a bearer token or durable audit event.
type Decision struct {
	outcome        DecisionOutcome
	reason         DecisionReason
	policyIdentity PolicyIdentity
	principal      Principal
	resource       Resource
	action         Action
	auditContext   AuditContext
	matches        []DecisionMatch
}

// Validate rejects zero, partial, contradictory, non-canonical, and excessive
// evidence. A valid default deny intentionally has no matches.
func (decision Decision) Validate() error {
	if !decision.outcome.Valid() {
		return fmt.Errorf("%w: outcome %q", ErrDecisionInvalid, decision.outcome)
	}
	if !decision.reason.Valid() {
		return fmt.Errorf("%w: reason %q", ErrDecisionInvalid, decision.reason)
	}
	if err := decision.policyIdentity.Validate(); err != nil {
		return fmt.Errorf("%w: policy identity: %w", ErrDecisionInvalid, err)
	}
	request := AuthorizationRequest{
		principal:    decision.principal,
		resource:     decision.resource,
		action:       decision.action,
		auditContext: decision.auditContext,
	}
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: request evidence: %w", ErrDecisionInvalid, err)
	}
	if len(decision.matches) > MaxDecisionMatches {
		return fmt.Errorf(
			"%w: match count %d exceeds %d",
			ErrDecisionInvalid,
			len(decision.matches),
			MaxDecisionMatches,
		)
	}
	if !slices.IsSortedFunc(decision.matches, compareDecisionMatch) {
		return fmt.Errorf("%w: matches are not canonical", ErrDecisionInvalid)
	}

	allowCount := 0
	denyCount := 0
	seenBindings := make(map[RoleBindingID]struct{}, len(decision.matches))
	for index, match := range decision.matches {
		if err := match.Validate(); err != nil {
			return fmt.Errorf("%w: match %d: %w", ErrDecisionInvalid, index, err)
		}
		if _, exists := seenBindings[match.bindingID]; exists {
			return fmt.Errorf("%w: duplicate match binding %q", ErrDecisionInvalid, match.bindingID)
		}
		seenBindings[match.bindingID] = struct{}{}
		if !match.permission.matches(decision.resource, decision.action) {
			return fmt.Errorf("%w: match %q capability differs from request", ErrDecisionInvalid, match.bindingID)
		}
		switch match.effect {
		case BindingEffectAllow:
			allowCount++
		case BindingEffectDeny:
			denyCount++
		}
	}

	switch decision.reason {
	case DecisionReasonExplicitAllow:
		if decision.outcome != DecisionOutcomeAllow || allowCount == 0 || denyCount != 0 {
			return fmt.Errorf("%w: explicit allow evidence is contradictory", ErrDecisionInvalid)
		}
	case DecisionReasonExplicitDeny:
		if decision.outcome != DecisionOutcomeDeny || denyCount == 0 || allowCount != 0 {
			return fmt.Errorf("%w: explicit deny evidence is contradictory", ErrDecisionInvalid)
		}
	case DecisionReasonExplicitDenyOverrodeAllow:
		if decision.outcome != DecisionOutcomeDeny || denyCount == 0 || allowCount == 0 {
			return fmt.Errorf("%w: deny-overrode-allow evidence is contradictory", ErrDecisionInvalid)
		}
	case DecisionReasonNoBinding,
		DecisionReasonNoPermission,
		DecisionReasonScopeMismatch:
		if decision.outcome != DecisionOutcomeDeny || len(decision.matches) != 0 {
			return fmt.Errorf("%w: default deny evidence is contradictory", ErrDecisionInvalid)
		}
	}
	return nil
}

// Confirmed reports whether the value is a complete allow/deny decision.
func (decision Decision) Confirmed() bool { return decision.Validate() == nil }

// Allowed reports true only for a confirmed allow outcome.
func (decision Decision) Allowed() bool {
	return decision.outcome == DecisionOutcomeAllow && decision.Validate() == nil
}

// Outcome returns allow or deny for a confirmed decision.
func (decision Decision) Outcome() DecisionOutcome { return decision.outcome }

// Reason returns the internal low-cardinality explanation.
func (decision Decision) Reason() DecisionReason { return decision.reason }

// PolicyIdentity returns the exact evaluated policy snapshot.
func (decision Decision) PolicyIdentity() PolicyIdentity { return decision.policyIdentity }

// Principal returns the exact evaluated subject.
func (decision Decision) Principal() Principal { return decision.principal }

// Resource returns the exact evaluated target facts.
func (decision Decision) Resource() Resource { return decision.resource }

// Action returns the exact evaluated business verb.
func (decision Decision) Action() Action { return decision.action }

// AuditContext returns the evaluation correlation and canonical instant.
func (decision Decision) AuditContext() AuditContext { return decision.auditContext }

// Matches returns a defensive copy in canonical BindingID order.
func (decision Decision) Matches() []DecisionMatch {
	return append([]DecisionMatch(nil), decision.matches...)
}

// Evaluate forms one deterministic access decision from an immutable policy
// snapshot and request. Any invalid state returns a strict zero Decision.
func (policy Policy) Evaluate(request AuthorizationRequest) (Decision, error) {
	if err := policy.Validate(); err != nil {
		return Decision{}, fmt.Errorf(
			"%w: policy: %w",
			ErrAuthorizationEvaluationInvalid,
			err,
		)
	}
	if err := request.Validate(); err != nil {
		return Decision{}, fmt.Errorf(
			"%w: request: %w",
			ErrAuthorizationEvaluationInvalid,
			err,
		)
	}

	hasBinding := false
	hasPermission := false
	matches := make([]DecisionMatch, 0)
	allowCount := 0
	denyCount := 0
	for _, binding := range policy.bindings {
		if binding.principal != request.principal {
			continue
		}
		hasBinding = true
		role, exists := policy.role(binding.roleID)
		if !exists {
			return Decision{}, fmt.Errorf(
				"%w: policy changed after validation: role %q missing",
				ErrAuthorizationEvaluationInvalid,
				binding.roleID,
			)
		}
		for _, permission := range role.permissions {
			if !permission.matches(request.resource, request.action) {
				continue
			}
			hasPermission = true
			if !binding.scope.matches(request.principal, request.resource) {
				continue
			}
			match := DecisionMatch{
				bindingID:  binding.id,
				roleID:     binding.roleID,
				effect:     binding.effect,
				scopeKind:  binding.scope.kind,
				permission: permission,
			}
			matches = append(matches, match)
			if binding.effect == BindingEffectDeny {
				denyCount++
			} else {
				allowCount++
			}
		}
	}
	if len(matches) > MaxDecisionMatches {
		return Decision{}, fmt.Errorf(
			"%w: match count %d exceeds %d",
			ErrAuthorizationEvaluationInvalid,
			len(matches),
			MaxDecisionMatches,
		)
	}
	slices.SortFunc(matches, compareDecisionMatch)

	outcome := DecisionOutcomeDeny
	reason := DecisionReasonNoBinding
	switch {
	case denyCount > 0 && allowCount > 0:
		reason = DecisionReasonExplicitDenyOverrodeAllow
	case denyCount > 0:
		reason = DecisionReasonExplicitDeny
	case allowCount > 0:
		outcome = DecisionOutcomeAllow
		reason = DecisionReasonExplicitAllow
	case !hasBinding:
		reason = DecisionReasonNoBinding
	case !hasPermission:
		reason = DecisionReasonNoPermission
	default:
		reason = DecisionReasonScopeMismatch
	}

	decision := Decision{
		outcome:        outcome,
		reason:         reason,
		policyIdentity: policy.identity,
		principal:      request.principal,
		resource:       request.resource,
		action:         request.action,
		auditContext:   request.auditContext,
		matches:        matches,
	}
	if err := decision.Validate(); err != nil {
		return Decision{}, fmt.Errorf(
			"%w: result: %w",
			ErrAuthorizationEvaluationInvalid,
			err,
		)
	}
	return decision, nil
}

func compareDecisionMatch(left, right DecisionMatch) int {
	if comparison := cmp.Compare(left.bindingID, right.bindingID); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(left.roleID, right.roleID); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(left.effect, right.effect); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(left.scopeKind, right.scopeKind); comparison != 0 {
		return comparison
	}
	return comparePermission(left.permission, right.permission)
}
