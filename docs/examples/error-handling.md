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
        return                                          // [1] No error - nothing to handle
    }

    // Check for authentication errors
    if types.IsAuthError(err) {                        // [2] Detect auth failures
        log.Fatalf("Authentication failed: check your API key: %v", err)
    }

    // Check for rate limit errors
    if types.IsRateLimitError(err) {                    // [3] Detect rate limiting
        retryAfter := types.GetRetryAfter(err)          // [3a] Get suggested delay
        log.Printf("Rate limited. Retry after %v", retryAfter)
        return
    }

    // Check for model errors
    if types.IsModelError(err) {                        // [4] Detect model-related issues
        log.Fatalf("Model error: %v. Try a different model.", err)
    }

    // Check for network errors
    if types.IsNetworkError(err) {                      // [5] Detect network failures
        log.Printf("Network error: %v. Retrying...", err)
        return
    }

    // Check for timeout errors
    if types.IsTimeoutError(err) {                      // [6] Detect timeout conditions
        log.Printf("Request timed out: %v", err)
        return
    }

    // Check for validation errors
    if types.IsValidationError(err) {                   // [7] Detect input validation failures
        if vErr, ok := types.AsValidationError(err); ok {  // [8] Extract validation details
            log.Fatalf("Validation failed for field '%s': %s", vErr.Field, vErr.Message)
        }
    }

    // Check for middleware errors (circuit breaker, rate limiter)
    if types.IsMiddlewareError(err) {                   // [9] Detect middleware-related failures
        log.Printf("Middleware error: %v", err)
        return
    }

    // Generic error handling
    log.Fatalf("Request failed: %v", err)               // [10] Catch-all for unhandled errors
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
        handleGenerateError(err)                        // [11] Delegate error handling
        return
    }

    fmt.Println(resp.Content)
}
```

### What's happening?

1. **Early return** - If there's no error, return immediately.
2. **Auth check** - Detects invalid API keys, missing credentials. Returns `true` for 401/403 errors.
3. **Rate limit check** - Detects HTTP 429 responses. `GetRetryAfter()` extracts the `Retry-After` header value.
4. **Model check** - Detects unknown models, unsupported model capabilities.
5. **Network check** - Detects connection failures, DNS errors, network timeouts.
6. **Timeout check** - Detects context cancellations and client-side timeouts.
7. **Validation check** - Detects request validation failures (invalid parameters).
8. **Validation extraction** - `AsValidationError` performs type assertion to access field-level error details.
9. **Middleware check** - Detects errors from circuit breakers, rate limiters, and other middleware.
10. **Generic handler** - Final catch-all for errors that don't match specific categories.
11. **Handler delegation** - The `handleGenerateError` function centralizes all error logic.

> [!TIP]
> Check errors in order of specificity: auth -> rate limit -> validation -> network -> timeout -> generic. More specific checks should come first.

> [!WARNING]
> Auth errors (401/403) are NOT retryable - fix your API key before retrying. Rate limit errors (429) ARE retryable - wait for the `Retry-After` duration.

### Accessing Detailed Error Information

```go
func detailedErrorHandling(err error) {
    // Extract WormholeError for detailed information
    if wormholeErr, ok := types.AsWormholeError(err); ok {  // [1] Type assert to WormholeError
        fmt.Printf("Error Code: %s\n", wormholeErr.Code)    // [2] Machine-readable error code
        fmt.Printf("Message: %s\n", wormholeErr.Message)    // [3] Human-readable description
        fmt.Printf("Retryable: %v\n", wormholeErr.Retryable) // [4] Whether retry is appropriate
        fmt.Printf("Status Code: %d\n", wormholeErr.StatusCode) // [5] HTTP status code
        fmt.Printf("Provider: %s\n", wormholeErr.Provider)  // [6] Which provider had the error
        fmt.Printf("Model: %s\n", wormholeErr.Model)        // [7] Model being used
        fmt.Printf("Details: %s\n", wormholeErr.Details)    // [8] Additional debugging info

        // Check if error is retryable
        if wormholeErr.IsRetryable() {                      // [9] Convenience method
            fmt.Println("This error can be retried")
        }
    }
}
```

### What's happening?

1. **Type assertion** - `AsWormholeError` safely converts to `*WormholeError` if the error is a Wormhole error type.
2. **Error code** - Machine-readable code like `ErrorCodeAuth`, `ErrorCodeRateLimit`, `ErrorCodeTimeout`.
3. **Message** - Human-readable description of what went wrong.
4. **Retryable flag** - Indicates if this error type should be retried (rate limits: yes, auth: no).
5. **Status code** - HTTP status code (401, 429, 500, etc.) if applicable.
6. **Provider** - Which provider returned the error (openai, anthropic, etc.).
7. **Model** - The model being used when the error occurred.
8. **Details** - Additional context like headers, request IDs, etc.
9. **Retryable check** - Convenience method that combines multiple conditions to determine retry suitability.

> [!TIP]
> Use `AsWormholeError` for logging and debugging - it provides the complete picture of what went wrong. The helper functions like `IsAuthError()` are more convenient for control flow.

> [!WARNING]
> Not all errors are `WormholeError` instances. Network errors or context cancellations may be standard Go errors. Always check the `ok` return value from type assertions.

### Checking for Retryable Errors

```go
func shouldRetry(err error) bool {
    // Use the helper function to check if an error is retryable
    if types.IsRetryableError(err) {                     // [1] Check if error warrants retry
        retryAfter := types.GetRetryAfter(err)           // [2] Get suggested delay
        if retryAfter > 0 {
            fmt.Printf("Error is retryable after %v\n", retryAfter)
        }
        return true
    }
    return false
}

