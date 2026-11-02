package main

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/kirill010106/todo-notificator/root/pkg/activitylog"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func logsBatch(cursor int64, docs ...any) bson.D {
	if docs == nil {
		docs = []any{}
	}
	return bson.D{{"ok", 1}, {"cursor", bson.D{{"id", cursor}, {"ns", "security.activities"}, {"firstBatch", bson.A(docs)}}}}
}

func mockLogServer(t *testing.T, monitor *event.CommandMonitor, responses ...bson.D) *server {
	t.Helper()
	options := options.Client().SetRetryReads(false).SetMonitor(monitor)
	options.Deployment = drivertest.NewMockDeployment(responses...)
	client, err := mongo.Connect(options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })
	return &server{collection: client.Database("security").Collection("activities")}
}

func TestGetLogsCapsMongoQueryLimit(t *testing.T) {
	for _, test := range []struct {
		requested int32
		expected  int64
	}{
		{2147483647, 100}, {101, 100}, {100, 100}, {1, 1}, {0, 50}, {-1, 50},
	} {
		var limit int64
		s := mockLogServer(t, &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
			if e.CommandName == "find" {
				limit = e.Command.Lookup("limit").AsInt64()
			}
		}}, logsBatch(0))
		_, err := s.GetLogs(context.Background(), &pb.GetLogsRequest{UserId: 7, Limit: test.requested})
		require.NoError(t, err)
		require.Equal(t, test.expected, limit)
	}
}

func TestGetLogsBoundsAccumulatedResponseBytes(t *testing.T) {
	doc := bson.D{{"_id", bson.NewObjectID()}, {"user_id", int64(7)}, {"details_json", strings.Repeat("x", activitylog.MaxResponseBytes/2)}, {"created_at", time.Now()}}
	s := mockLogServer(t, nil, logsBatch(0, doc, doc))
	response, err := s.GetLogs(context.Background(), &pb.GetLogsRequest{UserId: 7, Limit: 100})
	require.Nil(t, response)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestGetLogsRejectsPartialHistoryOnCursorFailure(t *testing.T) {
	doc := bson.D{{"_id", bson.NewObjectID()}, {"user_id", int64(7)}, {"action", "first"}, {"created_at", time.Now()}}
	s := mockLogServer(t, nil, logsBatch(123, doc),
		bson.D{{"ok", 0}, {"code", int32(43)}, {"errmsg", "injected private MongoDB diagnostic"}}, bson.D{{"ok", 1}})
	response, err := s.GetLogs(context.Background(), &pb.GetLogsRequest{UserId: 7})
	require.Nil(t, response, "a successful first batch must not mask a failing getMore")
	require.Equal(t, codes.Internal, status.Code(err))
	require.NotContains(t, err.Error(), "private MongoDB")
}

func TestGetLogsRejectsPartialHistoryOnDecodeFailure(t *testing.T) {
	good := bson.D{{"_id", bson.NewObjectID()}, {"user_id", int64(7)}, {"created_at", time.Now()}}
	bad := bson.D{{"user_id", "incompatible value"}}
	s := mockLogServer(t, nil, logsBatch(0, good, bad))
	response, err := s.GetLogs(context.Background(), &pb.GetLogsRequest{UserId: 7})
	require.Nil(t, response)
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestGetLogsReturnsCompleteOrEmptyHistory(t *testing.T) {
	for _, docs := range [][]any{nil, {bson.D{{"_id", bson.NewObjectID()}, {"user_id", int64(7)}, {"created_at", time.Now()}}}} {
		s := mockLogServer(t, nil, logsBatch(0, docs...))
		response, err := s.GetLogs(context.Background(), &pb.GetLogsRequest{UserId: 7})
		require.NoError(t, err)
		require.Len(t, response.Logs, len(docs))
	}
}
