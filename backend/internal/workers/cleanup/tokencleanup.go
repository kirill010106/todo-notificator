// Package cleanup cleanups expired tokens
package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
)

func StartTokenCleanup(ctx context.Context, log *slog.Logger, st *postgres.Storage, interval time.Duration) {
	log.Info("starting background token cleanup worker", slog.String("interval", interval.String()))

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Info("stoping background token cleanup worker")
				return
			case <-ticker.C:
				log.Debug("running expired tokens cleanup")

				opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				if err := st.DeleteExpiredRefreshTokens(opCtx); err != nil {
					log.Error("failed to delete expired tokens", slog.String("error", err.Error()))
				}

				if err := st.DeleteExpiredEmailVerificationTokens(opCtx); err != nil {
					log.Error("failed to delete expired verification tokens", slog.String("error", err.Error()))
				}
				cancel()
			}
		}
	}()

}
