package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const serviceShutdownTimeout = 65 * time.Second

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

type httpRuntime interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}

// All workers and legacy webhook sends share one shutdown budget. The separate
// HTTP budget forces slow connections closed without consuming all SMTP time.
func serve(ctx context.Context, srv httpRuntime, grace time.Duration, stopWebhook func(context.Context) error, workers ...func(context.Context)) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(runCtx)
		}()
	}
	workersDone := make(chan struct{})
	go func() { wg.Wait(); close(workersDone) }()
	serverDone := make(chan struct{})
	serverError := make(chan error, 1)
	go func() {
		defer close(serverDone)
		serverError <- srv.ListenAndServe()
	}()
	var result error
	select {
	case <-ctx.Done():
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	}
	cancel() // Stop new delivery claims before waiting for current sends/acks.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), grace)
	defer stopCancel()
	webhookDone := make(chan error, 1)
	go func() { webhookDone <- stopWebhook(stopCtx) }()
	httpCtx, httpCancel := context.WithTimeout(stopCtx, 10*time.Second)
	if err := srv.Shutdown(httpCtx); err != nil {
		result = errors.Join(result, err, srv.Close())
	}
	httpCancel()
	for _, done := range []<-chan struct{}{workersDone, serverDone} {
		select {
		case <-done:
		case <-stopCtx.Done():
			return errors.Join(result, stopCtx.Err())
		}
	}
	select {
	case err := <-webhookDone:
		return errors.Join(result, err)
	case <-stopCtx.Done():
		return errors.Join(result, stopCtx.Err())
	}
}
