package activitylogger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockActivityLoggerClient struct {
	pb.ActivityLoggerClient
	getLogsFunc func(ctx context.Context, in *pb.GetLogsRequest, opts ...grpc.CallOption) (*pb.GetLogsResponse, error)
}

func (m *mockActivityLoggerClient) GetLogs(ctx context.Context, in *pb.GetLogsRequest, opts ...grpc.CallOption) (*pb.GetLogsResponse, error) {
	if m.getLogsFunc != nil {
		return m.getLogsFunc(ctx, in, opts...)
	}
	return &pb.GetLogsResponse{}, nil
}

func TestClient_GetLogs(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockAPI := &mockActivityLoggerClient{}

	client := &Client{
		api:     mockAPI,
		log:     log,
		timeout: 5 * time.Second,
	}

	ctx := context.Background()

	now := time.Now().UnixMilli()

	t.Run("Success", func(t *testing.T) {
		mockAPI.getLogsFunc = func(ctx context.Context, in *pb.GetLogsRequest, opts ...grpc.CallOption) (*pb.GetLogsResponse, error) {
			require.Equal(t, int64(1), in.GetUserId())
			require.Equal(t, int32(10), in.GetLimit())
			require.Equal(t, int32(0), in.GetOffset())

			return &pb.GetLogsResponse{
				Logs: []*pb.ActivityLog{
					{
						Id:          "test-id",
						UserId:      1,
						Action:      "TEST_ACTION",
						EntityId:    100,
						DetailsJson: `{"key": "value"}`,
						Timestamp:   now,
					},
				},
			}, nil
		}

		logs, err := client.GetLogs(ctx, 1, 10, 0)
		require.NoError(t, err)
		require.Len(t, logs, 1)

		expectedDetails := map[string]any{"key": "value"}

		require.Equal(t, domain.ActivityLog{
			ID:          "test-id",
			UserID:      1,
			Action:      "TEST_ACTION",
			EntityID:    100,
			DetailsJSON: expectedDetails,
			Timestamp:   time.UnixMilli(now),
		}, logs[0])
	})

	t.Run("Error from grpc", func(t *testing.T) {
		mockAPI.getLogsFunc = func(ctx context.Context, in *pb.GetLogsRequest, opts ...grpc.CallOption) (*pb.GetLogsResponse, error) {
			return nil, errors.New("grpc connection error")
		}

		logs, err := client.GetLogs(ctx, 1, 10, 0)
		require.Error(t, err)
		require.Nil(t, logs)
		require.Contains(t, err.Error(), "grpc connection error")
	})

	t.Run("Invalid details json", func(t *testing.T) {
		mockAPI.getLogsFunc = func(ctx context.Context, in *pb.GetLogsRequest, opts ...grpc.CallOption) (*pb.GetLogsResponse, error) {
			return &pb.GetLogsResponse{
				Logs: []*pb.ActivityLog{
					{
						Id:          "test-id-2",
						UserId:      1,
						Action:      "TEST_ACTION_2",
						EntityId:    101,
						DetailsJson: `{"invalid" json}`,
						Timestamp:   now,
					},
				},
			}, nil
		}

		logs, err := client.GetLogs(ctx, 1, 10, 0)
		require.NoError(t, err)
		require.Len(t, logs, 1)

		require.Equal(t, domain.ActivityLog{
			ID:          "test-id-2",
			UserID:      1,
			Action:      "TEST_ACTION_2",
			EntityID:    101,
			DetailsJSON: nil, // Should be nil on unmarshal error
			Timestamp:   time.UnixMilli(now),
		}, logs[0])
	})
}
