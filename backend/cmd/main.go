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

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/clients/activitylogger"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	slogpretty "github.com/kirill010106/todo-notificator/internal/lib/handlers"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/kirill010106/todo-notificator/internal/workers/cleanup"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("info: .env file not found, trying to read from system environment")
	}
	cfg := config.MustLoad()

	appLog := setupLogger(cfg.Env)
	appLog.Info("starting todo-notificator", slog.String("env", cfg.Env))

	dbURL := os.Getenv("DATABASE_URL")
	storage, err := postgres.New(dbURL)

	// Экспорт метрик пула БД в Prometheus
	prometheus.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "db_pool_open_connections",
			Help: "The number of established connections both in use and idle.",
		}, func() float64 { return float64(storage.DB.Stats().OpenConnections) }),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "db_pool_in_use_connections",
			Help: "The number of connections currently in use.",
		}, func() float64 { return float64(storage.DB.Stats().InUse) }),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "db_pool_idle_connections",
			Help: "The number of idle connections.",
		}, func() float64 { return float64(storage.DB.Stats().Idle) }),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "db_pool_wait_count_total",
			Help: "The total number of connections waited for.",
		}, func() float64 { return float64(storage.DB.Stats().WaitCount) }),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "db_pool_wait_duration_seconds_total",
			Help: "The total time blocked waiting for a new connection.",
		}, func() float64 { return storage.DB.Stats().WaitDuration.Seconds() }),
	)

	if err != nil {
		appLog.Error("failed to init db", sl.Err(err))
		os.Exit(1)
	}
	defer storage.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loggerClient, err := activitylogger.New(ctx, appLog, cfg.Clients.ActivityLogger.Address, cfg.Clients.ActivityLogger.Timeout)
	if err != nil {
		appLog.Error("failed to init activity logger client", sl.Err(err))
	}

	cleanup.StartTokenCleanup(ctx, appLog, storage, 24*time.Hour)

	// Database Migrations
	goose.SetBaseFS(todonotificator.MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		appLog.Error("failed to setup migrations", sl.Err(err))
		os.Exit(1)
	}

	appLog.Info("Running migrations...")
	if err := goose.Up(storage.DB, "migrations"); err != nil {
		appLog.Error("failed to run migrations", sl.Err(err))
		os.Exit(1)
	}
	appLog.Info("Migrations applied successfully!")

	// YooKassa Client
	var (
		yooCli         *yookassa.Client
		paymentHandler *yookassa.PaymentHandler
	)
	if cfg.YooKassa.ShopID != "" && cfg.YooKassa.SecretKey != "" {
		yooCli = yookassa.NewClient(cfg.YooKassa.ShopID, cfg.YooKassa.SecretKey)
		paymentHandler = yookassa.NewPaymentHandler(yooCli)
		appLog.Info("yookassa client initialized")
	} else {
		appLog.Info("yookassa client skipped (no credentials configured)")
	}

	// Metrics

	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsSrv := &http.Server{
		Addr:    cfg.MetricsServer.Address,
		Handler: metricsMux,
	}

	go func() {
		appLog.Info("starting metrics server", slog.String("address", metricsSrv.Addr))
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLog.Error("metrics server error", sl.Err(err))
		}
	}()

	// ----

	// Application Router
	appRouter := router.New(router.Config{
		Log:            appLog,
		Storage:        storage,
		AppConfig:      cfg,
		LoggerClient:   loggerClient,
		YooClient:      yooCli,
		PaymentHandler: paymentHandler,
	})

	srv := &http.Server{
		Addr:        cfg.Address,
		Handler:     appRouter,
		ReadTimeout: cfg.Timeout,
		IdleTimeout: cfg.IdleTimeout,
	}

	go func() {
		appLog.Info("starting HTTP server", slog.String("address", cfg.Address))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLog.Error("server error", sl.Err(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	appLog.Info("shutdown signal received", slog.String("signal", sig.String()))

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := metricsSrv.Shutdown(ctx); err != nil {
		appLog.Error("forced metrics server shutdown", sl.Err(err))
	}

	if err := srv.Shutdown(ctx); err != nil {
		appLog.Error("forced shutdown", sl.Err(err))
		os.Exit(1)
	}

	appLog.Info("server stopped gracefully")
}

func setupLogger(env string) *slog.Logger {
	var l *slog.Logger
	switch env {
	case envLocal:
		l = setupPrettySlog()
	case envDev, envProd:
		level := slog.LevelInfo
		if env == envDev {
			level = slog.LevelDebug
		}
		l = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	default:
		l = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return l
}

func setupPrettySlog() *slog.Logger {
	opts := slogpretty.PrettyHandlerOptions{
		SlogOpts: &slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	}
	handler := opts.NewPrettyHandler(os.Stdout)
	return slog.New(handler)
}
