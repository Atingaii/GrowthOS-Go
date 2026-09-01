package domain

import (
	"errors"
	"strings"
	"testing"

	governance "github.com/Atingaii/GrowthOS-Go/internal/governance/domain"
)

func TestCanonicalIdentifiersPreserveExactGovernanceCompatibleValues(t *testing.T) {
	t.Parallel()

	valid := []string{
		"a",
		"42",
		"human:operator-1",
		"account.release_v1",
		strings.Repeat("a", MaxCanonicalIdentifierBytes),
	}
	invalid := []string{
		"",
		"Admin",
		" admin",
		"admin ",
		"-leading",
		"trailing-",
		"a/b",
		"a*b",
		"租户",
		strings.Repeat("a", MaxCanonicalIdentifierBytes+1),
	}

	constructors := []struct {
		name string
		make func(string) (string, error)
	}{
		{
			name: "account id",
			make: func(value string) (string, error) {
				got, err := NewAccountID(value)
				return got.String(), err
			},
		},
		{
			name: "principal id",
			make: func(value string) (string, error) {
				got, err := NewPrincipalID(value)
				return got.String(), err
			},
		},
		{
			name: "session reference",
			make: func(value string) (string, error) {
				got, err := NewSessionRef(value)
				return got.String(), err
			},
		},
		{
			name: "operation reference",
			make: func(value string) (string, error) {
				got, err := NewOperationRef(value)
				return got.String(), err
			},
		},
	}

	for _, constructor := range constructors {
		constructor := constructor
		t.Run(constructor.name, func(t *testing.T) {
			t.Parallel()
			for _, value := range valid {
				got, err := constructor.make(value)
				if err != nil {
					t.Fatalf("construct %q: %v", value, err)
				}
				if got != value {
					t.Fatalf("constructor normalized %q to %q", value, got)
				}
			}
			for _, value := range invalid {
				if _, err := constructor.make(value); err == nil {
					t.Fatalf("construct invalid %q: nil error", value)
				}
			}
		})
	}

	for _, value := range append(valid, invalid...) {
		_, identityErr := NewPrincipalID(value)
		_, governanceErr := governance.NewPrincipalID(value)
		if (identityErr == nil) != (governanceErr == nil) {
			t.Fatalf(
				"principal compatibility for %q: identity=%v governance=%v",
				value,
				identityErr,
				governanceErr,
			)
		}
	}
}

func TestLoginNameExactGrammar(t *testing.T) {
	t.Parallel()

	valid := []string{
		"abc",
		"a12",
		"operator-1",
		"a.b_c-",
		"a..",
		"a__",
		"a--",
		"a" + strings.Repeat("b", MaxLoginNameBytes-1),
	}
	invalid := []string{
		"",
		"a",
		"ab",
		"1ab",
		"Abc",
		" abc",
		"abc ",
		"a:b",
		"a/b",
		"操作员",
		"a" + strings.Repeat("b", MaxLoginNameBytes),
	}
	for _, value := range valid {
		name, err := NewLoginName(value)
		if err != nil {
			t.Fatalf("new valid login %q: %v", value, err)
		}
		if name.String() != value {
			t.Fatalf("login normalized %q to %q", value, name)
		}
	}
	for _, value := range invalid {
		name, err := NewLoginName(value)
		if !errors.Is(err, ErrLoginNameInvalid) {
			t.Fatalf("new invalid login %q: name=%q err=%v", value, name, err)
		}
		if name != "" {
			t.Fatalf("failed login construction returned %q", name)
		}
	}
}

func FuzzPrincipalIDGrammarMatchesGovernance(f *testing.F) {
	for _, seed := range []string{"a", "42", "human:1", "-bad", "Admin", "租户", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		identityID, identityErr := NewPrincipalID(value)
		governanceID, governanceErr := governance.NewPrincipalID(value)
		if (identityErr == nil) != (governanceErr == nil) {
			t.Fatalf(
				"compatibility mismatch for %q: identity=%v governance=%v",
				value,
				identityErr,
				governanceErr,
			)
		}
		if identityErr == nil && (identityID.String() != value || governanceID.String() != value) {
			t.Fatalf("successful constructor normalized %q", value)
		}
	})
}

func FuzzLoginNameNeverNormalizes(f *testing.F) {
	for _, seed := range []string{"abc", "operator-1", "Abc", " abc", "操作员", ""} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		name, err := NewLoginName(value)
		wantValid := validLoginNameForTest(value)
		if (err == nil) != wantValid {
			t.Fatalf("login %q valid=%v err=%v", value, wantValid, err)
		}
		if err == nil && name.String() != value {
			t.Fatalf("login normalized %q to %q", value, name)
		}
	})
}

func validLoginNameForTest(value string) bool {
	if len(value) < MinLoginNameBytes || len(value) > MaxLoginNameBytes ||
		value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
