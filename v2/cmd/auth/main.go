package main

import (
	"log"
	"net"

	auth_grpc "github.com/vnukovmikhail/graphql-grpc-s3/v2/internal/auth/transport/grpc"
	authv1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	g := grpc.NewServer()
	authv1.RegisterAuthServiceServer(g, auth_grpc.New())
	reflection.Register(g)

	log.Println("auth gRPC on :50051")
	log.Fatal(g.Serve(lis))
}
