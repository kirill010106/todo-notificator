package postgres

import (
	"context"
	"database/sql"
	"errors"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPaymentPremiumTransactionFailureBoundaries(t *testing.T) {
	sentinel := errors.New("injected storage failure")
	for _, stage := range []string{"begin", "missing", "payment update", "premium update", "commit", "success"} {
		t.Run(stage, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			begin := mock.ExpectBegin()
			if stage == "begin" {
				begin.WillReturnError(sentinel)
			} else {
				payment := mock.ExpectQuery("UPDATE payments").WithArgs("succeeded", "payment-1")
				switch stage {
				case "missing":
					payment.WillReturnError(sql.ErrNoRows)
					mock.ExpectRollback()
				case "payment update":
					payment.WillReturnError(sentinel)
					mock.ExpectRollback()
				default:
					payment.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(42))
					premium := mock.ExpectExec("UPDATE users SET is_premium = true").WithArgs(int64(42))
					if stage == "premium update" {
						premium.WillReturnError(sentinel)
						mock.ExpectRollback()
					} else {
						premium.WillReturnResult(sqlmock.NewResult(0, 1))
						commit := mock.ExpectCommit()
						if stage == "commit" {
							commit.WillReturnError(sentinel)
						}
					}
				}
			}
			id, err := (&Storage{DB: db}).CompletePaymentAndGrantPremium(context.Background(), "payment-1")
			if stage == "success" {
				require.NoError(t, err)
				require.Equal(t, int64(42), id)
			} else {
				require.Error(t, err)
				require.Zero(t, id)
				if stage == "missing" {
					require.ErrorIs(t, err, storage.ErrPaymentNotFound)
				} else {
					require.ErrorIs(t, err, sentinel)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
