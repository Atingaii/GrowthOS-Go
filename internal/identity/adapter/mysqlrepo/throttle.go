package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/jmoiron/sqlx"
)

const (
	ensureThrottleSQL = `
		INSERT INTO identity_authentication_throttle
			(dimension, subject_digest, window_started_at, window_expires_at,
			 failure_count, inflight_count, admission_epoch, inflight_expires_at,
			 blocked_until, updated_at, row_expires_at)
		VALUES (?, ?, ?, ?, 0, 0, 1, NULL, NULL, ?, ?)
		ON DUPLICATE KEY UPDATE dimension = VALUES(dimension)`
	selectThrottleForUpdateSQL = `
		SELECT dimension, subject_digest, window_started_at, window_expires_at,
		       failure_count, inflight_count, admission_epoch,
		       inflight_expires_at, blocked_until, updated_at, row_expires_at
		FROM identity_authentication_throttle
		WHERE dimension = ? AND subject_digest = ?
		FOR UPDATE`
	updateThrottleSQL = `
		UPDATE identity_authentication_throttle
		SET window_started_at = ?, window_expires_at = ?, failure_count = ?,
		    inflight_count = ?, admission_epoch = ?, inflight_expires_at = ?,
		    blocked_until = ?, updated_at = ?, row_expires_at = ?
		WHERE dimension = ? AND subject_digest = ?
		  AND admission_epoch = ? AND inflight_count = ? AND updated_at = ?`
)

type throttleKey struct {
	dimension identity.ThrottleDimension
	digest    identity.ThrottleDigest
}

type storedThrottle struct {
	dimension         string
	digest            []byte
	windowStartedAt   time.Time
	windowExpiresAt   time.Time
	failureCount      uint64
	inflightCount     uint64
	admissionEpoch    uint64
	inflightExpiresAt sql.NullTime
	blockedUntil      sql.NullTime
	updatedAt         time.Time
	rowExpiresAt      time.Time
}

type throttleRecord struct {
	state        identity.ThrottleState
	updatedAt    time.Time
	rowExpiresAt time.Time
}

