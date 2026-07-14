# Error Handling

Error types, patterns, and handling strategies in Wormhole.

## Overview

Wormhole provides a structured error handling system with context preservation, retry classification, and type-safe error checking. All errors implement standard Go interfaces for `errors.Is` and `errors.As` compatibility.

## Error Types

### WormholeError

Core structured error type with rich context:

```go
type WormholeError struct {
    Code       ErrorCode     // Error categorization
    Message    string        // Human-readable description
    Retryable  bool          // Can this error be retried?
    StatusCode int           // HTTP status code (if applicable)
    Provider   string        // Provider name (e.g., "openai", "gemini")
    Model      string        // Model name (e.g., "gpt-5.2")
    Details    string        // Additional context
    Cause      error         // Underlying error
}
```

**Error Codes:**

| Code | Description | Retryable |
|------|-------------|-----------|
| `AUTH_ERROR` | Authentication failures | No |
| `MODEL_ERROR` | Model not available | No |
| `RATE_LIMIT_ERROR` | Rate limit exceeded | Yes |
| `REQUEST_ERROR` | Invalid request | No |
| `TIMEOUT_ERROR` | Request timeout | Yes |
| `PROVIDER_ERROR` | Provider-side failure | Contextual |
| `NETWORK_ERROR` | Network issues | Yes |
| `VALIDATION_ERROR` | Input validation failed | No |
| `MIDDLEWARE_ERROR` | Middleware failure | Contextual |
| `UNKNOWN_ERROR` | Unclassified error | No |

### ModelConstraintError

Model-specific constraint violations:

```go
type ModelConstraintError struct {
    *WormholeError
    Constraint string  // e.g., "max_tokens", "context_length"
    Expected   any     // Expected value/range
    Actual     any     // Actual value received
}
```

**Example:**

```go
if err != nil {
    var merr *ModelConstraintError
    if errors.As(err, &merr) {
        log.Printf("Constraint %s: expected %v, got %v",
            merr.Constraint, merr.Expected, merr.Actual)
    }
}
```

### ValidationError

Field-level validation failures:

```go
type ValidationError struct {
    *WormholeError
    Field      string  // Field name that failed validation
    Constraint string  // Validation rule violated
    Value      any     // The invalid value
}
```

**Example:**

```go
if err != nil {
    var verr *ValidationError
    if errors.As(err, &verr) {
        log.Printf("Field '%s' failed '%s': %v", verr.Field, verr.Constraint, verr.Value)
    }
}
```

### ValidationErrors

Collection of multiple validation errors:

```go
type ValidationErrors struct {
    *WormholeError
    Errors []*ValidationError
}
```

### MiddlewareError

Middleware operation failures:

```go
type MiddlewareError struct {
    Operation  string    // Operation being performed
    Middleware string    // Middleware name (e.g., "circuit_breaker")
    Cause      error     // Underlying error
    Timestamp  time.Time // When error occurred
}
```

## Error Checking

### Type Checking with errors.As

Extract specific error types from wrapped errors:

```go
// Check if error is a WormholeError
var wormholeErr *WormholeError
if errors.As(err, &wormholeErr) {
    log.Printf("Error code: %s, retryable: %v", wormholeErr.Code, wormholeErr.Retryable)
}

// Check for constraint violations
var constraintErr *ModelConstraintError
if errors.As(err, &constraintErr) {
    // Handle constraint-specific logic
}

// Check for validation errors
var validErr *ValidationError
if errors.As(err, &validErr) {
    // Handle field validation
}
```

### Predicate Functions

Use built-in type-checking helpers:

```go
import "github.com/garyblankenship/wormhole/v2/types"

if types.IsAuthError(err) {
    // Handle authentication failures
}

if types.IsRateLimitError(err) {
    // Handle rate limiting
}

if types.IsTimeoutError(err) {
    // Handle timeouts
}

if types.IsValidationError(err) {
    // Handle validation failures
}

if types.IsMiddlewareError(err) {
    // Handle middleware errors
}
```

### Error Comparison with errors.Is

Check for specific error instances:

```go
import "github.com/garyblankenship/wormhole/v2/types"

if errors.Is(err, types.ErrInvalidAPIKey) {
    // Prompt for new API key
}

if errors.Is(err, types.ErrRateLimited) {
    // Apply backoff
}

if errors.Is(err, types.ErrCircuitOpen) {
    // Wait for circuit to close
}
```

## Error Wrapping Patterns

### Basic Wrapping

```go
// Standard Go error wrapping
err := fmt.Errorf("operation failed: %w", originalErr)
```

### Wormhole Error Wrapping

```go
import "github.com/garyblankenship/wormhole/v2/types"

// Create a classified WormholeError while preserving the cause
err := types.WrapError(types.ErrorCodeAuth, "authentication failed", false, originalErr)

// Create with formatting
err := types.Errorff("auth for %s failed", originalErr, userID)

// Wrap existing error with context
err := types.WrapError(types.ErrorCodeProvider, "provider unavailable", true, originalErr).
    WithProvider("openai").
    WithModel("gpt-5.2").
    WithStatusCode(503)
```

### Context Builders

Add context to WormholeError:

```go
err := types.NewWormholeError(types.ErrorCodeModel, "model error", false).
    WithProvider("anthropic").
    WithModel("claude-sonnet-4-5").
    WithDetails("temperature out of range").
    WithStatusCode(400)
```

### Middleware Wrapping

Middleware preserves WormholeError types:

```go
// In middleware implementation
if err != nil {
    return wrapIfNotWormholeError(err, "circuit_breaker", "execute")
}

// wrapIfNotWormholeError preserves WormholeError
// Non-WormholeErrors become MiddlewareError
```

## Retry Strategies

### Retry Configuration

