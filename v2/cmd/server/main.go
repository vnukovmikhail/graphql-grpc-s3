package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vnukovmikhail/graphql-grpc-s3/v2/internal/transport/graphql/generated"
	"github.com/vnukovmikhail/graphql-grpc-s3/v2/internal/transport/graphql/resolver"
	authv1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/auth/v1"
	filev1 "github.com/vnukovmikhail/graphql-grpc-s3/v2/pkg/pb/file/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultPort = "8080"

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	authConn, err := grpc.NewClient("localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer authConn.Close()

	fileConn, err := grpc.NewClient("localhost:50052",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer fileConn.Close()

	srv := handler.New(
		generated.NewExecutableSchema(generated.Config{
			Resolvers: &resolver.Resolver{
				Auth: authv1.NewAuthServiceClient(authConn),
				File: filev1.NewFileServiceClient(fileConn),
			},
		}),
	)

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{
		MaxUploadSize: 100 << 20,
		MaxMemory:     32 << 20,
	})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
