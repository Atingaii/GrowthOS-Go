package fault

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestKindValidationCoversStableKinds(t *testing.T) {
	validKinds := []Kind{
		KindInvalid,
		KindUnauthenticated,
		KindForbidden,
		KindNotFound,
		KindConflict,
		KindRateLimited,
		KindInternal,
	}
	for _, kind := range validKinds {
		if !kind.Valid() {
			t.Fatalf("Kind(%q).Valid() = false, want true", kind)
		}
	}
	if Kind("unknown").Valid() {
		t.Fatal("unknown kind is valid")
	}
}

func TestFaultKeepsPublicContractSeparateFromCause(t *testing.T) {
	cause := errors.New("sql: password=do-not-expose")
	failure, err := New(KindConflict, "campaign_version_conflict", "campaign was updated", cause)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if failure.Kind() != KindConflict {
		t.Fatalf("Kind() = %q, want %q", failure.Kind(), KindConflict)
	}
	if failure.Code() != "campaign_version_conflict" {
		t.Fatalf("Code() = %q", failure.Code())
	}
	if failure.PublicMessage() != "campaign was updated" {
		t.Fatalf("PublicMessage() = %q", failure.PublicMessage())
	}
	if !errors.Is(failure, cause) {
		t.Fatal("fault does not unwrap to cause")
	}
	if got := failure.Error(); got != "campaign_version_conflict" {
		t.Fatalf("Error() = %q, want stable code only", got)
	}
	if strings.Contains(failure.Error(), "password") {
		t.Fatalf("Error() leaked internal cause: %q", failure.Error())
	}

	wrapped := fmt.Errorf("application operation: %w", failure)
	got, ok := As(wrapped)
	if !ok || got != failure {
		t.Fatalf("As() = (%v, %v), want original fault", got, ok)
	}
}

func TestMustNewAndNilFaultBehavior(t *testing.T) {
	failure := MustNew(KindNotFound, "customer_not_found", "customer not found", nil)
	if got := failure.Error(); got != "customer_not_found" {
		t.Fatalf("MustNew().Error() = %q", got)
	}

	var nilFault *Error
	if got := nilFault.Error(); got != "<nil>" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilFault.Unwrap() != nil {
		t.Fatal("nil Unwrap() is not nil")
	}
	if got := nilFault.Kind(); got != KindInternal {
		t.Fatalf("nil Kind() = %q", got)
	}
	if got := nilFault.Code(); got != "internal_error" {
		t.Fatalf("nil Code() = %q", got)
	}
	if got := nilFault.PublicMessage(); got != "internal server error" {
		t.Fatalf("nil PublicMessage() = %q", got)
	}
	if got, ok := As(nilFault); ok || got != nil {
		t.Fatalf("As(typed nil) = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestZeroAndMalformedFaultsFailClosed(t *testing.T) {
	tests := []*Error{
		{},
		{kind: KindInvalid, code: "valid_code", publicMessage: "unsafe\nmessage"},
	}
	for _, failure := range tests {
		if got := failure.Error(); got != "internal_error" {
			t.Fatalf("invalid Error() = %q", got)
		}
		if got := failure.Kind(); got != KindInternal {
			t.Fatalf("invalid Kind() = %q", got)
		}
		if got := failure.Code(); got != "internal_error" {
			t.Fatalf("invalid Code() = %q", got)
		}
		if got := failure.PublicMessage(); got != "internal server error" {
			t.Fatalf("invalid PublicMessage() = %q", got)
		}
		if got, ok := As(failure); ok || got != nil {
			t.Fatalf("As(invalid fault) = (%v, %v), want (nil, false)", got, ok)
		}
	}
}

func TestMustNewPanicsForInvalidConstants(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("MustNew() did not panic for invalid code")
		}
	}()
	MustNew(KindInvalid, "INVALID", "safe message", nil)
}

func TestNewRejectsUnstablePublicContracts(t *testing.T) {
	tests := []struct {
		name    string
		kind    Kind
		code    string
		message string
	}{
		{name: "kind", kind: Kind("other"), code: "valid_code", message: "valid message"},
		{name: "empty code", kind: KindInvalid, code: "", message: "valid message"},
		{name: "uppercase code", kind: KindInvalid, code: "Invalid_Code", message: "valid message"},
		{name: "punctuated code", kind: KindInvalid, code: "invalid-code", message: "valid message"},
		{name: "long code", kind: KindInvalid, code: "a" + strings.Repeat("b", maxCodeLength), message: "valid message"},
		{name: "empty message", kind: KindInvalid, code: "invalid_request", message: ""},
		{name: "trimmed message", kind: KindInvalid, code: "invalid_request", message: " client message "},
		{name: "control message", kind: KindInvalid, code: "invalid_request", message: "client\nmessage"},
		{name: "long message", kind: KindInvalid, code: "invalid_request", message: strings.Repeat("界", maxPublicMessageLength+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.kind, test.code, test.message, nil); err == nil {
				t.Fatal("New() error = nil, want validation failure")
			}
		})
	}
}
