package webhook

import (
	"log/slog"
	"net/http"
)

type Scheduler interface {
	Reschedule()
}

type Handler struct {
	log       *slog.Logger
	scheduler Scheduler
	secret    string
}

func New(log *slog.Logger, scheduler Scheduler, secret string) *Handler {
	return &Handler{
		log:       log,
		scheduler: scheduler,
		secret:    secret,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.secret == "" {
		h.log.Error("webhook: secret is not set")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get("X-Webhook-Secret") != h.secret {
		h.log.Warn("webhook: unauthorized request",
			slog.String("remote_addr", r.RemoteAddr))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	h.log.Info("webhook: task created, triggering reschedule")
	h.scheduler.Reschedule()

	w.WriteHeader(http.StatusOK)
}
