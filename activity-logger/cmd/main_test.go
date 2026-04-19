package main

import (
	"context"
	"testing"
	"time"

	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestServerIntegration(t *testing.T) {
	ctx := context.Background()

	// Spin up MongoDB container
	mongodbContainer, err := mongodb.RunContainer(ctx)
	require.NoError(t, err)

	// Clean up the container
	defer func() {
		if err := mongodbContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}()

	uri, err := mongodbContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Connect to the DB
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	defer func() {
		_ = client.Disconnect(ctx)
	}()

	collection := client.Database("test_db").Collection("activities")

	srv := &server{collection: collection}

	now := time.Now().UnixMilli()

	// 1. Test LogEvent
	logReq := &pb.LogRequest{
		UserId:      1,
		Action:      "TEST_ACTION",
		EntityId:    42,
		DetailsJson: `{"key":"value"}`,
	}

	logResp, err := srv.LogEvent(ctx, logReq)
	require.NoError(t, err)
	require.True(t, logResp.GetSuccess())
	require.Empty(t, logResp.GetErrorMessage())

	// 2. Validate using direct Mongo query
	count, err := collection.CountDocuments(ctx, bson.M{"user_id": 1})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	// 3. Test GetLogs limit & offset
	// Insert one more event for sorting and pagination
	time.Sleep(50 * time.Millisecond) // ensure timestamp diff
	logReq2 := &pb.LogRequest{
		UserId:      1,
		Action:      "TEST_ACTION_2",
		EntityId:    43,
		DetailsJson: `{"key":"value2"}`,
	}
	_, err = srv.LogEvent(ctx, logReq2)
	require.NoError(t, err)

	// User 1 should have 2 logs. GetLogs should sort by created_at DESC (newest first).
	getReq := &pb.GetLogsRequest{
		UserId: 1,
		Limit:  10,
		Offset: 0,
	}

	getResp, err := srv.GetLogs(ctx, getReq)
	require.NoError(t, err)
	require.Len(t, getResp.GetLogs(), 2)

	// First should be logReq2 (newest)
	require.Equal(t, "TEST_ACTION_2", getResp.GetLogs()[0].GetAction())
	require.Equal(t, int64(43), getResp.GetLogs()[0].GetEntityId())
	require.True(t, getResp.GetLogs()[0].GetTimestamp() > now)

	// Second should be logReq1
	require.Equal(t, "TEST_ACTION", getResp.GetLogs()[1].GetAction())
	require.Equal(t, int64(42), getResp.GetLogs()[1].GetEntityId())

	// 4. Test Offset
	getReqOffset := &pb.GetLogsRequest{
		UserId: 1,
		Limit:  10,
		Offset: 1,
	}
	getRespOffset, err := srv.GetLogs(ctx, getReqOffset)
	require.NoError(t, err)
	require.Len(t, getRespOffset.GetLogs(), 1)
	require.Equal(t, "TEST_ACTION", getRespOffset.GetLogs()[0].GetAction())

	// 5. Test another user
	getReqUser2 := &pb.GetLogsRequest{
		UserId: 2,
		Limit:  10,
		Offset: 0,
	}
	getRespUser2, err := srv.GetLogs(ctx, getReqUser2)
	require.NoError(t, err)
	require.Len(t, getRespUser2.GetLogs(), 0)
}
