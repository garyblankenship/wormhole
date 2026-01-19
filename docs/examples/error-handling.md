# Error Handling Examples

This document demonstrates comprehensive error handling patterns for the Wormhole SDK, including specific error type checking, retry logic, and rate limit handling.

## Table of Contents

- [Checking Specific Error Types](#checking-specific-error-types)
- [Retry Logic](#retry-logic)
- [Rate Limit Handling](#rate-limit-handling)
- [Validation Errors](#validation-errors)
- [Circuit Breaker Errors](#circuit-breaker-errors)

---

## Checking Specific Error Types

The SDK provides helper functions to check for specific error types. Use these to handle different failure scenarios appropriately.

### Basic Error Type Checks

```go
package main

import (
    "fmt"
    "log"

    "github.com/garyblankenship/wormhole/pkg/types"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func handleGenerateError(err error) {
    if err == nil {
        return
    }

    // Check for authentication errors
    if types.IsAuthError(err) {
        log.Fatalf("Authentication failed: check your API key: %v", err)
    }

    // Check for rate limit errors
    if types.IsRateLimitError(err) {
        retryAfter := types.GetRetryAfter(err)
        log.Printf("Rate limited. Retry after %v", retryAfter)
        return
    }

    // Check for model errors
    if types.IsModelError(err) {
        log.Fatalf("Model error: %v. Try a different model.", err)
    }

    // Check for network errors
    if types.IsNetworkError(err) {
        log.Printf("Network error: %v. Retrying...", err)
        return
    }

    // Check for timeout errors
    if types.IsTimeoutError(err) {
        log.Printf("Request timed out: %v", err)
        return
    }

    // Check for validation errors
    if types.IsValidationError(err) {
        if vErr, ok := types.AsValidationError(err); ok {
            log.Fatalf("Validation failed for field '%s': %s", vErr.Field, vErr.Message)
        }
    }

    // Check for middleware errors (circuit breaker, rate limiter)
    if types.IsMiddlewareError(err) {
        log.Printf("Middleware error: %v", err)
        return
    }

    // Generic error handling
    log.Fatalf("Request failed: %v", err)
}

func main() {
    client, _ := wormhole.NewClient(
        wormhole.WithProvider("gemini"),
        wormhole.WithAPIKey("your-api-key"),
    )

    builder := client.TextRequest().
        WithModel("gemini-2.5-flash").
        WithMessage("user", "Hello!")

    resp, err := builder.Generate()
    if err != nil {
        handleGenerateError(err)
        return
    }

    fmt.Println(resp.Content)
}
```

### Accessing Detailed Error Information

```go
func detailedErrorHandling(err error) {
    // Extract WormholeError for detailed information
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        fmt.Printf("Error Code: %s\n", wormholeErr.Code)
        fmt.Printf("Message: %s\n", wormholeErr.Message)
        fmt.Printf("Retryable: %v\n", wormholeErr.Retryable)
        fmt.Printf("Status Code: %d\n", wormholeErr.StatusCode)
        fmt.Printf("Provider: %s\n", wormholeErr.Provider)
        fmt.Printf("Model: %s\n", wormholeErr.Model)
        fmt.Printf("Details: %s\n", wormholeErr.Details)

        // Check if error is retryable
        if wormholeErr.IsRetryable() {
            fmt.Println("This error can be retried")
        }
    }
}
```

### Checking for Retryable Errors

```go
func shouldRetry(err error) bool {
    // Use the helper function to check if an error is retryable
    if types.IsRetryableError(err) {
        retryAfter := types.GetRetryAfter(err)
        if retryAfter > 0 {
            fmt.Printf("Error is retryable after %v\n", retryAfter)
        }
        return true
    }
    return false
}

// Usage in retry loop
for attempt := 0; attempt < 3; attempt++ {
    resp, err := builder.Generate()
    if err == nil {
        break // Success
    }

    if !shouldRetry(err) {
        log.Fatalf("Non-retryable error: %v", err)
    }

    // Wait before retrying
    retryAfter := types.GetRetryAfter(err)
    if retryAfter == 0 {
        retryAfter = 5 * time.Second // Default delay
    }
    time.Sleep(retryAfter)
}
```

---

## Retry Logic

The SDK provides built-in retry utilities for handling transient failures.

### Using the Built-in Retryable HTTP Client

```go
package main

import (
    "context"
    "log"
    "net/http"
    "time"

    "github.com/garyblankenship/wormhole/internal/utils"
)

func retryableHTTPExample() {
    // Create a retryable HTTP client with custom configuration
    config := utils.RetryConfig{
        MaxRetries:      5,
        InitialDelay:    1 * time.Second,
        MaxDelay:        30 * time.Second,
        BackoffMultiple: 2.0, // Exponential backoff
        Jitter:          true, // Add jitter to prevent thundering herd
    }

    client := utils.NewRetryableHTTPClient(&http.Client{}, config)

    req, _ := http.NewRequestWithContext(
        context.Background(),
        "GET",
        "https://api.example.com/endpoint",
        nil,
    )

    // The client will automatically retry on retryable status codes
    resp, err := client.Do(req)
    if err != nil {
        log.Fatalf("Request failed after retries: %v", err)
    }
    defer resp.Body.Close()

    log.Printf("Request succeeded with status: %d", resp.StatusCode)
}
```

### Using WithRetry for Custom Functions

```go
func customRetryExample() error {
    ctx := context.Background()
    config := utils.DefaultRetryConfig()

    // Use WithRetry to execute any function with retry logic
    err := utils.WithRetry(ctx, config, func() error {
        // Your operation here
        return attemptOperation()
    })

    if err != nil {
        return fmt.Errorf("operation failed after retries: %w", err)
    }

    return nil
}

func attemptOperation() error {
    // Simulated operation that may fail
    return nil
}
```

### Manual Retry with Exponential Backoff

```go
func manualRetryWithBackoff(builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    maxRetries := 3
    baseDelay := 1 * time.Second

    for attempt := 0; attempt <= maxRetries; attempt++ {
        resp, err := builder.Generate()

        if err == nil {
            return resp, nil // Success
        }

        // Check if error is retryable
        if !types.IsRetryableError(err) {
            return nil, err // Don't retry non-retryable errors
        }

        // Don't sleep after the last attempt
        if attempt == maxRetries {
            break
        }

        // Calculate delay with exponential backoff
        delay := baseDelay * time.Duration(1<<uint(attempt))

        // Get server-suggested retry delay if available
        if retryAfter := types.GetRetryAfter(err); retryAfter > 0 {
            delay = retryAfter
        }

        log.Printf("Attempt %d failed: %v. Retrying after %v...", attempt+1, err, delay)
        time.Sleep(delay)
    }

    return nil, fmt.Errorf("max retries (%d) exceeded", maxRetries)
}
```

### Context-Aware Retry

```go
func contextAwareRetry(ctx context.Context, builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    for {
        resp, err := builder.Generate()
        if err == nil {
            return resp, nil
        }

        // Check if context is cancelled
        if ctx.Err() != nil {
            return nil, ctx.Err()
        }

        // Check if error is retryable
        if !types.IsRetryableError(err) {
            return nil, err
        }

        // Wait for retry delay or context cancellation
        retryAfter := types.GetRetryAfter(err)
        if retryAfter == 0 {
            retryAfter = 5 * time.Second
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(retryAfter):
            // Retry
        }
    }
}
```

---

## Rate Limit Handling

Rate limiting errors require special handling to respect provider limits and avoid being blocked.

### Basic Rate Limit Handling

```go
func handleRateLimit(builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    for {
        resp, err := builder.Generate()
        if err == nil {
            return resp, nil
        }

        // Check if this is a rate limit error
        if !types.IsRateLimitError(err) {
            return nil, err // Not a rate limit error
        }

        // Get the suggested retry delay
        retryAfter := types.GetRetryAfter(err)
        if retryAfter == 0 {
            retryAfter = 30 * time.Second // Default for rate limits
        }

        log.Printf("Rate limited. Waiting %v before retry...", retryAfter)
        time.Sleep(retryAfter)
    }
}
```

### Handling HTTP 429 Too Many Requests

```go
func handleHTTP429(err error) time.Duration {
    // Extract the WormholeError to get HTTP status
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        if wormholeErr.StatusCode == 429 {
            // Check for Retry-After header information in details
            log.Printf("HTTP 429: %s", wormholeErr.Message)

            // Use GetRetryAfter for suggested delay
            retryAfter := types.GetRetryAfter(err)
            if retryAfter > 0 {
                return retryAfter
            }

            // Default to 30 seconds for rate limits
            return 30 * time.Second
        }
    }

    return 0
}
```

### Using the Rate Limiter Middleware

```go
package main

import (
    "context"

    "github.com/garyblankenship/wormhole/pkg/middleware"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func rateLimitMiddlewareExample() {
    client, _ := wormhole.NewClient(
        wormhole.WithProvider("gemini"),
        wormhole.WithAPIKey("your-api-key"),
    )

    // Add rate limiting middleware (10 requests per second)
    client.Use(middleware.RateLimitMiddleware(10))

    builder := client.TextRequest().
        WithModel("gemini-2.5-flash").
        WithMessage("user", "Hello!")

    // The middleware will automatically handle rate limiting
    resp, err := builder.Generate()
    if err != nil {
        if types.IsRateLimitError(err) {
            // Handle rate limit (though middleware usually prevents this)
            log.Printf("Rate limit exceeded: %v", err)
        }
    }

    _ = resp
}
```

### Adaptive Rate Limiting

```go
func adaptiveRateLimitExample() {
    client, _ := wormhole.NewClient(
        wormhole.WithProvider("gemini"),
        wormhole.WithAPIKey("your-api-key"),
    )

    // Create adaptive rate limiter
    // Starts at 10 req/s, adjusts between 5-20 req/s based on latency
    client.Use(middleware.AdaptiveRateLimitMiddleware(
        10,  // initial rate
        5,   // min rate
        20,  // max rate
        500*time.Millisecond, // target latency
    ))

    // The rate limiter will automatically adjust based on response times
    for i := 0; i < 100; i++ {
        builder := client.TextRequest().
            WithModel("gemini-2.5-flash").
            WithMessage("user", fmt.Sprintf("Message %d", i))

        resp, err := builder.Generate()
        if err != nil {
            log.Printf("Request %d failed: %v", i, err)
            continue
        }

        _ = resp
    }
}
```

### Exponential Backoff for Rate Limits

```go
func rateLimitBackoff(attempt int) time.Duration {
    // Implement exponential backoff with jitter for rate limits
    // Start at 1 second, double each time, cap at 5 minutes
    baseDelay := time.Second
    maxDelay := 5 * time.Minute

    delay := baseDelay * time.Duration(1<<uint(attempt))
    if delay > maxDelay {
        delay = maxDelay
    }

    // Add jitter (±25%)
    jitter := delay / 4
    delay += time.Duration(float64(jitter) * (2.0*rand.Float64() - 1.0))

    return delay
}

func retryWithRateLimitBackoff(builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    maxAttempts := 5

    for attempt := 0; attempt < maxAttempts; attempt++ {
        resp, err := builder.Generate()
        if err == nil {
            return resp, nil
        }

        if !types.IsRateLimitError(err) {
            return nil, err
        }

        delay := rateLimitBackoff(attempt)
        log.Printf("Rate limited (attempt %d/%d). Waiting %v...", attempt+1, maxAttempts, delay)
        time.Sleep(delay)
    }

    return nil, fmt.Errorf("max attempts exceeded due to rate limiting")
}
```

---

## Validation Errors

Validation errors occur when request parameters don't meet requirements.

### Single Field Validation

```go
func singleFieldValidation() {
    client, _ := wormhole.NewClient(
        wormhole.WithProvider("gemini"),
        wormhole.WithAPIKey("your-api-key"),
    )

    builder := client.TextRequest().
        WithModel("gemini-2.5-flash").
        WithMessage("user", "Hello!")

    // Validate before generating
    if err := builder.Validate(); err != nil {
        if vErr, ok := types.AsValidationError(err); ok {
            log.Printf("Field '%s' failed validation: %s", vErr.Field, vErr.Message)
            log.Printf("Constraint: %s", vErr.Constraint)
            log.Printf("Value: %v", vErr.Value)
        }
        return
    }

    resp, err := builder.Generate()
    _ = resp
    _ = err
}
```

### Multiple Field Validation

```go
func multipleFieldValidation(model string, temperature float64, messages []string) error {
    var errs types.ValidationErrors

    // Validate model
    if model == "" {
        errs.Add("model", "required", model, "model is required")
    }

    // Validate temperature
    if temperature < 0 || temperature > 2 {
        errs.Add("temperature", "range", temperature, "must be between 0.0 and 2.0")
    }

    // Validate messages
    if len(messages) == 0 {
        errs.Add("messages", "required", messages, "at least one message is required")
    }

    // Check if any validation failed
    if errs.HasErrors() {
        log.Printf("Validation failed for fields: %v", errs.Fields())
        return errs.Error()
    }

    return nil
}
```

### Handling Constraint Violations

```go
func handleConstraintError(err error) {
    // Check for model constraint errors
    if constraintErr, ok := err.(*types.ModelConstraintError); ok {
        log.Printf("Constraint '%s' violated for model '%s'", constraintErr.Constraint, constraintErr.Model)
        log.Printf("Expected: %v", constraintErr.Expected)
        log.Printf("Actual: %v", constraintErr.Actual)

        // Handle specific constraints
        switch constraintErr.Constraint {
        case "max_tokens":
            log.Printf("Reduce max_tokens from %v to %v", constraintErr.Actual, constraintErr.Expected)
        case "temperature":
            log.Printf("Adjust temperature from %v to be within %v", constraintErr.Actual, constraintErr.Expected)
        }
    }
}
```

---

## Circuit Breaker Errors

Circuit breakers prevent cascading failures by stopping requests to failing providers.

### Handling Circuit Breaker States

```go
func handleCircuitBreaker(err error) {
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        // Check if error is due to open circuit breaker
        if errors.Is(err, types.ErrCircuitOpen) {
            log.Println("Circuit breaker is OPEN. Requests are being blocked.")

            // Wait for circuit to enter half-open state
            // or switch to a different provider
            return
        }

        // Check for middleware errors
        if types.IsMiddlewareError(err) {
            log.Printf("Middleware error: %s", wormholeErr.Message)

            // The circuit breaker may be in half-open or closed state
            // Implement fallback logic here
        }
    }
}
```

### Fallback Provider Strategy

```go
func generateWithFallback(client *wormhole.Client, model string, prompt string) (*wormhole.TextResponse, error) {
    // Try primary provider
    resp, err := client.TextRequest().
        WithModel(model).
        WithMessage("user", prompt).
        Generate()

    if err == nil {
        return resp, nil
    }

    // Check if circuit breaker is open
    if errors.Is(err, types.ErrCircuitOpen) {
        log.Println("Primary provider circuit is open. Trying fallback...")

        // Switch to a different provider/model
        resp, err = client.TextRequest().
            WithModel("gemini-2.5-flash"). // Fallback model
            WithMessage("user", prompt).
            Generate()

        if err == nil {
            return resp, nil
        }
    }

    return nil, err
}
```

### Waiting for Circuit Recovery

```go
func waitForCircuitRecovery(ctx context.Context) error {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            // Try a test request to see if circuit has recovered
            err := attemptTestRequest()
            if err == nil {
                log.Println("Circuit has recovered!")
                return nil
            }

            if !errors.Is(err, types.ErrCircuitOpen) {
                // Different error - circuit may be closing
                return err
            }

            log.Println("Circuit still open. Waiting...")
        }
    }
}

func attemptTestRequest() error {
    // Implement a lightweight test request
    return nil
}
```

---

## Summary Table

| Error Type | Helper Function | Retryable | Default Delay |
|------------|-----------------|-----------|---------------|
| Authentication | `types.IsAuthError()` | No | - |
| Rate Limit | `types.IsRateLimitError()` | Yes | 30s |
| Model | `types.IsModelError()` | No | - |
| Network | `types.IsNetworkError()` | Yes | 5s |
| Timeout | `types.IsTimeoutError()` | Yes | 10s |
| Validation | `types.IsValidationError()` | No | - |
| Middleware | `types.IsMiddlewareError()` | Depends | - |
| Generic Retryable | `types.IsRetryableError()` | Yes | 1s |
