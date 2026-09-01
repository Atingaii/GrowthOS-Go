package mysqlrepo

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/jmoiron/sqlx"
)

const (
	selectExpiredSessionMaintenanceCandidatesSQL = `
		SELECT session_ref, absolute_expires_at AS cleanup_at
		FROM identity_session USE INDEX (idx_identity_session_absolute_cleanup)
		WHERE absolute_expires_at <= ?
		ORDER BY absolute_expires_at ASC, session_ref ASC
		LIMIT ?`
	selectRevokedSessionMaintenanceCandidatesSQL = `
		SELECT session_ref, revoked_at AS cleanup_at
		FROM identity_session USE INDEX (idx_identity_session_revoked_cleanup)
		WHERE revoked_at IS NOT NULL AND revoked_at <= ?
		ORDER BY revoked_at ASC, session_ref ASC
		LIMIT ?`
	deleteEligibleSessionMaintenancePrefixSQL = `
		DELETE FROM identity_session
		WHERE session_ref IN (`
	deleteEligibleSessionMaintenanceSuffixSQL = `)
		  AND (
			absolute_expires_at <= ?
			OR (revoked_at IS NOT NULL AND revoked_at <= ?)
		  )`
	selectThrottleMaintenanceCandidatesSQL = `
		SELECT dimension, subject_digest, row_expires_at
		FROM identity_authentication_throttle
		USE INDEX (idx_identity_throttle_cleanup)
		WHERE row_expires_at <= ?
		  AND inflight_count = 0
		  AND inflight_expires_at IS NULL
		ORDER BY row_expires_at ASC, dimension ASC, subject_digest ASC
		LIMIT ?`
	deleteEligibleThrottleMaintenancePrefixSQL = `
		DELETE FROM identity_authentication_throttle
		WHERE (dimension, subject_digest) IN (`
	deleteEligibleThrottleMaintenanceSuffixSQL = `)
		  AND row_expires_at <= ?
		  AND inflight_count = 0
		  AND inflight_expires_at IS NULL`
)

var _ identityapp.MaintenanceRepository = (*Repository)(nil)

type sessionMaintenanceCandidate struct {
	reference string
	cleanupAt time.Time
}

type throttleMaintenanceCandidate struct {
	dimension string
	digest    []byte
	cleanupAt time.Time
}

// RunMaintenance executes two independently bounded transactions. The fixed
// 250/250 allocation is intentionally not work-conserving: lending an empty
// table's budget would allow a permanently busy table to starve the other.
// The session transaction always runs first; any failure or unknown COMMIT
// acknowledgement stops the operation before throttle maintenance.
func (repository *Repository) RunMaintenance(
	ctx context.Context,
	operation identityapp.MaintenanceOperation,
) (identityapp.MaintenanceResult, error) {
	if err := repository.validateCall(ctx); err != nil {
		return identityapp.MaintenanceResult{}, err
	}
	if operation.Validate() != nil {
		return identityapp.MaintenanceResult{}, dependencyError(
			identityapp.ErrDependencyInvalidArgument,
			identityapp.ErrInvalidArgument,
		)
	}
	sessionsDeleted, err := repository.cleanupSessionHistory(ctx, operation)
	if err != nil {
		return identityapp.MaintenanceResult{}, err
	}
	throttlesDeleted, err := repository.cleanupInactiveThrottle(ctx, operation)
	if err != nil {
		return identityapp.MaintenanceResult{}, err
	}
	result, err := identityapp.NewMaintenanceResult(sessionsDeleted, throttlesDeleted)
	if err != nil {
		return identityapp.MaintenanceResult{}, storedIdentityInvalid(err)
	}
	return result, nil
}

func (repository *Repository) cleanupSessionHistory(
	ctx context.Context,
	operation identityapp.MaintenanceOperation,
) (int, error) {
	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return 0, classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()

	expired, err := selectSessionMaintenanceCandidates(
		ctx,
		tx,
		selectExpiredSessionMaintenanceCandidatesSQL,
		operation.SessionCutoff(),
		operation.SessionBudget(),
	)
	if err != nil {
		return 0, err
	}
	revoked, err := selectSessionMaintenanceCandidates(
		ctx,
		tx,
		selectRevokedSessionMaintenanceCandidatesSQL,
		operation.SessionCutoff(),
		operation.SessionBudget(),
	)
	if err != nil {
		return 0, err
	}
	candidates, err := mergeSessionMaintenanceCandidates(
		expired,
		revoked,
		operation.SessionBudget(),
	)
	if err != nil {
		return 0, storedIdentityInvalid(err)
	}
	if len(candidates) == 0 {
		if err := tx.Rollback(); err != nil {
			return 0, classifyOperationError(ctx, err)
		}
		return 0, nil
	}
	query, err := buildSessionMaintenanceDeleteSQL(len(candidates))
	if err != nil {
		return 0, dependencyError(identityapp.ErrDependencyInvalidArgument, err)
	}
	arguments := make([]any, 0, len(candidates)+2)
	for _, candidate := range candidates {
		arguments = append(arguments, candidate.reference)
	}
	arguments = append(arguments, operation.SessionCutoff(), operation.SessionCutoff())
	result, err := tx.ExecContext(ctx, query, arguments...)
	if err != nil {
		return 0, classifyOperationError(ctx, err)
	}
	if err := requireAffectedRows(result, int64(len(candidates))); err != nil {
		return 0, dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, classifyWriteCommitError(ctx, err)
	}
	return len(candidates), nil
}

func selectSessionMaintenanceCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	query string,
	cutoff time.Time,
	limit int,
) ([]sessionMaintenanceCandidate, error) {
	if ctx == nil || tx == nil || cutoff.IsZero() || canonicalTime(cutoff) != cutoff ||
		limit <= 0 || limit > identityapp.MaintenanceSessionBudget {
		return nil, dependencyError(identityapp.ErrDependencyInvalidArgument, identityapp.ErrInvalidArgument)
	}
	rows, err := tx.QueryxContext(ctx, query, cutoff, limit)
	if err != nil {
		return nil, classifyOperationError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]sessionMaintenanceCandidate, 0, limit)
	for rows.Next() {
		if len(candidates) >= limit {
			return nil, storedIdentityInvalid(errMaintenanceCandidateOverflow)
		}
		var candidate sessionMaintenanceCandidate
		if err := rows.Scan(&candidate.reference, &candidate.cleanupAt); err != nil {
			return nil, classifyOperationError(ctx, err)
		}
		storedCleanupAt := candidate.cleanupAt
		candidate.cleanupAt = canonicalTime(storedCleanupAt)
		if reference, err := identity.NewSessionRef(candidate.reference); err != nil ||
			reference.String() != candidate.reference || candidate.cleanupAt.IsZero() ||
			candidate.cleanupAt != storedCleanupAt || candidate.cleanupAt.After(cutoff) {
			return nil, storedIdentityInvalid(errMaintenanceCandidateInvalid)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyOperationError(ctx, err)
	}
	return candidates, nil
}

