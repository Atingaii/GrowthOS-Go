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
	selectSessionAccountIDByDigestSQL = `
		SELECT account_id
		FROM identity_session
		WHERE token_digest = ?`
	selectSessionByDigestSQL = `
		SELECT session_ref, issue_operation_ref, account_id, token_digest,
		       authentication_epoch, issued_at, last_seen_at, idle_expires_at,
		       absolute_expires_at, revoked_at, revoke_reason,
		       revoke_operation_ref, updated_at
		FROM identity_session
		WHERE token_digest = ?`
	selectSessionByDigestForUpdateSQL = `
		SELECT session_ref, issue_operation_ref, account_id, token_digest,
		       authentication_epoch, issued_at, last_seen_at, idle_expires_at,
		       absolute_expires_at, revoked_at, revoke_reason,
		       revoke_operation_ref, updated_at
		FROM identity_session
		WHERE token_digest = ? AND account_id = ?
		FOR UPDATE`
	selectSessionByReferenceSQL = `
		SELECT session_ref, issue_operation_ref, account_id, token_digest,
		       authentication_epoch, issued_at, last_seen_at, idle_expires_at,
		       absolute_expires_at, revoked_at, revoke_reason,
		       revoke_operation_ref, updated_at
		FROM identity_session
		WHERE session_ref = ?`
	selectSessionByReferenceForUpdateSQL = `
		SELECT session_ref, issue_operation_ref, account_id, token_digest,
		       authentication_epoch, issued_at, last_seen_at, idle_expires_at,
		       absolute_expires_at, revoked_at, revoke_reason,
		       revoke_operation_ref, updated_at
		FROM identity_session
		WHERE session_ref = ? AND account_id = ?
		FOR UPDATE`
	selectActiveSessionsForUpdateSQL = `
		SELECT session_ref, issue_operation_ref, account_id, token_digest,
		       authentication_epoch, issued_at, last_seen_at, idle_expires_at,
		       absolute_expires_at, revoked_at, revoke_reason,
		       revoke_operation_ref, updated_at
		FROM identity_session
		WHERE account_id = ? AND authentication_epoch = ?
		  AND revoked_at IS NULL
		  AND idle_expires_at > ? AND absolute_expires_at > ?
		ORDER BY last_seen_at ASC, issued_at ASC, session_ref ASC
		LIMIT 6
		FOR UPDATE`
	insertSessionSQL = `
		INSERT INTO identity_session
			(session_ref, issue_operation_ref, account_id, token_digest,
			 authentication_epoch, issued_at, last_seen_at, idle_expires_at,
			 absolute_expires_at, revoked_at, revoke_reason,
			 revoke_operation_ref, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?)`
	updateSessionRevocationSQL = `
		UPDATE identity_session
		SET revoked_at = ?, revoke_reason = ?, revoke_operation_ref = ?,
		    updated_at = GREATEST(updated_at, ?)
		WHERE session_ref = ? AND account_id = ? AND issue_operation_ref = ?
		  AND token_digest = ? AND authentication_epoch = ? AND issued_at = ?
		  AND absolute_expires_at = ? AND revoked_at IS NULL`
	updateSessionTouchSQL = `
		UPDATE identity_session
		SET last_seen_at = ?, idle_expires_at = ?,
		    updated_at = GREATEST(updated_at, ?)
		WHERE session_ref = ? AND account_id = ? AND token_digest = ?
		  AND authentication_epoch = ? AND issued_at = ?
		  AND last_seen_at = ? AND idle_expires_at = ?
		  AND absolute_expires_at = ? AND revoked_at IS NULL`
)

type storedSession struct {
	reference           string
	issueOperationRef   string
	accountID           string
	tokenDigest         []byte
	authenticationEpoch uint64
	issuedAt            time.Time
	lastSeenAt          time.Time
	idleExpiresAt       time.Time
	absoluteExpiresAt   time.Time
	revokedAt           sql.NullTime
	revokeReason        sql.NullString
	revokeOperationRef  sql.NullString
	updatedAt           time.Time
}