// Usage in retry loop
for attempt := 0; attempt < 3; attempt++ {              // [3] Retry up to 3 times
    resp, err := builder.Generate()
    if err == nil {
        break                                            // [4] Success - exit loop
    }

    if !shouldRetry(err) {                               // [5] Check retryability
        log.Fatalf("Non-retryable error: %v", err)       // [6] Give up on non-retryable errors
    }

    // Wait before retrying
    retryAfter := types.GetRetryAfter(err)               // [7] Get server-suggested delay
    if retryAfter == 0 {                                 // [8] Fallback to default
        retryAfter = 5 * time.Second // Default delay
    }
    time.Sleep(retryAfter)                               // [9] Wait before retry
}
```

### What's happening?

1. **Retryability check** - `IsRetryableError()` returns `true` for rate limits, timeouts, network errors, and 5xx server errors.
2. **Retry delay** - `GetRetryAfter()` returns the server's suggested delay from the `Retry-After` header.
3. **Retry loop** - Attempt the request up to 3 times.
4. **Success exit** - If the request succeeds, break from the loop immediately.
5. **Error handling** - Check if the error is retryable before attempting another try.
6. **Fatal exit** - Non-retryable errors (auth, validation, model errors) should fail immediately.
7. **Server delay** - Use the server's suggested delay when available.
8. **Default delay** - If no suggestion, use 5 seconds as a reasonable default.
9. **Backoff wait** - Sleep before the next retry attempt.

> [!TIP]
> `IsRetryableError()` combines multiple error checks: rate limits, network errors, timeouts, and 5xx server errors. It excludes auth errors, validation errors, and 4xx client errors.

> [!WARNING]
> Always implement maximum retry attempts. Without a limit, a retry loop could run indefinitely on persistent errors.

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
        MaxRetries:      5,                               // [1] Maximum retry attempts
        InitialDelay:    1 * time.Second,                 // [2] Starting delay before first retry
        MaxDelay:        30 * time.Second,                // [3] Maximum cap on delay duration
        BackoffMultiple: 2.0,                             // [4] Exponential backoff multiplier
        Jitter:          true,                            // [5] Add randomness to prevent thundering herd
    }

    client := utils.NewRetryableHTTPClient(&http.Client{}, config)  // [6] Wrap standard client

    req, _ := http.NewRequestWithContext(
        context.Background(),
        "GET",
        "https://api.example.com/endpoint",
        nil,
    )

    // The client will automatically retry on retryable status codes
    resp, err := client.Do(req)                          // [7] Execute with automatic retries
    if err != nil {
        log.Fatalf("Request failed after retries: %v", err)
    }
    defer resp.Body.Close()

    log.Printf("Request succeeded with status: %d", resp.StatusCode)
}
```

### What's happening?

