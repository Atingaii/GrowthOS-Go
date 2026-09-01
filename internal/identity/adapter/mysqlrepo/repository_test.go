package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	drivermysql "github.com/go-sql-driver/mysql"
)

func TestStorageAndCommitErrorsRemainLowDisclosure(t *testing.T) {
	t.Parallel()
	privateCause := &drivermysql.MySQLError{Number: 1213, Message: "password token digest private detail"}
	err := classifyOperationError(context.Background(), privateCause)
	assertSafeDependencyError(t, err, identityapp.ErrDependencyUnavailable)
	if strings.Contains(fmt.Sprintf("%v %#v", err, err), "private") {
		t.Fatalf("generic formatting leaked cause: %v %#v", err, err)
	}
	var dependency *identityapp.DependencyError
	if !errors.As(err, &dependency) || dependency.Cause() != privateCause {
		t.Fatal("trusted diagnostic cause was not retained")
	}

	commitErr := classifyWriteCommitError(context.Background(), privateCause)
	assertSafeDependencyError(t, commitErr, identityapp.ErrCommitOutcomeUnknown)
	readErr := classifyReadCommitError(context.Background(), privateCause)
	assertSafeDependencyError(t, readErr, identityapp.ErrDependencyUnavailable)

	restoreErr := &storedRestoreError{cause: privateCause}
	if strings.Contains(fmt.Sprintf("%v %#v", restoreErr, restoreErr), "private") {
		t.Fatalf("restore formatting leaked cause: %v %#v", restoreErr, restoreErr)
	}
}

func TestTypedNilMySQLErrorCannotPanicClassifiers(t *testing.T) {
	t.Parallel()
	var mysqlError *drivermysql.MySQLError
	var err error = mysqlError
	if isDuplicateKey(err, "uq_identity_session_token_digest") {
		t.Fatal("typed-nil MySQL error classified as duplicate")
	}
}

func TestCommitCancellationRequiresDefiniteNonCommitEvidence(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := classifyWriteCommitError(ctx, sql.ErrTxDone); !errors.Is(err, context.Canceled) {
		t.Fatalf("definite canceled commit = %v", err)
	}
	indeterminate := classifyWriteCommitError(ctx, errors.New("driver acknowledgement lost"))
	if !errors.Is(indeterminate, identityapp.ErrCommitOutcomeUnknown) {
		t.Fatalf("network-like commit error plus cancellation = %v", indeterminate)
	}
}

func TestRepositorySecurityVocabularyAndLockOrderStayInsideIdentity(t *testing.T) {
	t.Parallel()
	production := strings.Join([]string{
		selectAccountForUpdateSQL,
		selectThrottleForUpdateSQL,
		selectActiveSessionsForUpdateSQL,
		updateSessionRevocationSQL,
		updateSessionTouchSQL,
	}, "\n")
	for _, forbidden := range []string{"governance", "permission", " role ", " scope "} {
		if strings.Contains(strings.ToLower(production), forbidden) {
			t.Fatalf("repository contains authorization vocabulary %q", forbidden)
		}
	}
	if !strings.Contains(selectActiveSessionsForUpdateSQL,
		"ORDER BY last_seen_at ASC, issued_at ASC, session_ref ASC") ||
		!strings.Contains(selectActiveSessionsForUpdateSQL, "LIMIT 6") {
		t.Fatal("bounded active-session proof or deterministic oldest order drifted")
	}
	if strings.Contains(updateSessionRevocationSQL, "last_seen_at = ?") ||
		strings.Contains(updateSessionRevocationSQL, "idle_expires_at = ?") {
		t.Fatal("revocation regressed to a stale touch-image CAS")
	}
}
