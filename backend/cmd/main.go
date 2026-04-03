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
	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/login"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/logout"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/refresh"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/register"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/verify"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/create"
	categoriesdelete "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/delete"
	categoriesget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/get"
	categoriesupdate "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/update"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/health"
	pomodoropause "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/pause"
	pomodorostart "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/start"
	pomodorostop "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/stop"
	statsget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/stats/get"
	statsupdate "github.com/kirill010106/todo-notificator/internal/http-server/handlers/stats/update"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/http-server/tasks/delete"
	"github.com/kirill010106/todo-notificator/internal/http-server/tasks/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/tasks/save"
	"github.com/kirill010106/todo-notificator/internal/http-server/tasks/update"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/pressly/goose/v3"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	"github.com/kirill010106/todo-notificator/internal/config"
	slogpretty "github.com/kirill010106/todo-notificator/internal/lib/handlers"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

const (
	envLocal = "local"
	envDev   = "dev"
	envProd  = "prod"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error while loading .env file: %v", err)
	}
	cfg := config.MustLoad()

	log := setupLogger(cfg.Env)

	log.Info("starting todo-notificator", slog.String("env", cfg.Env))
	log.Debug("debug messages are enabled")

	DBUrl := os.Getenv("DATABASE_URL")
	storage, err := postgres.New(DBUrl)
	if err != nil {
		log.Error("failed to init db", sl.Err(err))
		os.Exit(1)
	}
	defer storage.Close()

	goose.SetBaseFS(todonotificator.MigrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		log.Error("failed to setup migrations", sl.Err(err))
		os.Exit(1)
	}

	log.Info("Running migrations...")
	if err := goose.Up(storage.DB, "migrations"); err != nil {
		log.Error("failed to run migrations", sl.Err(err))
		os.Exit(1)
	}
	log.Info("Migrations applied successfully!")

	router := chi.NewRouter()

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.URLFormat)

	router.Route("/api/v1", func(r chi.Router) {

		r.Post("/register", register.New(log, storage, cfg.Webhook.URL, cfg.Webhook.Secret))
		r.Post("/login", login.New(log, storage, cfg))
		r.Get("/health", health.New(log, storage.DB))
		r.Post("/refresh", refresh.New(log, storage, cfg))
		r.Get("/verify", verify.New(log, storage))

		r.Group(func(r chi.Router) {
			r.Use(auth.New(cfg.AppSecret))

			r.Post("/logout", logout.New(log, storage))
			r.Get("/tasks", get.New(log, storage))
			r.Post("/tasks", save.New(log, storage, cfg.Webhook.URL, cfg.Webhook.Secret))
			r.Delete("/tasks/{task_id}", delete.New(log, storage))
			r.Patch("/tasks/{task_id}", update.New(log, storage))

			r.Post("/categories", create.New(log, storage))
			r.Get("/categories", categoriesget.New(log, storage))
			r.Patch("/categories/{category_id}", categoriesupdate.New(log, storage))
			r.Delete("/categories/{category_id}", categoriesdelete.New(log, storage))

			r.Get("/me/stats", statsget.New(log, storage))
			r.Patch("/me/stats", statsupdate.New(log, storage))

			r.Post("/pomodoros/start", pomodorostart.New(log, storage))
			r.Post("/pomodoros/{id}/pause", pomodoropause.New(log, storage))
			r.Post("/pomodoros/{id}/stop", pomodorostop.New(log, storage))

		})

	})

	srv := &http.Server{
		Addr:        cfg.Address,
		Handler:     router,
		ReadTimeout: cfg.Timeout,
		IdleTimeout: cfg.IdleTimeout,
	}

	go func() {
		log.Info("starting HTTP server", slog.String("address", cfg.Address))

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", sl.Err(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info("shutdown signal received", slog.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("forced shutdown", sl.Err(err))
		os.Exit(1)
	}

	log.Info("server stopped gracefully")
}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger
	switch env {
	case envLocal:
		log = setupPrettySlog()
	case envDev:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}),
		)
	case envProd:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}),
		)
	default:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}),
		)
	}

	return log
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
