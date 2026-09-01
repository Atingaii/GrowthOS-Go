package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestErrorClassesDoNotLeakThroughFormattingOrUnwrap(t *testing.T) {
	t.Parallel()

	privateCause := errors.New("sql host=db.internal password=do-not-disclose")
	dependencyError := WrapDependencyError(ErrAccountNotFound, privateCause)
	operationError := wrapOperationError(ErrAuthenticationFailed, dependencyError)
	for name, value := range map[string]error{
		"dependency": dependencyError,
		"operation":  operationError,
	} {
		t.Run(name, func(t *testing.T) {
			renderings := []string{
				value.Error(),
				fmt.Sprint(value),
				fmt.Sprintf("%+v", value),
				fmt.Sprintf("%#v", value),
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			renderings = append(renderings, string(encoded))
			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
			logger.Info("error", "value", value)
			renderings = append(renderings, logBuffer.String())
			for _, rendering := range renderings {
				if strings.Contains(rendering, "db.internal") || strings.Contains(rendering, "do-not-disclose") {
					t.Fatalf("%s leaked private cause: %s", name, rendering)
				}
			}
			if errors.Unwrap(value) != nil || errors.Is(value, privateCause) {
				t.Fatalf("%s exposed a standard unwrap path", name)
			}
		})
	}
	if dependencyError.Cause() != privateCause || operationError.Cause() != dependencyError {
		t.Fatal("explicit trusted Cause access lost diagnostic evidence")
	}
	if !errors.Is(dependencyError, ErrAccountNotFound) ||
		!errors.Is(operationError, ErrAuthenticationFailed) {
		t.Fatal("stable class matching failed")
	}
}

func TestSensitiveApplicationValuesAreRedactedInEveryGenericSink(t *testing.T) {
	t.Parallel()

	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	digest := mustApplicationDigest(t, 0x71)
	session := mustApplicationSession(
		t,
		account,
		digest,
		"session-secret-correlation",
		"issue-secret-correlation",
		applicationTestNow.Add(-time.Minute),
	)
	principalID, err := principalIDFromAccount(account)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := newVerifiedSession(principalID, session)
	if err != nil {
		t.Fatal(err)
	}
	rawToken := bytes.Repeat([]byte("R"), SessionTokenBytes)
	issued, err := newIssuedSession(verified, rawToken)
	if err != nil {
		t.Fatal(err)
	}
	command := mustApplicationCommand(t, bytes.Repeat([]byte("P"), SessionTokenBytes))
	issueAttempt, err := newSessionIssueAttempt(account, session, identity.TokenDigest{}, false)
	if err != nil {
		t.Fatal(err)
	}
	revokeOperation, _ := identity.NewOperationRef("revoke-secret-correlation")
	revoked, err := session.Revoke(applicationTestNow, identity.SessionRevokeReasonLogout, revokeOperation)
	if err != nil {
		t.Fatal(err)
	}
	revokeAttempt, err := newSessionRevokeAttempt(account, session, revoked)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := newRevokeCommitReceipt(session, revoked)
	if err != nil {
		t.Fatal(err)
	}
	observation := ObserveSessionCommitState(revoked)
	request, err := NewAdmissionRequest(
		mustApplicationThrottleDigest(t, 0x41),
		mustApplicationThrottleDigest(t, 0x42),
		applicationTestNow,
		applicationTestNow.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	epoch, _ := identity.NewAdmissionEpoch(1)
	grant, _ := NewAdmissionGrant(epoch, epoch, request.Deadline())
	admissionReceipt, _ := newAdmissionReceipt(request, grant)

	values := map[string]any{
		"command":            command,
		"verified":           verified,
		"issued":             issued,
		"issue attempt":      issueAttempt,
		"revoke attempt":     revokeAttempt,
		"commit receipt":     receipt,
		"commit observation": observation,
		"admission request":  request,
		"admission grant":    grant,
		"admission receipt":  admissionReceipt,
	}
	for name, value := range values {
		t.Run(name, func(t *testing.T) {
			renderings := []string{fmt.Sprint(value), fmt.Sprintf("%#v", value)}
			encoded, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			renderings = append(renderings, string(encoded))
			var logBuffer bytes.Buffer
			slog.New(slog.NewJSONHandler(&logBuffer, nil)).Info("value", "payload", value)
			renderings = append(renderings, logBuffer.String())
			for _, rendering := range renderings {
				if !strings.Contains(rendering, "redacted") {
					t.Fatalf("%s has no redaction marker: %s", name, rendering)
				}
				for _, forbidden := range []string{
					"correct horse battery staple",
					strings.Repeat("R", SessionTokenBytes),
					"session-secret-correlation",
					"issue-secret-correlation",
					"revoke-secret-correlation",
					string(account.CredentialEnvelope().Bytes()),
				} {
					if strings.Contains(rendering, forbidden) {
						t.Fatalf("%s leaked %q: %s", name, forbidden, rendering)
					}
				}
			}
		})
	}
}

func TestCommitReceiptReconciliationIsExactAndNeverSuggestsReplay(t *testing.T) {
	t.Parallel()

	account := mustApplicationAccount(t, identity.AccountStatusEnabled)
	after := mustApplicationSession(
		t,
		account,
		mustApplicationDigest(t, 0x22),
		"session-commit-exact",
		"issue-commit-exact",
		applicationTestNow,
	)
	receipt, err := newIssueCommitReceipt(after)
	if err != nil {
		t.Fatal(err)
	}
	if got := ReconcileSessionCommit(receipt, ObserveSessionCommitAbsence()); got != SessionCommitReconciliationNotCommitted {
		t.Fatalf("absent issue = %q", got)
	}
	if got := ReconcileSessionCommit(receipt, ObserveSessionCommitState(after)); got != SessionCommitReconciliationCommitted {
		t.Fatalf("exact issue = %q", got)
	}
	other := mustApplicationSession(
		t,
		account,
		mustApplicationDigest(t, 0x23),
		"session-commit-other",
		"issue-commit-other",
		applicationTestNow,
	)
	if got := ReconcileSessionCommit(receipt, ObserveSessionCommitState(other)); got != SessionCommitReconciliationIndeterminate {
		t.Fatalf("mismatched issue = %q", got)
	}
	if got := ReconcileSessionCommit(SessionCommitReceipt{}, ObserveSessionCommitState(after)); got != SessionCommitReconciliationIndeterminate {
		t.Fatalf("forged receipt = %q", got)
	}
}

func TestPasswordVerificationRejectsImpossibleRehashSignal(t *testing.T) {
	t.Parallel()

	if result, err := NewPasswordVerification(false, true); err == nil || result != (PasswordVerification{}) {
		t.Fatalf("non-match requested rehash: %#v, %v", result, err)
	}
	for _, values := range [][2]bool{{false, false}, {true, false}, {true, true}} {
		result, err := NewPasswordVerification(values[0], values[1])
		if err != nil || result.Validate() != nil || result.Matched() != values[0] || result.NeedsRehash() != values[1] {
			t.Fatalf("verification %v = %#v, %v", values, result, err)
		}
	}
}

func TestConstructorsRejectTypedNilDependencies(t *testing.T) {
	t.Parallel()

	var reader *typedNilCredentialReader
	service, err := NewLoginService(LoginDependencies{
		Clock:       ClockFunc(func() time.Time { return applicationTestNow }),
		Credentials: reader,
		Passwords: passwordVerifierStub{
			verifyLogin: func(context.Context, []byte, string) (PasswordVerification, error) {
				return PasswordVerification{}, nil
			},
			verifyUnknown: func(context.Context, []byte) error { return nil },
		},
		Admissions: concurrentAdmissionStub(t),
		Entropy:    &sequenceEntropy{},
		Issuer:     sessionIssuerFunc(func(context.Context, SessionIssueAttempt) error { return nil }),
	})
	if !errors.Is(err, ErrNotConfigured) || service != nil {
		t.Fatalf("typed nil dependency accepted: %#v, %v", service, err)
	}
}

type typedNilCredentialReader struct{}

func (*typedNilCredentialReader) FindByLogin(context.Context, identity.LoginName) (identity.WorkforceAccount, error) {
	return identity.WorkforceAccount{}, nil
}
