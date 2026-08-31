package domain

import (
	"fmt"
	"time"
)

// AuditContext is the minimum immutable correlation carried through one pure
// policy decision. It does not authenticate a request and is not a persisted
// audit event, HTTP request identifier, trace context, or arbitrary metadata.
type AuditContext struct {
	evaluationReference  AuditReference
	correlationReference AuditReference
	evaluatedAt          time.Time
}

// NewAuditContext canonicalizes evaluatedAt to UTC microsecond precision and
// validates both bounded opaque references.
func NewAuditContext(
	evaluationReference AuditReference,
	correlationReference AuditReference,
	evaluatedAt time.Time,
) (AuditContext, error) {
	context := AuditContext{
		evaluationReference:  evaluationReference,
		correlationReference: correlationReference,
		evaluatedAt:          canonicalGovernanceInstant(evaluatedAt),
	}
	if err := context.Validate(); err != nil {
		return AuditContext{}, err
	}
	return context, nil
}

// Validate rejects zero, partial, non-canonical, and forged correlation state.
func (context AuditContext) Validate() error {
	if err := context.evaluationReference.Validate(); err != nil {
		return fmt.Errorf("%w: evaluation reference: %w", ErrAuditContextInvalid, err)
	}
	if err := context.correlationReference.Validate(); err != nil {
		return fmt.Errorf("%w: correlation reference: %w", ErrAuditContextInvalid, err)
	}
	if context.evaluatedAt.IsZero() || context.evaluatedAt != canonicalGovernanceInstant(context.evaluatedAt) {
		return fmt.Errorf(
			"%w: evaluated-at must be canonical UTC microsecond precision",
			ErrAuditContextInvalid,
		)
	}
	return nil
}

// EvaluationReference returns the opaque identity of this policy evaluation.
func (context AuditContext) EvaluationReference() AuditReference {
	return context.evaluationReference
}

// CorrelationReference returns the opaque caller-supplied operation grouping.
func (context AuditContext) CorrelationReference() AuditReference {
	return context.correlationReference
}

// EvaluatedAt returns the single canonical decision instant.
func (context AuditContext) EvaluatedAt() time.Time { return context.evaluatedAt }

func canonicalGovernanceInstant(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0).Truncate(time.Microsecond)
}
