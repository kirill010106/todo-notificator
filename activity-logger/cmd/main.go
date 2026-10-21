package main

import (
	"context"
	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"github.com/kirill010106/todo-notificator/root/pkg/activitylog"
	"github.com/kirill010106/todo-notificator/root/pkg/envconfig"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"os"
	"time"
)

const (
	port     = ":50051"
	mongoURI = "mongodb://localhost:27017"
)

type server struct {
	pb.UnimplementedActivityLoggerServer

	collection *mongo.Collection
}

type activityDoc struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	UserID      int64         `bson:"user_id"`
	Action      string        `bson:"action"`
	EntityID    int64         `bson:"entity_id"`
	DetailsJSON string        `bson:"details_json"`
	CreatedAt   time.Time     `bson:"created_at"`
}

func (s *server) LogEvent(ctx context.Context, req *pb.LogRequest) (*pb.LogResponse, error) {
	log.Printf("Received event: UserID=%d Action=%s EntityID=%d JSON=%s\n",
		req.GetUserId(), req.GetAction(), req.GetEntityId(), req.GetDetailsJson())

	doc := bson.M{
		"user_id":      req.GetUserId(),
		"action":       req.GetAction(),
		"entity_id":    req.GetEntityId(),
		"details_json": req.GetDetailsJson(),
		"created_at":   time.Now(),
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.collection.InsertOne(ctxTimeout, doc)
	if err != nil {
		log.Printf("failed to insert event into MongoDB: %v\n", err)
		return &pb.LogResponse{
			Success:      false,
			ErrorMessage: "Internal MondoDB error",
		}, nil
	}

	return &pb.LogResponse{
		Success:      true,
		ErrorMessage: "",
	}, nil
}

func (s *server) GetLogs(ctx context.Context, req *pb.GetLogsRequest) (*pb.GetLogsResponse, error) {
	log.Printf("Received GetLogs request: UserID=%d Limit=%d Offset=%d\n", req.GetUserId(), req.GetLimit(), req.GetOffset())

	filter := bson.M{"user_id": req.GetUserId()}

	findOptions := options.Find().SetSort(bson.D{{"created_at", -1}})

	findOptions.SetLimit(int64(activitylog.PageSize(req.GetLimit())))

	if req.GetOffset() > 0 {
		findOptions.SetSkip(int64(req.GetOffset()))
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := s.collection.Find(ctxTimeout, filter, findOptions)
	if err != nil {
		log.Printf("failed to find logs in MongoDB: %v\n", err)
		return nil, status.Error(codes.Internal, "failed to read activity logs")
	}
	defer cursor.Close(ctxTimeout)

	var results []*pb.ActivityLog
	responseBytes := 0
	for cursor.Next(ctxTimeout) {
		var doc activityDoc
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("failed to decode mongodb doc: %v\n", err)
			return nil, status.Error(codes.Internal, "failed to decode activity log")
		}

		entry := &pb.ActivityLog{
			Id:          doc.ID.Hex(),
			UserId:      doc.UserID,
			Action:      doc.Action,
			EntityId:    doc.EntityID,
			DetailsJson: doc.DetailsJSON,
			Timestamp:   doc.CreatedAt.UnixMilli(),
		}
		responseBytes += protowire.SizeTag(1) + protowire.SizeBytes(proto.Size(entry))
		if responseBytes > activitylog.MaxResponseBytes {
			return nil, status.Error(codes.ResourceExhausted, "activity log page exceeds response limit; request a smaller page")
		}
		results = append(results, entry)
	}
	if err := cursor.Err(); err != nil {
		log.Printf("failed to iterate logs in MongoDB: %v\n", err)
		return nil, status.Error(codes.Internal, "failed to read activity logs")
	}

	return &pb.GetLogsResponse{
		Logs: results,
	}, nil
}

func main() {
	if err := envconfig.Load(); err != nil {
		log.Fatal(err)
	}
	uri := mongoURI
	if v := os.Getenv("MONGO_URL"); v != "" {
		uri = v
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v\n", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		log.Fatalf("failed to ping MongoDB: %v\n", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Printf("failed to disconnect from MongoDB: %v", err)
		}
	}()

	collection := client.Database("todo_logs").Collection("activities")

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	pb.RegisterActivityLoggerServer(grpcServer, &server{collection: collection})

	log.Printf("Activity logger gRPC server is listening at %v", lis.Addr())

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
