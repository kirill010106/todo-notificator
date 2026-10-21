package activitylogger

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type blockedEvents struct {
	pb.ActivityLoggerClient
	started chan struct{}
	active  atomic.Int32
	peak    atomic.Int32
}

func (b *blockedEvents) LogEvent(ctx context.Context, _ *pb.LogRequest, _ ...grpc.CallOption) (*pb.LogResponse, error) {
	active := b.active.Add(1)
	defer b.active.Add(-1)
	for peak := b.peak.Load(); active > peak; peak = b.peak.Load() {
		if b.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	b.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

type countedCloser struct{ calls atomic.Int32 }

func (c *countedCloser) Close() error { c.calls.Add(1); return nil }

func closeClient(t *testing.T, client *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, client.Close(ctx))
}

func dropCount(reason string) float64 {
	var metric dto.Metric
	if err := droppedEvents.WithLabelValues(reason).Write(&metric); err != nil {
		panic(err)
	}
	return metric.GetCounter().GetValue()
}

func TestEventsBoundConcurrencyQueueAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := &blockedEvents{started: make(chan struct{}, eventWorkers)}
	closer := &countedCloser{}
	client := newClient(ctx, slog.New(slog.DiscardHandler), api, time.Minute, closer)
	t.Cleanup(func() { closeClient(t, client) })
	for i := 0; i < eventWorkers; i++ {
		client.LogEvent(7, "TEST", int64(i), nil)
	}
	for i := 0; i < eventWorkers; i++ {
		select {
		case <-api.started:
		case <-time.After(time.Second):
			t.Fatal("worker did not enter RPC")
		}
	}
	for i := 0; i < eventCapacity; i++ {
		client.LogEvent(7, "TEST", int64(i), nil)
	}
	require.Len(t, client.queue.pending, eventCapacity)
	before := dropCount("queue_full")
	started := time.Now()
	client.LogEvent(7, "OVERFLOW", 0, nil)
	require.Less(t, time.Since(started), time.Second, "full queues must not wait for the RPC")
	require.Equal(t, before+1, dropCount("queue_full"))
	require.Equal(t, int32(eventWorkers), api.peak.Load())
	shutdownBefore := dropCount("shutdown")
	cancel()
	closeClient(t, client)
	require.Zero(t, api.active.Load())
	require.Empty(t, client.queue.pending)
	require.Equal(t, shutdownBefore+eventCapacity+eventWorkers, dropCount("shutdown"))
	require.Equal(t, int32(1), closer.calls.Load())
	stoppedBefore := dropCount("stopped")
	client.LogEvent(7, "AFTER_STOP", 0, nil)
	require.Equal(t, stoppedBefore+1, dropCount("stopped"))
	closeClient(t, client)
	require.Equal(t, int32(1), closer.calls.Load(), "close must be idempotent")
}

func TestEventsSnapshotDetailsAndRejectOversizedPayload(t *testing.T) {
	received := make(chan *pb.LogRequest, 1)
	client := newClient(context.Background(), slog.New(slog.DiscardHandler), eventAPI{received: received, success: true}, time.Second, nil)
	t.Cleanup(func() { closeClient(t, client) })
	before := dropCount("oversized")
	client.LogEvent(7, "TOO_LARGE", 1, map[string]any{"value": strings.Repeat("x", maxEventBytes)})
	client.LogEvent(7, strings.Repeat("x", maxEventBytes+1), 1, nil)
	require.Equal(t, before+2, dropCount("oversized"))
	details := map[string]any{"value": "original"}
	client.LogEvent(7, "SNAPSHOT", 1, details)
	details["value"] = "changed"
	select {
	case event := <-received:
		require.JSONEq(t, `{"value":"original"}`, event.DetailsJson)
		require.Equal(t, "SNAPSHOT", event.Action)
	case <-time.After(time.Second):
		t.Fatal("event not sent")
	}
}

func TestConcurrentProducersAndCancellation(t *testing.T) {
	api := &blockedEvents{started: make(chan struct{}, eventWorkers)}
	client := newClient(context.Background(), slog.New(slog.DiscardHandler), api, time.Minute, nil)
	t.Cleanup(func() { closeClient(t, client) })
	var producers sync.WaitGroup
	for i := 0; i < 32; i++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for i := 0; i < 100; i++ {
				client.LogEvent(7, "TEST", 0, nil)
			}
		}()
	}
	closeClient(t, client)
	producers.Wait()
	require.LessOrEqual(t, api.peak.Load(), int32(eventWorkers))
	require.Empty(t, client.queue.pending)
}

type eventFunc struct {
	pb.ActivityLoggerClient
	call func(context.Context) (*pb.LogResponse, error)
}

func (e eventFunc) LogEvent(ctx context.Context, _ *pb.LogRequest, _ ...grpc.CallOption) (*pb.LogResponse, error) {
	return e.call(ctx)
}

func TestEventDeliveryFailuresAndTimeoutReleaseWorkers(t *testing.T) {
	for _, mode := range []string{"error", "rejected", "nil response", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			api := eventFunc{call: func(ctx context.Context) (*pb.LogResponse, error) {
				switch mode {
				case "error":
					return nil, errors.New("unavailable")
				case "rejected":
					return &pb.LogResponse{Success: false}, nil
				case "timeout":
					<-ctx.Done()
					return nil, ctx.Err()
				default:
					return nil, nil
				}
			}}
			client := newClient(context.Background(), slog.New(slog.DiscardHandler), api, 20*time.Millisecond, nil)
			t.Cleanup(func() { closeClient(t, client) })
			before := dropCount("delivery_failed")
			client.LogEvent(7, "TEST", 1, nil)
			require.Eventually(t, func() bool {
				return dropCount("delivery_failed") == before+1
			}, time.Second, time.Millisecond)
			closeClient(t, client)
		})
	}
}

func TestNewRejectsUnboundedTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		client, err := New(context.Background(), slog.New(slog.DiscardHandler), "unused", timeout)
		require.Error(t, err)
		require.Nil(t, client)
	}
}

func TestCloseWaitRespectsCallerDeadline(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	api := eventFunc{call: func(context.Context) (*pb.LogResponse, error) {
		close(entered)
		<-release // Simulate a faulty dependency that ignores cancellation.
		return nil, context.Canceled
	}}
	client := newClient(context.Background(), slog.New(slog.DiscardHandler), api, time.Minute, nil)
	t.Cleanup(func() { close(release); closeClient(t, client) })
	client.LogEvent(7, "TEST", 1, nil)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("RPC not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, client.Close(ctx), context.DeadlineExceeded)
}
