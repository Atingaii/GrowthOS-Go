package mysqlrepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
)

func TestRepositoryMySQL84Acceptance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), identityRepositoryAcceptanceTimeout)
	defer cancel()
	environment := openIdentityRepositoryAcceptance(t, ctx)

	if !t.Run("restores exact workforce credential", func(t *testing.T) {
		assertIdentityRepositoryCredentialRoundTrip(t, ctx, environment)
	}) {
		return
	}
	if !t.Run("serializes reservation capacity and fences expired epochs", func(t *testing.T) {
		assertIdentityRepositoryThrottleConcurrency(t, ctx, environment)
	}) {
		return
	}
	if !t.Run("cancels an account lock wait without a partial session", func(t *testing.T) {
		assertIdentityRepositoryAccountLockCancellation(t, ctx, environment)
	}) {
		return
	}
	if !t.Run("enforces session cap touch and logout truth", func(t *testing.T) {
		assertIdentityRepositorySessionLifecycle(t, ctx, environment)
	}) {
		return
	}
	if !t.Run("cleans only closed-boundary history", func(t *testing.T) {
		assertIdentityRepositoryMaintenance(t, ctx, environment)
	}) {
		return
	}
	t.Run("runtime identity cannot cross the grant boundary", func(t *testing.T) {
		assertIdentityRepositoryPermissionDenials(t, ctx, environment)
	})
}