// BeginAdmission creates and locks the login row before the source row. The
// two reservations are written and committed atomically; no transaction or
// connection survives this method.
func (repository *Repository) BeginAdmission(
	ctx context.Context,
	request identityapp.AdmissionRequest,
) (identityapp.AdmissionGrant, error) {
	if err := repository.validateCall(ctx); err != nil {
		return identityapp.AdmissionGrant{}, err
	}
	if err := request.Validate(); err != nil {
		return identityapp.AdmissionGrant{}, dependencyError(
			identityapp.ErrDependencyInvalidArgument,
			err,
		)
	}
	keys := admissionRequestKeys(request)
	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return identityapp.AdmissionGrant{}, classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert and lock in the same canonical order. Insert-before-select closes
	// the missing-row race without holding a transaction during Argon2 work.
	for _, key := range keys {
		if err := ensureThrottleRow(ctx, tx, key, request); err != nil {
			return identityapp.AdmissionGrant{}, err
		}
	}

	originals := make([]throttleRecord, 0, len(keys))
	for _, key := range keys {
		record, err := loadThrottleForUpdate(ctx, tx, key)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return identityapp.AdmissionGrant{}, storedIdentityInvalid(errStoredRowInvalid)
			}
			return identityapp.AdmissionGrant{}, err
		}
		originals = append(originals, record)
	}
	lockedAt, err := repository.currentTime()
	if err != nil {
		return identityapp.AdmissionGrant{}, dependencyError(identityapp.ErrDependencyUnavailable, err)
	}

	// Recovery fencing is normalization, not admission. Normalize both locked
	// rows first so an expired old batch is durably fenced even when the other
	// dimension ultimately rejects the request.
	normalized := make([]throttleRecord, 0, len(keys))
	normalizationChanged := false
	for index, key := range keys {
		next, changed, err := normalizeThrottle(originals[index], key, request)
		if err != nil {
			return identityapp.AdmissionGrant{}, err
		}
		normalized = append(normalized, next)
		normalizationChanged = normalizationChanged || changed
	}
	if !lockedAt.Before(request.Deadline()) {
		if normalizationChanged {
			if err := commitThrottleNormalization(ctx, tx, keys, normalized, originals); err != nil {
				return identityapp.AdmissionGrant{}, err
			}
		}
		return identityapp.AdmissionGrant{}, dependencyError(
			identityapp.ErrAdmissionStale,
			errAdmissionReceiptStale,
		)
	}

	admitted := true
	for index, key := range keys {
		allowed, err := throttleAllowsReservation(normalized[index], key, request.Policy(), request.AdmittedAt())
		if err != nil {
			return identityapp.AdmissionGrant{}, err
		}
		admitted = admitted && allowed
	}
	if !admitted {
		if normalizationChanged {
			if err := commitThrottleNormalization(ctx, tx, keys, normalized, originals); err != nil {
				return identityapp.AdmissionGrant{}, err
			}
		}
		return identityapp.AdmissionGrant{}, dependencyError(
			identityapp.ErrAdmissionRejected,
			errAdmissionCapacity,
		)
	}

	reserved := make([]throttleRecord, 0, len(keys))
	for index, key := range keys {
		next, err := addThrottleReservation(normalized[index], key, request)
		if err != nil {
			return identityapp.AdmissionGrant{}, err
		}
		reserved = append(reserved, next)
	}
	for index := range keys {
		if err := saveThrottle(ctx, tx, keys[index], reserved[index], originals[index]); err != nil {
			return identityapp.AdmissionGrant{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return identityapp.AdmissionGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return identityapp.AdmissionGrant{}, classifyWriteCommitError(ctx, err)
	}
	grant, err := identityapp.NewAdmissionGrant(
		reserved[0].state.AdmissionEpoch(),
		reserved[1].state.AdmissionEpoch(),
		request.Deadline(),
	)
	if err != nil {
		return identityapp.AdmissionGrant{}, storedIdentityInvalid(err)
	}
	return grant, nil
}

func commitThrottleNormalization(
	ctx context.Context,
	tx *sqlx.Tx,
	keys []throttleKey,
	normalized []throttleRecord,
	originals []throttleRecord,
) error {
	for index := range keys {
		if throttleRecordsEqual(originals[index], normalized[index]) {
			continue
		}
		if err := saveThrottle(ctx, tx, keys[index], normalized[index], originals[index]); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

// FinalizeAdmission locks the exact same two rows in the exact same order. A
// stale epoch, elapsed individual lease, missing row, or empty inflight batch
// rejects the whole finalization without a partial decrement.
func (repository *Repository) FinalizeAdmission(
	ctx context.Context,
	receipt identityapp.AdmissionReceipt,
	outcome identityapp.AdmissionFinalOutcome,
) error {
	if err := repository.validateCall(ctx); err != nil {
		return err
	}
	if err := receipt.Validate(); err != nil || !outcome.Valid() {
		if err == nil {
			err = identityapp.ErrInvalidArgument
		}
		return dependencyError(identityapp.ErrDependencyInvalidArgument, err)
	}
	now, err := repository.currentTime()
	if err != nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	if !now.Before(receipt.Deadline()) {
		return dependencyError(identityapp.ErrAdmissionStale, errAdmissionReceiptStale)
	}
	keys := admissionReceiptKeys(receipt)
	epochs := []identity.AdmissionEpoch{receipt.LoginEpoch(), receipt.SourceEpoch()}
	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()

	originals := make([]throttleRecord, 0, len(keys))
	for _, key := range keys {
		record, err := loadThrottleForUpdate(ctx, tx, key)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return dependencyError(identityapp.ErrAdmissionStale, err)
			}
			return err
		}
		originals = append(originals, record)
	}
	// The receipt may cross its lease boundary while waiting for either row
	// lock. Re-read the controlled clock only after both locks are held and
	// reject before the first UPDATE when the individual deadline elapsed.
	now, err = repository.currentTime()
	if err != nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	if !now.Before(receipt.Deadline()) {
		return dependencyError(identityapp.ErrAdmissionStale, errAdmissionReceiptStale)
	}
	nextRecords := make([]throttleRecord, 0, len(keys))
	for index, key := range keys {
		next, err := finalizeThrottle(
			originals[index],
			key,
			epochs[index],
			receipt.Deadline(),
			outcome,
			now,
			identityapp.V1AdmissionPolicy(),
		)
		if err != nil {
			return err
		}
		nextRecords = append(nextRecords, next)
	}
	for index := range keys {
		if err := saveThrottle(ctx, tx, keys[index], nextRecords[index], originals[index]); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classifyWriteCommitError(ctx, err)
	}
	return nil
}

func admissionRequestKeys(request identityapp.AdmissionRequest) []throttleKey {
	return []throttleKey{
		{dimension: identity.ThrottleDimensionLogin, digest: request.LoginDigest()},
		{dimension: identity.ThrottleDimensionSource, digest: request.SourceDigest()},
	}
}

func admissionReceiptKeys(receipt identityapp.AdmissionReceipt) []throttleKey {
	return []throttleKey{
		{dimension: identity.ThrottleDimensionLogin, digest: receipt.LoginDigest()},
		{dimension: identity.ThrottleDimensionSource, digest: receipt.SourceDigest()},
	}
}

func ensureThrottleRow(
	ctx context.Context,
	tx *sqlx.Tx,
	key throttleKey,
	request identityapp.AdmissionRequest,
) error {
	windowExpiresAt := request.AdmittedAt().Add(request.Policy().ObservationWindow())
	rowExpiresAt := request.AdmittedAt().Add(ThrottleRowRetention)
	digest := key.digest.Bytes()
	defer clearBytes(digest)
	result, err := tx.ExecContext(
		ctx,
		ensureThrottleSQL,
		string(key.dimension),
		digest,
		request.AdmittedAt(),
		windowExpiresAt,
		request.AdmittedAt(),
		rowExpiresAt,
	)
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	if result == nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, errUnexpectedAffectedRows)
	}
	return nil
}

