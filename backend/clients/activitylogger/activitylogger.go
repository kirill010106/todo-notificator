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
}

func New(ctx context.Context, log *slog.Logger, addr string, timeout time.Duration) (*Client, error) {
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create grpc client: %w", err)
	}

	return &Client{
		api:     pb.NewActivityLoggerClient(cc),
		log:     log,
		timeout: timeout,
	}, nil
}

func (c *Client) LogEvent(userID int64, action string, entityID int64, details map[string]any) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		defer cancel()

		var detailsJSON string
		if details != nil {
			b, err := json.Marshal(details)
			if err == nil {
				detailsJSON = string(b)
			} else {
				c.log.Warn("activitylogger: failed to marshal details",
					slog.String("error", err.Error()),
					slog.String("action", action))
			}
		}
		req := &pb.LogRequest{
			UserId:      userID,
			Action:      action,
			EntityId:    entityID,
			DetailsJson: detailsJSON,
		}

		resp, err := c.api.LogEvent(ctx, req)
		if err != nil {
			c.log.Error("activitylogger: failed to send event", slog.String("error", err.Error()))
			return
		}

		if !resp.Success {
			c.log.Error("activitylogger: server returned error", slog.String("error", resp.ErrorMessage))
		}
	}()
}

// GetLogs fetches activity logs from the microservice
func (c *Client) GetLogs(ctx context.Context, userID int64, limit, offset int32) ([]domain.ActivityLog, error) {
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
