.PHONY: all build test clean lint fmt help

# Default target
all: fmt lint test build

# Build the project
build:
	@echo "Building..."
	@go build ./...

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./... -cover

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	@go test -v ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f coverage.out coverage.html
	@go clean

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run examples
example:
	@echo "Running example..."
	@go run cmd/example/main.go

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Update dependencies
update-deps:
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy

# Help
help:
	@echo "Available targets:"
	@echo "  make all          - Format, lint, test, and build"
	@echo "  make build        - Build the project"
	@echo "  make test         - Run tests"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make lint         - Run linter"
	@echo "  make fmt          - Format code"
	@echo "  make example      - Run example"
	@echo "  make deps         - Install dependencies"
	@echo "  make update-deps  - Update dependencies"
	@echo "  make help         - Show this help message"