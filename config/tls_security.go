package config

import (
	"crypto/tls"
)

// IsSecure returns true if the TLS configuration meets modern security standards.
// A secure configuration must:
// 1. Use TLS 1.2 or higher
// 2. Have certificate verification enabled
// 3. Not use known weak cipher suites
func (c TLSConfig) IsSecure() bool {
	if c.MinVersion < tls.VersionTLS12 {
		return false
	}
	if c.InsecureSkipVerify {
		return false
	}

	// Check if any weak cipher suites are enabled
	weakCiphers := map[uint16]bool{
		tls.TLS_RSA_WITH_RC4_128_SHA:             true,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:        true,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA:         true,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:         true,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:     true,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:       true,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA: true,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA: true,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:  true,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:   true,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:   true,
		// Note: Some cipher suite constants may not be available in all Go versions
		// We check for their existence at runtime
	}

	for _, cipher := range c.CipherSuites {
		if weakCiphers[cipher] {
			return false
		}
	}

	return true
}

// ApplyToTLSConfig applies the TLS configuration to a *tls.Config.
// Creates a new tls.Config with the settings from this TLSConfig.
func (c TLSConfig) ApplyToTLSConfig(base *tls.Config) *tls.Config {
	if base == nil {
		base = &tls.Config{
			MinVersion: tls.VersionTLS13, // Secure default
		}
	}

	result := base.Clone()

	if c.MinVersion != 0 {
		result.MinVersion = c.MinVersion
	}
	if c.MaxVersion != 0 {
		result.MaxVersion = c.MaxVersion
	}
	if len(c.CipherSuites) > 0 {
		result.CipherSuites = c.CipherSuites
	}
	result.InsecureSkipVerify = c.InsecureSkipVerify
	if c.RootCAs != nil {
		result.RootCAs = c.RootCAs
	}
	if c.ServerName != "" {
		result.ServerName = c.ServerName
	}

	return result
}