// IssueSession serializes all session-cap decisions through the account row.
// It rechecks the complete credential snapshot, optionally replaces the exact
// same-account incoming token, evicts at most one deterministic oldest row,
// and inserts the fresh candidate in one transaction.
func (repository *Repository) IssueSession(
	ctx context.Context,
	attempt identityapp.SessionIssueAttempt,
) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if err := attempt.Validate(); err != nil {
		return dependencyError(identityapp.ErrDependencyInvalidArgument, err)
	}
	expectedAccount := attempt.Account()
	candidate := attempt.Session()

	// A caller may present a valid token owned by another account. Resolve the
	// immutable owner without a lock first, then only lock a hinted session after
	// the matching account lock is held; this prevents a cross-account lock cycle.
	hintBelongsToAccount := false
	previousDigest, hasPreviousHint := attempt.PreviousTokenDigest()
	if hasPreviousHint {
		owner, found, err := repository.findSessionAccountID(ctx, previousDigest)
		if err != nil {
			return err
		}
		hintBelongsToAccount = found && owner == expectedAccount.ID()
	}

	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()

	currentAccount, err := loadAccountForUpdate(ctx, tx, expectedAccount.ID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dependencyError(identityapp.ErrAccountStateConflict, err)
		}
		return classifyRestoreOperationError(ctx, err)
	}
	if !accountsEqual(currentAccount, expectedAccount) ||
		currentAccount.Status() != identity.AccountStatusEnabled ||
		currentAccount.AuthenticationEpoch() != candidate.AuthenticationEpoch() {
		return dependencyError(identityapp.ErrAccountStateConflict, errStoredRowInvalid)
	}
	txNow, err := repository.currentTime()
	if err != nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	if txNow.Before(candidate.IssuedAt()) {
		return dependencyError(identityapp.ErrDependencyUnavailable, errClockRegressed)
	}
	activeCandidate, err := candidate.ActiveAt(txNow)
	if err != nil || !activeCandidate {
		if err == nil {
			err = errStoredRowInvalid
		}
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}

	// Prove the persisted cap before mutating any hinted row. A valid hint must
	// never disguise a pre-existing sixth active session as repairable state.
	activeSessions, err := loadActiveSessionsForUpdate(
		ctx,
		tx,
		currentAccount.ID(),
		currentAccount.AuthenticationEpoch(),
		txNow,
	)
	if err != nil {
		return err
	}
	if uint64(len(activeSessions)) > MaximumActiveSessions {
		return storedIdentityInvalid(errStoredSessionOverflow)
	}

	if hintBelongsToAccount {
		previous, err := loadSessionByDigestForUpdate(ctx, tx, previousDigest, expectedAccount.ID())
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return classifyRestoreOperationError(ctx, err)
		}
		if err == nil {
			active, activeErr := previous.ActiveAt(txNow)
			if activeErr != nil {
				return storedIdentityInvalid(activeErr)
			}
			if active && previous.AuthenticationEpoch() == currentAccount.AuthenticationEpoch() {
				previousIndex := activeSessionIndex(activeSessions, previous.Reference())
				if previousIndex < 0 {
					return storedIdentityInvalid(errStoredRowInvalid)
				}
				if txNow.Before(previous.LastSeenAt()) {
					return storedIdentityInvalid(errClockRegressed)
				}
				reference, deriveErr := derivedRevokeOperationRef("replace:", candidate.IssueOperationRef())
				if deriveErr != nil {
					return dependencyError(identityapp.ErrDependencyInvalidArgument, deriveErr)
				}
				if err := writeSessionRevocation(
					ctx,
					tx,
					previous,
					txNow,
					identity.SessionRevokeReasonSecurityResponse,
					reference,
				); err != nil {
					return err
				}
				activeSessions = append(
					activeSessions[:previousIndex],
					activeSessions[previousIndex+1:]...,
				)
			}
		}
	}
	if uint64(len(activeSessions)) == MaximumActiveSessions {
		oldest := activeSessions[0]
		if txNow.Before(oldest.LastSeenAt()) {
			return storedIdentityInvalid(errClockRegressed)
		}
		reference, err := derivedRevokeOperationRef("evict:", candidate.IssueOperationRef())
		if err != nil {
			return dependencyError(identityapp.ErrDependencyInvalidArgument, err)
		}
		if err := writeSessionRevocation(
			ctx,
			tx,
			oldest,
			txNow,
			identity.SessionRevokeReasonConcurrencyLimit,
			reference,
		); err != nil {
			return err
		}
	}
	if err := insertSession(ctx, tx, candidate, txNow); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// ResolveAndTouch proves both the account and session under account->session
