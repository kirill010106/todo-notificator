package postgres

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/stretchr/testify/require"
)

func expectUserTransaction(mock sqlmock.Sqlmock, userID int64) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM users").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userID))
}

func expectNoNewAchievements(mock sqlmock.Sqlmock, userID int64) {
	mock.ExpectExec("INSERT INTO user_stats").WithArgs(userID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"streak", "best", "pomodoros", "burnt"}).AddRow(0, 0, 0, 0))
	mock.ExpectQuery("SELECT COUNT").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"done", "night", "early"}).AddRow(0, 0, 0))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"premium"}).AddRow(false))
}

func TestUpdateTaskRewardFailureRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &Storage{DB: db}
	expectUserTransaction(mock, 1)
	mock.ExpectQuery("SELECT id, status, reward_claimed FROM tasks").WithArgs(int64(1), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "claimed"}).AddRow(10, "pending", false))
	mock.ExpectExec("UPDATE tasks").WillReturnResult(sqlmock.NewResult(0, 1))
	dbFailure := errors.New("stats write failed")
	mock.ExpectExec("INSERT INTO user_stats").WillReturnError(dbFailure)
	mock.ExpectRollback()
	status := domain.TaskStatusDone
	err = s.UpdateTask(context.Background(), 1, 10, domain.TaskUpdate{Status: &status})
	require.ErrorIs(t, err, dbFailure)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsLevelFailureRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &Storage{DB: db}
	expectUserTransaction(mock, 1)
	mock.ExpectExec("INSERT INTO user_stats").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE user_stats").WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow(100))
	dbFailure := errors.New("level write failed")
	mock.ExpectExec("UPDATE user_stats SET level").WillReturnError(dbFailure)
	mock.ExpectRollback()
	require.ErrorIs(t, s.ApplyStatsDelta(context.Background(), 1, domain.StatsDelta{PointsDelta: 10}), dbFailure)
	require.NoError(t, mock.ExpectationsWereMet())
}
