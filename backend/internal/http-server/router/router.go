package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/go-chi/render"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"

	"github.com/kirill010106/todo-notificator/clients/activitylogger"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/login"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/logout"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/refresh"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/register"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/resend"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/verify"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/create"
	categoriesdelete "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/delete"
	categoriesget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/get"
	categoriesgetone "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/getone"
	categoriesupdate "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/update"
	devpremium "github.com/kirill010106/todo-notificator/internal/http-server/handlers/dev/premium"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/health"
	logsget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/logs/get"
	createpayment "github.com/kirill010106/todo-notificator/internal/http-server/handlers/payments/create"
	syncpayment "github.com/kirill010106/todo-notificator/internal/http-server/handlers/payments/sync"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/payments/webhook"
	pomodoroactive "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/active"
	pomodoropause "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/pause"
	pomodorostart "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/start"
	pomodorostop "github.com/kirill010106/todo-notificator/internal/http-server/handlers/pomodoros/stop"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/profile/achievements"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/profile/bootstrap"
	profileget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/profile/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/profile/quests"
	statsget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/stats/get"
	statsupdate "github.com/kirill010106/todo-notificator/internal/http-server/handlers/stats/update"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/bulk"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/delete"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/save"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/update"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	customlogger "github.com/kirill010106/todo-notificator/internal/http-server/middleware/logger"
	metricsMiddleware "github.com/kirill010106/todo-notificator/internal/http-server/middleware/metrics"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
)

type Config struct {
	Log            *slog.Logger
	Storage        *postgres.Storage
	AppConfig      *config.Config
	LoggerClient   *activitylogger.Client
	YooClient      *yookassa.Client
	PaymentHandler *yookassa.PaymentHandler
}

// New builds and configures the complete application Chi router.
func New(cfg Config) *chi.Mux {
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(middleware.RequestID)
	r.Use(customlogger.New(cfg.Log))
	r.Use(metricsMiddleware.New())
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)
	r.Use(middleware.RedirectSlashes)
	r.Use(httprate.LimitByIP(100, 1*time.Minute))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		helpers.SendError(w, http.StatusNotFound, "endpoint does not exist", "NOT_FOUND")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		helpers.SendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusOK)
		render.JSON(w, r, map[string]string{
			"status":  "active",
			"project": "todo-notificator",
			"info":    "Use /api/v1 for requests",
		})
	})

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	// API V1
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			render.Status(r, http.StatusOK)
			render.JSON(w, r, map[string]string{
				"status":  "active",
				"project": "todo-notificator",
				"info":    "Use /api/v1/health for checking availability of the service",
			})
		})

		// Public endpoints
		r.Post("/register", register.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret, cfg.LoggerClient))
		r.Post("/login", login.New(cfg.Log, cfg.Storage, cfg.AppConfig, cfg.LoggerClient))
		r.Get("/health", health.New(cfg.Log, cfg.Storage.DB))
		r.Post("/refresh", refresh.New(cfg.Log, cfg.Storage, cfg.AppConfig))
		r.Get("/verify", verify.New(cfg.Log, cfg.Storage))
		r.Post("/webhooks/yookassa", webhook.New(cfg.Log, cfg.Storage, cfg.PaymentHandler))

		// Protected endpoints
		r.Group(func(r chi.Router) {
			r.Use(auth.New(cfg.AppConfig.AppSecret))

			r.Post("/logout", logout.New(cfg.Log, cfg.Storage))

			// Aggregated initial state & profile
			r.Get("/me/bootstrap", bootstrap.New(cfg.Log, cfg.Storage))
			r.Get("/me/profile", profileget.New(cfg.Log, cfg.Storage))
			r.Get("/me/achievements", achievements.New(cfg.Log, cfg.Storage))
			r.Get("/me/quests", quests.New(cfg.Log, cfg.Storage))
			r.Get("/me/logs", logsget.New(cfg.Log, cfg.LoggerClient))
			r.Get("/me/stats", statsget.New(cfg.Log, cfg.Storage))
			r.Patch("/me/stats", statsupdate.New(cfg.Log, cfg.Storage))

			// Tasks
			r.Get("/tasks", get.New(cfg.Log, cfg.Storage))
			r.Post("/tasks", save.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret, cfg.LoggerClient))
			r.Delete("/tasks/{task_id}", delete.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Patch("/tasks/{task_id}", update.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret, cfg.LoggerClient))
			r.Post("/tasks/bulk-complete", bulk.NewComplete(cfg.Log, cfg.Storage))
			r.Post("/tasks/bulk-delete", bulk.NewDelete(cfg.Log, cfg.Storage))

			// Categories
			r.Post("/categories", create.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Get("/categories", categoriesget.New(cfg.Log, cfg.Storage))
			r.Get("/categories/{category_id}", categoriesgetone.New(cfg.Log, cfg.Storage))
			r.Patch("/categories/{category_id}", categoriesupdate.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Delete("/categories/{category_id}", categoriesdelete.New(cfg.Log, cfg.Storage, cfg.LoggerClient))

			// Pomodoros
			r.Post("/pomodoros/start", pomodorostart.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Get("/pomodoros/active", pomodoroactive.New(cfg.Log, cfg.Storage))
			r.Post("/pomodoros/{id}/pause", pomodoropause.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Post("/pomodoros/{id}/stop", pomodorostop.New(cfg.Log, cfg.Storage, cfg.LoggerClient))

			// Verification & Payments
			r.Post("/verify/resend", resend.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret))
			r.Post("/payments/create", createpayment.New(cfg.Log, cfg.Storage, cfg.YooClient, cfg.AppConfig.ClientURL))
			r.Post("/payments/sync", syncpayment.New(cfg.Log, cfg.Storage, cfg.PaymentHandler))

			// Dev endpoints (local env only)
			if cfg.AppConfig.Env == "local" {
				r.Post("/dev/toggle-premium", devpremium.New(cfg.Log, cfg.Storage))
			}
		})
	})

	return r
}
