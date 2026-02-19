package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	todonotificator "github.com/kirill010106/todo-notificator/backend"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/handlers/auth/login"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/handlers/auth/logout"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/handlers/auth/refresh"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/handlers/auth/register"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/handlers/health"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/tasks/delete"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/tasks/get"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/tasks/save"
	"github.com/kirill010106/todo-notificator/backend/internal/http-server/tasks/update"
	"github.com/kirill010106/todo-notificator/backend/internal/storage/postgres"
	"github.com/pressly/goose/v3"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	"github.com/kirill010106/todo-notificator/backend/internal/config"
	slogpretty "github.com/kirill010106/todo-notificator/backend/internal/lib/handlers"
	"github.com/kirill010106/todo-notificator/backend/internal/lib/sl"
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
	if err := goose.Up(storage.Db, "migrations"); err != nil {
		log.Error("failed to run migrations", sl.Err(err))
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

		r.Post("/register", register.New(log, storage))
		r.Post("/login", login.New(log, storage, cfg))
		r.Get("/health", health.New(log, storage.Db))
		r.Post("/refresh", refresh.New(log, storage, cfg))

		r.Group(func(r chi.Router) {
			r.Use(auth.New(cfg.AppSecret))

			r.Post("/logout", logout.New(log, storage))
			r.Get("/tasks", get.New(log, storage))
			r.Post("/tasks", save.New(log, storage))
			r.Delete("/tasks/{task_id}", delete.New(log, storage))
			r.Patch("/tasks/{task_id}", update.New(log, storage))
		})

	})

	log.Info("starting HTTP server", slog.String("address", cfg.Address))
	srv := &http.Server{
		Addr:        cfg.Address,
		Handler:     router,
		ReadTimeout: cfg.Timeout,
		IdleTimeout: cfg.IdleTimeout,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Error("failed to start server")
	}
	log.Error("server stopped")
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
