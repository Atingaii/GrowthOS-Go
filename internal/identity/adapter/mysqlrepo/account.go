package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/jmoiron/sqlx"
)

const (
	selectAccountByLoginSQL = `
		SELECT account_id, login_name, principal_id, password_envelope,
		       account_status, credential_version, authentication_epoch,
		       created_at, updated_at
		FROM identity_workforce_account
		WHERE login_name = ?`
	selectAccountByIDSQL = `
		SELECT account_id, login_name, principal_id, password_envelope,
		       account_status, credential_version, authentication_epoch,
		       created_at, updated_at
		FROM identity_workforce_account
		WHERE account_id = ?`
	selectAccountForUpdateSQL = `
		SELECT account_id, login_name, principal_id, password_envelope,
		       account_status, credential_version, authentication_epoch,
		       created_at, updated_at
		FROM identity_workforce_account
		WHERE account_id = ?
		FOR UPDATE`
)

type storedAccount struct {
	accountID           string
	loginName           string
	principalID         string
	passwordEnvelope    []byte
	status              string
	credentialVersion   uint64
	authenticationEpoch uint64
	createdAt           time.Time
	updatedAt           time.Time
}

// FindByLogin performs an exact binary-collation lookup and strictly restores
// the complete local credential snapshot. It never normalizes an identifier.
func (repository *Repository) FindByLogin(
	ctx context.Context,
	login identity.LoginName,
) (identity.WorkforceAccount, error) {
	if err := repository.validateCall(ctx); err != nil {
		return identity.WorkforceAccount{}, err
	}
	if err := login.Validate(); err != nil {
		return identity.WorkforceAccount{}, dependencyError(
			identityapp.ErrDependencyInvalidArgument,
			err,
		)
	}

	row, err := scanAccount(repository.database.QueryRowxContext(ctx, selectAccountByLoginSQL, login.String()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.WorkforceAccount{}, dependencyError(identityapp.ErrAccountNotFound, err)
		}
		return identity.WorkforceAccount{}, classifyOperationError(ctx, err)
	}
	defer clearBytes(row.passwordEnvelope)
	account, err := restoreAccount(row)
	if err != nil {
		return identity.WorkforceAccount{}, storedIdentityInvalid(err)
	}
	return account, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanAccount(scanner rowScanner) (storedAccount, error) {
	var row storedAccount
	err := scanner.Scan(
		&row.accountID,
		&row.loginName,
		&row.principalID,
		&row.passwordEnvelope,
		&row.status,
		&row.credentialVersion,
		&row.authenticationEpoch,
		&row.createdAt,
		&row.updatedAt,
	)
	return row, err
}

func loadAccountForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	accountID identity.AccountID,
) (identity.WorkforceAccount, error) {
	row, err := scanAccount(tx.QueryRowxContext(ctx, selectAccountForUpdateSQL, accountID.String()))
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	defer clearBytes(row.passwordEnvelope)
	account, err := restoreAccount(row)
	if err != nil {
		return identity.WorkforceAccount{}, &storedRestoreError{cause: err}
	}
	return account, nil
}

func loadAccountByID(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	accountID identity.AccountID,
) (identity.WorkforceAccount, error) {
	row, err := scanAccount(queryer.QueryRowxContext(ctx, selectAccountByIDSQL, accountID.String()))
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	defer clearBytes(row.passwordEnvelope)
	account, err := restoreAccount(row)
	if err != nil {
		return identity.WorkforceAccount{}, &storedRestoreError{cause: err}
	}
	return account, nil
}

func restoreAccount(row storedAccount) (identity.WorkforceAccount, error) {
	accountID, err := identity.NewAccountID(row.accountID)
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	login, err := identity.NewLoginName(row.loginName)
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	principalID, err := identity.NewPrincipalID(row.principalID)
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	envelope, err := identity.NewPasswordEnvelope(row.passwordEnvelope)
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	credentialVersion, err := identity.NewCredentialVersion(row.credentialVersion)
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	authenticationEpoch, err := identity.NewAuthenticationEpoch(row.authenticationEpoch)
	if err != nil {
		return identity.WorkforceAccount{}, err
	}
	return identity.NewWorkforceAccount(
		accountID,
		login,
		principalID,
		identity.AccountStatus(row.status),
		credentialVersion,
		authenticationEpoch,
		envelope,
		row.createdAt,
		row.updatedAt,
	)
}

func accountsEqual(left, right identity.WorkforceAccount) bool {
	if left.Validate() != nil || right.Validate() != nil ||
		left.ID() != right.ID() || left.LoginName() != right.LoginName() ||
		left.PrincipalID() != right.PrincipalID() || left.Status() != right.Status() ||
		left.CredentialVersion() != right.CredentialVersion() ||
		left.AuthenticationEpoch() != right.AuthenticationEpoch() ||
		left.CreatedAt() != right.CreatedAt() || left.UpdatedAt() != right.UpdatedAt() {
		return false
	}
	leftEnvelope := left.CredentialEnvelope().Bytes()
	rightEnvelope := right.CredentialEnvelope().Bytes()
	defer clearBytes(leftEnvelope)
	defer clearBytes(rightEnvelope)
	return digestEqual(leftEnvelope, rightEnvelope)
}
