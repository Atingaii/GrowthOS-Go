package mysqlrepo

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

const (
	identityRepositoryAcceptanceAuthorization = "lesson-32-disposable-mysql-8.4"
	identityRepositoryAcceptanceTimeout       = 2 * time.Minute
	identityRepositoryAdminDSN                = "GROWTHOS_TEST_IDENTITY_MYSQL_ADMIN_DSN"
	identityRepositoryRuntimeDSN              = "GROWTHOS_TEST_IDENTITY_MYSQL_RUNTIME_DSN"
	identityRepositoryAcceptanceOptIn         = "GROWTHOS_TEST_IDENTITY_MYSQL_ACCEPTANCE"
)

type identityRepositoryAcceptanceEnvironment struct {
	admin      *sqlx.DB
	runtime    *sqlx.DB
	repository *Repository
	clock      *identityRepositoryAcceptanceClock
	account    identity.WorkforceAccount
	runID      string

	concurrentLoginDigest  identity.ThrottleDigest
	concurrentSourceDigest identity.ThrottleDigest
	sessionLoginDigest     identity.ThrottleDigest
	sessionSourceDigest    identity.ThrottleDigest
	ddlProbeNameReserved   bool
}

type identityRepositoryAcceptanceClock struct {
	microseconds atomic.Int64
}

func newIdentityRepositoryAcceptanceClock(at time.Time) *identityRepositoryAcceptanceClock {
	clock := &identityRepositoryAcceptanceClock{}
	clock.Set(at)
	return clock
}

func (clock *identityRepositoryAcceptanceClock) Set(at time.Time) {
	clock.microseconds.Store(canonicalTime(at).UnixMicro())
}

func (clock *identityRepositoryAcceptanceClock) Now() time.Time {
	return time.UnixMicro(clock.microseconds.Load()).UTC()
}

func openIdentityRepositoryAcceptance(
	t *testing.T,
	ctx context.Context,
) *identityRepositoryAcceptanceEnvironment {
	t.Helper()
	adminRaw, adminPresent := os.LookupEnv(identityRepositoryAdminDSN)
	runtimeRaw, runtimePresent := os.LookupEnv(identityRepositoryRuntimeDSN)
	if !adminPresent && !runtimePresent {
		t.Skipf(
			"Identity repository acceptance requires explicit %s and %s",
			identityRepositoryAdminDSN,
			identityRepositoryRuntimeDSN,
		)
	}
	if !adminPresent || !runtimePresent || adminRaw == "" || runtimeRaw == "" {
		t.Fatalf(
			"Identity repository acceptance requires both non-empty %s and %s",
			identityRepositoryAdminDSN,
			identityRepositoryRuntimeDSN,
		)
	}
	if os.Getenv(identityRepositoryAcceptanceOptIn) != identityRepositoryAcceptanceAuthorization {
		t.Skipf(
			"Identity repository acceptance requires %s=%s",
			identityRepositoryAcceptanceOptIn,
			identityRepositoryAcceptanceAuthorization,
		)
	}

	adminConfig := parseIdentityRepositoryAcceptanceDSN(t, adminRaw, "admin")
	runtimeConfig := parseIdentityRepositoryAcceptanceDSN(t, runtimeRaw, "runtime")
	if adminConfig.Net != runtimeConfig.Net || adminConfig.Addr != runtimeConfig.Addr ||
		adminConfig.DBName != runtimeConfig.DBName {
		t.Fatal("Identity acceptance DSNs must target the same network endpoint and database")
	}
	if adminConfig.User == runtimeConfig.User {
		t.Fatal("Identity acceptance admin and runtime DSNs must use distinct MySQL users")
	}

	admin := openIdentityRepositoryAcceptanceDatabase(t, ctx, adminConfig, 4)
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("close Identity admin acceptance pool: %v", err)
		}
	})
	runtime := openIdentityRepositoryAcceptanceDatabase(t, ctx, runtimeConfig, 16)
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("close Identity runtime acceptance pool: %v", err)
		}
	})

	runID := newIdentityRepositoryAcceptanceRunID(t)
	account := newIdentityRepositoryAcceptanceAccount(t, runID)
	clock := newIdentityRepositoryAcceptanceClock(identityRepositoryAcceptanceBaseTime())
	repository, err := newRepository(runtime, clock.Now)
	if err != nil {
		t.Fatalf("construct Identity MySQL repository: %v", err)
	}
	environment := &identityRepositoryAcceptanceEnvironment{
		admin:                  admin,
		runtime:                runtime,
		repository:             repository,
		clock:                  clock,
		account:                account,
		runID:                  runID,
		concurrentLoginDigest:  identityRepositoryAcceptanceThrottleDigest(t, runID+":concurrent:login"),
		concurrentSourceDigest: identityRepositoryAcceptanceThrottleDigest(t, runID+":concurrent:source"),
		sessionLoginDigest:     identityRepositoryAcceptanceThrottleDigest(t, runID+":session:login"),
		sessionSourceDigest:    identityRepositoryAcceptanceThrottleDigest(t, runID+":session:source"),
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cleanupIdentityRepositoryAcceptance(t, cleanupCtx, environment)
	})

	assertIdentityRepositoryAcceptanceServer(t, ctx, admin, runtime)
	assertIdentityRepositoryAcceptanceSchema(t, ctx, admin)
	assertIdentityRepositoryAcceptanceGrants(t, ctx, runtime, runtimeConfig.DBName)
	assertIdentityRepositoryAcceptanceStartsEmpty(t, ctx, admin)
	environment.ddlProbeNameReserved = true
	seedIdentityRepositoryAcceptanceAccount(t, ctx, admin, account)
	return environment
}

