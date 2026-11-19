package types

import "strings"

// ProviderFactory defines the function signature for creating a new provider instance.
// This enables dynamic provider registration without modifying core code.
type ProviderFactory func(config ProviderConfig) (Provider, error)

// Utility functions for capability checking - simplified since all providers now implement Provider interface
// These functions check if a method call would return a NotImplementedError
func IsMethodSupported(provider Provider, method string) bool {
	// This is a runtime check - we could enhance this by having providers expose their capabilities
	// For now, we rely on the runtime error to determine support
	return true // All providers implement all methods, some just return NotImplementedError
}

// Error checking utility - determines if an error indicates unsupported functionality
func IsNotSupportedError(err error) bool {
	if err == nil {
		return false
	}
	// Check if it's a non-retryable WormholeError with ErrorCodeProvider indicating unsupported functionality
	if wormholeErr, ok := AsWormholeError(err); ok {
		if wormholeErr.Code != ErrorCodeProvider || wormholeErr.Retryable {
			return false
		}
		return isNotSupportedMessage(wormholeErr.Message)
	}
	// Fallback: check error message for backward compatibility
	return isNotSupportedMessage(err.Error())
}

func isNotSupportedMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "does not support") ||
		strings.Contains(lower, "not implemented") ||
		strings.Contains(lower, "not supported")
}
