package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestPaymentTransitionClassifiesConflictAndCreationRace(t *testing.T) {
	for _, test := range []struct {
		current string
		err     error
	}{
		{"succeeded", storage.ErrPaymentStateConflict},
		{"pending", storage.ErrPaymentNotFound}, // Insert after UPDATE: retry, never acknowledge without processing.
		{"canceled", storage.ErrPaymentNotFound},
		{"", storage.ErrPaymentNotFound},
	} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectQuery("UPDATE payments").WithArgs("canceled", "id").WillReturnError(sql.ErrNoRows)
		query := mock.ExpectQuery("SELECT status FROM payments").WithArgs("id")
		if test.current == "" {
			query.WillReturnError(sql.ErrNoRows)
		} else {
			query.WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(test.current))
		}
		_, err = (&Storage{DB: db}).UpdatePaymentStatus(context.Background(), "id", "canceled")
		require.ErrorIs(t, err, test.err)
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestUnsupportedStatusCannotBypassAtomicPremiumGrant(t *testing.T) {
	_, err := (&Storage{}).UpdatePaymentStatus(context.Background(), "id", "succeeded")
	require.ErrorIs(t, err, storage.ErrPaymentStateConflict)
}