func assertIdentityRepositoryAccountLockCancellation(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	lock, err := environment.admin.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin acceptance account lock: %v", err)
	}
	lockReleased := false
	defer func() {
		if !lockReleased {
			_ = lock.Rollback()
		}
	}()
	var lockedAccount string
	if err := lock.GetContext(ctx, &lockedAccount, `
		SELECT account_id
		FROM identity_workforce_account
		WHERE account_id = ?
		FOR UPDATE`, environment.account.ID().String()); err != nil {
		t.Fatalf("take acceptance account lock: %v", err)
	}
	if lockedAccount != environment.account.ID().String() {
		t.Fatalf("locked account = %q", lockedAccount)
	}

	entropy := identityRepositoryAcceptanceEntropy(environment.runID+":lock-wait", 1)
	defer clear(entropy)
	loginService, err := identityapp.NewLoginService(identityapp.LoginDependencies{
		Clock:       identityapp.ClockFunc(environment.clock.Now),
		Credentials: environment.repository,
		Passwords:   passwordVerifierStub{},
		Admissions:  environment.repository,
		Entropy:     newIdentityRepositoryAcceptanceEntropyReader(entropy),
		Issuer:      environment.repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment.clock.Set(identityRepositoryAcceptanceBaseTime())
	waitCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	issued, loginErr := loginService.Login(
		waitCtx,
		newIdentityRepositoryAcceptanceLoginCommand(t, environment),
	)
	cancel()
	if !errors.Is(loginErr, identityapp.ErrOperationCanceled) ||
		issued != (identityapp.IssuedSession{}) {
		t.Fatalf("locked Login() = %#v, %v, want canceled and no issued token", issued, loginErr)
	}
	if err := lock.Rollback(); err != nil {
		t.Fatalf("release acceptance account lock: %v", err)
	}
	lockReleased = true
	var sessions int
	if err := environment.admin.GetContext(ctx, &sessions, `
		SELECT COUNT(*)
		FROM identity_session
		WHERE account_id = ?`, environment.account.ID().String()); err != nil {
		t.Fatalf("inspect canceled session issue: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("canceled account lock wait persisted %d sessions", sessions)
	}
}

func assertIdentityRepositoryCredentialRoundTrip(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	stored, err := environment.repository.FindByLogin(ctx, environment.account.LoginName())
	if err != nil {
		t.Fatalf("FindByLogin(valid account): %v", err)
	}
	if !accountsEqual(stored, environment.account) {
		t.Fatal("FindByLogin did not restore the exact authoritative credential snapshot")
	}
	missing, err := identity.NewLoginName("missing_" + environment.runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.repository.FindByLogin(ctx, missing); !errors.Is(err, identityapp.ErrAccountNotFound) {
		t.Fatalf("FindByLogin(missing) error = %v, want account-not-found class", err)
	}
}

func assertIdentityRepositoryThrottleConcurrency(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	const contenders = 8
	admittedAt := identityRepositoryAcceptanceBaseTime()
	deadline := admittedAt.Add(identityapp.MaximumAdmissionLease)
	environment.clock.Set(admittedAt)
	request, err := identityapp.NewAdmissionRequest(
		environment.concurrentLoginDigest,
		environment.concurrentSourceDigest,
		admittedAt,
		deadline,
	)
	if err != nil {
		t.Fatal(err)
	}

	errorsByContender := make(chan error, contenders)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < contenders; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, beginErr := environment.repository.BeginAdmission(ctx, request)
			errorsByContender <- beginErr
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByContender)

	granted := 0
	rejected := 0
	for beginErr := range errorsByContender {
		switch {
		case beginErr == nil:
			granted++
		case errors.Is(beginErr, identityapp.ErrAdmissionRejected):
			rejected++
		default:
			t.Fatalf("concurrent BeginAdmission returned unexpected error: %v", beginErr)
		}
	}
	if granted != int(identityapp.LoginFailureThreshold) || rejected != contenders-granted {
		t.Fatalf("concurrent reservations granted=%d rejected=%d, want %d/%d", granted, rejected, identityapp.LoginFailureThreshold, contenders-int(identityapp.LoginFailureThreshold))
	}
	assertIdentityRepositoryThrottleRows(t, ctx, environment, 5, 1)

	// The old batch has reached its closed lease boundary. A new Begin must
	// durably clear all five abandoned reservations, increment the fencing
	// epoch once, and then install exactly one reservation in that new epoch.
	recoveredAt := deadline
	environment.clock.Set(recoveredAt)
	recoveryRequest, err := identityapp.NewAdmissionRequest(
		environment.concurrentLoginDigest,
		environment.concurrentSourceDigest,
		recoveredAt,
		recoveredAt.Add(identityapp.MaximumAdmissionLease),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.repository.BeginAdmission(ctx, recoveryRequest); err != nil {
		t.Fatalf("recover expired throttle reservation batch: %v", err)
	}
	assertIdentityRepositoryThrottleRows(t, ctx, environment, 1, 2)
}

func assertIdentityRepositoryThrottleRows(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
	wantInflight uint32,
	wantEpoch uint64,
) {
	t.Helper()
	type row struct {
		Dimension    string       `db:"dimension"`
		FailureCount uint64       `db:"failure_count"`
		Inflight     uint64       `db:"inflight_count"`
		Epoch        uint64       `db:"admission_epoch"`
		ExpiresAt    sql.NullTime `db:"inflight_expires_at"`
		BlockedUntil sql.NullTime `db:"blocked_until"`
	}
	loginBytes := environment.concurrentLoginDigest.Bytes()
	sourceBytes := environment.concurrentSourceDigest.Bytes()
	defer clearBytes(loginBytes)
	defer clearBytes(sourceBytes)
	var rows []row
	if err := environment.admin.SelectContext(ctx, &rows, `
		SELECT dimension, failure_count, inflight_count, admission_epoch,
		       inflight_expires_at, blocked_until
		FROM identity_authentication_throttle
		WHERE (dimension = 'login' AND subject_digest = ?)
		   OR (dimension = 'source' AND subject_digest = ?)
		ORDER BY dimension`, loginBytes, sourceBytes); err != nil {
		t.Fatalf("inspect throttle reservations: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("throttle rows = %d, want exact login/source pair", len(rows))
	}
	for _, stored := range rows {
		if stored.FailureCount != 0 || stored.Inflight != uint64(wantInflight) ||
			stored.Epoch != wantEpoch || !stored.ExpiresAt.Valid || stored.BlockedUntil.Valid {
			t.Fatalf("throttle row %s = %+v", stored.Dimension, stored)
		}
	}
}

func assertIdentityRepositorySessionLifecycle(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	const issues = 6
	entropy := identityRepositoryAcceptanceEntropy(environment.runID, issues)
	defer clear(entropy)
	loginService, err := identityapp.NewLoginService(identityapp.LoginDependencies{
		Clock:       identityapp.ClockFunc(environment.clock.Now),
		Credentials: environment.repository,
		Passwords:   passwordVerifierStub{},
		Admissions:  environment.repository,
		Entropy:     newIdentityRepositoryAcceptanceEntropyReader(entropy),
		Issuer:      environment.repository,
	})
	if err != nil {
		t.Fatal(err)
	}

	tokens := make([][]byte, 0, issues)
	references := make([]identity.SessionRef, 0, issues)
	issueTimes := make([]time.Time, 0, issues)
	defer clearIdentityRepositoryAcceptanceTokens(tokens)
	for index := 0; index < issues; index++ {
		issuedAt := identityRepositoryAcceptanceBaseTime().Add(time.Duration(index) * 2 * time.Minute)
		environment.clock.Set(issuedAt)
		issued, err := loginService.Login(ctx, newIdentityRepositoryAcceptanceLoginCommand(t, environment))
		if err != nil {
			t.Fatalf("issue session %d: %v", index+1, err)
		}
		if issued.Validate() != nil {
			t.Fatalf("issued session %d is invalid", index+1)
		}
		tokens = append(tokens, issued.RawToken())
		references = append(references, issued.VerifiedSession().SessionReference())
		issueTimes = append(issueTimes, issuedAt)
	}

	var total, active, evicted int
	if err := environment.admin.QueryRowxContext(ctx, `
		SELECT COUNT(*),
		       SUM(revoked_at IS NULL AND idle_expires_at > ? AND absolute_expires_at > ?),
		       SUM(revoke_reason = 'concurrency_limit')
		FROM identity_session
		WHERE account_id = ?`,
		issueTimes[issues-1],
		issueTimes[issues-1],
		environment.account.ID().String(),
	).Scan(&total, &active, &evicted); err != nil {
		t.Fatalf("inspect session cap: %v", err)
	}
	if total != issues || active != int(MaximumActiveSessions) || evicted != 1 {
		t.Fatalf("session cap rows total=%d active=%d evicted=%d, want 6/5/1", total, active, evicted)
	}
	var evictedReference string
	if err := environment.admin.GetContext(ctx, &evictedReference, `
		SELECT session_ref
		FROM identity_session
		WHERE account_id = ? AND revoke_reason = 'concurrency_limit'`,
		environment.account.ID().String(),
	); err != nil {
		t.Fatalf("read deterministic eviction: %v", err)
	}
	if evictedReference != references[0].String() {
		t.Fatalf("evicted session = %q, want oldest %q", evictedReference, references[0])
	}

	resolveService, err := identityapp.NewResolveService(
		identityapp.ClockFunc(environment.clock.Now),
		environment.repository,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveService.Resolve(ctx, tokens[0]); !errors.Is(err, identityapp.ErrUnauthenticated) {
		t.Fatalf("resolve evicted token error = %v, want unauthenticated", err)
	}

	touchedAt := issueTimes[issues-1].Add(identityapp.SessionTouchWindow + time.Second)
	environment.clock.Set(touchedAt)
	verified, err := resolveService.Resolve(ctx, tokens[1])
	if err != nil {
		t.Fatalf("resolve and touch active session: %v", err)
	}
	if verified.SessionReference() != references[1] ||
		verified.IdleExpiresAt() != touchedAt.Add(identityapp.SessionIdleLifetime) {
		t.Fatalf("touched verified session = %#v", verified)
	}
	var storedLastSeen, storedIdleExpiry time.Time
	if err := environment.admin.QueryRowxContext(ctx, `
		SELECT last_seen_at, idle_expires_at
		FROM identity_session
		WHERE session_ref = ?`, references[1].String()).Scan(&storedLastSeen, &storedIdleExpiry); err != nil {
		t.Fatalf("inspect persisted session touch: %v", err)
	}
	if canonicalTime(storedLastSeen) != touchedAt ||
		canonicalTime(storedIdleExpiry) != touchedAt.Add(identityapp.SessionIdleLifetime) {
		t.Fatalf("persisted touch last_seen=%v idle=%v", storedLastSeen, storedIdleExpiry)
	}

	logoutAt := touchedAt.Add(time.Second)
	environment.clock.Set(logoutAt)
	revokeEntropy := sha256.Sum256([]byte(environment.runID + ":logout"))
	revokeService, err := identityapp.NewRevokeCurrentService(identityapp.RevokeCurrentDependencies{
		Clock:   identityapp.ClockFunc(environment.clock.Now),
		Reader:  environment.repository,
		Revoker: environment.repository,
		Entropy: newIdentityRepositoryAcceptanceEntropyReader(revokeEntropy[:identityapp.OperationReferenceEntropyBytes]),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := revokeService.RevokeCurrent(ctx, tokens[1]); err != nil {
		t.Fatalf("revoke current session: %v", err)
	}
	var revokedAt time.Time
	var revokeReason string
	if err := environment.admin.QueryRowxContext(ctx, `
		SELECT revoked_at, revoke_reason
		FROM identity_session
		WHERE session_ref = ?`, references[1].String()).Scan(&revokedAt, &revokeReason); err != nil {
		t.Fatalf("inspect persisted logout: %v", err)
	}
	if canonicalTime(revokedAt) != logoutAt || revokeReason != string(identity.SessionRevokeReasonLogout) {
		t.Fatalf("persisted logout revoked_at=%v reason=%q", revokedAt, revokeReason)
	}
}

func assertIdentityRepositoryMaintenance(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	observedAt := identityRepositoryAcceptanceBaseTime().Add(8 * 24 * time.Hour)
	sentinel := mustTestSession(
		t,
		environment.account,
		"active:"+environment.runID,
		"active-issue:"+environment.runID,
		identityRepositoryAcceptanceTokenDigest(t, environment.runID+":active:token"),
		observedAt.Add(-time.Minute),
		observedAt.Add(-time.Minute),
	)
	insertIdentityRepositoryAcceptanceSession(t, ctx, environment.admin, sentinel)
	operation, err := identityapp.NewMaintenanceOperation(observedAt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := environment.repository.RunMaintenance(ctx, operation)
	if err != nil {
		t.Fatalf("run Identity repository maintenance: %v", err)
	}
	if result.SessionsDeleted() != 6 || result.ThrottlesDeleted() != 2 || result.TotalDeleted() != 8 {
		t.Fatalf("maintenance result sessions=%d throttles=%d total=%d, want 6/2/8", result.SessionsDeleted(), result.ThrottlesDeleted(), result.TotalDeleted())
	}
	var sessionReferences []string
	if err := environment.admin.SelectContext(ctx, &sessionReferences, `
		SELECT session_ref
		FROM identity_session
		WHERE account_id = ?
		ORDER BY session_ref`, environment.account.ID().String()); err != nil {
		t.Fatalf("inspect maintenance session survivors: %v", err)
	}
	if len(sessionReferences) != 1 || sessionReferences[0] != sentinel.Reference().String() {
		t.Fatalf("maintenance session survivors = %q, want current sentinel only", sessionReferences)
	}

	loginBytes := environment.concurrentLoginDigest.Bytes()
	sourceBytes := environment.concurrentSourceDigest.Bytes()
	defer clearBytes(loginBytes)
	defer clearBytes(sourceBytes)
	var retainedInflight int
	if err := environment.admin.GetContext(ctx, &retainedInflight, `
		SELECT COUNT(*)
		FROM identity_authentication_throttle
		WHERE inflight_count = 1
		  AND ((dimension = 'login' AND subject_digest = ?)
		    OR (dimension = 'source' AND subject_digest = ?))`, loginBytes, sourceBytes); err != nil {
		t.Fatalf("inspect maintenance throttle survivors: %v", err)
	}
	if retainedInflight != 2 {
		t.Fatalf("maintenance retained %d inflight throttle rows, want 2", retainedInflight)
	}
	var sessionThrottleRows int
	sessionLoginBytes := environment.sessionLoginDigest.Bytes()
	sessionSourceBytes := environment.sessionSourceDigest.Bytes()
	defer clearBytes(sessionLoginBytes)
	defer clearBytes(sessionSourceBytes)
	if err := environment.admin.GetContext(ctx, &sessionThrottleRows, `
		SELECT COUNT(*)
		FROM identity_authentication_throttle
		WHERE (dimension = 'login' AND subject_digest = ?)
		   OR (dimension = 'source' AND subject_digest = ?)`, sessionLoginBytes, sessionSourceBytes); err != nil {
		t.Fatalf("inspect deleted inactive throttle rows: %v", err)
	}
	if sessionThrottleRows != 0 {
		t.Fatalf("maintenance left %d eligible inactive throttle rows", sessionThrottleRows)
	}
}

func assertIdentityRepositoryPermissionDenials(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	tx, err := environment.runtime.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin account-lock permission probe: %v", err)
	}
	var accountID string
	if err := tx.GetContext(ctx, &accountID, `
		SELECT account_id
		FROM identity_workforce_account
		WHERE account_id = ?
		FOR UPDATE`, environment.account.ID().String()); err != nil {
		_ = tx.Rollback()
		t.Fatalf("runtime identity cannot take required account lock: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback account-lock permission probe: %v", err)
	}
	if accountID != environment.account.ID().String() {
		t.Fatalf("account-lock probe returned %q", accountID)
	}
	if _, err := environment.runtime.ExecContext(ctx, `
		UPDATE identity_workforce_account
		SET updated_at = updated_at
		WHERE account_id = ?`, environment.account.ID().String()); err != nil {
		t.Fatalf("runtime identity cannot exercise updated_at-only account grant: %v", err)
	}

	for _, probe := range []struct {
		name    string
		query   string
		args    []any
		numbers []uint16
	}{
		{
			name:    "credential column update",
			query:   "UPDATE identity_workforce_account SET password_envelope = password_envelope WHERE account_id = ?",
			args:    []any{environment.account.ID().String()},
			numbers: []uint16{1143},
		},
		{
			name:    "login column update",
			query:   "UPDATE identity_workforce_account SET login_name = login_name WHERE account_id = ?",
			args:    []any{environment.account.ID().String()},
			numbers: []uint16{1143},
		},
		{
			name:    "account insert",
			query:   "INSERT INTO identity_workforce_account (account_id, login_name, principal_id, password_envelope, account_status, credential_version, authentication_epoch, created_at, updated_at) SELECT 'denied:l32', 'denied_l32', 'denied:l32', '$argon2id$denied', 'enabled', 1, 1, NOW(6), NOW(6) WHERE FALSE",
			numbers: []uint16{1142},
		},
		{
			name:    "account delete",
			query:   "DELETE FROM identity_workforce_account WHERE account_id = ?",
			args:    []any{environment.account.ID().String()},
			numbers: []uint16{1142},
		},
		{
			name:    "migration metadata read",
			query:   "SELECT version FROM schema_migrations LIMIT 0",
			numbers: []uint16{1142},
		},
		{
			name:    "Lottery read",
			query:   "SELECT strategy_id FROM lottery_strategy LIMIT 0",
			numbers: []uint16{1142},
		},
		{
			name:    "Marketing read",
			query:   "SELECT activity_id FROM marketing_activity LIMIT 0",
			numbers: []uint16{1142},
		},
		{
			name:    "DDL",
			query:   "CREATE TABLE identity_l32_forbidden (id INT NOT NULL)",
			numbers: []uint16{1142},
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			_, probeErr := environment.runtime.ExecContext(ctx, probe.query, probe.args...)
			expectIdentityRepositoryAcceptanceMySQLError(t, probeErr, probe.numbers...)
		})
	}
	var accountRows int
	if err := environment.admin.GetContext(ctx, &accountRows, `
		SELECT COUNT(*)
		FROM identity_workforce_account
		WHERE account_id = ?`, environment.account.ID().String()); err != nil {
		t.Fatalf("verify protected account after permission probes: %v", err)
	}
	if accountRows != 1 {
		t.Fatalf("protected account rows = %d, want 1", accountRows)
	}
	var forbiddenTables int
	if err := environment.admin.GetContext(ctx, &forbiddenTables, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = 'identity_l32_forbidden'`); err != nil {
		t.Fatalf("verify denied DDL probe: %v", err)
	}
	if forbiddenTables != 0 {
		t.Fatal("denied DDL permission probe unexpectedly created a table")
	}
}
