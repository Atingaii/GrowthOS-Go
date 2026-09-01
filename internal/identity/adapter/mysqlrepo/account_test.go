package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	identityapp "github.com/Atingaii/GrowthOS-Go/internal/identity/application"
	identity "github.com/Atingaii/GrowthOS-Go/internal/identity/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestFindByLoginStrictlyRestoresExactAccount(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	repository, mock := newRepositoryMock(t, func() time.Time { return testInstant(1) })
	mock.ExpectQuery(sqlPattern(selectAccountByLoginSQL)).
		WithArgs(account.LoginName().String()).
		WillReturnRows(accountRows(account))

	got, err := repository.FindByLogin(context.Background(), account.LoginName())
	if err != nil || !accountsEqual(got, account) {
		t.Fatalf("FindByLogin() = %#v, %v", got, err)
	}
	assertMockExpectations(t, mock)
}

func TestFindByLoginClassifiesMissingCorruptAndCanceled(t *testing.T) {
	t.Parallel()
	account := mustTestAccount(t)
	t.Run("missing", func(t *testing.T) {
		repository, mock := newRepositoryMock(t, func() time.Time { return testInstant(1) })
		mock.ExpectQuery(sqlPattern(selectAccountByLoginSQL)).
			WithArgs(account.LoginName().String()).
			WillReturnError(sql.ErrNoRows)
		_, err := repository.FindByLogin(context.Background(), account.LoginName())
		assertSafeDependencyError(t, err, identityapp.ErrAccountNotFound)
		assertMockExpectations(t, mock)
	})
	t.Run("corrupt", func(t *testing.T) {
		repository, mock := newRepositoryMock(t, func() time.Time { return testInstant(1) })
		rows := accountRows(account)
		rows = sqlmock.NewRows(accountColumns()).AddRow(
			account.ID().String(), account.LoginName().String(), account.PrincipalID().String(),
			[]byte("bad envelope with spaces"), string(account.Status()), uint64(3), uint64(7),
			account.CreatedAt(), account.UpdatedAt(),
		)
		mock.ExpectQuery(sqlPattern(selectAccountByLoginSQL)).WillReturnRows(rows)
		_, err := repository.FindByLogin(context.Background(), account.LoginName())
		assertSafeDependencyError(t, err, identityapp.ErrStoredIdentityInvalid)
		if strings.Contains(err.Error(), "envelope") {
			t.Fatalf("stored detail leaked: %v", err)
		}
		assertMockExpectations(t, mock)
	})
	t.Run("canceled", func(t *testing.T) {
		repository, _ := newRepositoryMock(t, func() time.Time { return testInstant(1) })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := repository.FindByLogin(ctx, account.LoginName())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestRepositoryConstructorAndArgumentsFailClosed(t *testing.T) {
	t.Parallel()
	if repository, err := New(nil); repository != nil || !errors.Is(err, identityapp.ErrDependencyUnavailable) {
		t.Fatalf("New(nil) = %#v, %v", repository, err)
	}
	if repository, err := New(&sqlx.DB{}); repository != nil || !errors.Is(err, identityapp.ErrDependencyUnavailable) {
		t.Fatalf("New(empty wrapper) = %#v, %v", repository, err)
	}
	account := mustTestAccount(t)
	repository, _ := newRepositoryMock(t, func() time.Time { return testInstant(1) })
	if _, err := repository.FindByLogin(nil, account.LoginName()); !errors.Is(err, identityapp.ErrDependencyInvalidArgument) {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := repository.FindByLogin(context.Background(), identity.LoginName("INVALID")); !errors.Is(err, identityapp.ErrDependencyInvalidArgument) {
		t.Fatalf("invalid login error = %v", err)
	}
	emptyWrapperRepository := &Repository{database: &sqlx.DB{}, now: func() time.Time { return testInstant(1) }}
	if _, err := emptyWrapperRepository.FindByLogin(context.Background(), account.LoginName()); !errors.Is(err, identityapp.ErrDependencyUnavailable) {
		t.Fatalf("empty wrapper call error = %v", err)
	}
}
