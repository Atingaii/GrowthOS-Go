package application

import (
	"errors"
	"strings"
	"testing"
)

func TestLowDisclosureErrorWrappersRetainOnlyExplicitDiagnosticCause(t *testing.T) {
	secret := errors.New("mysql://operator:password@private/schema publication payload")
	for _, test := range []struct {
		name  string
		err   error
		class error
		cause func(error) error
	}{
		{
			name:  "repository",
			err:   WrapRepositoryError(ErrCommitOutcomeUnknown, secret),
			class: ErrCommitOutcomeUnknown,
			cause: func(err error) error { return err.(*RepositoryError).Cause() },
		},
		{
			name:  "approval",
			err:   WrapApprovalError(ErrActivityApprovalRejected, secret),
			class: ErrActivityApprovalRejected,
			cause: func(err error) error { return err.(*DependencyError).Cause() },
		},
		{
			name:  "Lottery",
			err:   WrapLotteryVerificationError(ErrLotteryPublicationInvalid, secret),
			class: ErrLotteryPublicationInvalid,
			cause: func(err error) error { return err.(*DependencyError).Cause() },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !errors.Is(test.err, test.class) || errors.Is(test.err, secret) {
				t.Fatalf("error class/chain = %v", test.err)
			}
			if strings.Contains(test.err.Error(), "password") || test.err.Error() != test.class.Error() {
				t.Fatalf("rendered error leaked: %q", test.err)
			}
			if !errors.Is(test.cause(test.err), secret) {
				t.Fatal("trusted Cause did not retain diagnostic")
			}
		})
	}
}

func TestUnknownWrapperClassesFailClosed(t *testing.T) {
	unknown := errors.New("unknown")
	if err := WrapRepositoryError(unknown, unknown); !errors.Is(err, ErrRepositoryFailure) {
		t.Fatalf("repository wrapper = %v", err)
	}
	if err := WrapApprovalError(unknown, unknown); !errors.Is(err, ErrActivityApprovalUnavailable) {
		t.Fatalf("approval wrapper = %v", err)
	}
	if err := WrapLotteryVerificationError(unknown, unknown); !errors.Is(err, ErrLotteryPublicationUnavailable) {
		t.Fatalf("Lottery wrapper = %v", err)
	}
}
