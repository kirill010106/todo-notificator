package activitylogger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	api     pb.ActivityLoggerClient
	log     *slog.Logger
	timeout time.Duration
	queue   *eventQueue
}

func New(ctx context.Context, log *slog.Logger, addr string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("activity logger timeout must be positive")
	}
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc client: %w", err)
	}

	return newClient(ctx, log, pb.NewActivityLoggerClient(cc), timeout, cc), nil
}

// GetLogs fetches activity logs from the microservice
func (c *Client) GetLogs(ctx context.Context, userID int64, limit, offset int32) ([]domain.ActivityLog, error) {
	if c == nil || c.api == nil {
		return nil, fmt.Errorf("activity logger client is unavailable")
	}

	req := &pb.GetLogsRequest{
		UserId: userID,
		Limit:  limit,
		Offset: offset,
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.GetLogs(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	var logs []domain.ActivityLog
	for _, l := range resp.GetLogs() {
		var details map[string]any
		if l.GetDetailsJson() != "" {
			_ = json.Unmarshal([]byte(l.GetDetailsJson()), &details)
		}

		logs = append(logs, domain.ActivityLog{
			ID:          l.GetId(),
			UserID:      l.GetUserId(),
			Action:      l.GetAction(),
			EntityID:    l.GetEntityId(),
			DetailsJSON: details,
			Timestamp:   time.UnixMilli(l.GetTimestamp()),
		})
	}

	return logs, nil
}
