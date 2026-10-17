package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAccountPasswordByteLimit(t *testing.T) {
	require.NoError(t, ValidatePassword(strings.Repeat("я", 36)))
	require.Error(t, ValidatePassword(strings.Repeat("я", 37)))
	require.Error(t, ValidatePassword("short"))
	require.NoError(t, ValidateDisplayName("  "+strings.Repeat("я", 80)+"  "))
	require.Error(t, ValidateDisplayName(strings.Repeat("я", 81)))
}
func TestRegistrationRollsBackWhenDeliveryCannotBeQueued(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectExec("DELETE FROM email_verification_tokens").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE verification_email_outbox").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO email_verification_tokens").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO verification_email_outbox").WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()
	_, err = (&Storage{DB: db}).RegisterAccount(context.Background(), "test@example.com", "Name", []byte("hash"), "verify", "refresh", time.Now().Add(time.Hour))
	require.EqualError(t, err, "outbox unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestResetDeliveryLimitPreservesExistingLink(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(false))
	mock.ExpectRollback()
	require.NoError(t, (&Storage{DB: db}).RequestPasswordReset(context.Background(), "test@example.com", "secret"))
	require.NoError(t, mock.ExpectationsWereMet())
}
