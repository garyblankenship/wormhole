.PHONY: all build test clean lint fmt help bench release prepare-release

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

# Run performance benchmarks
bench:
	@echo "Running performance benchmarks..."
	@go test -bench=. -benchmem ./pkg/prism/ -run="^$$"

# Run comprehensive benchmarks with profiling
bench-profile:
	@echo "Running benchmarks with CPU and memory profiling..."
	@go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof ./pkg/prism/ -run="^$$"
	@echo "Profiles generated: cpu.prof, mem.prof"
	@echo "Analyze with: go tool pprof cpu.prof"

# Prepare release (requires version argument)
prepare-release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make prepare-release VERSION=v1.0.0"; \
		exit 1; \
	fi
	@./scripts/prepare-release.sh $(VERSION)

# Create GitHub release using goreleaser
release:
	@echo "Creating release with goreleaser..."
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "Installing goreleaser..."; \
		go install github.com/goreleaser/goreleaser@latest; \
	fi
	@goreleaser release --clean

# Validate release configuration
release-check:
	@echo "Validating release configuration..."
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "Installing goreleaser..."; \
		go install github.com/goreleaser/goreleaser@latest; \
	fi
	@goreleaser check

# Create snapshot release (for testing)
release-snapshot:
	@echo "Creating snapshot release..."
	@if ! command -v goreleaser >/dev/null 2>&1; then \
		echo "Installing goreleaser..."; \
		go install github.com/goreleaser/goreleaser@latest; \
	fi
	@goreleaser release --snapshot --clean

# Setup development environment
setup:
	@echo "Setting up development environment..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/goreleaser/goreleaser@latest
	@go mod download
	@go mod tidy
	@echo "Development environment ready!"

# Security audit
security:
	@echo "Running security audit..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "Installing govulncheck..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
		govulncheck ./...; \
	fi

# Performance regression test
perf-test:
	@echo "Running performance regression tests..."
	@go test -bench=BenchmarkTextRequestBuilder -count=5 ./pkg/prism/ | tee bench.txt
	@echo "Performance results saved to bench.txt"

# Help
help:
	@echo "🚀 Prism Go - Ultra-Fast LLM SDK"
	@echo ""
	@echo "Available targets:"
	@echo "  📦 Build & Test"
	@echo "    make all           - Format, lint, test, and build"
	@echo "    make build         - Build the project"
	@echo "    make test          - Run tests"
	@echo "    make test-coverage - Run tests with coverage report"
	@echo "    make clean         - Clean build artifacts"
	@echo ""
	@echo "  🔍 Code Quality"  
	@echo "    make lint          - Run linter"
	@echo "    make fmt           - Format code"
	@echo "    make security      - Run security audit"
	@echo ""
	@echo "  ⚡ Performance"
	@echo "    make bench         - Run performance benchmarks"
	@echo "    make bench-profile - Run benchmarks with profiling"
	@echo "    make perf-test     - Performance regression test"
	@echo ""
	@echo "  🚀 Release"
	@echo "    make prepare-release VERSION=v1.0.0 - Prepare release"
	@echo "    make release       - Create GitHub release"
	@echo "    make release-check - Validate release config"
	@echo "    make release-snapshot - Create test release"
	@echo ""
	@echo "  🛠️  Development"
	@echo "    make setup         - Setup dev environment"
	@echo "    make example       - Run example"
	@echo "    make deps          - Install dependencies"
	@echo "    make update-deps   - Update dependencies"
	@echo ""
	@echo "  ℹ️  Help"
	@echo "    make help          - Show this help message"