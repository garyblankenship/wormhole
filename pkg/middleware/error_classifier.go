package middleware

import (
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

type circuitErrorClass int

const (
	circuitErrorTransient circuitErrorClass = iota
	circuitErrorRateLimit
	circuitErrorQuota
	circuitErrorAuth
	circuitErrorConfig
)

func classifyCircuitError(err error) circuitErrorClass {
	if wormholeErr, ok := types.AsWormholeError(err); ok {
		if class, ok := classifyStatusCode(wormholeErr.StatusCode); ok {
			return class
		}
		switch wormholeErr.Code {
		case types.ErrorCodeAuth:
			return circuitErrorAuth
		case types.ErrorCodeRateLimit:
			if !wormholeErr.Retryable || strings.Contains(strings.ToLower(wormholeErr.Message+" "+wormholeErr.Details), "quota") {
				return circuitErrorQuota
			}
			return circuitErrorRateLimit
		case types.ErrorCodeRequest, types.ErrorCodeModel, types.ErrorCodeValidation:
			return circuitErrorConfig
		case types.ErrorCodeProvider:
			if !wormholeErr.Retryable {
				return circuitErrorConfig
			}
			return circuitErrorTransient
		default:
			return circuitErrorTransient
		}
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "rate limit"), strings.Contains(msg, "too many requests"), strings.Contains(msg, "http 429"):
		return circuitErrorRateLimit
	case strings.Contains(msg, "quota"):
		return circuitErrorQuota
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "invalid api key"), strings.Contains(msg, "auth"):
		return circuitErrorAuth
	case strings.Contains(msg, "not configured"), strings.Contains(msg, "invalid request"), strings.Contains(msg, "model not"):
		return circuitErrorConfig
	default:
		return circuitErrorTransient
	}
}

func classifyStatusCode(statusCode int) (circuitErrorClass, bool) {
	switch statusCode {
	case http.StatusTooManyRequests:
		return circuitErrorRateLimit, true
	case http.StatusUnauthorized:
		return circuitErrorAuth, true
	case http.StatusForbidden:
		return circuitErrorQuota, true
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
		return circuitErrorConfig, true
	default:
		return circuitErrorTransient, false
	}
}

func circuitFailureWeight(err error, threshold int) int {
	if threshold < 1 {
		threshold = 1
	}
	switch classifyCircuitError(err) {
	case circuitErrorRateLimit, circuitErrorQuota, circuitErrorAuth, circuitErrorConfig:
		return threshold
	default:
		return 1
	}
}
