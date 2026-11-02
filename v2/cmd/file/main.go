package main

import (
	"log"
	"net"

	file_grpc "github.com/vnukovmikhail/graphql-grpc-s3/v2/internal/file/transport/grpc"
	filev1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/file/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	srv, err := file_grpc.New("./storage")
	if err != nil {
		log.Fatal(err)
	}

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatal(err)
	}

	g := grpc.NewServer()
	filev1.RegisterFileServiceServer(g, srv)
	reflection.Register(g)

	log.Println("file gRPC on :50052")
	log.Fatal(g.Serve(lis))
}