1. **Max retries** - Total number of retry attempts before giving up. Default is 3.
2. **Initial delay** - First retry waits 1 second. Subsequent retries use exponential backoff.
3. **Max delay** - Even with exponential backoff, delay won't exceed 30 seconds.
4. **Backoff multiplier** - Each retry's delay is `previous_delay * 2.0` (exponential: 1s, 2s, 4s, 8s...).
5. **Jitter** - Adds random variation to delays to prevent synchronized retries from multiple clients.
6. **Client wrapping** - Wraps a standard `http.Client` with retry logic.
7. **Automatic retries** - Retries happen automatically for 429, 500, 502, 503, 504 status codes.

> [!TIP]
> Exponential backoff with jitter is the industry standard for retry logic. It prevents overwhelming recovering services and avoids the "thundering herd" problem where many clients retry simultaneously.

> [!WARNING]
> The retryable client does NOT retry on 4xx errors (except 429). These are client errors that won't be fixed by retrying - fix your request instead.

### Using WithRetry for Custom Functions

```go
func customRetryExample() error {
    ctx := context.Background()
    config := utils.DefaultRetryConfig()                 // [1] Get default retry settings

    // Use WithRetry to execute any function with retry logic
    err := utils.WithRetry(ctx, config, func() error {   // [2] Wrap operation in retry logic
        // Your operation here
        return attemptOperation()                        // [3] Function that may fail transiently
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

### What's happening?

1. **Default config** - Provides sensible defaults: 3 retries, exponential backoff, jitter enabled.
2. **WithRetry wrapper** - Wraps any function with automatic retry logic on failure.
3. **Operation function** - Your custom operation that might fail transiently.

> [!TIP]
> `WithRetry` is useful for any operation that can fail transiently: database queries, external API calls, file system operations under load, etc.

> [!WARNING]
> Ensure your operation is idempotent before wrapping with retry logic. If the operation isn't idempotent, retries could cause duplicate side effects (e.g., charging a credit card twice).

### Manual Retry with Exponential Backoff

```go
func manualRetryWithBackoff(builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    maxRetries := 3                                        // [1] Maximum retry attempts
    baseDelay := 1 * time.Second                           // [2] Starting delay

    for attempt := 0; attempt <= maxRetries; attempt++ {   // [3] Try up to maxRetries + 1
        resp, err := builder.Generate()

        if err == nil {
            return resp, nil                               // [4] Success - return immediately
        }

        // Check if error is retryable
        if !types.IsRetryableError(err) {                  // [5] Test retryability
            return nil, err                                // [6] Don't retry non-retryable errors
        }

        // Don't sleep after the last attempt
        if attempt == maxRetries {                         // [7] Check if last attempt
            break
        }

        // Calculate delay with exponential backoff
        delay := baseDelay * time.Duration(1<<uint(attempt)) // [8] 1s, 2s, 4s, 8s...

        // Get server-suggested retry delay if available
        if retryAfter := types.GetRetryAfter(err); retryAfter > 0 { // [9] Server suggestion
            delay = retryAfter                             // [10] Use server's delay
        }

        log.Printf("Attempt %d failed: %v. Retrying after %v...", attempt+1, err, delay)
        time.Sleep(delay)                                  // [11] Wait before retry
    }

    return nil, fmt.Errorf("max retries (%d) exceeded", maxRetries)
}
```

### What's happening?

1. **Max retries** - Maximum number of retry attempts after the initial failure.
2. **Base delay** - Starting point for exponential backoff (1 second).
3. **Loop range** - Iterate from 0 to maxRetries inclusive (initial + 3 retries = 4 total attempts).
4. **Success return** - If successful, return immediately without further retries.
5. **Retryability test** - Check if the error type warrants a retry.
6. **Non-retryable exit** - Auth errors, validation errors, etc. should fail immediately.
7. **Last attempt check** - Don't sleep after the final failed attempt.
8. **Exponential calculation** - `1 << uint(attempt)` produces 1, 2, 4, 8... (bit shift for power of 2).
9. **Server suggestion** - Provider may suggest a specific retry delay via `Retry-After` header.
10. **Override delay** - Use server's suggestion when available.
11. **Backoff sleep** - Wait before the next retry attempt.

> [!TIP]
> The bit shift `1 << uint(attempt)` is an efficient way to calculate powers of 2: 1, 2, 4, 8, 16... This is exponential backoff.

> [!WARNING]
> Exponential backoff without max delay can lead to very long waits. Add a cap: `if delay > maxDelay { delay = maxDelay }`.

### Context-Aware Retry

```go
func contextAwareRetry(ctx context.Context, builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    for {
        resp, err := builder.Generate()
        if err == nil {
            return resp, nil                               // [1] Success - return
        }

        // Check if context is cancelled
        if ctx.Err() != nil {                              // [2] Context cancelled?
            return nil, ctx.Err()                          // [3] Propagate cancellation
        }

        // Check if error is retryable
        if !types.IsRetryableError(err) {                   // [4] Test retryability
            return nil, err                                // [5] Non-retryable - fail
        }

        // Wait for retry delay or context cancellation
        retryAfter := types.GetRetryAfter(err)
        if retryAfter == 0 {
            retryAfter = 5 * time.Second
        }

        select {                                            // [6] Wait for delay OR cancellation
        case <-ctx.Done():                                  // [7] Context cancelled first
            return nil, ctx.Err()
        case <-time.After(retryAfter):                      // [8] Delay elapsed - retry
            // Retry
        }
    }
}
```

### What's happening?

1. **Success return** - Return immediately on success.
2. **Context check** - Before retry, check if the context has been cancelled.
3. **Propagation** - Return the context error (usually `context.Canceled` or `context.DeadlineExceeded`).
4. **Retryability test** - Only retry appropriate error types.
5. **Non-retryable exit** - Fail immediately for auth, validation, etc.
6. **Select statement** - Wait for either the retry delay OR context cancellation, whichever happens first.
7. **Cancellation branch** - If context is cancelled during wait, abort immediately.
8. **Delay branch** - If delay completes, continue to next retry iteration.

> [!TIP]
> The `select` statement with `ctx.Done()` allows external cancellation to interrupt retry waits. This is critical for graceful shutdown.

> [!WARNING]
> Always check `ctx.Err()` before and during retry loops. Without context awareness, your retries may continue after a shutdown is initiated.

---

## Rate Limit Handling

Rate limiting errors require special handling to respect provider limits and avoid being blocked.

### Basic Rate Limit Handling

```go
func handleRateLimit(builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    for {
        resp, err := builder.Generate()
        if err == nil {
            return resp, nil                               // [1] Success - return
        }

        // Check if this is a rate limit error
        if !types.IsRateLimitError(err) {                   // [2] Detect HTTP 429
            return nil, err                                // [3] Not rate limit - fail
        }

        // Get the suggested retry delay
        retryAfter := types.GetRetryAfter(err)             // [4] Extract Retry-After header
        if retryAfter == 0 {
            retryAfter = 30 * time.Second                  // [5] Default for rate limits
        }

        log.Printf("Rate limited. Waiting %v before retry...", retryAfter)
        time.Sleep(retryAfter)                             // [6] Respect provider's limit
    }
}
```

### What's happening?

1. **Success return** - Return the response on success.
2. **Rate limit detection** - `IsRateLimitError()` returns `true` for HTTP 429 responses.
3. **Non-rate-limit exit** - If it's not a rate limit error, return immediately for other handling.
4. **Extract delay** - `GetRetryAfter()` reads the `Retry-After` header value.
5. **Default delay** - If no header, use 30 seconds (standard for rate limits).
6. **Respect limit** - Wait the full duration before retrying to avoid being blocked.

> [!TIP]
> Always respect `Retry-After` headers. Ignoring them can lead to temporary or permanent account suspensions.

> [!WARNING]
> Rate limiting is per API key. If you're using the same key across multiple services, you need to coordinate rate limiting at the application level.

### Handling HTTP 429 Too Many Requests

```go
func handleHTTP429(err error) time.Duration {
    // Extract the WormholeError to get HTTP status
    if wormholeErr, ok := types.AsWormholeError(err); ok {     // [1] Type assert to WormholeError
        if wormholeErr.StatusCode == 429 {                     // [2] Check for HTTP 429
            // Check for Retry-After header information in details
            log.Printf("HTTP 429: %s", wormholeErr.Message)    // [3] Log rate limit message

            // Use GetRetryAfter for suggested delay
            retryAfter := types.GetRetryAfter(err)             // [4] Extract Retry-After header
            if retryAfter > 0 {
                return retryAfter                              // [5] Return server's suggestion
            }

            // Default to 30 seconds for rate limits
            return 30 * time.Second                            // [6] Fallback default
        }
    }

    return 0                                                  // [7] Not a 429 error
}
```

### What's happening?

1. **Type assertion** - Convert the error to `WormholeError` to access HTTP status.
2. **Status check** - Look for HTTP 429 specifically.
3. **Message logging** - The provider may include additional info in the message.
4. **Header extraction** - `GetRetryAfter()` parses the `Retry-After` header.
5. **Server suggestion** - Use the provider's suggested delay when available.
6. **Default fallback** - 30 seconds is a common default for rate limits.
7. **Non-429 return** - Return 0 to indicate this isn't a rate limit error.

> [!TIP]
> Some providers use `Retry-After` in seconds (integer) while others use a date. `GetRetryAfter()` handles both formats automatically.

> [!WARNING]
> HTTP 429 can also include a `RateLimit-Reset` header or other provider-specific headers. Check your provider's documentation for additional rate limit information.

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
    client.Use(middleware.RateLimitMiddleware(10))           // [1] Apply rate limiter

    builder := client.TextRequest().
        WithModel("gemini-2.5-flash").
        WithMessage("user", "Hello!")

    // The middleware will automatically handle rate limiting
    resp, err := builder.Generate()
    if err != nil {
        if types.IsRateLimitError(err) {
            // Handle rate limit (though middleware usually prevents this)
            log.Printf("Rate limit exceeded: %v", err)        // [2] Fallback error handling
        }
    }

    _ = resp
}
```