```go
type RetryConfig struct {
    MaxRetries      int           // Maximum retry attempts (default: 3)
    InitialDelay    time.Duration // Initial backoff delay (default: 1s)
    MaxDelay        time.Duration // Maximum backoff delay (default: 30s)
    BackoffMultiple float64       // Backoff multiplier (default: 2.0)
    Jitter          bool          // Add randomness to prevent thundering herd
    RetryableFunc   func(error) bool // Custom retryable check
}
```

### Default Retryable Errors

The following errors are considered retryable by default:

| Error Type | Description |
|------------|-------------|
| HTTP 429   | Rate limit exceeded |
| HTTP 500   | Internal server error |
| HTTP 502   | Bad gateway |
| HTTP 503   | Service unavailable |
| HTTP 504   | Gateway timeout |
| HTTP 408   | Request timeout |
| `ErrTimeout` | Wormhole timeout error |
| `ErrRateLimited` | Wormhole rate limit error |
| Network errors | Connection failures |

### Exponential Backoff

Default retry strategy with exponential backoff:

```go
config := middleware.DefaultRetryConfig()

// Delay calculation:
// attempt 0: 1s ± 25% (with jitter)
// attempt 1: 2s ± 25%
// attempt 2: 4s ± 25%
```

### Custom Retryable Function

Define custom retry logic:

```go
config := &middleware.RetryConfig{
    MaxRetries: 5,
    RetryableFunc: func(err error) bool {
        // Only retry network errors
        var werr *types.WormholeError
        if errors.As(err, &werr) {
            return werr.Code == types.ErrorCodeNetwork
        }
        return false
    },
}
```

### Retry After

Get suggested retry delay from error:

```go
import "github.com/garyblankenship/wormhole/v2/types"

// GetRetryAfter returns a suggested delay, or 0 when the error is not retryable.
// It prefers a provider-supplied hint when present, then falls back to
// code-based defaults (rate limit 30s, network 5s, timeout 10s).
delay := types.GetRetryAfter(err)
if delay > 0 {
    log.Printf("Retry suggested after %v", delay)
    time.Sleep(delay)
}
```

#### Provider retry hints

HTTP providers normalize `Retry-After` and rate-limit reset headers onto the
error as `WormholeError.RetryAfter`. `GetRetryAfter` (above) returns this value
when it is positive before falling back to code-based defaults.

```go
var werr *types.WormholeError
if errors.As(err, &werr) && werr.RetryAfter > 0 {
    time.Sleep(werr.RetryAfter)
}
```

To parse a delay from raw response headers yourself, use
`types.ParseRetryAfterHeader`. It reads `Retry-After` (integer seconds or an
HTTP-date) and then `x-ratelimit-reset-requests` (integer seconds or a Go-style
duration such as `1m26.4s`), returning `0` when no usable hint is present:

```go
delay := types.ParseRetryAfterHeader(resp.Header, time.Now())
err = werr.WithRetryAfter(delay) // copy-style setter; returns a new *WormholeError
```

### Configuring Provider Retries

```go
client := wormhole.New(
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    wormhole.WithRetries(3, time.Second),
)
```

`WithRetries` sets the client default for providers that do not specify
`ProviderConfig.MaxRetries` or `ProviderConfig.RetryDelay`. Set `MaxRetries` on a
provider config when one provider needs different behavior.

## Error Creation Patterns

### Predefined Errors

Use predefined errors for common cases:

```go
import "github.com/garyblankenship/wormhole/v2/types"

var (
    ErrInvalidAPIKey    = types.ErrInvalidAPIKey
    ErrMissingAPIKey    = types.ErrMissingAPIKey
    ErrModelNotFound    = types.ErrModelNotFound
    ErrRateLimited      = types.ErrRateLimited
    ErrTimeout          = types.ErrTimeout
    ErrCircuitOpen      = types.ErrCircuitOpen
)
```

### Provider Error Creation

Convert HTTP status codes to WormholeErrors:

```go
import "github.com/garyblankenship/wormhole/v2/types"

statusCode := 429
err := types.HTTPStatusToError(statusCode, responseBody)
// Returns ErrRateLimited (retryable)
```

### Custom Error Creation

```go
import "github.com/garyblankenship/wormhole/v2/types"

// Simple error
err := types.NewWormholeError(
    types.ErrorCodeAuth,
    "invalid credentials",
    false, // not retryable
)

// Error with cause
err := types.NewWormholeError(
    types.ErrorCodeNetwork,
    "connection failed",
    true, // retryable
).WithCause(originalErr)

// Fully specified error
err := &types.WormholeError{
    Code:       types.ErrorCodeModel,
    Message:    "model not supported",
    Retryable:  false,
    StatusCode: 400,
    Provider:   "custom",
    Model:      "my-model",
    Details:    "requires v2 API",
    Cause:      nil,
}
```

## Best Practices

### DO

- Use `errors.As` for type checking
- Wrap errors with context at each layer
- Check `Retryable` field before retrying
- Use structured error types for domain-specific errors
- Preserve original error as `Cause`

### DON'T

- Use `errors.Is` for type checking (use `errors.As`)
- Ignore `WormholeError` context
- Retry non-retryable errors indefinitely
- Create generic errors without context
- Discard original errors when wrapping

### Error Handling Pattern

```go
func DoWork() error {
    // ... work ...
    if err != nil {
        // Wrap with context
        return types.WrapError(types.ErrorCodeProvider, "work failed", true, err).
            WithProvider("openai")
    }
    return nil
}

// Caller
if err := DoWork(); err != nil {
    var werr *types.WormholeError
    if errors.As(err, &werr) && werr.Retryable {
        // Retry logic
    } else {
        // Handle fatal error
    }
}
```

## See Also

- [Providers](../providers/anthropic.md) - Provider-specific error handling
