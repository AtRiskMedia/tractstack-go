BINARY_NAME=tractstack-go
GO_FLAGS=-tags fts5

build:
	go build $(GO_FLAGS) -o $(BINARY_NAME) ./cmd/tractstack-go

run: build
	./$(BINARY_NAME)

clean:
	go clean
	rm -f $(BINARY_NAME)

lint:
	golangci-lint run --build-tags fts5 --enable revive,gocritic ./...

fmt:
	go fmt ./...

check:
	go vet $(GO_FLAGS) ./...