### What's happening?

1. **Middleware application** - `RateLimitMiddleware(10)` limits to 10 requests per second.
2. **Fallback handling** - The middleware should prevent rate limits, but handle edge cases.

> [!TIP]
> Middleware-based rate limiting prevents hitting provider limits entirely. It's better than reactive retry-based handling.

> [!WARNING]
> Rate limit middleware is per-client instance. If you create multiple clients with the same API key, rate limiting won't be coordinated between them.

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
        10,                  // [1] initial rate (requests per second)
        5,                   // [2] min rate (don't go below this)
        20,                  // [3] max rate (don't exceed this)
        500*time.Millisecond, // [4] target latency (adjust to maintain this)
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

### What's happening?

1. **Initial rate** - Start with 10 requests per second.
2. **Minimum rate** - Don't drop below 5 req/s even if latency is high.
3. **Maximum rate** - Don't exceed 20 req/s even if latency is low.
4. **Target latency** - The rate limiter adjusts to maintain ~500ms per request.

> [!TIP]
> Adaptive rate limiting is ideal for high-volume applications. It automatically finds the optimal rate based on current conditions.

> [!WARNING]
> Adaptive rate limiting needs time to "warm up" - it starts at the initial rate and adjusts based on actual response times. Don't set the initial rate too high.

