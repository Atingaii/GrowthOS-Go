package domain

import (
	"errors"
	"testing"
	"time"
)

func TestAuditContextCanonicalizesAndPreservesMinimalCorrelation(t *testing.T) {
	t.Parallel()

	evaluationReference := mustAuditReference(t, "evaluation-31")
	correlationReference := mustAuditReference(t, "operation-9")
	input := time.Date(2026, 8, 31, 12, 13, 14, 123456789, time.FixedZone("cst", 8*60*60))
	context, err := NewAuditContext(evaluationReference, correlationReference, input)
	if err != nil {
		t.Fatalf("new audit context: %v", err)
	}

	wantTime := time.Date(2026, 8, 31, 4, 13, 14, 123456000, time.UTC)
	if context.EvaluationReference() != evaluationReference {
		t.Fatalf("evaluation reference = %q", context.EvaluationReference())
	}
	if context.CorrelationReference() != correlationReference {
		t.Fatalf("correlation reference = %q", context.CorrelationReference())
	}
	if got := context.EvaluatedAt(); got != wantTime {
		t.Fatalf("evaluated at = %s, want %s", got, wantTime)
	}
	if err := context.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestAuditContextRejectsZeroPartialAndNonCanonicalState(t *testing.T) {
	t.Parallel()

	reference := mustAuditReference(t, "evaluation-1")
	canonical := time.Date(2026, 8, 31, 1, 2, 3, 456000, time.UTC)
	invalid := []AuditContext{
		{},
		{correlationReference: reference, evaluatedAt: canonical},
		{evaluationReference: reference, evaluatedAt: canonical},
		{
			evaluationReference:  reference,
			correlationReference: reference,
		},
		{
			evaluationReference:  reference,
			correlationReference: reference,
			evaluatedAt:          canonical.Add(time.Nanosecond),
		},
		{
			evaluationReference:  reference,
			correlationReference: reference,
			evaluatedAt:          time.Date(2026, 8, 31, 9, 2, 3, 456000, time.FixedZone("cst", 8*60*60)),
		},
	}
	for _, context := range invalid {
		if err := context.Validate(); !errors.Is(err, ErrAuditContextInvalid) {
			t.Fatalf("validate %#v: got %v", context, err)
		}
	}

	if _, err := NewAuditContext("", reference, canonical); !errors.Is(err, ErrAuditContextInvalid) {
		t.Fatalf("new with empty reference: %v", err)
	}
	if _, err := NewAuditContext(reference, reference, time.Time{}); !errors.Is(err, ErrAuditContextInvalid) {
		t.Fatalf("new with zero time: %v", err)
	}
}

func mustAuditReference(t *testing.T, value string) AuditReference {
	t.Helper()
	reference, err := NewAuditReference(value)
	if err != nil {
		t.Fatalf("new audit reference %q: %v", value, err)
	}
	return reference
}
