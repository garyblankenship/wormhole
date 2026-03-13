package providers

import (
	"time"

	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
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
			if v, ok := value.(float64); ok {
				tlsConfig.MinVersion = uint16(v)
			} else if v, ok := value.(uint16); ok {
				tlsConfig.MinVersion = v
			}
		case "max_version":
			if v, ok := value.(float64); ok {
				tlsConfig.MaxVersion = uint16(v)
			} else if v, ok := value.(uint16); ok {
				tlsConfig.MaxVersion = v
			}
		case "cipher_suites":
			if slice, ok := value.([]any); ok {
				var cipherSuites []uint16
				for _, item := range slice {
					if v, ok := item.(float64); ok {
						cipherSuites = append(cipherSuites, uint16(v))
					} else if v, ok := item.(uint16); ok {
						cipherSuites = append(cipherSuites, v)
					}
				}
				if len(cipherSuites) > 0 {
					tlsConfig.CipherSuites = cipherSuites
				}
			}
		case "insecure_skip_verify":
			if v, ok := value.(bool); ok {
				tlsConfig.InsecureSkipVerify = v
			}
		case "server_name":
			if v, ok := value.(string); ok {
				tlsConfig.ServerName = v
			}
		case "handshake_timeout":
			if v, ok := value.(float64); ok {
				tlsConfig.HandshakeTimeout = time.Duration(v) * time.Second
			} else if v, ok := value.(time.Duration); ok {
				tlsConfig.HandshakeTimeout = v
			}
		}
	}

	return &tlsConfig
}
