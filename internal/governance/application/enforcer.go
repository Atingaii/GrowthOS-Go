package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Atingaii/GrowthOS-Go/internal/governance/domain"
)

var (
	// ErrAuthorizationInvalidArgument means a caller supplied an invalid
	// context, Principal, Resource, Action, or correlation reference. It is a
	// programming/adapter contract failure, not a confirmed policy denial.
	ErrAuthorizationInvalidArgument = errors.New("governance authorization: invalid argument")
	// ErrAuthorizationNotConfigured means one of the required application ports
	// was absent. Production composition must reject this before serving traffic.
	ErrAuthorizationNotConfigured = errors.New("governance authorization: not configured")
	// ErrAuthorizationDenied is returned only for a complete, confirmed domain
	// deny. Callers must not expose the embedded Decision reason to clients.
	ErrAuthorizationDenied = errors.New("governance authorization: denied")
	// ErrAuthorizationUnavailable means no trustworthy allow/deny result can be
	// used, or an allow could not be durably audited. It must fail closed.
	ErrAuthorizationUnavailable = errors.New("governance authorization: unavailable")
	// ErrDecisionAuditUnavailable classifies an audit append failure. For allow
	// it is also authorization unavailable; for deny the deny remains binding.
	ErrDecisionAuditUnavailable = errors.New("governance authorization: decision audit unavailable")
)

// ActivePolicyReader returns one complete immutable active Policy snapshot.
// Implementations must not assemble rows from multiple revisions or fall back
// to a stale revision when the active one is missing or corrupt.
type ActivePolicyReader interface {
	LoadActivePolicy(context.Context) (domain.Policy, error)
}

// DecisionAuditSink durably appends one confirmed Decision and its bounded
// evidence. An implementation must report COMMIT outcome unknown as an error.
type DecisionAuditSink interface {
	AppendDecision(context.Context, domain.Decision) error
}

// Clock provides the single server-owned evaluation instant.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function to Clock.
type ClockFunc func() time.Time

// Now returns the function result, or a zero instant for a nil function.
func (clock ClockFunc) Now() time.Time {
	if clock == nil {
		return time.Time{}
	}
	return clock()
}

// EvaluationReferenceSource creates a fresh, non-secret correlation identity
// for every authorization attempt that reaches policy evaluation.
type EvaluationReferenceSource interface {
	NewEvaluationReference() (domain.AuditReference, error)
}

// EvaluationReferenceSourceFunc adapts a function to a reference source.
type EvaluationReferenceSourceFunc func() (domain.AuditReference, error)

// NewEvaluationReference returns a fresh reference, or an error for a nil
// function. The Enforcer revalidates the returned domain value.
func (source EvaluationReferenceSourceFunc) NewEvaluationReference() (domain.AuditReference, error) {
	if source == nil {
		return "", ErrAuthorizationNotConfigured
	}
	return source()
}

// Dependencies are the four required authority/correlation ports.
type Dependencies struct {
	Policies   ActivePolicyReader
	Audit      DecisionAuditSink
	Clock      Clock
	References EvaluationReferenceSource
}

// Enforcer turns trusted facts into a recorded allow or a closed failure. A
// successful return is intentionally not a reusable bearer capability.
type Enforcer struct {
	policies   ActivePolicyReader
	audit      DecisionAuditSink
	clock      Clock
	references EvaluationReferenceSource
}

// NewEnforcer rejects nil and typed-nil dependencies during composition.
func NewEnforcer(dependencies Dependencies) (*Enforcer, error) {
	enforcer := &Enforcer{
		policies:   dependencies.Policies,
		audit:      dependencies.Audit,
		clock:      dependencies.Clock,
		references: dependencies.References,
	}
	if err := enforcer.Validate(); err != nil {
		return nil, err
	}
	return enforcer, nil
}

// Validate reports whether all required dependencies remain present.
func (enforcer *Enforcer) Validate() error {
	if enforcer == nil || dependencyIsNil(enforcer.policies) || dependencyIsNil(enforcer.audit) ||
		dependencyIsNil(enforcer.clock) || dependencyIsNil(enforcer.references) {
		return ErrAuthorizationNotConfigured
	}
	return nil
}

// Require evaluates and records one exact authorization question. It returns
// nil only for a confirmed allow whose audit append completed successfully.
// A confirmed deny remains ErrAuthorizationDenied even if its audit append
// fails; callers can additionally detect ErrDecisionAuditUnavailable for an
// emergency operational signal without weakening the deny.
func (enforcer *Enforcer) Require(
	ctx context.Context,
	principal domain.Principal,
	resource domain.Resource,
	action domain.Action,
	correlationReference domain.AuditReference,
) error {
	if ctx == nil {
		return ErrAuthorizationInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateQuestion(principal, resource, action, correlationReference); err != nil {
		return errors.Join(ErrAuthorizationInvalidArgument, err)
	}
	if err := enforcer.Validate(); err != nil {
		return err
	}

	policy, err := enforcer.policies.LoadActivePolicy(ctx)
	if contextError := ctx.Err(); contextError != nil {
		return contextError
	}
	if err != nil {
		return unavailable("load active policy", err)
	}
	if err := policy.Validate(); err != nil {
		return unavailable("validate active policy", err)
	}

	evaluationReference, err := enforcer.references.NewEvaluationReference()
	if err != nil {
		return unavailable("create evaluation reference", err)
	}
	auditContext, err := domain.NewAuditContext(
		evaluationReference,
		correlationReference,
		enforcer.clock.Now(),
	)
	if err != nil {
		return unavailable("construct audit context", err)
	}
	request, err := domain.NewAuthorizationRequest(principal, resource, action, auditContext)
	if err != nil {
		return unavailable("construct authorization request", err)
	}
	decision, err := policy.Evaluate(request)
	if contextError := ctx.Err(); contextError != nil {
		return contextError
	}
	if err != nil || !decision.Confirmed() {
		if err == nil {
			err = errors.New("policy evaluator returned an unconfirmed decision")
		}
		return unavailable("evaluate policy", err)
	}

	auditErr := enforcer.audit.AppendDecision(ctx, decision)
	if contextError := ctx.Err(); contextError != nil {
		return contextError
	}
	if decision.Allowed() {
		if auditErr != nil {
			return errors.Join(
				ErrAuthorizationUnavailable,
				ErrDecisionAuditUnavailable,
				fmt.Errorf("append allow decision: %w", auditErr),
			)
		}
		return nil
	}

	if auditErr != nil {
		return errors.Join(
			ErrAuthorizationDenied,
			ErrDecisionAuditUnavailable,
			fmt.Errorf("append deny decision: %w", auditErr),
		)
	}
	return ErrAuthorizationDenied
}

func validateQuestion(
	principal domain.Principal,
	resource domain.Resource,
	action domain.Action,
	correlationReference domain.AuditReference,
) error {
	if err := principal.Validate(); err != nil {
		return fmt.Errorf("principal: %w", err)
	}
	if err := resource.Validate(); err != nil {
		return fmt.Errorf("resource: %w", err)
	}
	if err := domain.ValidateCapability(resource.Kind(), resource.Type(), action); err != nil {
		return fmt.Errorf("capability: %w", err)
	}
	if err := correlationReference.Validate(); err != nil {
		return fmt.Errorf("correlation reference: %w", err)
	}
	return nil
}

func unavailable(phase string, cause error) error {
	if cause == nil {
		cause = errors.New("unknown authorization dependency failure")
	}
	return errors.Join(ErrAuthorizationUnavailable, fmt.Errorf("%s: %w", phase, cause))
}

func dependencyIsNil(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return value.IsNil()
	default:
		return false
	}
}
