package middleware

import "github.com/garyblankenship/wormhole/v3/types"

type circuitErrorClass int

const (
	circuitErrorTransient circuitErrorClass = iota
	circuitErrorNeutral
	circuitErrorRateLimit
	circuitErrorQuota
	circuitErrorAuth
	circuitErrorConfig
)

func classifyCircuitError(err error) circuitErrorClass {
	if _, ok := types.AsIncompleteGenerationError(err); ok {
		return circuitErrorNeutral
	}
	switch types.ClassifyError(err) {
	case types.ErrorClassRateLimit:
		return circuitErrorRateLimit
	case types.ErrorClassQuota:
		return circuitErrorQuota
	case types.ErrorClassAuth:
		return circuitErrorAuth
	case types.ErrorClassConfig:
		return circuitErrorConfig
	default:
		return circuitErrorTransient
	}
}

func circuitFailureWeight(err error, threshold int) int {
	if threshold < 1 {
		threshold = 1
	}
	switch classifyCircuitError(err) {
	case circuitErrorNeutral:
		return 0
	case circuitErrorRateLimit, circuitErrorQuota, circuitErrorAuth, circuitErrorConfig:
		return threshold
	default:
		return 1
	}
}
