package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/notifiers/email/internal/webhook"
)

type emailScheduler struct{ calls atomic.Int32 }

func (s *emailScheduler) Reschedule() { s.calls.Add(1) }

type emailSender struct{ calls atomic.Int32 }

func (s *emailSender) SendVerificationEmail(string, string) error { s.calls.Add(1); return nil }

func TestHTTPTimeoutsReleaseSlowConnections(t *testing.T) {
	for _, test := range []struct {
		name, request string
		wait          time.Duration
	}{
		{"headers", "POST / HTTP/1.1\r\nHost: test\r\n", 5 * time.Second},
		{"body", "POST / HTTP/1.1\r\nHost: test\r\nX-Webhook-Secret: test-secret\r\nContent-Length: 100\r\n\r\n{", 13 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scheduler, sender := &emailScheduler{}, &emailSender{}
			handler := webhook.New(slog.New(slog.DiscardHandler), scheduler, sender, "test-secret")
			srv := newHTTPServer("127.0.0.1:0", handler)
			for _, timeout := range []time.Duration{srv.ReadHeaderTimeout, srv.ReadTimeout, srv.WriteTimeout, srv.IdleTimeout} {
				if timeout <= 0 || timeout > 2*time.Minute {
					t.Fatalf("unbounded HTTP timeout: %s", timeout)
				}
			}
			listener, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { defer close(done); _ = srv.Serve(listener) }()
			t.Cleanup(func() { _ = srv.Close(); <-done })
			conn, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(test.wait))
			if _, err := io.WriteString(conn, test.request); err != nil {
				t.Fatal(err)
			}
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				t.Fatal("server left the slow connection open past its timeout")
			}
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode < 400 {
					t.Fatalf("incomplete request accepted: %d", response.StatusCode)
				}
			}
			if scheduler.calls.Load() != 0 || sender.calls.Load() != 0 {
				t.Fatal("incomplete request caused a delivery side effect")
			}
		})
	}
}

type runtimeStub struct {
	started   chan struct{}
	stopped   chan struct{}
	once      sync.Once
	forced    atomic.Int32
	shutdown  func(context.Context) error
	listenErr error
}

func newRuntime() *runtimeStub {
	return &runtimeStub{started: make(chan struct{}), stopped: make(chan struct{})}
}
func (s *runtimeStub) ListenAndServe() error {
	close(s.started)
	if s.listenErr != nil {
		return s.listenErr
	}
	<-s.stopped
	return http.ErrServerClosed
}
func (s *runtimeStub) Shutdown(ctx context.Context) error {
	if s.shutdown != nil {
		return s.shutdown(ctx)
	}
	s.once.Do(func() { close(s.stopped) })
	return nil
}
func (s *runtimeStub) Close() error {
	s.forced.Add(1)
	s.once.Do(func() { close(s.stopped) })
	return nil
}

func await(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lifecycle step did not complete")
	}
}

func TestServiceWaitsForWorkersAcknowledgementsAndLegacySends(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := newRuntime()
	workerCanceled, releaseAck, releaseLegacy := make(chan struct{}), make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, srv, time.Second, func(context.Context) error { <-releaseLegacy; return nil },
			func(ctx context.Context) { <-ctx.Done(); close(workerCanceled); <-releaseAck })
	}()
	await(t, srv.started)
	cancel()
	await(t, workerCanceled)
	select {
	case <-result:
		t.Fatal("stopped before worker acknowledgement")
	default:
	}
	close(releaseAck)
	select {
	case <-result:
		t.Fatal("stopped before legacy SMTP completion")
	default:
	}
	close(releaseLegacy)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func TestServiceForcesHTTPClosedAtDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := newRuntime()
	srv.shutdown = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	result := make(chan error, 1)
	go func() { result <- serve(ctx, srv, 30*time.Millisecond, func(context.Context) error { return nil }) }()
	await(t, srv.started)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) || srv.forced.Load() != 1 {
			t.Fatalf("shutdown=%v, forced=%d", err, srv.forced.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP shutdown exceeded budget")
	}
}

func TestServiceBoundsStuckWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := newRuntime()
	release, finished := make(chan struct{}), make(chan struct{})
	defer close(release)
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, srv, 30*time.Millisecond, func(context.Context) error { return nil },
			func(ctx context.Context) { <-ctx.Done(); <-release; close(finished) })
	}()
	await(t, srv.started)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker wait exceeded budget")
	}
}

func TestListenFailureStopsWorkers(t *testing.T) {
	srv := newRuntime()
	srv.listenErr = errors.New("injected bind failure")
	stopped := make(chan struct{})
	err := serve(context.Background(), srv, time.Second, func(context.Context) error { return nil },
		func(ctx context.Context) { <-ctx.Done(); close(stopped) })
	if !errors.Is(err, srv.listenErr) {
		t.Fatal(err)
	}
	await(t, stopped)
}