func loadThrottleForUpdate(
	ctx context.Context,
	tx *sqlx.Tx,
	key throttleKey,
) (throttleRecord, error) {
	var row storedThrottle
	digest := key.digest.Bytes()
	defer clearBytes(digest)
	err := tx.QueryRowxContext(
		ctx,
		selectThrottleForUpdateSQL,
		string(key.dimension),
		digest,
	).Scan(
		&row.dimension,
		&row.digest,
		&row.windowStartedAt,
		&row.windowExpiresAt,
		&row.failureCount,
		&row.inflightCount,
		&row.admissionEpoch,
		&row.inflightExpiresAt,
		&row.blockedUntil,
		&row.updatedAt,
		&row.rowExpiresAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return throttleRecord{}, err
		}
		return throttleRecord{}, classifyOperationError(ctx, err)
	}
	defer clearBytes(row.digest)
	record, err := restoreThrottle(row)
	if err != nil {
		return throttleRecord{}, storedIdentityInvalid(err)
	}
	restoredDigest := record.state.Digest().Bytes()
	defer clearBytes(restoredDigest)
	if record.state.Dimension() != key.dimension || !digestEqual(restoredDigest, digest) {
		return throttleRecord{}, storedIdentityInvalid(errStoredRowInvalid)
	}
	return record, nil
}

