package activitylogger

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"log/slog"
	"testing"
	"time"
)

type eventAPI struct {
	pb.ActivityLoggerClient
	received chan *pb.LogRequest
	err      error
	success  bool
}

func (e eventAPI) LogEvent(ctx context.Context, request *pb.LogRequest, _ ...grpc.CallOption) (*pb.LogResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("logging request has no deadline")
	}
	e.received <- request
	return &pb.LogResponse{Success: e.success, ErrorMessage: "rejected"}, e.err
}
func TestLogEventSerializesContextAndToleratesFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details map[string]any
		err     error
		success bool
	}{
		{"success", map[string]any{"title": "Work"}, nil, true}, {"no details", nil, nil, true},
		{"bad details", map[string]any{"bad": make(chan int)}, nil, true},
		{"transport failed", nil, errors.New("unavailable"), false}, {"server rejected", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan *pb.LogRequest, 1)
			client := newClient(context.Background(), slog.New(slog.DiscardHandler), eventAPI{received: received, err: tc.err, success: tc.success}, time.Second, nil)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, client.Close(ctx))
			})
			client.LogEvent(42, "TASK_CREATED", 7, tc.details)
			select {
			case event := <-received:
				require.Equal(t, int64(42), event.UserId)
				require.Equal(t, int64(7), event.EntityId)
				require.Equal(t, "TASK_CREATED", event.Action)
				if tc.name == "success" {
					var details map[string]any
					require.NoError(t, json.Unmarshal([]byte(event.DetailsJson), &details))
					require.Equal(t, tc.details, details)
				} else {
					require.Empty(t, event.DetailsJson)
				}
			case <-time.After(time.Second):
				t.Fatal("event was not sent")
			}
		})
	}
}
func TestUnavailableLoggerIsSafe(t *testing.T) {
	var client *Client
	client.LogEvent(1, "event", 1, nil)
	_, err := client.GetLogs(context.Background(), 1, 1, 0)
	require.Error(t, err)
	(&Client{}).LogEvent(1, "event", 1, nil)
	_, err = (&Client{}).GetLogs(context.Background(), 1, 1, 0)
	require.Error(t, err)
}
