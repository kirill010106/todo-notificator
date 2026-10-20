package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"

	"github.com/kirill010106/todo-notificator/clients/activitylogger"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/account"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/login"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/logout"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/refresh"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/auth/resend"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/create"
	categoriesdelete "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/delete"
	categoriesget "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/get"
	categoriesgetone "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/getone"
	categoriesupdate "github.com/kirill010106/todo-notificator/internal/http-server/handlers/categories/update"
	contractHandlers "github.com/kirill010106/todo-notificator/internal/http-server/handlers/contracts"
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
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/bulk"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/delete"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/get"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/save"
	"github.com/kirill010106/todo-notificator/internal/http-server/handlers/tasks/update"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/clientip"
	contractsMiddleware "github.com/kirill010106/todo-notificator/internal/http-server/middleware/contracts"
	customlogger "github.com/kirill010106/todo-notificator/internal/http-server/middleware/logger"
	metricsMiddleware "github.com/kirill010106/todo-notificator/internal/http-server/middleware/metrics"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/ratelimit"
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
	// Keep optional interfaces genuinely nil when payments are disabled.
	var paymentFinder syncpayment.PaymentFinder
	if cfg.PaymentHandler != nil {
		paymentFinder = cfg.PaymentHandler
	}
	r := chi.NewRouter()
	proxies, err := cfg.AppConfig.ProxyPrefixes()
	if err != nil {
		panic(err)
	} // invalid configuration must fail before serving
	r.Use(clientip.New(proxies))
	publicLimit := ratelimit.IP(100)
	loginLimit := ratelimit.IP(20)
	callbackLimit := ratelimit.IP(300)

	// Preserve a truly nil interface when the optional activity logger failed
	// to initialize; converting a nil *Client directly would produce a non-nil
	// interface value.
	var logsGetter logsget.LogsGetter
	if cfg.LoggerClient != nil {
		logsGetter = cfg.LoggerClient
	}

	// Global Middlewares
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(middleware.RequestID)
	r.Use(contractsMiddleware.Errors)
	r.Use(customlogger.New(cfg.Log))
	r.Use(metricsMiddleware.New())
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)
	r.Use(middleware.RedirectSlashes)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		helpers.SendError(w, http.StatusNotFound, "endpoint does not exist", "NOT_FOUND")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		helpers.SendError(w, http.StatusMethodNotAllowed, "method not allowed", "METHOD_NOT_ALLOWED")
	})

	r.With(publicLimit).Get("/", func(w http.ResponseWriter, r *http.Request) {
		render.Status(r, http.StatusOK)
		render.JSON(w, r, map[string]string{
			"status":  "active",
			"project": "todo-notificator",
			"info":    "Use /api/v1 for requests",
		})
	})

	r.With(publicLimit).Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	// API V1
	r.Route("/api/v1", func(r chi.Router) {
		r.With(publicLimit).Get("/", func(w http.ResponseWriter, r *http.Request) {
			render.Status(r, http.StatusOK)
			render.JSON(w, r, map[string]string{
				"status":  "active",
				"project": "todo-notificator",
				"info":    "Use /api/v1/health for checking availability of the service",
			})
		})

		accounts := account.Handler{Store: cfg.Storage, Config: cfg.AppConfig}
		if cfg.LoggerClient != nil {
			accounts.Events = cfg.LoggerClient
		}
		// Public endpoints
		r.With(loginLimit).Post("/register", accounts.Register)
		r.With(loginLimit).Post("/password/forgot", accounts.Forgot)
		r.With(loginLimit).Post("/password/reset", accounts.Reset)
		r.With(loginLimit).Post("/login", login.New(cfg.Log, cfg.Storage, cfg.AppConfig, cfg.LoggerClient))
		r.Get("/health", health.New(cfg.Log, cfg.Storage.DB))
		r.With(publicLimit).Post("/refresh", refresh.New(cfg.Log, cfg.Storage, cfg.AppConfig))
		r.With(publicLimit).Get("/verify", accounts.Verify)
		r.With(callbackLimit).Post("/webhooks/yookassa", webhook.New(cfg.Log, cfg.Storage, paymentFinder))

		// Protected endpoints
		r.Group(func(r chi.Router) {
			r.Use(ratelimit.InvalidAuth(cfg.AppConfig.AppSecret))
			r.Use(auth.New(cfg.AppConfig.AppSecret, cfg.Storage))
			r.Use(ratelimit.User(100))
			extended := contractHandlers.Handler{Store: cfg.Storage}
			r.Get("/me/focus", extended.Focus)
			r.Get("/me/shop", extended.Shop)
			r.Post("/me/shop/actions", contractsMiddleware.Operation(cfg.Storage, extended.ShopAction))
			r.Post("/pomodoros/{id}/cancel", contractsMiddleware.Operation(cfg.Storage, extended.Cancel))
			r.Post("/me/focus/resume", contractsMiddleware.Operation(cfg.Storage, extended.Resume))
			r.Post("/me/focus/rest/finish", contractsMiddleware.Operation(cfg.Storage, extended.FinishRest))
			r.Get("/me/today", extended.Today)
			r.Patch("/me/profile", accounts.Profile)
			r.Post("/me/email", accounts.Change)
			r.Post("/me/password", accounts.Change)
			r.Post("/me/quests/{code}/claim", contractsMiddleware.Operation(cfg.Storage, extended.Claim))
			r.Post("/me/rewards/daily-chest/claim", contractsMiddleware.Operation(cfg.Storage, extended.Claim))

			r.Post("/logout", logout.New(cfg.Log, cfg.Storage))

			// Aggregated initial state & profile
			r.Get("/me/bootstrap", bootstrap.New(cfg.Log, cfg.Storage))
			r.Get("/me/profile", profileget.New(cfg.Log, cfg.Storage))
			r.Get("/me/achievements", achievements.New(cfg.Log, cfg.Storage))
			r.Get("/me/quests", quests.New(cfg.Log, cfg.Storage))
			r.Get("/me/logs", logsget.New(cfg.Log, logsGetter))
			r.Get("/me/stats", statsget.New(cfg.Log, cfg.Storage))

			// Tasks
			r.Get("/tasks", get.New(cfg.Log, cfg.Storage))
			r.Post("/tasks", contractsMiddleware.Operation(cfg.Storage, save.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret, cfg.LoggerClient)))
			r.Delete("/tasks/{task_id}", delete.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Patch("/tasks/{task_id}", contractsMiddleware.Operation(cfg.Storage, update.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret, cfg.LoggerClient)))
			r.Post("/tasks/bulk-complete", contractsMiddleware.Operation(cfg.Storage, bulk.NewComplete(cfg.Log, cfg.Storage)))
			r.Post("/tasks/bulk-delete", bulk.NewDelete(cfg.Log, cfg.Storage))

			// Categories
			r.Post("/categories", create.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Get("/categories", categoriesget.New(cfg.Log, cfg.Storage))
			r.Get("/categories/{category_id}", categoriesgetone.New(cfg.Log, cfg.Storage))
			r.Patch("/categories/{category_id}", categoriesupdate.New(cfg.Log, cfg.Storage, cfg.LoggerClient))
			r.Delete("/categories/{category_id}", categoriesdelete.New(cfg.Log, cfg.Storage, cfg.LoggerClient))

			// Pomodoros
			r.Post("/pomodoros/start", contractsMiddleware.Operation(cfg.Storage, pomodorostart.New(cfg.Log, cfg.Storage, cfg.LoggerClient)))
			r.Get("/pomodoros/active", pomodoroactive.New(cfg.Log, cfg.Storage))
			r.Post("/pomodoros/{id}/pause", contractsMiddleware.Operation(cfg.Storage, pomodoropause.New(cfg.Log, cfg.Storage, cfg.LoggerClient)))
			r.Post("/pomodoros/{id}/stop", contractsMiddleware.Operation(cfg.Storage, pomodorostop.New(cfg.Log, cfg.Storage, cfg.LoggerClient)))

			// Verification & Payments
			r.Post("/verify/resend", resend.New(cfg.Log, cfg.Storage, cfg.AppConfig.Webhook.URL, cfg.AppConfig.Webhook.Secret))
			// Provider idempotency and the payment UPSERT handle retries without holding
			// the user's game-action transaction across a network call.
			r.Post("/payments/create", createpayment.New(cfg.Log, cfg.Storage, cfg.YooClient, cfg.AppConfig.ClientURL))
			r.Post("/payments/sync", syncpayment.New(cfg.Log, cfg.Storage, paymentFinder))

			// Dev endpoints (local env only)
			if cfg.AppConfig.Env == "local" {
				r.Post("/dev/toggle-premium", devpremium.New(cfg.Log, cfg.Storage))
			}
		})
	})

	return r
}