func restoreThrottle(row storedThrottle) (throttleRecord, error) {
	if row.failureCount > math.MaxUint32 || row.inflightCount > math.MaxUint32 {
		return throttleRecord{}, errStoredRowInvalid
	}
	digest, err := identity.NewThrottleDigest(row.digest)
	if err != nil {
		return throttleRecord{}, err
	}
	epoch, err := identity.NewAdmissionEpoch(row.admissionEpoch)
	if err != nil {
		return throttleRecord{}, err
	}
	var inflightExpiresAt time.Time
	if row.inflightExpiresAt.Valid {
		inflightExpiresAt = row.inflightExpiresAt.Time
	}
	var blockedUntil time.Time
	if row.blockedUntil.Valid {
		blockedUntil = row.blockedUntil.Time
	}
	state, err := identity.NewThrottleState(
		identity.ThrottleDimension(row.dimension),
		digest,
		row.windowStartedAt,
		row.windowExpiresAt,
		uint32(row.failureCount),
		uint32(row.inflightCount),
		epoch,
		inflightExpiresAt,
		blockedUntil,
	)
	if err != nil {
		return throttleRecord{}, err
	}
	if canonicalTime(row.updatedAt) != row.updatedAt ||
		canonicalTime(row.rowExpiresAt) != row.rowExpiresAt ||
		row.updatedAt.Before(state.WindowStartedAt()) ||
		!row.updatedAt.Before(row.rowExpiresAt) ||
		row.rowExpiresAt.Before(state.WindowExpiresAt()) ||
		(row.inflightExpiresAt.Valid && row.rowExpiresAt.Before(row.inflightExpiresAt.Time)) ||
		(row.blockedUntil.Valid && row.rowExpiresAt.Before(row.blockedUntil.Time)) {
		return throttleRecord{}, errStoredRowInvalid
	}
	return throttleRecord{state: state, updatedAt: row.updatedAt, rowExpiresAt: row.rowExpiresAt}, nil
}

func normalizeThrottle(
	record throttleRecord,
	key throttleKey,
	request identityapp.AdmissionRequest,
) (throttleRecord, bool, error) {
	now := request.AdmittedAt()
	policy := request.Policy()
	if now.Before(record.updatedAt) || now.Before(record.state.WindowStartedAt()) {
		return throttleRecord{}, false, storedIdentityInvalid(errClockRegressed)
	}
	state := record.state
	recoveredBatch := false
	if expired, err := state.InflightExpiredAt(now); err != nil {
		return throttleRecord{}, false, storedIdentityInvalid(err)
	} else if expired {
		recovered, didRecover, recoverErr := state.RecoverExpiredInflight(now)
		if recoverErr != nil || !didRecover {
			if recoverErr == nil {
				recoverErr = errStoredRowInvalid
			}
			return throttleRecord{}, false, storedIdentityInvalid(recoverErr)
		}
		state = recovered
		recoveredBatch = true
	}
	windowActive, err := state.WindowActiveAt(now)
	if err != nil {
		return throttleRecord{}, false, storedIdentityInvalid(err)
	}
	if !windowActive {
		if state.InflightCount() != 0 {
			return record, false, nil
		}
		epoch := state.AdmissionEpoch()
		if !recoveredBatch {
			epoch, err = incrementAdmissionEpoch(epoch)
			if err != nil {
				return throttleRecord{}, false, storedIdentityInvalid(err)
			}
		}
		state, err = identity.NewThrottleState(
			key.dimension,
			key.digest,
			now,
			now.Add(policy.ObservationWindow()),
			0,
			0,
			epoch,
			time.Time{},
			time.Time{},
		)
		if err != nil {
			return throttleRecord{}, false, storedIdentityInvalid(err)
		}
	}
	normalized := throttleRecord{
		state:        state,
		updatedAt:    record.updatedAt,
		rowExpiresAt: record.rowExpiresAt,
	}
	if !throttleRecordsEqual(record, normalized) {
		normalized.updatedAt = now
		normalized.rowExpiresAt = maxTime(
			record.rowExpiresAt,
			now.Add(ThrottleRowRetention),
			state.WindowExpiresAt(),
		)
		if expiry, ok := state.InflightExpiresAt(); ok {
			normalized.rowExpiresAt = maxTime(normalized.rowExpiresAt, expiry)
		}
		if blockedUntil, ok := state.BlockedUntil(); ok {
			normalized.rowExpiresAt = maxTime(normalized.rowExpiresAt, blockedUntil)
		}
	}
	return normalized, !throttleRecordsEqual(record, normalized), nil
}

