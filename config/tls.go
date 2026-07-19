package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
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

	// AllowInsecure permits intentionally insecure TLS settings such as
	// InsecureSkipVerify or TLS versions below 1.2. Keep false for provider
	// config loaded from generic parameters unless the caller explicitly opted in.
	AllowInsecure bool
}

// DefaultTLSConfig returns a secure TLS configuration with modern defaults:
// - Minimum TLS 1.3 (blocks TLS 1.2 and below for maximum security)
// - Modern cipher suites only (excludes weak ciphers like RC4, 3DES, CBC)
// - Certificate verification enabled
// - System root CAs
func DefaultTLSConfig() TLSConfig {
	return TLSConfig{
		MinVersion:         tls.VersionTLS13, // TLS 1.3 for modern security
		MaxVersion:         0,                // No maximum, allows TLS 1.3
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