### Exponential Backoff for Rate Limits

```go
func rateLimitBackoff(attempt int) time.Duration {
    // Implement exponential backoff with jitter for rate limits
    // Start at 1 second, double each time, cap at 5 minutes
    baseDelay := time.Second
    maxDelay := 5 * time.Minute

    delay := baseDelay * time.Duration(1<<uint(attempt))     // [1] Exponential: 1s, 2s, 4s, 8s...
    if delay > maxDelay {                                    // [2] Cap at max
        delay = maxDelay
    }

    // Add jitter (±25%)
    jitter := delay / 4                                      // [3] Calculate jitter range
    delay += time.Duration(float64(jitter) * (2.0*rand.Float64() - 1.0)) // [4] Add random variation

    return delay
}

func retryWithRateLimitBackoff(builder *wormhole.TextRequestBuilder) (*wormhole.TextResponse, error) {
    maxAttempts := 5                                         // [5] Maximum retry attempts

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

### What's happening?

1. **Exponential calculation** - Bit shift produces powers of 2: 1, 2, 4, 8, 16...
2. **Max cap** - Don't wait longer than 5 minutes between retries.
3. **Jitter range** - Calculate 25% of delay as the jitter range.
4. **Random variation** - Add or subtract up to 25% randomly to prevent synchronized retries.
5. **Max attempts** - Give up after 5 total attempts.

> [!TIP]
> Jitter prevents the "thundering herd" problem where multiple clients retry simultaneously, overwhelming the recovering service.

> [!WARNING]
> Rate limiting backoff should be more aggressive than general retry backoff. Rate limits indicate the system is overloaded - be conservative.

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
    if err := builder.Validate(); err != nil {            // [1] Validate builder configuration
        if vErr, ok := types.AsValidationError(err); ok {  // [2] Extract validation details
            log.Printf("Field '%s' failed validation: %s", vErr.Field, vErr.Message)
            log.Printf("Constraint: %s", vErr.Constraint)  // [3] What constraint was violated
            log.Printf("Value: %v", vErr.Value)            // [4] The invalid value
        }
        return
    }

    resp, err := builder.Generate()
    _ = resp
    _ = err
}
```