// lock order. A touch is a conditional extension clamped by the domain's
// absolute deadline; a write acknowledgement loss returns no trusted state.
func (repository *Repository) ResolveAndTouch(
	ctx context.Context,
	digest identity.TokenDigest,
	now time.Time,
	idleLifetime time.Duration,
	touchWindow time.Duration,
) (identity.WorkforceAccount, identity.Session, error) {
	if err := repository.validateCall(ctx); err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, err
	}
	if digest.Validate() != nil || canonicalTime(now) != now ||
		idleLifetime <= 0 || idleLifetime%time.Microsecond != 0 ||
		touchWindow <= 0 || touchWindow%time.Microsecond != 0 || touchWindow >= idleLifetime {
		return identity.WorkforceAccount{}, identity.Session{}, dependencyError(
			identityapp.ErrDependencyInvalidArgument,
			identityapp.ErrInvalidArgument,
		)
	}
	accountID, found, err := repository.findSessionAccountID(ctx, digest)
	if err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, err
	}
	if !found {
		return identity.WorkforceAccount{}, identity.Session{}, dependencyError(identityapp.ErrSessionNotFound, sql.ErrNoRows)
	}
	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()

	account, err := loadAccountForUpdate(ctx, tx, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.WorkforceAccount{}, identity.Session{}, storedIdentityInvalid(err)
		}
		return identity.WorkforceAccount{}, identity.Session{}, classifyRestoreOperationError(ctx, err)
	}
	session, err := loadSessionByDigestForUpdate(ctx, tx, digest, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.WorkforceAccount{}, identity.Session{}, dependencyError(identityapp.ErrSessionNotFound, err)
		}
		return identity.WorkforceAccount{}, identity.Session{}, classifyRestoreOperationError(ctx, err)
	}
	if account.Status() != identity.AccountStatusEnabled ||
		account.AuthenticationEpoch() != session.AuthenticationEpoch() ||
		account.ID() != session.AccountID() ||
		!tokenDigestsEqual(digest, session.TokenDigest()) {
		return identity.WorkforceAccount{}, identity.Session{}, dependencyError(identityapp.ErrSessionInactive, errStoredRowInvalid)
	}
	active, err := session.ActiveAt(now)
	if err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, storedIdentityInvalid(err)
	}
	if !active {
		return identity.WorkforceAccount{}, identity.Session{}, dependencyError(identityapp.ErrSessionInactive, errStoredRowInvalid)
	}
	touched := false
	if now.Sub(session.LastSeenAt()) >= touchWindow {
		next, err := session.Touch(now, idleLifetime)
		if err != nil {
			return identity.WorkforceAccount{}, identity.Session{}, storedIdentityInvalid(err)
		}
		if err := writeSessionTouch(ctx, tx, session, next); err != nil {
			return identity.WorkforceAccount{}, identity.Session{}, err
		}
		session = next
		touched = true
	}
	if err := ctx.Err(); err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		if touched {
			return identity.WorkforceAccount{}, identity.Session{}, classifyWriteCommitError(ctx, err)
		}
		return identity.WorkforceAccount{}, identity.Session{}, classifyReadCommitError(ctx, err)
	}
	return account, session, nil
}

