package main

import (
	"context"
	"log"
	"net"
	"time"

	pb "github.com/kirill010106/todo-notificator/root/pkg/activity_logger/v1"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc"
)

const (
	port     = ":50051"
	mongoURI = "mongodb://localhost:27017"
)

type server struct {
	pb.UnimplementedActivityLoggerServer

	collection *mongo.Collection
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

func main() {

	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
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
