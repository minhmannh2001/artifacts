// proto/service.proto
syntax = "proto3";
package example;
option go_package = "./proto";

service ExampleService {
  rpc SayHello (HelloRequest) returns (HelloReply) {}
}

message HelloRequest {
  string name = 1;
}

message HelloReply {
  string message = 1;
}

// server/main.go
package main

import (
	"context"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "path/to/your/proto"
)

type server struct {
	pb.UnimplementedExampleServiceServer
}

func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", in.GetName())
	
	// Simulate some work
	time.Sleep(100 * time.Millisecond)
	
	// Deliberately close the connection
	return nil, status.Error(codes.Unavailable, "connection closed")
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterExampleServiceServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// client/main.go
package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "path/to/your/proto"
)

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewExampleServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	r, err := c.SayHello(ctx, &pb.HelloRequest{Name: "world"})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unavailable {
			log.Printf("Connection reset by peer: %v", err)
		} else {
			log.Fatalf("could not greet: %v", err)
		}
		return
	}
	log.Printf("Greeting: %s", r.GetMessage())
}