func parseIdentityRepositoryAcceptanceDSN(
	t *testing.T,
	raw string,
	label string,
) *drivermysql.Config {
	t.Helper()
	config, err := drivermysql.ParseDSN(raw)
	if err != nil {
		t.Fatalf("parse Identity %s acceptance DSN: invalid DSN", label)
	}
	if config.Net == "" || config.Addr == "" || config.DBName == "" || config.User == "" {
		t.Fatalf("Identity %s acceptance DSN must name network, address, database, and user", label)
	}
	for _, character := range config.DBName {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' {
			continue
		}
		t.Fatalf("Identity %s acceptance database must use [A-Za-z0-9_] only", label)
	}
	config.ParseTime = true
	config.Loc = time.UTC
	config.MultiStatements = false
	config.InterpolateParams = false
	config.ClientFoundRows = false
	return config
}

func openIdentityRepositoryAcceptanceDatabase(
	t *testing.T,
	ctx context.Context,
	config *drivermysql.Config,
	maximumOpen int,
) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatalf("open Identity acceptance database: %v", err)
	}
	database.SetMaxOpenConns(maximumOpen)
	database.SetMaxIdleConns(maximumOpen / 2)
	database.SetConnMaxLifetime(time.Minute)
	database.SetConnMaxIdleTime(30 * time.Second)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("ping Identity acceptance database: %v", err)
	}
	return database
}

func assertIdentityRepositoryAcceptanceServer(
	t *testing.T,
	ctx context.Context,
	admin *sqlx.DB,
	runtime *sqlx.DB,
) {
	t.Helper()
	var adminVersion, runtimeVersion, adminUUID, runtimeUUID string
	if err := admin.QueryRowxContext(ctx, "SELECT VERSION(), @@server_uuid").Scan(&adminVersion, &adminUUID); err != nil {
		t.Fatalf("inspect Identity admin MySQL server: %v", err)
	}
	if err := runtime.QueryRowxContext(ctx, "SELECT VERSION(), @@server_uuid").Scan(&runtimeVersion, &runtimeUUID); err != nil {
		t.Fatalf("inspect Identity runtime MySQL server: %v", err)
	}
	if !strings.HasPrefix(adminVersion, "8.4.") || runtimeVersion != adminVersion {
		t.Fatalf("Identity repository acceptance requires one exact MySQL 8.4.x server, got admin=%q runtime=%q", adminVersion, runtimeVersion)
	}
	if adminUUID == "" || runtimeUUID != adminUUID {
		t.Fatal("Identity admin and runtime acceptance pools reached different MySQL server identities")
	}
}

