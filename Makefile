.PHONY: build install test clean swagger

# Build CLI tool
build:
	go build -o bin/slotmodule ./cmd/slotmodule

# Install CLI tool globally
install:
	go install ./cmd/slotmodule

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Download dependencies
deps:
	go mod tidy
	go mod download

# Example: Create a new game
example-create:
	go run ./cmd/slotmodule create --name new-game --port 8081 --output ./examples

# Lint
lint:
	golangci-lint run ./...

# Install swag CLI (one-time setup)
install-swag:
	go install github.com/swaggo/swag/cmd/swag@latest

