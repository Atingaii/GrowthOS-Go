package domain

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWorkforceAccountIsImmutableAndExact(t *testing.T) {
	t.Parallel()

	rawEnvelope := []byte("$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA")
	envelope, err := NewPasswordEnvelope(rawEnvelope)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	createdAt := canonicalTime(2026, 9, 1, 1, 2, 3, 0)
	updatedAt := createdAt.Add(time.Microsecond)
	account, err := NewWorkforceAccount(
		mustAccountID(t, "account-1"),
		mustLoginName(t, "operator-1"),
		mustIdentityPrincipalID(t, "human:operator-1"),
		AccountStatusEnabled,
		mustCredentialVersion(t, 3),
		mustAuthenticationEpoch(t, 7),
		envelope,
		createdAt,
		updatedAt,
	)
	if err != nil {
		t.Fatalf("new account: %v", err)
	}

	rawEnvelope[0] = '!'
	firstRead := account.CredentialEnvelope().Bytes()
	if string(firstRead) != "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA" {
		t.Fatalf("input mutation changed account envelope: %q", firstRead)
	}
	firstRead[0] = '!'
	if secondRead := account.CredentialEnvelope().Bytes(); secondRead[0] != '$' {
		t.Fatalf("output mutation changed account envelope: %q", secondRead)
	}

	if account.ID() != "account-1" || account.LoginName() != "operator-1" ||
		account.PrincipalID() != "human:operator-1" || account.Status() != AccountStatusEnabled ||
		account.CredentialVersion() != 3 || account.AuthenticationEpoch() != 7 ||
		account.CreatedAt() != createdAt || account.UpdatedAt() != updatedAt {
		t.Fatalf("unexpected account getters: %#v", account)
	}
	if err := account.Validate(); err != nil {
		t.Fatalf("account no longer validates: %v", err)
	}
}

func TestPasswordEnvelopeBoundariesAndZeroResults(t *testing.T) {
	t.Parallel()

	valid := [][]byte{
		{'!'},
		[]byte("$argon2id$v=19$m=19456,t=2,p=1$salt$hash"),
		[]byte(strings.Repeat("x", MaxPasswordEnvelopeBytes)),
	}
	for _, value := range valid {
		envelope, err := NewPasswordEnvelope(value)
		if err != nil {
			t.Fatalf("new valid envelope of %d bytes: %v", len(value), err)
		}
		if string(envelope.Bytes()) != string(value) {
			t.Fatal("envelope changed bytes")
		}
	}
	invalid := [][]byte{
		nil,
		{},
		{0},
		{' '},
		{0x7f},
		{0xc3, 0xa9},
		[]byte(strings.Repeat("x", MaxPasswordEnvelopeBytes+1)),
	}
	for _, value := range invalid {
		envelope, err := NewPasswordEnvelope(value)
		if !errors.Is(err, ErrPasswordEnvelopeInvalid) {
			t.Fatalf("new invalid envelope %v: got %v", value, err)
		}
		if envelope.value != nil {
			t.Fatalf("failed envelope construction returned partial value %#v", envelope)
		}
	}
}

func TestPasswordEnvelopeFormattingAndLoggingAreRedacted(t *testing.T) {
	t.Parallel()

	secret := "sentinel-envelope-value"
	envelope, err := NewPasswordEnvelope([]byte(secret))
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	for _, formatted := range []string{
		fmt.Sprint(envelope),
		fmt.Sprintf("%s", envelope),
		fmt.Sprintf("%v", envelope),
		fmt.Sprintf("%#v", envelope),
	} {
		if strings.Contains(formatted, secret) || !strings.Contains(formatted, redactedValue) {
			t.Fatalf("unsafe formatting %q", formatted)
		}
	}
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	logger.Info("credential", "envelope", envelope)
	if got := output.String(); strings.Contains(got, secret) || !strings.Contains(got, redactedValue) {
		t.Fatalf("unsafe structured log %q", got)
	}
}

func TestWorkforceAccountRejectsPartialAndUnsupportedState(t *testing.T) {
	t.Parallel()

	envelope := mustPasswordEnvelope(t)
	valid := WorkforceAccount{
		id:                  "account-1",
		loginName:           "operator-1",
		principalID:         "human:operator-1",
		status:              AccountStatusEnabled,
		credentialVersion:   1,
		authenticationEpoch: 1,
		passwordEnvelope:    envelope,
		createdAt:           canonicalTime(2026, 9, 1, 1, 0, 0, 0),
		updatedAt:           canonicalTime(2026, 9, 1, 1, 0, 0, 0),
	}
	invalid := []WorkforceAccount{
		{},
		withAccountMutation(valid, func(account *WorkforceAccount) { account.id = "Bad" }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.loginName = "op" }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.principalID = "-bad" }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.status = "locked" }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.credentialVersion = 0 }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.authenticationEpoch = 0 }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.passwordEnvelope = PasswordEnvelope{} }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.createdAt = time.Time{} }),
		withAccountMutation(valid, func(account *WorkforceAccount) { account.updatedAt = time.Time{} }),
		withAccountMutation(valid, func(account *WorkforceAccount) {
			account.updatedAt = account.createdAt.Add(-time.Microsecond)
		}),
	}
	for _, account := range invalid {
		if err := account.Validate(); !errors.Is(err, ErrWorkforceAccountInvalid) {
			t.Fatalf("validate invalid account %#v: %v", account, err)
		}
	}

	account, err := NewWorkforceAccount(
		valid.id,
		valid.loginName,
		valid.principalID,
		"locked",
		valid.credentialVersion,
		valid.authenticationEpoch,
		valid.passwordEnvelope,
		valid.createdAt,
		valid.updatedAt,
	)
	if !errors.Is(err, ErrAccountStatusUnsupported) || !isZeroWorkforceAccount(account) {
		t.Fatalf("unsupported status result = %#v, %v", account, err)
	}
}

func isZeroWorkforceAccount(account WorkforceAccount) bool {
	return account.id == "" && account.loginName == "" && account.principalID == "" &&
		account.status == "" && account.credentialVersion == 0 && account.authenticationEpoch == 0 &&
		account.passwordEnvelope.value == nil && account.createdAt.IsZero() && account.updatedAt.IsZero()
}

func TestAccountClosedStatusAndNonzeroVersions(t *testing.T) {
	t.Parallel()

	if !AccountStatusEnabled.Valid() || !AccountStatusDisabled.Valid() ||
		AccountStatus("locked").Valid() || AccountStatus("").Valid() {
		t.Fatal("account status vocabulary is not closed")
	}
	if version, err := NewCredentialVersion(1); err != nil || version != 1 {
		t.Fatalf("credential version = %d, %v", version, err)
	}
	if version, err := NewCredentialVersion(0); !errors.Is(err, ErrCredentialVersionInvalid) || version != 0 {
		t.Fatalf("zero credential version = %d, %v", version, err)
	}
	if epoch, err := NewAuthenticationEpoch(1); err != nil || epoch != 1 {
		t.Fatalf("authentication epoch = %d, %v", epoch, err)
	}
	if epoch, err := NewAuthenticationEpoch(0); !errors.Is(err, ErrAuthenticationEpochInvalid) || epoch != 0 {
		t.Fatalf("zero authentication epoch = %d, %v", epoch, err)
	}
}

func withAccountMutation(
	account WorkforceAccount,
	mutate func(*WorkforceAccount),
) WorkforceAccount {
	mutate(&account)
	return account
}