func assertIdentityRepositoryAcceptanceSchema(t *testing.T, ctx context.Context, admin *sqlx.DB) {
	t.Helper()
	var version uint
	var dirty bool
	if err := admin.QueryRowxContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("read Identity acceptance migration state: %v", err)
	}
	if version != 14 || dirty {
		t.Fatalf("Identity repository acceptance requires clean schema v14, got version=%d dirty=%t", version, dirty)
	}
	var tables int
	if err := admin.GetContext(ctx, &tables, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name IN (
			'identity_workforce_account',
			'identity_session',
			'identity_authentication_throttle'
		  )
		  AND engine = 'InnoDB'`); err != nil {
		t.Fatalf("inspect Identity acceptance tables: %v", err)
	}
	if tables != 3 {
		t.Fatalf("Identity repository acceptance found %d InnoDB authority tables, want 3", tables)
	}
}

func assertIdentityRepositoryAcceptanceGrants(
	t *testing.T,
	ctx context.Context,
	runtime *sqlx.DB,
	database string,
) {
	t.Helper()
	var currentUser string
	if err := runtime.GetContext(ctx, &currentUser, "SELECT CURRENT_USER()"); err != nil {
		t.Fatalf("read Identity runtime account identity: %v", err)
	}
	separator := strings.LastIndexByte(currentUser, '@')
	if separator <= 0 || separator == len(currentUser)-1 {
		t.Fatalf("MySQL returned an invalid CURRENT_USER identity %q", currentUser)
	}
	target := quoteIdentityRepositoryAcceptanceIdentifier(currentUser[:separator]) + "@" +
		quoteIdentityRepositoryAcceptanceIdentifier(currentUser[separator+1:])
	quotedDatabase := quoteIdentityRepositoryAcceptanceIdentifier(database)
	expected := []string{
		"GRANT SELECT, INSERT, UPDATE, DELETE ON " + quotedDatabase + ".`identity_authentication_throttle` TO " + target,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON " + quotedDatabase + ".`identity_session` TO " + target,
		"GRANT SELECT, UPDATE (`updated_at`) ON " + quotedDatabase + ".`identity_workforce_account` TO " + target,
		"GRANT USAGE ON *.* TO " + target,
	}
	var actual []string
	if err := runtime.SelectContext(ctx, &actual, "SHOW GRANTS FOR CURRENT_USER"); err != nil {
		t.Fatalf("read Identity runtime grants: %v", err)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if strings.Join(actual, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("Identity runtime direct grants differ from the exact three-table allowlist:\nactual=%q\nexpected=%q", actual, expected)
	}
	var mandatoryRoles sql.NullString
	if err := runtime.GetContext(ctx, &mandatoryRoles, "SELECT @@GLOBAL.mandatory_roles"); err != nil {
		t.Fatalf("read MySQL mandatory roles: %v", err)
	}
	if mandatoryRoles.Valid && mandatoryRoles.String != "" {
		t.Fatalf("Identity acceptance requires no mandatory MySQL roles, got %q", mandatoryRoles.String)
	}
}

func quoteIdentityRepositoryAcceptanceIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func assertIdentityRepositoryAcceptanceStartsEmpty(t *testing.T, ctx context.Context, admin *sqlx.DB) {
	t.Helper()
	for _, table := range []string{
		"identity_session",
		"identity_authentication_throttle",
		"identity_workforce_account",
	} {
		var count int
		if err := admin.GetContext(ctx, &count, "SELECT COUNT(*) FROM "+table); err != nil {
			t.Fatalf("count disposable Identity table %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("Identity acceptance refuses non-empty disposable table %s (%d rows)", table, count)
		}
	}
	var forbiddenProbeTable int
	if err := admin.GetContext(ctx, &forbiddenProbeTable, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = 'identity_l32_forbidden'`); err != nil {
		t.Fatalf("inspect denied Identity DDL probe table: %v", err)
	}
	if forbiddenProbeTable != 0 {
		t.Fatal("Identity acceptance refuses a schema containing its reserved DDL probe table")
	}
}

func newIdentityRepositoryAcceptanceRunID(t *testing.T) string {
	t.Helper()
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate Identity acceptance run identity: %v", err)
	}
	return hex.EncodeToString(random)
}

func identityRepositoryAcceptanceBaseTime() time.Time {
	return time.Date(2026, 9, 1, 12, 0, 0, 123456000, time.UTC)
}

