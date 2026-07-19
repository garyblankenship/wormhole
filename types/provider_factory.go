package types

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
	// Check if it's a WormholeError with ErrorCodeProvider
	if wormholeErr, ok := AsWormholeError(err); ok {
		return wormholeErr.Code == ErrorCodeProvider
	}
	// Fallback: check error message for backward compatibility
	return err.Error() != "" &&
		(len(err.Error()) > 20 &&
			err.Error()[len(err.Error())-20:] == "does not support")
}