// FindForRevocation returns an exact repeatable-read snapshot and never
// touches it. RevokeSession later rechecks immutable identity under locks.
func (repository *Repository) FindForRevocation(
	ctx context.Context,
	digest identity.TokenDigest,
) (identity.WorkforceAccount, identity.Session, error) {
	if err := repository.validateCall(ctx); err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, err
	}
	if digest.Validate() != nil {
		return identity.WorkforceAccount{}, identity.Session{}, dependencyError(
			identityapp.ErrDependencyInvalidArgument,
			identityapp.ErrInvalidArgument,
		)
	}
	tx, err := repository.database.BeginTxx(ctx, readTxOptions())
	if err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()
	session, err := loadSessionByDigest(ctx, tx, digest)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.WorkforceAccount{}, identity.Session{}, dependencyError(identityapp.ErrSessionNotFound, err)
		}
		return identity.WorkforceAccount{}, identity.Session{}, classifyRestoreOperationError(ctx, err)
	}
	account, err := loadAccountByID(ctx, tx, session.AccountID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.WorkforceAccount{}, identity.Session{}, storedIdentityInvalid(err)
		}
		return identity.WorkforceAccount{}, identity.Session{}, classifyRestoreOperationError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return identity.WorkforceAccount{}, identity.Session{}, classifyReadCommitError(ctx, err)
	}
	return account, session, nil
}

// RevokeSession tolerates a legitimate intervening touch but never tolerates
// an immutable identity/epoch change. It locks account then current session,
// chooses a revoke instant no earlier than current last-seen, and performs one
// confirmed transition.
func (repository *Repository) RevokeSession(
	ctx context.Context,
	attempt identityapp.SessionRevokeAttempt,
) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if err := attempt.Validate(); err != nil {
		return dependencyError(identityapp.ErrDependencyInvalidArgument, err)
	}
	expectedAccount := attempt.Account()
	before := attempt.Before()
	after := attempt.After()
	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()
	account, err := loadAccountForUpdate(ctx, tx, expectedAccount.ID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dependencyError(identityapp.ErrAccountStateConflict, err)
		}
		return classifyRestoreOperationError(ctx, err)
	}
	if !accountsEqual(account, expectedAccount) ||
		account.Status() != identity.AccountStatusEnabled ||
		account.AuthenticationEpoch() != before.AuthenticationEpoch() {
		return dependencyError(identityapp.ErrAccountStateConflict, errStoredRowInvalid)
	}
	current, err := loadSessionByReferenceForUpdate(ctx, tx, before.Reference(), account.ID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dependencyError(identityapp.ErrSessionNotFound, err)
		}
		return classifyRestoreOperationError(ctx, err)
	}
	if !sameImmutableSessionIdentity(current, before) {
		return dependencyError(identityapp.ErrSessionInactive, errStoredRowInvalid)
	}
	txNow, err := repository.currentTime()
	if err != nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	if txNow.Before(current.LastSeenAt()) {
		return storedIdentityInvalid(errClockRegressed)
	}
	active, err := current.ActiveAt(txNow)
	if err != nil {
		return storedIdentityInvalid(err)
	}
	if !active {
		return dependencyError(identityapp.ErrSessionInactive, errStoredRowInvalid)
	}
	revokedAt, reason, operationRef, revoked := after.Revocation()
	if !revoked {
		return dependencyError(identityapp.ErrDependencyInvalidArgument, identityapp.ErrInvalidArgument)
	}
	if revokedAt.Before(current.LastSeenAt()) {
		revokedAt = txNow
	}
	if revokedAt.Before(current.LastSeenAt()) {
		return storedIdentityInvalid(errClockRegressed)
	}
	if _, err := current.Revoke(revokedAt, reason, operationRef); err != nil {
		return storedIdentityInvalid(err)
	}
	if err := writeSessionRevocation(ctx, tx, current, revokedAt, reason, operationRef); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// ObserveSessionCommit performs a bounded, token-free readback by the
