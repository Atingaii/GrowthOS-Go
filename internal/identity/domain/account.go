package domain

import (
	"fmt"
	"log/slog"
	"time"
)

const MaxPasswordEnvelopeBytes = 256

// AccountStatus is the closed lifecycle state relevant to authentication.
type AccountStatus string

const (
	AccountStatusEnabled  AccountStatus = "enabled"
	AccountStatusDisabled AccountStatus = "disabled"
)

func (status AccountStatus) Valid() bool {
	switch status {
	case AccountStatusEnabled, AccountStatusDisabled:
		return true
	default:
		return false
	}
}

// CredentialVersion identifies a nonzero credential-envelope revision.
type CredentialVersion uint64

func NewCredentialVersion(value uint64) (CredentialVersion, error) {
	version := CredentialVersion(value)
	if err := version.Validate(); err != nil {
		return 0, err
	}
	return version, nil
}

func (version CredentialVersion) Validate() error {
	if version == 0 {
		return ErrCredentialVersionInvalid
	}
	return nil
}

// AuthenticationEpoch is a monotonic nonzero account-wide revocation version.
type AuthenticationEpoch uint64

func NewAuthenticationEpoch(value uint64) (AuthenticationEpoch, error) {
	epoch := AuthenticationEpoch(value)
	if err := epoch.Validate(); err != nil {
		return 0, err
	}
	return epoch, nil
}

func (epoch AuthenticationEpoch) Validate() error {
	if epoch == 0 {
		return ErrAuthenticationEpochInvalid
	}
	return nil
}

// PasswordEnvelope contains only an encoded one-way verifier envelope. Its
// constructor copies input bytes and its accessor returns a defensive copy.
type PasswordEnvelope struct {
	value []byte
}

func NewPasswordEnvelope(value []byte) (PasswordEnvelope, error) {
	envelope := PasswordEnvelope{value: cloneBytes(value)}
	if err := envelope.Validate(); err != nil {
		return PasswordEnvelope{}, err
	}
	return envelope, nil
}

func (envelope PasswordEnvelope) Validate() error {
	if len(envelope.value) == 0 {
		return fmt.Errorf("%w: value is required", ErrPasswordEnvelopeInvalid)
	}
	if len(envelope.value) > MaxPasswordEnvelopeBytes {
		return fmt.Errorf(
			"%w: value exceeds %d bytes",
			ErrPasswordEnvelopeInvalid,
			MaxPasswordEnvelopeBytes,
		)
	}
	for _, character := range envelope.value {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("%w: value must be visible ASCII", ErrPasswordEnvelopeInvalid)
		}
	}
	return nil
}

func (envelope PasswordEnvelope) Bytes() []byte { return cloneBytes(envelope.value) }

func (PasswordEnvelope) String() string { return redactedValue }

func (PasswordEnvelope) GoString() string {
	return "domain.PasswordEnvelope(" + redactedValue + ")"
}

func (PasswordEnvelope) LogValue() slog.Value { return slog.StringValue(redactedValue) }

// WorkforceAccount is an immutable local workforce authentication record. It
// contains no role, capability, scope, policy, tenant, or business resource.
type WorkforceAccount struct {
	id                  AccountID
	loginName           LoginName
	principalID         PrincipalID
	status              AccountStatus
	credentialVersion   CredentialVersion
	authenticationEpoch AuthenticationEpoch
	passwordEnvelope    PasswordEnvelope
	createdAt           time.Time
	updatedAt           time.Time
}

func NewWorkforceAccount(
	id AccountID,
	loginName LoginName,
	principalID PrincipalID,
	status AccountStatus,
	credentialVersion CredentialVersion,
	authenticationEpoch AuthenticationEpoch,
	passwordEnvelope PasswordEnvelope,
	createdAt time.Time,
	updatedAt time.Time,
) (WorkforceAccount, error) {
	account := WorkforceAccount{
		id:                  id,
		loginName:           loginName,
		principalID:         principalID,
		status:              status,
		credentialVersion:   credentialVersion,
		authenticationEpoch: authenticationEpoch,
		passwordEnvelope: PasswordEnvelope{
			value: passwordEnvelope.Bytes(),
		},
		createdAt: createdAt,
		updatedAt: updatedAt,
	}
	if err := account.Validate(); err != nil {
		return WorkforceAccount{}, err
	}
	return account, nil
}

func (account WorkforceAccount) Validate() error {
	if err := account.id.Validate(); err != nil {
		return fmt.Errorf("%w: id: %w", ErrWorkforceAccountInvalid, err)
	}
	if err := account.loginName.Validate(); err != nil {
		return fmt.Errorf("%w: login name: %w", ErrWorkforceAccountInvalid, err)
	}
	if err := account.principalID.Validate(); err != nil {
		return fmt.Errorf("%w: principal id: %w", ErrWorkforceAccountInvalid, err)
	}
	if !account.status.Valid() {
		return fmt.Errorf(
			"%w: %w: %q",
			ErrWorkforceAccountInvalid,
			ErrAccountStatusUnsupported,
			account.status,
		)
	}
	if err := account.credentialVersion.Validate(); err != nil {
		return fmt.Errorf("%w: credential version: %w", ErrWorkforceAccountInvalid, err)
	}
	if err := account.authenticationEpoch.Validate(); err != nil {
		return fmt.Errorf("%w: authentication epoch: %w", ErrWorkforceAccountInvalid, err)
	}
	if err := account.passwordEnvelope.Validate(); err != nil {
		return fmt.Errorf("%w: credential envelope: %w", ErrWorkforceAccountInvalid, err)
	}
	if err := validateCanonicalTime("created at", account.createdAt, ErrAccountTimeInvalid); err != nil {
		return fmt.Errorf("%w: %w", ErrWorkforceAccountInvalid, err)
	}
	if err := validateCanonicalTime("updated at", account.updatedAt, ErrAccountTimeInvalid); err != nil {
		return fmt.Errorf("%w: %w", ErrWorkforceAccountInvalid, err)
	}
	if account.updatedAt.Before(account.createdAt) {
		return fmt.Errorf("%w: %w: updated at precedes created at", ErrWorkforceAccountInvalid, ErrAccountTimeInvalid)
	}
	return nil
}

func (account WorkforceAccount) ID() AccountID { return account.id }

func (account WorkforceAccount) LoginName() LoginName { return account.loginName }

func (account WorkforceAccount) PrincipalID() PrincipalID { return account.principalID }

func (account WorkforceAccount) Status() AccountStatus { return account.status }

func (account WorkforceAccount) CredentialVersion() CredentialVersion {
	return account.credentialVersion
}

func (account WorkforceAccount) AuthenticationEpoch() AuthenticationEpoch {
	return account.authenticationEpoch
}

func (account WorkforceAccount) CredentialEnvelope() PasswordEnvelope {
	return PasswordEnvelope{value: account.passwordEnvelope.Bytes()}
}

func (account WorkforceAccount) CreatedAt() time.Time { return account.createdAt }

func (account WorkforceAccount) UpdatedAt() time.Time { return account.updatedAt }

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