func mergeSessionMaintenanceCandidates(
	expired []sessionMaintenanceCandidate,
	revoked []sessionMaintenanceCandidate,
	limit int,
) ([]sessionMaintenanceCandidate, error) {
	if limit <= 0 || limit > identityapp.MaintenanceSessionBudget ||
		len(expired) > limit || len(revoked) > limit {
		return nil, errMaintenanceCandidateOverflow
	}
	byReference := make(map[string]sessionMaintenanceCandidate, len(expired)+len(revoked))
	merge := func(candidate sessionMaintenanceCandidate) error {
		if candidate.reference == "" || candidate.cleanupAt.IsZero() {
			return errMaintenanceCandidateInvalid
		}
		current, exists := byReference[candidate.reference]
		if !exists || candidate.cleanupAt.Before(current.cleanupAt) {
			byReference[candidate.reference] = candidate
		}
		return nil
	}
	for _, candidate := range expired {
		if err := merge(candidate); err != nil {
			return nil, err
		}
	}
	for _, candidate := range revoked {
		if err := merge(candidate); err != nil {
			return nil, err
		}
	}
	merged := make([]sessionMaintenanceCandidate, 0, minInt(len(byReference), limit))
	for _, candidate := range byReference {
		merged = append(merged, candidate)
	}
	sort.Slice(merged, func(left, right int) bool {
		if merged[left].cleanupAt.Equal(merged[right].cleanupAt) {
			return merged[left].reference < merged[right].reference
		}
		return merged[left].cleanupAt.Before(merged[right].cleanupAt)
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

func (repository *Repository) cleanupInactiveThrottle(
	ctx context.Context,
	operation identityapp.MaintenanceOperation,
) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	tx, err := repository.database.BeginTxx(ctx, writeTxOptions())
	if err != nil {
		return 0, classifyOperationError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()
	candidates, err := selectThrottleMaintenanceCandidates(
		ctx,
		tx,
		operation.ObservedAt(),
		operation.ThrottleBudget(),
	)
	if err != nil {
		return 0, err
	}
	defer clearThrottleMaintenanceCandidates(candidates)
	if len(candidates) == 0 {
		if err := tx.Rollback(); err != nil {
			return 0, classifyOperationError(ctx, err)
		}
		return 0, nil
	}
	query, err := buildThrottleMaintenanceDeleteSQL(len(candidates))
	if err != nil {
		return 0, dependencyError(identityapp.ErrDependencyInvalidArgument, err)
	}
	arguments := make([]any, 0, len(candidates)*2+1)
	for _, candidate := range candidates {
		arguments = append(arguments, candidate.dimension, candidate.digest)
	}
	arguments = append(arguments, operation.ObservedAt())
	result, err := tx.ExecContext(ctx, query, arguments...)
	if err != nil {
		return 0, classifyOperationError(ctx, err)
	}
	if err := requireAffectedRows(result, int64(len(candidates))); err != nil {
		return 0, dependencyError(identityapp.ErrDependencyUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, classifyWriteCommitError(ctx, err)
	}
	return len(candidates), nil
}

func selectThrottleMaintenanceCandidates(
	ctx context.Context,
	tx *sqlx.Tx,
	cutoff time.Time,
	limit int,
) ([]throttleMaintenanceCandidate, error) {
	if ctx == nil || tx == nil || cutoff.IsZero() || canonicalTime(cutoff) != cutoff ||
		limit <= 0 || limit > identityapp.MaintenanceThrottleBudget {
		return nil, dependencyError(identityapp.ErrDependencyInvalidArgument, identityapp.ErrInvalidArgument)
	}
	rows, err := tx.QueryxContext(ctx, selectThrottleMaintenanceCandidatesSQL, cutoff, limit)
	if err != nil {
		return nil, classifyOperationError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	candidates := make([]throttleMaintenanceCandidate, 0, limit)
	for rows.Next() {
		if len(candidates) >= limit {
			clearThrottleMaintenanceCandidates(candidates)
			return nil, storedIdentityInvalid(errMaintenanceCandidateOverflow)
		}
		var candidate throttleMaintenanceCandidate
		if err := rows.Scan(&candidate.dimension, &candidate.digest, &candidate.cleanupAt); err != nil {
			clearBytes(candidate.digest)
			clearThrottleMaintenanceCandidates(candidates)
			return nil, classifyOperationError(ctx, err)
		}
		storedCleanupAt := candidate.cleanupAt
		candidate.cleanupAt = canonicalTime(storedCleanupAt)
		dimension := identity.ThrottleDimension(candidate.dimension)
		_, digestErr := identity.NewThrottleDigest(candidate.digest)
		if !dimension.Valid() || digestErr != nil || candidate.cleanupAt.IsZero() ||
			candidate.cleanupAt != storedCleanupAt ||
			candidate.cleanupAt.After(cutoff) {
			clearBytes(candidate.digest)
			clearThrottleMaintenanceCandidates(candidates)
			return nil, storedIdentityInvalid(errMaintenanceCandidateInvalid)
		}
		rawDigest := candidate.digest
		candidate.digest = append([]byte(nil), rawDigest...)
		clearBytes(rawDigest)
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		clearThrottleMaintenanceCandidates(candidates)
		return nil, classifyOperationError(ctx, err)
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].cleanupAt.Equal(candidates[right].cleanupAt) {
			if candidates[left].dimension == candidates[right].dimension {
				return bytes.Compare(candidates[left].digest, candidates[right].digest) < 0
			}
			return candidates[left].dimension < candidates[right].dimension
		}
		return candidates[left].cleanupAt.Before(candidates[right].cleanupAt)
	})
	for index := 1; index < len(candidates); index++ {
		if candidates[index-1].dimension == candidates[index].dimension &&
			bytes.Equal(candidates[index-1].digest, candidates[index].digest) {
			clearThrottleMaintenanceCandidates(candidates)
			return nil, storedIdentityInvalid(errMaintenanceCandidateInvalid)
		}
	}
	return candidates, nil
}

func buildSessionMaintenanceDeleteSQL(count int) (string, error) {
	placeholders, err := boundedMaintenancePlaceholders(
		count,
		identityapp.MaintenanceSessionBudget,
		"?",
	)
	if err != nil {
		return "", err
	}
	return deleteEligibleSessionMaintenancePrefixSQL + placeholders +
		deleteEligibleSessionMaintenanceSuffixSQL, nil
}

func buildThrottleMaintenanceDeleteSQL(count int) (string, error) {
	placeholders, err := boundedMaintenancePlaceholders(
		count,
		identityapp.MaintenanceThrottleBudget,
		"(?, ?)",
	)
	if err != nil {
		return "", err
	}
	return deleteEligibleThrottleMaintenancePrefixSQL + placeholders +
		deleteEligibleThrottleMaintenanceSuffixSQL, nil
}

func boundedMaintenancePlaceholders(count, maximum int, placeholder string) (string, error) {
	if count <= 0 || maximum <= 0 || count > maximum || placeholder == "" {
		return "", errMaintenanceCandidateOverflow
	}
	var builder strings.Builder
	builder.Grow(count * (len(placeholder) + 2))
	for index := 0; index < count; index++ {
		if index > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(placeholder)
	}
	return builder.String(), nil
}

func clearThrottleMaintenanceCandidates(candidates []throttleMaintenanceCandidate) {
	for index := range candidates {
		clearBytes(candidates[index].digest)
		candidates[index].digest = nil
	}
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var (
	errMaintenanceCandidateOverflow = errors.New("identity mysql maintenance candidate limit exceeded")
	errMaintenanceCandidateInvalid  = errors.New("identity mysql maintenance candidate is invalid")
)
