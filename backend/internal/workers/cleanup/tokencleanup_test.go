package cleanup

import (
	"context"
	"errors"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/stretchr/testify/require"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type signalHandler struct {
	stopped chan struct{}
	once    sync.Once
}

func (*signalHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *signalHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "stoping background token cleanup worker" {
		h.once.Do(func() { close(h.stopped) })
	}
	return nil
}
func (h *signalHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *signalHandler) WithGroup(string) slog.Handler      { return h }
func TestCleanupContinuesAfterFailureAndStopsOnCancellation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "storage error"}[fail], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			first := mock.ExpectExec("DELETE FROM refresh_tokens")
			second := mock.ExpectExec("DELETE FROM email_verification_tokens")
			if fail {
				first.WillReturnError(errors.New("refresh cleanup failed"))
				second.WillReturnError(errors.New("verification cleanup failed"))
			} else {
				first.WillReturnResult(sqlmock.NewResult(0, 2))
				second.WillReturnResult(sqlmock.NewResult(0, 3))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler := &signalHandler{stopped: make(chan struct{})}
			StartTokenCleanup(ctx, slog.New(handler), &postgres.Storage{DB: db}, 10*time.Millisecond)
			require.Eventually(t, func() bool { return mock.ExpectationsWereMet() == nil }, time.Second, time.Millisecond)
			cancel()
			select {
			case <-handler.stopped:
			case <-time.After(time.Second):
				t.Fatal("cleanup worker did not stop")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
