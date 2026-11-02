package postgres

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenDigest(t *testing.T) {
	require.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", refreshTokenHash("abc"))
	require.NotEqual(t, refreshTokenHash("abc"), refreshTokenHash(refreshTokenHash("abc")))
	data, err := json.Marshal(domain.RefreshToken{TokenHash: refreshTokenHash("abc")})
	require.NoError(t, err)
	require.NotContains(t, string(data), refreshTokenHash("abc"))
}

func TestVersionedSessionStoresOnlyDigest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	expires := time.Now().Add(time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,email,password_hash").WithArgs(int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified", "is_premium", "display_name", "auth_version"}).
			AddRow(7, "user@example.com", []byte("hash"), true, false, "", 3))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO refresh_tokens(user_id,token_hash,expires_at) VALUES($1,$2,$3)")).
		WithArgs(int64(7), refreshTokenHash("bearer-secret"), expires.UTC()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	require.NoError(t, (&Storage{DB: db}).SaveRefreshTokenVersion(context.Background(), 7, 3, "bearer-secret", expires))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoredDigestCannotBeReplayed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	stolenDigest := refreshTokenHash("bearer-secret")
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id FROM refresh_tokens WHERE token_hash=$1")).
		WithArgs(refreshTokenHash(stolenDigest)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	mock.ExpectRollback()
	_, err = (&Storage{DB: db}).RotateRefreshToken(context.Background(), stolenDigest, "replacement", time.Now().Add(time.Hour))
	require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRefreshRotationLosingRaceRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT user_id FROM refresh_tokens").WithArgs(refreshTokenHash("old")).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7))
	mock.ExpectQuery("SELECT id,email,password_hash").WithArgs(int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "email", "password_hash", "is_verified", "is_premium", "display_name", "auth_version"}).
			AddRow(7, "user@example.com", []byte("hash"), true, false, "", 0))
	mock.ExpectExec("UPDATE refresh_tokens SET token_hash=").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	_, err = (&Storage{DB: db}).RotateRefreshToken(context.Background(), "old", "new", time.Now().Add(time.Hour))
	require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}
