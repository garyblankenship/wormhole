package types

import (
	"net/http"
	"strings"
)

// ErrorClass is a stable operational class for provider and middleware errors.
type ErrorClass string

const (
	ErrorClassTransient ErrorClass = "transient"
	ErrorClassRateLimit ErrorClass = "rate_limit"
	ErrorClassQuota     ErrorClass = "quota"
	ErrorClassAuth      ErrorClass = "auth"
	ErrorClassConfig    ErrorClass = "config"
	ErrorClassTimeout   ErrorClass = "timeout"
	ErrorClassNetwork   ErrorClass = "network"
	ErrorClassUnknown   ErrorClass = "unknown"
)

// ClassifyError maps an error to its provider-health impact class.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassUnknown
	}
	if wormholeErr, ok := AsWormholeError(err); ok {
		// A quota/exhaustion signal in the message or details outranks the raw
		// status code: an OpenAI 429 "insufficient_quota" or Gemini
		// "RESOURCE_EXHAUSTED" is a quota cap (non-retryable), not a plain
		// rate-limit, yet both arrive as HTTP 429. Check before the status map
		// so the 429->rate_limit branch does not mask quota.
		if containsAny(wormholeErr.Message+" "+wormholeErr.Details, "quota", "exhausted") {
			return ErrorClassQuota
		}
		if class, ok := ClassifyStatusCode(wormholeErr.StatusCode); ok {
			return class
		}
		switch wormholeErr.Code {
		case ErrorCodeAuth:
			return ErrorClassAuth
		case ErrorCodeRateLimit:
			if !wormholeErr.Retryable || containsAny(wormholeErr.Message+" "+wormholeErr.Details, "quota", "exhausted") {
				return ErrorClassQuota
			}
			return ErrorClassRateLimit
		case ErrorCodeRequest, ErrorCodeModel, ErrorCodeValidation:
			return ErrorClassConfig
		case ErrorCodeTimeout:
			return ErrorClassTimeout
		case ErrorCodeNetwork:
			return ErrorClassNetwork
		case ErrorCodeProvider:
			if !wormholeErr.Retryable {
				return ErrorClassConfig
			}
			return ErrorClassTransient
		default:
			return ErrorClassUnknown
		}
	}

	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg, "rate limit", "too many requests", "http 429"):
		return ErrorClassRateLimit
	case strings.Contains(msg, "quota"):
		return ErrorClassQuota
	case containsAny(msg, "unauthorized", "forbidden", "invalid api key", "auth"):
		return ErrorClassAuth
	case containsAny(msg, "not configured", "invalid request", "model not", "validation"):
		return ErrorClassConfig
	case containsAny(msg, "timeout", "deadline exceeded"):
		return ErrorClassTimeout
	case containsAny(msg, "connection refused", "connection reset", "network"):
		return ErrorClassNetwork
	default:
		return ErrorClassTransient
	}
}

// ClassifyStatusCode maps HTTP status codes to provider-health error classes.
func ClassifyStatusCode(statusCode int) (ErrorClass, bool) {
	switch statusCode {
	case http.StatusTooManyRequests:
		return ErrorClassRateLimit, true
	case http.StatusUnauthorized:
		return ErrorClassAuth, true
	case http.StatusForbidden:
		return ErrorClassQuota, true
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
		return ErrorClassConfig, true
	default:
		return ErrorClassTransient, false
	}
}

// OpensProviderCircuit returns true when the class should immediately cool down
// a provider instead of waiting for repeated generic failures.
func (c ErrorClass) OpensProviderCircuit() bool {
	switch c {
	case ErrorClassRateLimit, ErrorClassQuota, ErrorClassAuth, ErrorClassConfig:
		return true
	default:
		return false
	}
}

func containsAny(s string, needles ...string) bool {
	s = strings.ToLower(s)
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
