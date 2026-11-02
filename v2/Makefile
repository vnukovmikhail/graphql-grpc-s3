run:
	@trap 'kill 0' INT TERM; go run ./cmd/auth & go run ./cmd/file & go run ./cmd/server & wait

gen:
	@echo "Generating GraphQL code with gqlgen..."
	go run github.com/99designs/gqlgen generate
	@echo "Generating gRPC code with buf..."
	buf generate
	@echo "Generation completed successfully!"


