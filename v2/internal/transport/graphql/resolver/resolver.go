// Package resolver
package resolver

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

import (
	authv1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/auth/v1"
	filev1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/file/v1"
)

type Resolver struct {
	Auth authv1.AuthServiceClient
	File filev1.FileServiceClient
}
