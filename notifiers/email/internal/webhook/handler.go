package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
)

type Scheduler interface {
	Reschedule()
}

type Handler struct {
	log       *slog.Logger
	scheduler Scheduler
	sender    EmailSender
	secret    string
	jobsMu    sync.Mutex
	jobs      sync.WaitGroup
	stopping  bool
	stopOnce  sync.Once
	done      chan struct{}
}

type EmailSender interface {
	SendVerificationEmail(email, token string) error
}

type Payload struct {
	Type  string `json:"type"`
	Email string `json:"email,omitempty"`
	Token string `json:"token,omitempty"`
}

func New(log *slog.Logger, scheduler Scheduler, sender EmailSender, secret string) *Handler {
	return &Handler{
		log:       log,
		scheduler: scheduler,
		sender:    sender,
		secret:    secret,
		done:      make(chan struct{}),
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
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Webhook-Secret")), []byte(h.secret)) != 1 {
		h.log.Warn("webhook: unauthorized request",
			slog.String("remote_addr", r.RemoteAddr))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var payload Payload

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		h.log.Error("webhook: failed to decode request body", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if payload.Type == "verification" && payload.Email != "" && payload.Token != "" {
		h.jobsMu.Lock()
		if h.stopping {
			h.jobsMu.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		h.jobs.Add(1)
		h.jobsMu.Unlock()
		h.log.Info("webhook: sending verification email", slog.String("email", payload.Email))
		go func() {
			defer h.jobs.Done()
			if err := h.sender.SendVerificationEmail(payload.Email, payload.Token); err != nil {
				h.log.Error("webhook: failed to send verification email", slog.String("error", err.Error()))
			}
		}()
		w.WriteHeader(http.StatusOK)
		return
	}

	switch payload.Type {
	case "task_created":
		h.log.Info("webhook: task created, triggering reschedule")
	case "task_updated":
		h.log.Info("webhook: task updated, triggering reschedule")
	case "":
		h.log.Info("webhook: task event received (legacy payload), triggering reschedule")
	default:
		h.log.Info("webhook: task event received, triggering reschedule", slog.String("type", payload.Type))
	}
	h.scheduler.Reschedule()

	w.WriteHeader(http.StatusOK)
}

// Shutdown stops admission before waiting, so no Add can race with Wait.
// SMTP already accepted by the legacy webhook may finish within the deadline.
func (h *Handler) Shutdown(ctx context.Context) error {
	h.stopOnce.Do(func() {
		h.jobsMu.Lock()
		h.stopping = true
		h.jobsMu.Unlock()
		go func() {
			h.jobs.Wait()
			close(h.done)
		}()
	})
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
