package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/description"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/mnet"
)

// Keep the actual MongoDB driver, but hold one wire response until its operation
// context is canceled. This tests both initial commands and a cursor's getMore.
type delayedDeployment struct {
	*drivertest.MockDeployment
	reads   atomic.Int32
	blockOn int32
	started chan context.Context
	release chan struct{}
}

func (d *delayedDeployment) SelectServer(ctx context.Context, _ description.ServerSelector) (driver.Server, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *delayedDeployment) Connection(ctx context.Context) (*mnet.Connection, error) {
	c, err := d.MockDeployment.Connection(ctx)
	if err != nil {
		return nil, err
	}
	return mnet.NewConnection(&delayedConnection{ReadWriteCloser: c.ReadWriteCloser, Describer: c.Describer, deployment: d}), nil
}

type delayedConnection struct {
	mnet.ReadWriteCloser
	mnet.Describer
	deployment *delayedDeployment
}

func (c *delayedConnection) Read(ctx context.Context) ([]byte, error) {
	d := c.deployment
	if d.reads.Add(1) == d.blockOn {
		d.started <- ctx
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-d.release:
			return nil, context.Canceled
		}
	}
	return c.ReadWriteCloser.Read(ctx)
}

func TestMongoOperationsInheritRPCCancellation(t *testing.T) {
	for _, operation := range []string{"insert", "find", "getMore"} {
		for _, mode := range []string{"already canceled", "in flight", "deadline"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				d := &delayedDeployment{
					MockDeployment: drivertest.NewMockDeployment(logsBatch(123,
						bson.D{{"_id", bson.NewObjectID()}, {"user_id", int64(7)}, {"created_at", time.Now()}})),
					blockOn: 1, started: make(chan context.Context, 1), release: make(chan struct{}),
				}
				if operation == "getMore" {
					d.blockOn = 2
				}
				t.Cleanup(func() { close(d.release) })
				opts := options.Client().SetRetryReads(false).SetRetryWrites(false)
				opts.Deployment = d
				client, err := mongo.Connect(opts)
				require.NoError(t, err)
				defer client.Disconnect(context.Background())
				s := &server{collection: client.Database("security").Collection("activities")}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if mode == "deadline" {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, 200*time.Millisecond)
					defer deadlineCancel()
				}
				if mode == "already canceled" {
					cancel()
				}
				type result struct {
					logs  *pb.GetLogsResponse
					event *pb.LogResponse
					err   error
				}
				done := make(chan result, 1)
				go func() {
					var r result
					if operation == "insert" {
						r.event, r.err = s.LogEvent(ctx, &pb.LogRequest{UserId: 7, Action: "TEST"})
					} else {
						r.logs, r.err = s.GetLogs(ctx, &pb.GetLogsRequest{UserId: 7})
					}
					done <- r
				}()
				if mode != "already canceled" {
					select {
					case child := <-d.started:
						deadline, ok := child.Deadline()
						require.True(t, ok, "the service timeout must remain bounded")
						if mode == "deadline" {
							parentDeadline, _ := ctx.Deadline()
							require.Equal(t, parentDeadline, deadline)
						} else {
							cancel()
						}
					case <-time.After(time.Second):
						t.Fatal("driver did not reach the delayed response")
					}
				}
				select {
				case r := <-done:
					if operation == "insert" {
						require.NoError(t, r.err) // Preserve the existing LogResponse failure contract.
						require.False(t, r.event.GetSuccess())
					} else {
						require.Error(t, r.err)
						require.Nil(t, r.logs, "cancellation must not return a successful partial page")
					}
					if mode == "already canceled" {
						require.Zero(t, d.reads.Load())
					}
				case <-time.After(time.Second):
					t.Fatal("operation outlived the canceled RPC")
				}
			})
		}
	}
}
