package activitylogger

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"

	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
)

const (
	eventWorkers  = 4
	eventCapacity = 256
	maxEventBytes = 64 << 10
)

var droppedEvents = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "activity_logger_events_dropped_total",
	Help: "Activity events dropped locally or whose delivery was not confirmed, partitioned by a fixed reason.",
}, []string{"reason"})

type eventQueue struct {
	mu       sync.Mutex
	pending  chan *pb.LogRequest
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	workers  sync.WaitGroup
	closer   io.Closer
	closeErr error
}

func newClient(ctx context.Context, log *slog.Logger, api pb.ActivityLoggerClient, timeout time.Duration, closer io.Closer) *Client {
	runCtx, cancel := context.WithCancel(ctx)
	q := &eventQueue{pending: make(chan *pb.LogRequest, eventCapacity), ctx: runCtx, cancel: cancel, done: make(chan struct{}), closer: closer}
	c := &Client{api: api, log: log, timeout: timeout, queue: q}
	q.workers.Add(eventWorkers)
	for i := 0; i < eventWorkers; i++ {
		go c.runEvents()
	}
	go func() {
		<-runCtx.Done()
		q.workers.Wait()
		// Admission uses the same lock, so no event can enter after this drain.
		q.mu.Lock()
		for len(q.pending) > 0 {
			<-q.pending
			droppedEvents.WithLabelValues("shutdown").Inc()
		}
		q.mu.Unlock()
		if q.closer != nil {
			q.closeErr = q.closer.Close()
		}
		close(q.done)
	}()
	return c
}

// LogEvent snapshots details before returning, then admits the event without
// waiting for a network operation. Full queues drop the newest event.
func (c *Client) LogEvent(userID int64, action string, entityID int64, details map[string]any) {
	if c == nil || c.api == nil || c.queue == nil {
		return
	}
	q := c.queue
	if q.ctx.Err() != nil {
		droppedEvents.WithLabelValues("stopped").Inc()
		return
	}
	var detailsJSON string
	if details != nil {
		b, err := json.Marshal(details)
		if err == nil {
			detailsJSON = string(b)
		} else {
			c.log.Warn("activitylogger: failed to marshal details", slog.String("error", err.Error()))
		}
	}
	req := &pb.LogRequest{UserId: userID, Action: action, EntityId: entityID, DetailsJson: detailsJSON}
	if proto.Size(req) > maxEventBytes {
		droppedEvents.WithLabelValues("oversized").Inc()
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ctx.Err() != nil {
		droppedEvents.WithLabelValues("stopped").Inc()
		return
	}
	select {
	case q.pending <- req:
	default:
		droppedEvents.WithLabelValues("queue_full").Inc()
	}
}

func (c *Client) runEvents() {
	q := c.queue
	defer q.workers.Done()
	for {
		if q.ctx.Err() != nil {
			return
		}
		select {
		case <-q.ctx.Done():
			return
		case req := <-q.pending:
			if q.ctx.Err() != nil {
				droppedEvents.WithLabelValues("shutdown").Inc()
				return
			}
			c.sendEvent(q.ctx, req)
		}
	}
}

func (c *Client) sendEvent(ctx context.Context, req *pb.LogRequest) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	response, err := c.api.LogEvent(callCtx, req)
	if err == nil && response != nil && response.Success {
		return
	}
	if ctx.Err() != nil {
		droppedEvents.WithLabelValues("shutdown").Inc()
		return
	}
	droppedEvents.WithLabelValues("delivery_failed").Inc()
	// Avoid logging event contents or an arbitrary server response.
	c.log.Warn("activitylogger: event delivery failed")
}

// Close cancels pending and active best-effort sends, waits for workers, then
// closes the gRPC connection. A caller deadline bounds the wait.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.queue == nil {
		return nil
	}
	c.queue.cancel()
	select {
	case <-c.queue.done:
		return c.queue.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