### What's happening?

1. **Validation call** - `Validate()` checks the builder configuration before making the API call.
2. **Error extraction** - `AsValidationError` converts to the validation error type.
3. **Constraint** - The type of constraint violated (required, range, minLength, etc.).
4. **Value** - The actual value that failed validation.

> [!TIP]
> Validate early and fail fast. Catch configuration errors before making API calls to save time and quota.

> [!WARNING]
> `Validate()` only checks client-side constraints. The provider may still reject requests for reasons not detectable client-side (e.g., content policy violations).

### Multiple Field Validation

```go
func multipleFieldValidation(model string, temperature float64, messages []string) error {
    var errs types.ValidationErrors                       // [1] Collect multiple validation errors

    // Validate model
    if model == "" {
        errs.Add("model", "required", model, "model is required") // [2] Add model error
    }

    // Validate temperature
    if temperature < 0 || temperature > 2 {
        errs.Add("temperature", "range", temperature, "must be between 0.0 and 2.0") // [3] Add temp error
    }

    // Validate messages
    if len(messages) == 0 {
        errs.Add("messages", "required", messages, "at least one message is required") // [4] Add messages error
    }

    // Check if any validation failed
    if errs.HasErrors() {                                // [5] Check if collection has errors
        log.Printf("Validation failed for fields: %v", errs.Fields()) // [6] List affected fields
        return errs.Error()                              // [7] Return combined error message
    }

    return nil
}
```

### What's happening?

1. **Error collection** - `ValidationErrors` accumulates multiple validation problems.
2. **Model validation** - Check if model is provided.
3. **Temperature validation** - Check if temperature is in valid range (0-2 for most models).
4. **Messages validation** - Check if at least one message is provided.
5. **Has errors** - Returns `true` if any errors were added to the collection.
6. **Fields list** - Returns the names of all fields that failed validation.
7. **Combined error** - `Error()` returns a formatted message listing all validation failures.

> [!TIP]
> Collect all validation errors before returning. This allows the user to fix all issues at once rather than one at a time through iterative attempts.

> [!WARNING]
> Some validation constraints are model-specific. For example, GPT-5 requires `temperature = 1.0`. The SDK handles this automatically, but custom validation should check model constraints.

### Handling Constraint Violations

```go
func handleConstraintError(err error) {
    // Check for model constraint errors
    if constraintErr, ok := err.(*types.ModelConstraintError); ok { // [1] Type assert to constraint error
        log.Printf("Constraint '%s' violated for model '%s'", constraintErr.Constraint, constraintErr.Model)
        log.Printf("Expected: %v", constraintErr.Expected)        // [2] What the model requires
        log.Printf("Actual: %v", constraintErr.Actual)            // [3] What was provided

        // Handle specific constraints
        switch constraintErr.Constraint {                         // [4] Match constraint type
        case "max_tokens":
            log.Printf("Reduce max_tokens from %v to %v", constraintErr.Actual, constraintErr.Expected)
        case "temperature":
            log.Printf("Adjust temperature from %v to be within %v", constraintErr.Actual, constraintErr.Expected)
        }
    }
}
```

### What's happening?

1. **Type assertion** - Convert the error to `ModelConstraintError` to access constraint details.
2. **Expected value** - What the model requires (e.g., `temperature = 1.0` for GPT-5).
3. **Actual value** - What was provided in the request.
4. **Constraint matching** - Different constraints need different handling.

