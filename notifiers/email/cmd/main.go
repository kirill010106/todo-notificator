package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kirill010106/todo-notificator/notifiers/email/internal/config"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/formatter"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/sender"
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

	fmtr, err := formatter.New()
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
		cfg.NotificationIntervals(),
	)

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	logger.Info("starting email notifier")
	sched.Start(ctx)

	logger.Info("email notifier stopped")
}
