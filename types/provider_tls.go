package types

// TLSConfigParamKey is the key used to store TLS configuration in Params map.
const TLSConfigParamKey = "tls_config"

// WithTLSConfigParam adds TLS configuration parameters to the provider config.
// The TLS configuration is stored as a map[string]any in the Params field.
// This allows type-safe extraction by providers that understand TLS configuration.
//
// Example TLS parameters:
//   - "min_version": uint16 (e.g., tls.VersionTLS12)
//   - "cipher_suites": []uint16
//   - "insecure_skip_verify": bool
//   - "server_name": string
func (c ProviderConfig) WithTLSConfigParam(key string, value any) ProviderConfig {
	if c.Params == nil {
		c.Params = make(map[string]any)
	}

	// Get or create TLS config map
	var tlsConfig map[string]any
	if existing, ok := c.Params[TLSConfigParamKey]; ok {
		if configMap, ok := existing.(map[string]any); ok {
			tlsConfig = configMap
		} else {
			tlsConfig = make(map[string]any)
		}
	} else {
		tlsConfig = make(map[string]any)
	}

	// Set the TLS parameter
	tlsConfig[key] = value
	c.Params[TLSConfigParamKey] = tlsConfig

	return c
}

// WithInsecureTLS enables insecure TLS configuration for legacy compatibility.
// WARNING: This should only be used for testing or legacy servers.
// The configuration will allow TLS 1.0 and disable certificate verification.
func (c ProviderConfig) WithInsecureTLS(skipVerify bool) ProviderConfig {
	return c.WithTLSConfigParam("min_version", uint16(0x0301)). // TLS 1.0
									WithTLSConfigParam("insecure_skip_verify", skipVerify).
									WithTLSConfigParam("allow_insecure", true)
}

// HasTLSConfig returns true if the provider config contains TLS configuration.
func (c ProviderConfig) HasTLSConfig() bool {
	if c.Params == nil {
		return false
	}
	_, ok := c.Params[TLSConfigParamKey]
	return ok
}