func throttleAllowsReservation(
	record throttleRecord,
	key throttleKey,
	policy identityapp.AdmissionPolicy,
	now time.Time,
) (bool, error) {
	state := record.state
	windowActive, err := state.WindowActiveAt(now)
	if err != nil {
		return false, storedIdentityInvalid(err)
	}
	if !windowActive {
		return false, nil
	}
	threshold, ok := policy.FailureThreshold(key.dimension)
	if !ok || threshold == 0 {
		return false, dependencyError(identityapp.ErrDependencyInvalidArgument, errStoredRowInvalid)
	}
	blockedUntil, hasBlock := state.BlockedUntil()
	if state.FailureCount() < threshold {
		if hasBlock {
			return false, storedIdentityInvalid(errStoredRowInvalid)
		}
		if uint64(state.FailureCount())+uint64(state.InflightCount()) >= uint64(threshold) {
			return false, nil
		}
	} else {
		if !hasBlock {
			return false, storedIdentityInvalid(errStoredRowInvalid)
		}
		if now.Before(blockedUntil) || state.InflightCount() != 0 {
			return false, nil
		}
	}
	if state.InflightCount() == math.MaxUint32 ||
		uint64(state.FailureCount())+uint64(state.InflightCount()) >= identity.MaxThrottleAggregateCount {
		return false, nil
	}
	return true, nil
}

func addThrottleReservation(
	record throttleRecord,
	key throttleKey,
	request identityapp.AdmissionRequest,
) (throttleRecord, error) {
	state := record.state
	blockedUntil, _ := state.BlockedUntil()
	inflightExpiresAt, hasInflight := state.InflightExpiresAt()
	if !hasInflight || inflightExpiresAt.Before(request.Deadline()) {
		inflightExpiresAt = request.Deadline()
	}
	nextState, err := identity.NewThrottleState(
		key.dimension,
		key.digest,
		state.WindowStartedAt(),
		state.WindowExpiresAt(),
		state.FailureCount(),
		state.InflightCount()+1,
		state.AdmissionEpoch(),
		inflightExpiresAt,
		blockedUntil,
	)
	if err != nil {
		return throttleRecord{}, storedIdentityInvalid(err)
	}
	return throttleRecord{
		state:        nextState,
		updatedAt:    request.AdmittedAt(),
		rowExpiresAt: maxTime(record.rowExpiresAt, request.AdmittedAt().Add(ThrottleRowRetention), inflightExpiresAt),
	}, nil
}

func throttleRecordsEqual(left, right throttleRecord) bool {
	return left.state == right.state && left.updatedAt == right.updatedAt &&
		left.rowExpiresAt == right.rowExpiresAt
}

