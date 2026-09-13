package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/config"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/formatter"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/sender"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/webhook"
	"github.com/kirill010106/todo-notificator/notifiers/shared/scheduler"
	"github.com/kirill010106/todo-notificator/notifiers/shared/storage/postgres"
)

func main() {
	cfg := config.MustLoad()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	storage, err := postgres.New(cfg.Database.DBUrl)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer storage.Close()
	logger.Info("database connected")

	fmtr, err := formatter.New(cfg.AppURL)
	if err != nil {
		log.Fatalf("failed to create formatter: %v", err)
	}
	logger.Info("formatter created")

	sndr := sender.New(logger, cfg.SMTP, fmtr)
	logger.Info("sender created",
		slog.String("host", cfg.SMTP.Host),
		slog.Int("port", cfg.SMTP.Port),
	)

	sched := scheduler.New(
		logger,
		storage,
		sndr,
		cfg.Intervals,
	)

	webhookHandler := webhook.New(logger, sched, sndr, cfg.Webhook.Secret)
	srv := &http.Server{
		Addr:    cfg.Webhook.Address,
		Handler: webhookHandler,
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	go func() {
		logger.Info("webhook server started",
			slog.String("address", cfg.Webhook.Address))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("webhook server error: %v", err)
		}
	}()

	logger.Info("starting email notifier")
	sched.Start(ctx)

	// Graceful shutdown webhook server
	if err := srv.Shutdown(context.Background()); err != nil {
		logger.Error("webhook server shutdown error", sl.Err(err))
	}

	logger.Info("email notifier stopped")
}