// non-secret session reference carried in the defensive receipt.
func (repository *Repository) ObserveSessionCommit(
	ctx context.Context,
	receipt identityapp.SessionCommitReceipt,
) (identityapp.SessionCommitObservation, error) {
	if err := repository.validateCall(ctx); err != nil {
		return identityapp.SessionCommitObservation{}, err
	}
	if err := receipt.Validate(); err != nil {
		return identityapp.SessionCommitObservation{}, dependencyError(
			identityapp.ErrDependencyInvalidArgument,
			err,
		)
	}
	session, err := loadSessionByReference(ctx, repository.database, receipt.After().Reference())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identityapp.ObserveSessionCommitAbsence(), nil
		}
		return identityapp.SessionCommitObservation{}, classifyRestoreOperationError(ctx, err)
	}
	return identityapp.ObserveSessionCommitState(session), nil
}

func (repository *Repository) findSessionAccountID(
	ctx context.Context,
	digest identity.TokenDigest,
) (identity.AccountID, bool, error) {
	var value string
	digestBytes := digest.Bytes()
	defer clearBytes(digestBytes)
	err := repository.database.QueryRowxContext(ctx, selectSessionAccountIDByDigestSQL, digestBytes).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, classifyOperationError(ctx, err)
	}
	accountID, err := identity.NewAccountID(value)
	if err != nil {
		return "", false, storedIdentityInvalid(err)
	}
	return accountID, true, nil
}

func scanSession(scanner rowScanner) (storedSession, error) {
	var row storedSession
	err := scanner.Scan(
		&row.reference,
		&row.issueOperationRef,
		&row.accountID,
		&row.tokenDigest,
		&row.authenticationEpoch,
		&row.issuedAt,
		&row.lastSeenAt,
		&row.idleExpiresAt,
		&row.absoluteExpiresAt,
		&row.revokedAt,
		&row.revokeReason,
		&row.revokeOperationRef,
		&row.updatedAt,
	)
	return row, err
}

func restoreSession(row storedSession) (identity.Session, error) {
	defer clearBytes(row.tokenDigest)
	reference, err := identity.NewSessionRef(row.reference)
	if err != nil {
		return identity.Session{}, err
	}
	issueOperationRef, err := identity.NewOperationRef(row.issueOperationRef)
	if err != nil {
		return identity.Session{}, err
	}
	accountID, err := identity.NewAccountID(row.accountID)
	if err != nil {
		return identity.Session{}, err
	}
	digest, err := identity.NewTokenDigest(row.tokenDigest)
	if err != nil {
		return identity.Session{}, err
	}
	epoch, err := identity.NewAuthenticationEpoch(row.authenticationEpoch)
	if err != nil {
		return identity.Session{}, err
	}
	var revokedAt time.Time
	if row.revokedAt.Valid {
		revokedAt = row.revokedAt.Time
	}
	var reason identity.SessionRevokeReason
	if row.revokeReason.Valid {
		reason = identity.SessionRevokeReason(row.revokeReason.String)
	}
	var revokeOperationRef identity.OperationRef
	if row.revokeOperationRef.Valid {
		revokeOperationRef = identity.OperationRef(row.revokeOperationRef.String)
	}
	session, err := identity.RestoreSession(
		reference,
		issueOperationRef,
		accountID,
		digest,
		epoch,
		row.issuedAt,
		row.lastSeenAt,
		row.idleExpiresAt,
		row.absoluteExpiresAt,
		revokedAt,
		reason,
		revokeOperationRef,
	)
	if err != nil {
		return identity.Session{}, err
	}
	if canonicalTime(row.updatedAt) != row.updatedAt ||
		row.updatedAt.Before(session.IssuedAt()) || row.updatedAt.Before(session.LastSeenAt()) {
		return identity.Session{}, errStoredRowInvalid
	}
	if revokedAt != (time.Time{}) && row.updatedAt.Before(revokedAt) {
		return identity.Session{}, errStoredRowInvalid
	}
	return session, nil
}