func newIdentityRepositoryAcceptanceAccount(
	t *testing.T,
	runID string,
) identity.WorkforceAccount {
	t.Helper()
	accountID, err := identity.NewAccountID("account:l32:" + runID)
	if err != nil {
		t.Fatal(err)
	}
	login, err := identity.NewLoginName("l32_" + runID)
	if err != nil {
		t.Fatal(err)
	}
	principalID, err := identity.NewPrincipalID("principal:l32:" + runID)
	if err != nil {
		t.Fatal(err)
	}
	credentialVersion, err := identity.NewCredentialVersion(3)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := identity.NewAuthenticationEpoch(7)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := identity.NewPasswordEnvelope([]byte(
		"$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0MTIzNA$ZGlnZXN0ZGlnZXN0ZGlnZXN0ZGlnZXN0MTIzNDU2Nzg",
	))
	if err != nil {
		t.Fatal(err)
	}
	createdAt := identityRepositoryAcceptanceBaseTime().Add(-24 * time.Hour)
	account, err := identity.NewWorkforceAccount(
		accountID,
		login,
		principalID,
		identity.AccountStatusEnabled,
		credentialVersion,
		epoch,
		envelope,
		createdAt,
		createdAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func seedIdentityRepositoryAcceptanceAccount(
	t *testing.T,
	ctx context.Context,
	admin *sqlx.DB,
	account identity.WorkforceAccount,
) {
	t.Helper()
	envelope := account.CredentialEnvelope().Bytes()
	defer clearBytes(envelope)
	result, err := admin.ExecContext(ctx, `
		INSERT INTO identity_workforce_account
			(account_id, login_name, principal_id, password_envelope,
			 account_status, credential_version, authentication_epoch,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		account.ID().String(),
		account.LoginName().String(),
		account.PrincipalID().String(),
		envelope,
		string(account.Status()),
		uint64(account.CredentialVersion()),
		uint64(account.AuthenticationEpoch()),
		account.CreatedAt(),
		account.UpdatedAt(),
	)
	if err != nil {
		t.Fatalf("seed Identity acceptance account: %v", err)
	}
	if err := requireAffectedRows(result, 1); err != nil {
		t.Fatalf("seed Identity acceptance account: %v", err)
	}
}

func cleanupIdentityRepositoryAcceptance(
	t *testing.T,
	ctx context.Context,
	environment *identityRepositoryAcceptanceEnvironment,
) {
	t.Helper()
	if environment == nil || environment.admin == nil {
		return
	}
	if environment.ddlProbeNameReserved {
		// Setup proved this fixed name absent. If it exists now—even after an
		// acknowledgement-loss error—it is owned by this exact DDL probe.
		var probeTables int
		if err := environment.admin.GetContext(ctx, &probeTables, `
			SELECT COUNT(*)
			FROM information_schema.tables
			WHERE table_schema = DATABASE()
			  AND table_name = 'identity_l32_forbidden'`); err != nil {
			t.Errorf("inspect denied Identity acceptance DDL probe cleanup: %v", err)
		} else if probeTables == 1 {
			if _, err := environment.admin.ExecContext(
				ctx,
				"DROP TABLE identity_l32_forbidden",
			); err != nil {
				t.Errorf("clean denied Identity acceptance DDL probe: %v", err)
			}
		}
	}
	if _, err := environment.admin.ExecContext(
		ctx,
		"DELETE FROM identity_session WHERE account_id = ?",
		environment.account.ID().String(),
	); err != nil {
		t.Errorf("clean Identity acceptance sessions: %v", err)
	}
	for _, item := range []struct {
		dimension identity.ThrottleDimension
		digest    identity.ThrottleDigest
	}{
		{identity.ThrottleDimensionLogin, environment.concurrentLoginDigest},
		{identity.ThrottleDimensionSource, environment.concurrentSourceDigest},
		{identity.ThrottleDimensionLogin, environment.sessionLoginDigest},
		{identity.ThrottleDimensionSource, environment.sessionSourceDigest},
	} {
		digest := item.digest.Bytes()
		_, err := environment.admin.ExecContext(
			ctx,
			"DELETE FROM identity_authentication_throttle WHERE dimension = ? AND subject_digest = ?",
			string(item.dimension),
			digest,
		)
		clearBytes(digest)
		if err != nil {
			t.Errorf("clean Identity acceptance throttle %s: %v", item.dimension, err)
		}
	}
	if _, err := environment.admin.ExecContext(
		ctx,
		"DELETE FROM identity_workforce_account WHERE account_id = ?",
		environment.account.ID().String(),
	); err != nil {
		t.Errorf("clean Identity acceptance account: %v", err)
	}
	loginConcurrent := environment.concurrentLoginDigest.Bytes()
	sourceConcurrent := environment.concurrentSourceDigest.Bytes()
	loginSession := environment.sessionLoginDigest.Bytes()
	sourceSession := environment.sessionSourceDigest.Bytes()
	defer clearBytes(loginConcurrent)
	defer clearBytes(sourceConcurrent)
	defer clearBytes(loginSession)
	defer clearBytes(sourceSession)
	var remaining int
	if err := environment.admin.GetContext(ctx, &remaining, `
		SELECT
			(SELECT COUNT(*) FROM identity_session WHERE account_id = ?) +
			(SELECT COUNT(*) FROM identity_workforce_account WHERE account_id = ?) +
			(SELECT COUNT(*)
			 FROM identity_authentication_throttle
			 WHERE (dimension = 'login' AND subject_digest IN (?, ?))
			    OR (dimension = 'source' AND subject_digest IN (?, ?)))`,
		environment.account.ID().String(),
		environment.account.ID().String(),
		loginConcurrent,
		loginSession,
		sourceConcurrent,
		sourceSession,
	); err != nil {
		t.Errorf("verify Identity acceptance artifact cleanup: %v", err)
	} else if remaining != 0 {
		t.Errorf("Identity acceptance cleanup left %d owned artifacts", remaining)
	}
}

func identityRepositoryAcceptanceThrottleDigest(
	t *testing.T,
	label string,
) identity.ThrottleDigest {
	t.Helper()
	value := sha256.Sum256([]byte(label))
	digest, err := identity.NewThrottleDigest(value[:])
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func identityRepositoryAcceptanceTokenDigest(
	t *testing.T,
	label string,
) identity.TokenDigest {
	t.Helper()
	value := sha256.Sum256([]byte(label))
	digest, err := identity.NewTokenDigest(value[:])
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func identityRepositoryAcceptanceEntropy(runID string, count int) []byte {
	material := make([]byte, 0, count*(identityapp.SessionTokenBytes+
		identityapp.SessionReferenceEntropyBytes+identityapp.OperationReferenceEntropyBytes))
	for index := 0; index < count; index++ {
		token := sha256.Sum256([]byte(fmt.Sprintf("%s:token:%d", runID, index)))
		reference := sha256.Sum256([]byte(fmt.Sprintf("%s:reference:%d", runID, index)))
		operation := sha256.Sum256([]byte(fmt.Sprintf("%s:operation:%d", runID, index)))
		material = append(material, token[:]...)
		material = append(material, reference[:identityapp.SessionReferenceEntropyBytes]...)
		material = append(material, operation[:identityapp.OperationReferenceEntropyBytes]...)
	}
	return material
}

func newIdentityRepositoryAcceptanceLoginCommand(
	t *testing.T,
	environment *identityRepositoryAcceptanceEnvironment,
) identityapp.LoginCommand {
	t.Helper()
	command, err := identityapp.NewLoginCommand(
		environment.account.LoginName(),
		[]byte("correct horse battery staple"),
		environment.sessionLoginDigest,
		environment.sessionSourceDigest,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func insertIdentityRepositoryAcceptanceSession(
	t *testing.T,
	ctx context.Context,
	admin *sqlx.DB,
	session identity.Session,
) {
	t.Helper()
	digest := session.TokenDigest().Bytes()
	defer clearBytes(digest)
	result, err := admin.ExecContext(ctx, `
		INSERT INTO identity_session
			(session_ref, issue_operation_ref, account_id, token_digest,
			 authentication_epoch, issued_at, last_seen_at, idle_expires_at,
			 absolute_expires_at, revoked_at, revoke_reason,
			 revoke_operation_ref, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?)`,
		session.Reference().String(),
		session.IssueOperationRef().String(),
		session.AccountID().String(),
		digest,
		uint64(session.AuthenticationEpoch()),
		session.IssuedAt(),
		session.LastSeenAt(),
		session.IdleExpiresAt(),
		session.AbsoluteExpiresAt(),
		session.LastSeenAt(),
	)
	if err != nil {
		t.Fatalf("insert Identity acceptance sentinel session: %v", err)
	}
	if err := requireAffectedRows(result, 1); err != nil {
		t.Fatalf("insert Identity acceptance sentinel session: %v", err)
	}
}

func expectIdentityRepositoryAcceptanceMySQLError(
	t *testing.T,
	err error,
	numbers ...uint16,
) {
	t.Helper()
	if err == nil {
		t.Fatal("expected MySQL permission error, got nil")
	}
	var mysqlError *drivermysql.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError == nil {
		t.Fatalf("error type = %T, want MySQL permission error", err)
	}
	for _, number := range numbers {
		if mysqlError.Number == number {
			return
		}
	}
	fatal := fmt.Sprintf("MySQL error number = %d, want one of %v", mysqlError.Number, numbers)
	if strings.Contains(strings.ToLower(mysqlError.Message), "password") {
		fatal += " (driver message redacted)"
	}
	t.Fatal(fatal)
}

func clearIdentityRepositoryAcceptanceTokens(tokens [][]byte) {
	for index := range tokens {
		clear(tokens[index])
		tokens[index] = nil
	}
}

func newIdentityRepositoryAcceptanceEntropyReader(material []byte) *staticEntropy {
	return &staticEntropy{reader: bytes.NewReader(material)}
}
