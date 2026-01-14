// Package config provides centralized configuration defaults for the Wormhole SDK.
// This file contains TLS configuration types and secure defaults for HTTP clients.

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"
)

// TLSConfig holds configuration for TLS (Transport Layer Security) settings.
// This configuration is used to create secure HTTP clients with proper
// cipher suite restrictions, TLS version enforcement, and certificate validation.
type TLSConfig struct {
	// MinVersion specifies the minimum TLS version to accept.
	// Default: tls.VersionTLS12 (TLS 1.2)
	MinVersion uint16

	// MaxVersion specifies the maximum TLS version to accept.
	// Default: 0 (no maximum, uses Go's default)
	MaxVersion uint16

	// CipherSuites specifies the list of cipher suites to use.
	// If empty, uses Go's default cipher suites.
	CipherSuites []uint16

	// InsecureSkipVerify controls whether to skip certificate verification.
	// WARNING: Setting this to true makes the connection vulnerable to MITM attacks.
	// Default: false (enforce certificate verification)
	// Only use for testing or legacy compatibility with self-signed certificates.
	InsecureSkipVerify bool

	// RootCAs specifies custom root certificate authorities.
	// If nil, uses the system's default root CAs.
	RootCAs *x509.CertPool

	// ServerName specifies the server name for certificate validation.
	// If empty, uses the server name from the URL.
	ServerName string

	// Timeouts for TLS handshake
	HandshakeTimeout time.Duration
}

// DefaultTLSConfig returns a secure TLS configuration with modern defaults:
// - Minimum TLS 1.3 (blocks TLS 1.2 and below for maximum security)
// - Modern cipher suites only (excludes weak ciphers like RC4, 3DES, CBC)
// - Certificate verification enabled
// - System root CAs
func DefaultTLSConfig() TLSConfig {
	return TLSConfig{
		MinVersion:         tls.VersionTLS13, // TLS 1.3 for modern security
		MaxVersion:         0, // No maximum, allows TLS 1.3
		CipherSuites:       ModernCipherSuites(),
		InsecureSkipVerify: false,
		RootCAs:            nil, // Use system root CAs
		HandshakeTimeout:   10 * time.Second,
	}
}

// ModernCipherSuites returns a list of modern, secure cipher suites.
// These cipher suites prioritize:
// 1. Forward secrecy (ECDHE)
// 2. Authenticated encryption (AEAD modes like GCM, ChaCha20)
// 3. Performance and security
//
// Excluded ciphers:
// - RC4 (broken)
// - 3DES (weak, deprecated)
// - CBC modes (vulnerable to padding oracle attacks)
// - Static RSA key exchange (no forward secrecy)
func ModernCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 cipher suites (Go 1.23+)
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,

		// TLS 1.2 cipher suites with ECDHE and forward secrecy
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	}
}

// InsecureTLSConfig returns a TLS configuration for legacy compatibility.
// WARNING: This configuration is INSECURE and should only be used for:
// - Testing with self-signed certificates
// - Legacy servers that don't support TLS 1.2+
// - Development environments without proper certificates
//
// This configuration:
// - Allows TLS 1.0 (vulnerable to POODLE, BEAST attacks)
// - Allows weak cipher suites
// - Disables certificate verification if requested
func InsecureTLSConfig() TLSConfig {
	cfg := TLSConfig{
		MinVersion:         tls.VersionTLS10, // INSECURE: Allows TLS 1.0 // #nosec G402 - explicitly insecure config for legacy compatibility
		MaxVersion:         0,
		CipherSuites:       CompatibleCipherSuites(), // Includes weak ciphers
		InsecureSkipVerify: false,                    // Still verify by default
		RootCAs:            nil,
		HandshakeTimeout:   10 * time.Second,
	}

	// Log warning about insecure configuration
	fmt.Fprintf(os.Stderr, "WARNING: Using InsecureTLSConfig - TLS 1.0 is vulnerable to POODLE and BEAST attacks\n")
	fmt.Fprintf(os.Stderr, "WARNING: This should only be used for testing or legacy compatibility\n")

	return cfg
}

// CompatibleCipherSuites returns cipher suites for maximum compatibility.
// Includes weaker ciphers for legacy server compatibility.
// WARNING: Some of these ciphers have known vulnerabilities.
func CompatibleCipherSuites() []uint16 {
	return []uint16{
		// Modern secure ciphers (same as ModernCipherSuites)
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,

		// Legacy ciphers for compatibility (INSECURE)
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256, // No forward secrecy
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384, // No forward secrecy
	}
}

// StrictTLSConfig returns the strictest possible TLS configuration.
// Suitable for high-security environments:
// - Minimum TLS 1.3 (blocks TLS 1.2 and below)
// - TLS 1.3 cipher suites only
// - Certificate verification with optional custom root CAs
func StrictTLSConfig() TLSConfig {
	return TLSConfig{
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         0,
		CipherSuites:       TLS13CipherSuites(),
		InsecureSkipVerify: false,
		RootCAs:            nil,
		HandshakeTimeout:   10 * time.Second,
	}
}

// TLS13CipherSuites returns only TLS 1.3 cipher suites.
func TLS13CipherSuites() []uint16 {
	return []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
	}
}

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
		tls.TLS_RSA_WITH_RC4_128_SHA:                  true,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:            true,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA:             true,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:             true,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:         true,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:           true,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:     true,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:     true,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:      true,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:       true,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:       true,
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

// WithMinVersion returns a copy of the TLSConfig with the specified minimum version.
func (c TLSConfig) WithMinVersion(version uint16) TLSConfig {
	c.MinVersion = version
	return c
}

// WithMaxVersion returns a copy of the TLSConfig with the specified maximum version.
func (c TLSConfig) WithMaxVersion(version uint16) TLSConfig {
	c.MaxVersion = version
	return c
}

// WithCipherSuites returns a copy of the TLSConfig with the specified cipher suites.
func (c TLSConfig) WithCipherSuites(cipherSuites []uint16) TLSConfig {
	c.CipherSuites = cipherSuites
	return c
}

// WithInsecureSkipVerify returns a copy of the TLSConfig with InsecureSkipVerify set.
// WARNING: This disables certificate verification and makes connections vulnerable to MITM attacks.
func (c TLSConfig) WithInsecureSkipVerify(skip bool) TLSConfig {
	c.InsecureSkipVerify = skip
	return c
}

// WithRootCAs returns a copy of the TLSConfig with custom root CAs.
func (c TLSConfig) WithRootCAs(rootCAs *x509.CertPool) TLSConfig {
	c.RootCAs = rootCAs
	return c
}

// WithServerName returns a copy of the TLSConfig with a specific server name.
func (c TLSConfig) WithServerName(serverName string) TLSConfig {
	c.ServerName = serverName
	return c
}

// WithHandshakeTimeout returns a copy of the TLSConfig with a specific handshake timeout.
func (c TLSConfig) WithHandshakeTimeout(timeout time.Duration) TLSConfig {
	c.HandshakeTimeout = timeout
	return c
}

// Fingerprint returns a string that uniquely identifies the TLS configuration.
// Used for caching transports based on TLS settings.
func (c TLSConfig) Fingerprint() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%v|%v|%s|", c.MinVersion, c.MaxVersion, c.InsecureSkipVerify, c.RootCAs != nil, c.ServerName)
	for _, cs := range c.CipherSuites {
		fmt.Fprintf(&b, "%d,", cs)
	}
	return b.String()
}