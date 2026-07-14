package providers

import (
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

// ExtractTLSConfigFromProviderConfig extracts TLS configuration from a ProviderConfig.
// If the ProviderConfig contains TLS parameters in the Params map, they are converted
// to a config.TLSConfig. Otherwise, returns nil (which will use default secure TLS).
func ExtractTLSConfigFromProviderConfig(providerConfig types.ProviderConfig) *config.TLSConfig {
	if !providerConfig.HasTLSConfig() {
		return nil
	}

	tlsParams, ok := providerConfig.Params[types.TLSConfigParamKey].(map[string]any)
	if !ok || len(tlsParams) == 0 {
		return nil
	}

	// Start with default secure configuration
	tlsConfig := config.DefaultTLSConfig()

	// Apply parameters from ProviderConfig
	for key, value := range tlsParams {
		switch key {
		case "min_version":
			if v, ok := tlsUint16(value); ok {
				tlsConfig.MinVersion = v
			}
		case "max_version":
			if v, ok := tlsUint16(value); ok {
				tlsConfig.MaxVersion = v
			}
		case "cipher_suites":
			if cipherSuites := tlsCipherSuites(value); len(cipherSuites) > 0 {
				tlsConfig.CipherSuites = cipherSuites
			}
		case "insecure_skip_verify":
			if v, ok := value.(bool); ok {
				tlsConfig.InsecureSkipVerify = v
			}
		case "allow_insecure":
			if v, ok := value.(bool); ok {
				tlsConfig.AllowInsecure = v
			}
		case "server_name":
			if v, ok := value.(string); ok {
				tlsConfig.ServerName = v
			}
		case "handshake_timeout":
			if v, ok := tlsDuration(value); ok {
				tlsConfig.HandshakeTimeout = v
			}
		}
	}

	return &tlsConfig
}

func tlsUint16(value any) (uint16, bool) {
	switch v := value.(type) {
	case float64:
		return uint16(v), true
	case uint16:
		return v, true
	default:
		return 0, false
	}
}

func tlsCipherSuites(value any) []uint16 {
	slice, ok := value.([]any)
	if !ok {
		return nil
	}
	cipherSuites := make([]uint16, 0, len(slice))
	for _, item := range slice {
		if v, ok := tlsUint16(item); ok {
			cipherSuites = append(cipherSuites, v)
		}
	}
	return cipherSuites
}

func tlsDuration(value any) (time.Duration, bool) {
	switch v := value.(type) {
	case float64:
		return time.Duration(v) * time.Second, true
	case time.Duration:
		return v, true
	default:
		return 0, false
	}
}
