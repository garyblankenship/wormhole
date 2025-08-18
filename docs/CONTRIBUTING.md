# Contributing to Wormhole Go

Thank you for your interest in contributing to Wormhole Go! This document provides comprehensive guidelines for contributing to the ultra-fast LLM SDK that bends spacetime for AI integration.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct: be respectful, inclusive, and professional. We're building the future of AI integration together.

## Quick Start for Contributors

### Prerequisites
- **Go 1.22+** (required for modern features and performance optimizations)
- **Git** for version control
- **Make** for build automation

### One-Command Setup
```bash
# Setup complete development environment
make setup
```

This installs all required tools: `golangci-lint`, `goreleaser`, dependencies, and validates your environment.

## How to Contribute

### Reporting Issues

Before creating an issue, please:

1. **Search existing issues** to avoid duplicates
2. **Use the issue template** with the following information:
   - Go version (`go version`)
   - Operating system and architecture
   - Wormhole version
   - **Minimal reproduction code** (essential)
   - Expected vs actual behavior
   - Complete error messages and stack traces
   - Provider and model being used

### Submitting Pull Requests

**Complete Development Workflow:**

1. **Fork and Clone**
   ```bash
   git clone https://github.com/your-username/wormhole.git
   cd wormhole
   make setup  # Complete environment setup
   ```

2. **Create Feature Branch**
   ```bash
   git checkout -b feature/your-descriptive-feature-name
   ```

3. **Development Cycle**
   ```bash
   # Format, lint, test, and build in one command
   make all
   
   # Or run individual steps:
   make fmt           # Format code
   make lint          # Run linter
   make test          # Run tests with coverage
   make build         # Build project
   ```

4. **Quality Assurance**
   ```bash
   make test-coverage    # Generate coverage report
   make security        # Security vulnerability scan
   make bench          # Performance benchmarks
   ```

5. **Commit and Push**
   ```bash
   git add .
   git commit -m "feat: descriptive commit message"
   git push origin feature/your-feature-name
   ```

6. **Create Pull Request**
   - Use descriptive title and detailed description
   - Reference any related issues
   - Include testing performed
   - Update documentation if needed

### Development Setup Details

**Project Structure:**
```
wormhole/
├── pkg/
│   ├── wormhole/          # Core SDK implementation
│   ├── providers/         # LLM provider integrations
│   ├── adapters/         # Provider adapters
│   ├── middleware/       # Middleware system
│   └── types/            # Type definitions
├── internal/
│   └── utils/            # Internal utilities
├── examples/             # Usage examples and demos
├── docs/                # Documentation
└── scripts/             # Build and release scripts
```

**Essential Commands:**
```bash
# Development workflow
make all              # Complete development cycle
make test-coverage    # Run tests with HTML coverage report
make bench           # Performance benchmarks
make example         # Run example application

# Quality assurance
make security        # Vulnerability scanning
make perf-test       # Performance regression testing

# Release preparation
make release-check   # Validate release configuration
```

## Development Standards

### Code Style and Quality

**Core Principles:**
- **Self-documenting code** over extensive comments
- **Performance-first** design with sub-microsecond latency goals
- **Type safety** with comprehensive Go type system usage
- **Error handling** using structured error types, never `fmt.Errorf`
- **Concurrent-safe** operations by default

**Formatting and Style:**
```bash
make fmt              # Automatic formatting (gofmt + goimports)
make lint             # Comprehensive linting with golangci-lint
```

**Documentation Requirements:**
- All exported functions and types must have godoc comments
- Include usage examples in godoc
- Update README.md for user-facing changes
- Add entries to CHANGELOG.md for all changes

### Testing Requirements

**Comprehensive Testing Strategy:**
```bash
make test             # Run all tests with coverage
make test-coverage    # Generate HTML coverage report
make bench           # Performance benchmarks
```

**Testing Standards:**
- **100% coverage** for core functionality
- **Table-driven tests** for multiple scenarios
- **Benchmark tests** for performance-critical code
- **Mock external dependencies** using interfaces
- **Test all error cases** and edge conditions
- **Integration tests** for provider functionality

**Performance Requirements:**
- Maintain **sub-microsecond latency** for core operations
- Benchmark all new features: `make bench`
- No performance regressions: `make perf-test`

### Adding New Providers

**Provider Implementation Guide:**

1. **Create Provider Package Structure:**
   ```
   pkg/providers/yourprovider/
   ├── yourprovider.go      # Main provider implementation
   ├── types.go             # Provider-specific request/response types
   ├── transform.go         # Request/response transformations
   ├── client.go           # HTTP client configuration
   └── yourprovider_test.go # Comprehensive test suite
   ```

