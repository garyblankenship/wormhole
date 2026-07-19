package types

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// IsAuthError checks if an error is authentication-related (invalid key, missing key).
// Use this to detect when credentials need to be checked or refreshed.
//
// Example:
//
//	if types.IsAuthError(err) {
//	    return fmt.Errorf("check your API key: %w", err)
//	}
func IsAuthError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeAuth
	}
	return false
}

// IsRateLimitError checks if an error is due to rate limiting.
// When true, you should back off and retry after a delay.
//
// Example:
//
//	if types.IsRateLimitError(err) {
//	    time.Sleep(types.GetRetryAfter(err))
//	    // retry request
//	}
func IsRateLimitError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeRateLimit
	}
	return false
}

// IsModelError checks if an error is model-related (not found, not supported).
// Use this to detect invalid model names or provider capability mismatches.
func IsModelError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeModel
	}
	return false
}

// IsProviderConfigError checks if an error is provider configuration-related
// (not configured, constraint violation). For general provider errors, use IsProviderError.
func IsProviderConfigError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeProvider
	}
	return false
}

// IsNetworkError checks if an error is network-related (connection failed, service unavailable).
// Network errors are typically retryable after a delay.
func IsNetworkError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeNetwork
	}
	return false
}

// IsTimeoutError checks if an error is a timeout.
func IsTimeoutError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeTimeout
	}
	return false
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeValidation
	}
	return false
}

// IsMiddlewareError checks if an error is middleware-related.
func IsMiddlewareError(err error) bool {
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeMiddleware
	}
	return false
}

// GetRetryAfter returns a suggested retry delay for retryable errors.
// Returns 0 if the error is not retryable or has no retry hint.
//
// The delay is based on the error type:
//   - Rate limit errors: 30 seconds (provider-specific may vary)
//   - Network errors: 5 seconds
//   - Timeout errors: 10 seconds
//   - Other retryable: 1 second
func GetRetryAfter(err error) time.Duration {
	wormholeErr, ok := AsWormholeError(err)
	if !ok || !wormholeErr.IsRetryable() {
		return 0
	}

	// Prefer an explicit provider-supplied hint over code-based defaults.
	if wormholeErr.RetryAfter > 0 {
		return wormholeErr.RetryAfter
	}

	switch wormholeErr.Code {
	case ErrorCodeRateLimit:
		return 30 * time.Second
	case ErrorCodeNetwork:
		return 5 * time.Second
	case ErrorCodeTimeout:
		return 10 * time.Second
	default:
		return 1 * time.Second
	}
}

// ParseRetryAfterHeader extracts a normalized retry delay from provider response
// headers, returning 0 when no usable hint is present. It checks Retry-After first
// (integer seconds or HTTP-date), then x-ratelimit-reset-requests (integer/float)
// seconds or a Go-style duration such as "1m26.4s", "205ms", "2h". Header
// lookups are case-insensitive via http.Header.Get canonicalization.
func ParseRetryAfterHeader(headers http.Header, now time.Time) time.Duration {
	if v := headers.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			if secs > 0 {
				return time.Duration(secs) * time.Second
			}
		} else if t, err := http.ParseTime(v); err == nil {
			if d := t.Sub(now); d > 0 {
				return d
			}
		}
	}

	if v := headers.Get("X-RateLimit-Reset-Requests"); v != "" {
		if d := parseResetDuration(v); d > 0 {
			return d
		}
	}

	return 0
}

// parseResetDuration parses a rate-limit reset value expressed either as a
// Go-style compact duration ("1m26.4s", "205ms", "2h") or as bare integer/float
// seconds ("13.5"). Returns 0 when unparseable or non-positive.
func parseResetDuration(v string) time.Duration {
	if d, err := time.ParseDuration(v); err == nil {
		if d > 0 {
			return d
		}
		return 0
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
		return time.Duration(f * float64(time.Second))
	}
	return 0
}

// Errorf creates a wrapped error with formatted message
// Usage: types.Errorf("marshal request body", err)
// Result: "failed to marshal request body: <original error>"
func Errorf(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s: %w", operation, err)
}