func loadSessionByDigest(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	digest identity.TokenDigest,
) (identity.Session, error) {
	digestBytes := digest.Bytes()
	defer clearBytes(digestBytes)
	row, err := scanSession(queryer.QueryRowxContext(ctx, selectSessionByDigestSQL, digestBytes))
	if err != nil {
		return identity.Session{}, err
	}
	session, err := restoreSession(row)
	if err != nil {
		return identity.Session{}, &storedRestoreError{cause: err}
	}
	return session, nil
}

func loadSessionByDigestForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	digest identity.TokenDigest,
	accountID identity.AccountID,
) (identity.Session, error) {
	digestBytes := digest.Bytes()
	defer clearBytes(digestBytes)
	row, err := scanSession(tx.QueryRowxContext(
		ctx,
		selectSessionByDigestForUpdateSQL,
		digestBytes,
		accountID.String(),
	))
	if err != nil {
		return identity.Session{}, err
	}
	session, err := restoreSession(row)
	if err != nil {
		return identity.Session{}, &storedRestoreError{cause: err}
	}
	return session, nil
}

func loadSessionByReference(
	ctx context.Context,
	queryer sqlx.QueryerContext,
	reference identity.SessionRef,
) (identity.Session, error) {
	row, err := scanSession(queryer.QueryRowxContext(ctx, selectSessionByReferenceSQL, reference.String()))
	if err != nil {
		return identity.Session{}, err
	}
	session, err := restoreSession(row)
	if err != nil {
		return identity.Session{}, &storedRestoreError{cause: err}
	}
	return session, nil
}

func loadSessionByReferenceForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	reference identity.SessionRef,
	accountID identity.AccountID,
) (identity.Session, error) {
	row, err := scanSession(tx.QueryRowxContext(
		ctx,
		selectSessionByReferenceForUpdateSQL,
		reference.String(),
		accountID.String(),
	))
	if err != nil {
		return identity.Session{}, err
	}
	session, err := restoreSession(row)
	if err != nil {
		return identity.Session{}, &storedRestoreError{cause: err}
	}
	return session, nil
}

func loadActiveSessionsForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	accountID identity.AccountID,
	epoch identity.AuthenticationEpoch,
	now time.Time,
) ([]identity.Session, error) {
	rows, err := tx.QueryxContext(
		ctx,
		selectActiveSessionsForUpdateSQL,
		accountID.String(),
		uint64(epoch),
		now,
		now,
	)
	if err != nil {
		return nil, classifyOperationError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	sessions := make([]identity.Session, 0, MaximumActiveSessions+1)
	for rows.Next() {
		row, scanErr := scanSession(rows)
		if scanErr != nil {
			return nil, classifyOperationError(ctx, scanErr)
		}
		session, restoreErr := restoreSession(row)
		if restoreErr != nil {
			return nil, storedIdentityInvalid(restoreErr)
		}
		if session.AccountID() != accountID || session.AuthenticationEpoch() != epoch {
			return nil, storedIdentityInvalid(errStoredRowInvalid)
		}
		active, activeErr := session.ActiveAt(now)
		if activeErr != nil || !active {
			if activeErr == nil {
				activeErr = errStoredRowInvalid
			}
			return nil, storedIdentityInvalid(activeErr)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyOperationError(ctx, err)
	}
	if err := rows.Close(); err != nil {
		return nil, classifyOperationError(ctx, err)
	}
	return sessions, nil
}

func derivedRevokeOperationRef(prefix string, issueRef identity.OperationRef) (identity.OperationRef, error) {
	return identity.NewOperationRef(prefix + issueRef.String())
}

func activeSessionIndex(sessions []identity.Session, reference identity.SessionRef) int {
	for index := range sessions {
		if sessions[index].Reference() == reference {
			return index
		}
	}
	return -1
}

func insertSession(
	ctx context.Context,
	tx *sqlx.Tx,
	session identity.Session,
	updatedAt time.Time,
) error {
	digest := session.TokenDigest().Bytes()
	defer clearBytes(digest)
	result, err := tx.ExecContext(
		ctx,
		insertSessionSQL,
		session.Reference().String(),
		session.IssueOperationRef().String(),
		session.AccountID().String(),
		digest,
		uint64(session.AuthenticationEpoch()),
		session.IssuedAt(),
		session.LastSeenAt(),
		session.IdleExpiresAt(),
		session.AbsoluteExpiresAt(),
		updatedAt,
	)
	if err != nil {
		if isDuplicateKey(err, "uq_identity_session_token_digest") {
			// Duplicate-entry values may contain the credential verifier itself.
			// Retain only a fixed diagnostic cause for this stable collision class.
			return dependencyError(identityapp.ErrTokenDigestCollision, errTokenDigestCollision)
		}
		return classifyOperationError(ctx, err)
	}
	if err := requireAffectedRows(result, 1); err != nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	return nil
}

func writeSessionRevocation(
	ctx context.Context,
	tx *sqlx.Tx,
	session identity.Session,
	revokedAt time.Time,
	reason identity.SessionRevokeReason,
	operationRef identity.OperationRef,
) error {
	digest := session.TokenDigest().Bytes()
	defer clearBytes(digest)
	result, err := tx.ExecContext(
		ctx,
		updateSessionRevocationSQL,
		revokedAt,
		string(reason),
		operationRef.String(),
		revokedAt,
		session.Reference().String(),
		session.AccountID().String(),
		session.IssueOperationRef().String(),
		digest,
		uint64(session.AuthenticationEpoch()),
		session.IssuedAt(),
		session.AbsoluteExpiresAt(),
	)
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	if err := requireAffectedRows(result, 1); err != nil {
		return dependencyError(identityapp.ErrSessionInactive, err)
	}
	return nil
}

func writeSessionTouch(
	ctx context.Context,
	tx *sqlx.Tx,
	before identity.Session,
	after identity.Session,
) error {
	digest := before.TokenDigest().Bytes()
	defer clearBytes(digest)
	result, err := tx.ExecContext(
		ctx,
		updateSessionTouchSQL,
		after.LastSeenAt(),
		after.IdleExpiresAt(),
		after.LastSeenAt(),
		before.Reference().String(),
		before.AccountID().String(),
		digest,
		uint64(before.AuthenticationEpoch()),
		before.IssuedAt(),
		before.LastSeenAt(),
		before.IdleExpiresAt(),
		before.AbsoluteExpiresAt(),
	)
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	if err := requireAffectedRows(result, 1); err != nil {
		return dependencyError(identityapp.ErrSessionInactive, err)
	}
	return nil
}

func classifyRestoreOperationError(ctx context.Context, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var dependency *identityapp.DependencyError
	if errors.As(err, &dependency) {
		return err
	}
	var restoreError *storedRestoreError
	if errors.As(err, &restoreError) {
		return storedIdentityInvalid(restoreError.cause)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		(ctx != nil && ctx.Err() != nil) {
		return classifyOperationError(ctx, err)
	}
	return classifyOperationError(ctx, err)
}

func tokenDigestsEqual(left, right identity.TokenDigest) bool {
	leftBytes := left.Bytes()
	rightBytes := right.Bytes()
	defer clearBytes(leftBytes)
	defer clearBytes(rightBytes)
	return digestEqual(leftBytes, rightBytes)
}

func sameImmutableSessionIdentity(left, right identity.Session) bool {
	return left.Reference() == right.Reference() &&
		left.IssueOperationRef() == right.IssueOperationRef() &&
		left.AccountID() == right.AccountID() &&
		tokenDigestsEqual(left.TokenDigest(), right.TokenDigest()) &&
		left.AuthenticationEpoch() == right.AuthenticationEpoch() &&
		left.IssuedAt() == right.IssuedAt() &&
		left.AbsoluteExpiresAt() == right.AbsoluteExpiresAt()
}
