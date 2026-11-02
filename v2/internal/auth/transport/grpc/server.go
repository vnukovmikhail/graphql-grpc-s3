package grpc

import (
	"context"

	authv1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/auth/v1"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
}

func New() *Server { return &Server{} }

func (s *Server) CheckList(ctx context.Context, _ *authv1.CheckListRequest) (*authv1.CheckListResponse, error) {
	return &authv1.CheckListResponse{Message: "We Are One!"}, nil
}
