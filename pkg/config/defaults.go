// Package config provides centralized configuration defaults for the Wormhole SDK.
// 
// ⚠️  CRITICAL: THIS IS THE ONLY FILE WITH HARDCODED DEFAULT VALUES ⚠️
// All other files MUST use the Get*() functions from this file only.
// These defaults exist ONLY as environment variable fallbacks.
package config

import (
	"os"
	"strconv"
	"time"
)

// HTTP Client Configuration Defaults
const (
	// UnlimitedTimeout represents no timeout (0 duration).
	// Used internally when unlimited timeout is configured.
	UnlimitedTimeout = 0
)

// ONLY_HARDCODED_DEFAULTS_IN_CODEBASE - Environment variable fallbacks
// ⚠️  These are the ONLY hardcoded values allowed anywhere in the codebase ⚠️
const (
	// FALLBACK_DefaultHTTPTimeout - Used only if WORMHOLE_DEFAULT_TIMEOUT not set
	FALLBACK_DefaultHTTPTimeout = 300 * time.Second // 5 minutes

	// FALLBACK_DefaultMaxRetries - Used only if WORMHOLE_MAX_RETRIES not set  
	FALLBACK_DefaultMaxRetries = 3

	// FALLBACK_DefaultInitialDelay - Used only if WORMHOLE_INITIAL_RETRY_DELAY not set
	FALLBACK_DefaultInitialDelay = 1 * time.Second

	// FALLBACK_DefaultMaxDelay - Used only if WORMHOLE_MAX_RETRY_DELAY not set
	FALLBACK_DefaultMaxDelay = 30 * time.Second

	// DefaultBackoffMultiple is the multiplier for exponential backoff.
	// Each retry delay is multiplied by this factor.
	DefaultBackoffMultiple = 2.0

	// DefaultJitterEnabled controls whether random jitter is added to retry delays.
	DefaultJitterEnabled = true

	// FALLBACK_DefaultCircuitBreakerTimeout - Used only if WORMHOLE_CIRCUIT_BREAKER_TIMEOUT not set
	FALLBACK_DefaultCircuitBreakerTimeout = 30 * time.Second

	// FALLBACK_DefaultHealthCheckInterval - Used only if WORMHOLE_HEALTH_CHECK_INTERVAL not set
	FALLBACK_DefaultHealthCheckInterval = 30 * time.Second

	// FALLBACK_DefaultLoadBalancerHealthInterval - Used only if env var not set
	FALLBACK_DefaultLoadBalancerHealthInterval = 30 * time.Second
)

// Backwards compatibility aliases - DO NOT USE DIRECTLY
// Use Get*() functions instead
const (
	DefaultMaxRetries                     = FALLBACK_DefaultMaxRetries
	DefaultInitialDelay                   = FALLBACK_DefaultInitialDelay  
	DefaultMaxDelay                       = FALLBACK_DefaultMaxDelay
	DefaultLoadBalancerHealthInterval     = FALLBACK_DefaultLoadBalancerHealthInterval
)

// Runtime Configuration Support
// These functions require environment variables - no hardcoded fallbacks

// GetDefaultHTTPTimeout returns the HTTP timeout with environment variable override.
// Environment variable: WORMHOLE_DEFAULT_TIMEOUT (optional, duration string like "45s", "5m")
// Falls back to FALLBACK_DefaultHTTPTimeout if not set
func GetDefaultHTTPTimeout() time.Duration {
	if env := os.Getenv("WORMHOLE_DEFAULT_TIMEOUT"); env != "" {
		if duration, err := time.ParseDuration(env); err == nil {
			return duration
		}
	}
	return FALLBACK_DefaultHTTPTimeout
}

// GetDefaultMaxRetries returns the max retries with environment variable override.
// Environment variable: WORMHOLE_MAX_RETRIES (optional, integer)
// Falls back to FALLBACK_DefaultMaxRetries if not set
func GetDefaultMaxRetries() int {
	if env := os.Getenv("WORMHOLE_MAX_RETRIES"); env != "" {
		if retries, err := strconv.Atoi(env); err == nil && retries >= 0 {
			return retries
		}
	}
	return FALLBACK_DefaultMaxRetries
}

// GetDefaultMaxDelay returns the max retry delay with environment variable override.
// Environment variable: WORMHOLE_MAX_RETRY_DELAY (optional, duration string like "45s", "2m")
// Falls back to FALLBACK_DefaultMaxDelay if not set
func GetDefaultMaxDelay() time.Duration {
	if env := os.Getenv("WORMHOLE_MAX_RETRY_DELAY"); env != "" {
		if duration, err := time.ParseDuration(env); err == nil {
			return duration
		}
	}
	return FALLBACK_DefaultMaxDelay
}

// GetDefaultInitialDelay returns the initial retry delay with environment variable override.
// Environment variable: WORMHOLE_INITIAL_RETRY_DELAY (optional, duration string like "1s", "500ms")
// Falls back to FALLBACK_DefaultInitialDelay if not set
func GetDefaultInitialDelay() time.Duration {
	if env := os.Getenv("WORMHOLE_INITIAL_RETRY_DELAY"); env != "" {
		if duration, err := time.ParseDuration(env); err == nil {
			return duration
		}
	}
	return FALLBACK_DefaultInitialDelay
}

// GetDefaultCircuitBreakerTimeout returns the circuit breaker timeout with environment variable override.
// Environment variable: WORMHOLE_CIRCUIT_BREAKER_TIMEOUT (optional, duration string)
// Falls back to FALLBACK_DefaultCircuitBreakerTimeout if not set
func GetDefaultCircuitBreakerTimeout() time.Duration {
	if env := os.Getenv("WORMHOLE_CIRCUIT_BREAKER_TIMEOUT"); env != "" {
		if duration, err := time.ParseDuration(env); err == nil {
			return duration
		}
	}
	return FALLBACK_DefaultCircuitBreakerTimeout
}

// GetDefaultHealthCheckInterval returns the health check interval with environment variable override.
// Environment variable: WORMHOLE_HEALTH_CHECK_INTERVAL (optional, duration string)
// Falls back to FALLBACK_DefaultHealthCheckInterval if not set
func GetDefaultHealthCheckInterval() time.Duration {
	if env := os.Getenv("WORMHOLE_HEALTH_CHECK_INTERVAL"); env != "" {
		if duration, err := time.ParseDuration(env); err == nil {
			return duration
		}
	}
	return FALLBACK_DefaultHealthCheckInterval
}