> [!TIP]
> The SDK automatically applies model constraints where possible. Constraint errors typically occur when explicitly overriding a constraint that the SDK would normally handle.

> [!WARNING]
> Model constraints are provider-specific and can change. Always check the provider's latest documentation for model-specific requirements.

---

## Circuit Breaker Errors

Circuit breakers prevent cascading failures by stopping requests to failing providers.

### Handling Circuit Breaker States

```go
func handleCircuitBreaker(err error) {
    if wormholeErr, ok := types.AsWormholeError(err); ok {
        // Check if error is due to open circuit breaker
        if errors.Is(err, types.ErrCircuitOpen) {            // [1] Detect open circuit
            log.Println("Circuit breaker is OPEN. Requests are being blocked.")

            // Wait for circuit to enter half-open state
            // or switch to a different provider
            return
        }

        // Check for middleware errors
        if types.IsMiddlewareError(err) {                     // [2] Detect middleware errors
            log.Printf("Middleware error: %s", wormholeErr.Message)

            // The circuit breaker may be in half-open or closed state
            // Implement fallback logic here
        }
    }
}
```

### What's happening?

1. **Circuit open check** - `errors.Is()` compares against `ErrCircuitOpen` sentinel.
2. **Middleware error check** - Circuit breaker errors are a type of middleware error.

> [!TIP]
> Circuit breakers have three states: CLOSED (normal), OPEN (blocking requests), and HALF-OPEN (testing if recovery occurred).

> [!WARNING]
> When a circuit is OPEN, requests fail immediately without attempting the provider. Implement fallback logic to switch to an alternative provider or cached responses.

### Fallback Provider Strategy

```go
func generateWithFallback(client *wormhole.Client, model string, prompt string) (*wormhole.TextResponse, error) {
    // Try primary provider
    resp, err := client.TextRequest().
        WithModel(model).
        WithMessage("user", prompt).
        Generate()

    if err == nil {
        return resp, nil                                // [1] Primary success
    }

    // Check if circuit breaker is open
    if errors.Is(err, types.ErrCircuitOpen) {           // [2] Detect circuit failure
        log.Println("Primary provider circuit is open. Trying fallback...")

        // Switch to a different provider/model
        resp, err = client.TextRequest().
            WithModel("gemini-2.5-flash").               // [3] Fallback model
            WithMessage("user", prompt).
            Generate()

        if err == nil {
            return resp, nil                            // [4] Fallback success
        }
    }

    return nil, err                                    // [5] Both failed
}
```

### What's happening?

1. **Primary attempt** - Try the primary model first.
2. **Circuit detection** - Check if the failure was due to an open circuit breaker.
3. **Fallback model** - Switch to a different provider/model that's likely healthy.
4. **Fallback success** - Return the fallback response.
5. **Both failed** - If fallback also fails, return the error.

> [!TIP]
> Implement fallback hierarchies: primary -> secondary -> tertiary -> cached response. This ensures service continuity even when multiple providers fail.

> [!WARNING]
> Fallback models may have different capabilities or pricing. Ensure your application can handle responses from different models (different response formats, token limits, etc.).

### Waiting for Circuit Recovery

```go
func waitForCircuitRecovery(ctx context.Context) error {
    ticker := time.NewTicker(10 * time.Second)              // [1] Check every 10 seconds
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():                                  // [2] Context cancelled?
            return ctx.Err()
        case <-ticker.C:                                    // [3] Time to check
            // Try a test request to see if circuit has recovered
            err := attemptTestRequest()
            if err == nil {
                log.Println("Circuit has recovered!")
                return nil
            }

            if !errors.Is(err, types.ErrCircuitOpen) {      // [4] Different error?
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

### What's happening?

1. **Ticker** - Creates a timer that fires every 10 seconds.
2. **Context check** - Allows external cancellation of the wait.
3. **Ticker channel** - Receives a value every 10 seconds.
4. **Error check** - If we get a non-circuit error, the circuit may be transitioning states.

> [!TIP]
> Use lightweight test requests for circuit recovery checks. You don't want to add load to a recovering system.

> [!WARNING]
> Don't wait indefinitely for circuit recovery. Implement a timeout and have a fallback strategy (cached responses, degraded service, etc.) for when recovery takes too long.

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