2. **Implement Core Interfaces:**
   ```go
   // Must implement types.Provider interface
   type YourProvider struct {
       config *Config
       client *http.Client
   }
   
   // Required methods:
   func (p *YourProvider) TextGeneration(ctx context.Context, req *types.TextRequest) (*types.TextResponse, error)
   func (p *YourProvider) StreamingGeneration(ctx context.Context, req *types.TextRequest) (<-chan types.StreamResponse, error)
   // ... other interface methods
   ```

3. **Provider Requirements:**
   - **Error handling** with structured error types
   - **Context support** for cancellation and timeouts
   - **Streaming support** for real-time responses
   - **Rate limiting** compliance
   - **Comprehensive testing** including integration tests
   - **Documentation** with usage examples

4. **Integration Steps:**
   - Add provider to `pkg/wormhole/provider_registry.go`
   - Create example in `examples/yourprovider_example/`
   - Update documentation in `docs/PROVIDERS.md`
   - Add to README.md provider list
   - Include in benchmark suite

### Architecture Guidelines

**Performance-First Design:**
- **Zero-allocation** paths for hot code
- **Concurrent-safe** operations with linear scaling
- **Memory efficient** with predictable allocation patterns
- **Context-aware** timeout and cancellation support

**Error Handling:**
```go
// ✅ Correct: Use structured errors
return nil, errors.Wrap(err, "failed to generate text")

// ❌ Incorrect: Never use fmt.Errorf
return nil, fmt.Errorf("failed to generate text: %v", err)
```

**Middleware Integration:**
- Support for rate limiting, circuit breakers, caching
- Chainable middleware with clean interfaces
- Observable operations with metrics and logging

## Pull Request Guidelines

### Pre-Submission Checklist

**Quality Assurance:**
- [ ] `make all` passes (format, lint, test, build)
- [ ] `make test-coverage` shows maintained/improved coverage
- [ ] `make bench` shows no performance regressions
- [ ] `make security` passes vulnerability scan
- [ ] All examples in `examples/` directory work correctly

**Documentation Updates:**
- [ ] README.md updated for user-facing changes
- [ ] CHANGELOG.md entry added with proper formatting
- [ ] Godoc comments for all exported functions
- [ ] Provider documentation updated (if applicable)
- [ ] Breaking changes clearly documented

**Code Quality:**
- [ ] Self-documenting code with minimal comments
- [ ] Proper error handling with structured errors
- [ ] Thread-safe implementation verified
- [ ] Integration tests for new providers
- [ ] Performance benchmarks included

### Review Process

1. **Automated Checks**
   - CI pipeline validates all quality gates
   - Performance regression testing
   - Security vulnerability scanning
   - Documentation generation

2. **Maintainer Review**
   - Code architecture and design patterns
   - Performance impact assessment
   - API design consistency
   - Documentation completeness

3. **Merge Requirements**
   - All CI checks passing
   - Maintainer approval
   - No merge conflicts
   - Commit history cleaned (squash if needed)

## Release Process

### Version Management
- Follow [Semantic Versioning](https://semver.org/)
- Use `make prepare-release VERSION=v1.x.y` for release preparation
- Automated release with `make release` using goreleaser

### Release Checklist
```bash
# Validate release configuration
make release-check

# Create test release
make release-snapshot

# Prepare official release
make prepare-release VERSION=v1.4.0

# Create GitHub release
make release
```

## Getting Help

### Documentation Resources
- **[README.md](../README.md)** - Getting started and basic usage
- **[PROVIDERS.md](PROVIDERS.md)** - Provider-specific documentation
- **[examples/](../examples/)** - Complete usage examples
- **[API Reference](https://pkg.go.dev/github.com/garyblankenship/wormhole)** - Complete API documentation

### Support Channels
- **GitHub Issues** - Bug reports and feature requests
- **GitHub Discussions** - General questions and community support
- **Code Review** - Detailed feedback through pull request reviews

### Development Environment Issues
If you encounter setup issues:
1. Run `make setup` for automated environment setup
2. Verify Go version: `go version` (requires 1.22+)
3. Check tool installation: `make help` for available commands
4. Open an issue with your environment details

---

**Ready to contribute?** Start with `make setup` and explore the `examples/` directory to understand the codebase. Every contribution helps make Wormhole the fastest LLM SDK in the universe! 🚀