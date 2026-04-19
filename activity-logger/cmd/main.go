package main

import (
	"context"
	"log"
	"net"
    "os"
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

	if req.GetLimit() > 0 {
		findOptions.SetLimit(int64(req.GetLimit()))
	} else {
		findOptions.SetLimit(50)
	}

	if req.GetOffset() > 0 {
		findOptions.SetSkip(int64(req.GetOffset()))
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := s.collection.Find(ctxTimeout, filter, findOptions)
	if err != nil {
		log.Printf("failed to find logs in MongoDB: %v\n", err)
		return nil, err
	}
	defer cursor.Close(ctxTimeout)

	var results []*pb.ActivityLog
	for cursor.Next(ctxTimeout) {
		var doc activityDoc
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("failed to decode mongodb doc: %v\n", err)
			continue
		}

		results = append(results, &pb.ActivityLog{
			Id:          doc.ID.Hex(),
			UserId:      doc.UserID,
			Action:      doc.Action,
			EntityId:    doc.EntityID,
			DetailsJson: doc.DetailsJSON,
			Timestamp:   doc.CreatedAt.UnixMilli(),
		})
	}

	return &pb.GetLogsResponse{
		Logs: results,
	}, nil
}

func main() {
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

