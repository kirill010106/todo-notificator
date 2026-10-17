package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/config"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/formatter"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/passwordreset"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/sender"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/verification"
	"github.com/kirill010106/todo-notificator/notifiers/email/internal/webhook"
	"github.com/kirill010106/todo-notificator/notifiers/shared/scheduler"
	"github.com/kirill010106/todo-notificator/notifiers/shared/storage/postgres"
	"github.com/kirill010106/todo-notificator/root/pkg/envconfig"
)

func main() {
	if err := envconfig.Load(); err != nil {
		log.Fatal(err)
	}
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
		slog.String("username", cfg.SMTP.Username),
		slog.String("from", cfg.SMTP.From),
	)

	sched := scheduler.New(
		logger,
		storage,
		sndr,
	)
	verificationWorker := verification.New(logger, storage, sndr)

	webhookHandler := webhook.New(logger, sched, sndr, cfg.Webhook.Secret)
	mux := http.NewServeMux()
	mux.Handle("/", webhookHandler)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := storage.DB.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{
		Addr:    cfg.Webhook.Address,
		Handler: mux,
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
	go verificationWorker.Run(ctx)
	go passwordreset.New(logger, storage, sndr).Run(ctx)
	sched.Start(ctx)

	// Graceful shutdown webhook server
	if err := srv.Shutdown(context.Background()); err != nil {
		logger.Error("webhook server shutdown error", sl.Err(err))
	}

	logger.Info("email notifier stopped")
}
