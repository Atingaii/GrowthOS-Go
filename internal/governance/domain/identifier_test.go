package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestOpaqueIdentifierConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		construct func(string) error
	}{
		{
			name: "principal",
			construct: func(value string) error {
				_, err := NewPrincipalID(value)
				return err
			},
		},
		{
			name: "resource",
			construct: func(value string) error {
				_, err := NewResourceID(value)
				return err
			},
		},
		{
			name: "tenant",
			construct: func(value string) error {
				_, err := NewTenantID(value)
				return err
			},
		},
		{
			name: "binding",
			construct: func(value string) error {
				_, err := NewRoleBindingID(value)
				return err
			},
		},
		{
			name: "policy",
			construct: func(value string) error {
				_, err := NewPolicyID(value)
				return err
			},
		},
		{
			name: "audit reference",
			construct: func(value string) error {
				_, err := NewAuditReference(value)
				return err
			},
		},
	}

	validValues := []string{
		"a",
		"42",
		"human:42",
		"tenant-01",
		"activity.release_v1",
		strings.Repeat("a", MaxOpaqueIdentifierBytes),
	}
	invalidValues := []string{
		"",
		"Admin",
		" admin",
		"admin ",
		"a/b",
		"a*b",
		"租户",
		"-leading",
		"trailing-",
		"a\nb",
		strings.Repeat("a", MaxOpaqueIdentifierBytes+1),
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, value := range validValues {
				if err := test.construct(value); err != nil {
					t.Fatalf("construct %q: %v", value, err)
				}
			}
			for _, value := range invalidValues {
				err := test.construct(value)
				if !errors.Is(err, ErrIdentifierInvalid) {
					t.Fatalf("construct invalid %q: got %v", value, err)
				}
			}
		})
	}
}

func TestOpaqueIdentifierStringMethods(t *testing.T) {
	t.Parallel()

	if got := PrincipalID("principal-1").String(); got != "principal-1" {
		t.Fatalf("principal string = %q", got)
	}
	if got := ResourceID("activity-1").String(); got != "activity-1" {
		t.Fatalf("resource string = %q", got)
	}
	if got := TenantID("tenant-1").String(); got != "tenant-1" {
		t.Fatalf("tenant string = %q", got)
	}
	if got := RoleBindingID("binding-1").String(); got != "binding-1" {
		t.Fatalf("binding string = %q", got)
	}
	if got := PolicyID("policy-1").String(); got != "policy-1" {
		t.Fatalf("policy string = %q", got)
	}
	if got := AuditReference("evaluation-1").String(); got != "evaluation-1" {
		t.Fatalf("audit reference string = %q", got)
	}
}
