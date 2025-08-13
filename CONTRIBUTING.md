# Contributing to Wormhole Go

Thank you for your interest in contributing to Wormhole Go! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct: be respectful, inclusive, and professional.

## How to Contribute

### Reporting Issues

- Check if the issue already exists
- Include Go version, OS, and minimal reproduction code
- Describe expected vs actual behavior
- Include relevant error messages and stack traces

### Submitting Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/your-feature`
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass: `make test`
6. Format your code: `make fmt`
7. Commit with descriptive messages
8. Push to your fork
9. Create a Pull Request

### Development Setup

```bash
# Clone your fork
git clone https://github.com/your-username/wormhole.git
cd wormhole

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint

# Run all checks
make all
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` and `goimports`
- Write clear, self-documenting code
- Add comments for exported functions
- Keep functions small and focused

### Testing

- Write tests for all new features
- Maintain or improve code coverage
- Use table-driven tests where appropriate
- Mock external dependencies
- Test error cases

### Adding New Providers

To add a new LLM provider:

1. Create a new package in `pkg/providers/yourprovider/`
2. Implement the `types.Provider` interface
3. Add provider-specific types and transformations
4. Include comprehensive tests
5. Update documentation

Example structure:
```
pkg/providers/yourprovider/
├── yourprovider.go      # Main provider implementation
├── types.go             # Provider-specific types
├── transform.go         # Request/response transformations
└── yourprovider_test.go # Tests
```

### Documentation

- Update README.md for user-facing changes
- Add package documentation with examples
- Document any breaking changes
- Include examples for new features

## Pull Request Process

1. Ensure CI passes
2. Update documentation
3. Add entries to CHANGELOG (if exists)
4. Squash commits if needed
5. Request review from maintainers

## Questions?

Feel free to open an issue for any questions about contributing.