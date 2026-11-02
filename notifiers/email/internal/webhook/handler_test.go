package webhook

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type schedulerStub struct{ calls atomic.Int32 }

func (s *schedulerStub) Reschedule() { s.calls.Add(1) }

type senderFunc func(string, string) error

func (s senderFunc) SendVerificationEmail(email, token string) error { return s(email, token) }

func verificationRequest(h *Handler, secret string) int {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"type":"verification","email":"test@example.com","token":"test-token"}`))
	r.Header.Set("X-Webhook-Secret", secret)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestShutdownWaitsForLegacySendAndStopsAdmission(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	h := New(slog.New(slog.DiscardHandler), &schedulerStub{}, senderFunc(func(string, string) error {
		close(entered)
		<-release
		return nil
	}), "test-secret")
	if code := verificationRequest(h, "test-secret"); code != 200 {
		t.Fatalf("first request: %d", code)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("sender not called")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := h.Shutdown(ctx); err != context.DeadlineExceeded {
		t.Fatalf("unfinished SMTP call must honor shutdown budget: %v", err)
	}
	if code := verificationRequest(h, "test-secret"); code != 503 {
		t.Errorf("send admitted after stop: %d", code)
	}
	close(release)
	wait, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := h.Shutdown(wait); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentAdmissionAndShutdown(t *testing.T) {
	var sends, accepted atomic.Int32
	h := New(slog.New(slog.DiscardHandler), &schedulerStub{}, senderFunc(func(string, string) error {
		sends.Add(1)
		return nil
	}), "test-secret")
	var requests sync.WaitGroup
	for i := 0; i < 100; i++ {
		requests.Add(1)
		go func() {
			defer requests.Done()
			code := verificationRequest(h, "test-secret")
			if code == 200 {
				accepted.Add(1)
			} else if code != 503 {
				t.Errorf("unexpected response: %d", code)
			}
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	requests.Wait()
	if sends.Load() != accepted.Load() {
		t.Fatalf("accepted=%d, completed=%d", accepted.Load(), sends.Load())
	}
}

func TestUnauthorizedRequestCannotStartBackgroundSend(t *testing.T) {
	var sends atomic.Int32
	h := New(slog.New(slog.DiscardHandler), &schedulerStub{}, senderFunc(func(string, string) error {
		sends.Add(1)
		return nil
	}), "test-secret")
	if code := verificationRequest(h, "wrong"); code != 401 {
		t.Fatalf("wrong secret response: %d", code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if sends.Load() != 0 {
		t.Fatal("unauthorized request reached SMTP")
	}
}