func finalizeThrottle(
	record throttleRecord,
	key throttleKey,
	expectedEpoch identity.AdmissionEpoch,
	receiptDeadline time.Time,
	outcome identityapp.AdmissionFinalOutcome,
	now time.Time,
	policy identityapp.AdmissionPolicy,
) (throttleRecord, error) {
	state := record.state
	if now.Before(record.updatedAt) || now.Before(state.WindowStartedAt()) {
		return throttleRecord{}, storedIdentityInvalid(errClockRegressed)
	}
	inflightExpiresAt, hasInflight := state.InflightExpiresAt()
	if state.AdmissionEpoch() != expectedEpoch || state.InflightCount() == 0 ||
		!hasInflight || inflightExpiresAt.Before(receiptDeadline) {
		return throttleRecord{}, dependencyError(identityapp.ErrAdmissionStale, errAdmissionReceiptStale)
	}
	threshold, ok := policy.FailureThreshold(key.dimension)
	if !ok || threshold == 0 {
		return throttleRecord{}, dependencyError(identityapp.ErrDependencyInvalidArgument, errStoredRowInvalid)
	}

	windowStartedAt := state.WindowStartedAt()
	windowExpiresAt := state.WindowExpiresAt()
	failureCount := state.FailureCount()
	blockedUntil, _ := state.BlockedUntil()
	if !now.Before(windowExpiresAt) {
		windowStartedAt = now
		windowExpiresAt = now.Add(policy.ObservationWindow())
		failureCount = 0
		blockedUntil = time.Time{}
	}

	inflightCount := state.InflightCount() - 1
	epoch := state.AdmissionEpoch()
	if inflightCount == 0 {
		var err error
		epoch, err = incrementAdmissionEpoch(epoch)
		if err != nil {
			return throttleRecord{}, storedIdentityInvalid(err)
		}
		inflightExpiresAt = time.Time{}
	}

	switch outcome {
	case identityapp.AdmissionFinalOutcomeFailure:
		if failureCount == math.MaxUint32 {
			return throttleRecord{}, storedIdentityInvalid(errStoredRowInvalid)
		}
		failureCount++
		windowExpiresAt = now.Add(policy.ObservationWindow())
		if failureCount >= threshold {
			blockedUntil = now.Add(authenticationBackoff(
				failureCount,
				threshold,
				policy.InitialBackoff(),
				policy.MaximumBackoff(),
			))
			if windowExpiresAt.Before(blockedUntil) {
				blockedUntil = windowExpiresAt
			}
		} else {
			blockedUntil = time.Time{}
		}
	case identityapp.AdmissionFinalOutcomeSuccess:
		if key.dimension == identity.ThrottleDimensionLogin {
			windowStartedAt = now
			windowExpiresAt = now.Add(policy.ObservationWindow())
			failureCount = 0
			blockedUntil = time.Time{}
		} else if failureCount >= threshold {
			// A successful credential must not wash the shared source budget.
			// Re-arm the same failure-count backoff from this probe completion;
			// otherwise an expired timestamp would degrade source throttling to
			// mere single-flight and a known-good account could accelerate hashes.
			blockedUntil = now.Add(authenticationBackoff(
				failureCount,
				threshold,
				policy.InitialBackoff(),
				policy.MaximumBackoff(),
			))
			if windowExpiresAt.Before(blockedUntil) {
				blockedUntil = windowExpiresAt
			}
		}
	case identityapp.AdmissionFinalOutcomeNeutral:
		// The reservation is released without changing credential evidence.
	default:
		return throttleRecord{}, dependencyError(identityapp.ErrDependencyInvalidArgument, identityapp.ErrInvalidArgument)
	}
	nextState, err := identity.NewThrottleState(
		key.dimension,
		key.digest,
		windowStartedAt,
		windowExpiresAt,
		failureCount,
		inflightCount,
		epoch,
		inflightExpiresAt,
		blockedUntil,
	)
	if err != nil {
		return throttleRecord{}, storedIdentityInvalid(err)
	}
	return throttleRecord{
		state:        nextState,
		updatedAt:    now,
		rowExpiresAt: maxTime(record.rowExpiresAt, now.Add(ThrottleRowRetention), windowExpiresAt, blockedUntil, inflightExpiresAt),
	}, nil
}

func incrementAdmissionEpoch(epoch identity.AdmissionEpoch) (identity.AdmissionEpoch, error) {
	if uint64(epoch) == math.MaxUint64 {
		return 0, errEpochExhausted
	}
	return identity.NewAdmissionEpoch(uint64(epoch) + 1)
}

func authenticationBackoff(
	failureCount uint32,
	threshold uint32,
	initial time.Duration,
	maximum time.Duration,
) time.Duration {
	if failureCount <= threshold {
		return initial
	}
	backoff := initial
	steps := failureCount - threshold
	for steps > 0 && backoff < maximum {
		if backoff > maximum/2 {
			return maximum
		}
		backoff *= 2
		steps--
	}
	if backoff > maximum {
		return maximum
	}
	return backoff
}

func saveThrottle(
	ctx context.Context,
	tx *sqlx.Tx,
	key throttleKey,
	next throttleRecord,
	before throttleRecord,
) error {
	inflightExpiresAt, _ := next.state.InflightExpiresAt()
	blockedUntil, _ := next.state.BlockedUntil()
	digest := key.digest.Bytes()
	defer clearBytes(digest)
	result, err := tx.ExecContext(
		ctx,
		updateThrottleSQL,
		next.state.WindowStartedAt(),
		next.state.WindowExpiresAt(),
		next.state.FailureCount(),
		next.state.InflightCount(),
		uint64(next.state.AdmissionEpoch()),
		nullTime(inflightExpiresAt),
		nullTime(blockedUntil),
		next.updatedAt,
		next.rowExpiresAt,
		string(key.dimension),
		digest,
		uint64(before.state.AdmissionEpoch()),
		before.state.InflightCount(),
		before.updatedAt,
	)
	if err != nil {
		return classifyOperationError(ctx, err)
	}
	if err := requireAffectedRows(result, 1); err != nil {
		return dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	return nil
}
