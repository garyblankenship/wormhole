package providers

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
)

// NewSecureHTTPClient creates a new HTTP client with secure TLS configuration
// and optimized transport settings. This standalone form does NOT share a
// transport cache; each call builds a fresh transport. For cache-backed reuse,
// HTTPClientWrapper routes through its instance-scoped *TransportCache.
func NewSecureHTTPClient(timeout time.Duration, tlsConfig *config.TLSConfig, transportConfig *HTTPTransportConfig, baseURL string) *http.Client {
	return buildSecureHTTPClient(timeout, tlsConfig, transportConfig, baseURL, nil)
}

// newSecureHTTPClient builds a client whose transport is cached in tc, keyed by
// transport-config fingerprint + base URL. Reuses an existing transport on hit.
func (tc *TransportCache) newSecureHTTPClient(timeout time.Duration, tlsConfig *config.TLSConfig, transportConfig *HTTPTransportConfig, baseURL string) *http.Client {
	return buildSecureHTTPClient(timeout, tlsConfig, transportConfig, baseURL, tc)
}

// buildSecureHTTPClient is the shared construction path. When tc is nil the
// transport is built fresh and uncached; otherwise it is looked up / stored in tc.
func buildSecureHTTPClient(timeout time.Duration, tlsConfig *config.TLSConfig, transportConfig *HTTPTransportConfig, baseURL string, tc *TransportCache) *http.Client {
	// Use default TLS config if not provided
	if tlsConfig == nil {
		defaultTLS := config.DefaultTLSConfig()
		tlsConfig = &defaultTLS
	}
	tlsConfig = approvedTLSConfig(tlsConfig)

	// Use default transport config if not provided
	if transportConfig == nil {
		defaultTransport := DefaultHTTPTransportConfig()
		transportConfig = &defaultTransport
	}
	transportCopy := *transportConfig
	transportCopy.TLSConfig = tlsConfig
	transportConfig = &transportCopy

	// Create TLS config
	var tlsClientConfig *tls.Config
	if tlsConfig != nil {
		tlsClientConfig = tlsConfig.ApplyToTLSConfig(nil)
	}

	if tc != nil {
		// Compute cache key based on transport configuration and base URL
		key := transportConfig.CacheKey(baseURL)
		if transport, ok := tc.get(key); ok {
			return &http.Client{Transport: transport, Timeout: timeout}
		}
		tc.recordMiss()
		transport := newTransportFromConfig(transportConfig, tlsClientConfig)
		tc.set(key, transport)
		return &http.Client{Transport: transport, Timeout: timeout}
	}

	transport := newTransportFromConfig(transportConfig, tlsClientConfig)
	return &http.Client{Transport: transport, Timeout: timeout}
}

func approvedTLSConfig(tlsConfig *config.TLSConfig) *config.TLSConfig {
	if tlsConfig == nil || tlsConfig.IsSecure() || tlsConfig.AllowInsecure {
		return tlsConfig
	}
	floored := *tlsConfig
	defaultTLS := config.DefaultTLSConfig()

	if floored.MinVersion < defaultTLS.MinVersion {
		floored.MinVersion = defaultTLS.MinVersion
	}
	if floored.MaxVersion != 0 && floored.MaxVersion < floored.MinVersion {
		floored.MaxVersion = floored.MinVersion
	}
	floored.InsecureSkipVerify = false
	if len(floored.CipherSuites) == 0 {
		floored.CipherSuites = defaultTLS.CipherSuites
	}
	if floored.HandshakeTimeout == 0 {
		floored.HandshakeTimeout = defaultTLS.HandshakeTimeout
	}

	return &floored
}

// newTransportFromConfig constructs an *http.Transport from the given config.
func newTransportFromConfig(transportConfig *HTTPTransportConfig, tlsClientConfig *tls.Config) *http.Transport {
	return &http.Transport{
		Proxy: transportConfig.Proxy,
		DialContext: (&net.Dialer{
			Timeout:   transportConfig.DialTimeout,
			KeepAlive: transportConfig.DialKeepAlive,
		}).DialContext,
		TLSHandshakeTimeout:   transportConfig.TLSHandshakeTimeout,
		ExpectContinueTimeout: transportConfig.ExpectContinueTimeout,
		ResponseHeaderTimeout: transportConfig.ResponseHeaderTimeout,
		MaxIdleConns:          transportConfig.MaxIdleConns,
		MaxIdleConnsPerHost:   transportConfig.MaxIdleConnsPerHost,
		MaxConnsPerHost:       transportConfig.MaxConnsPerHost,
		IdleConnTimeout:       transportConfig.IdleConnTimeout,
		TLSClientConfig:       tlsClientConfig,
		ForceAttemptHTTP2:     true, // Enable HTTP/2
	}
}

// NewInsecureHTTPClient creates an HTTP client with insecure TLS configuration
// for legacy compatibility.
//
// WARNING: This client is INSECURE and should only be used for:
//   - Testing with self-signed certificates
//   - Legacy servers that don't support TLS 1.2+
//   - Development environments
//
// The client allows:
//   - TLS 1.0 (vulnerable to POODLE, BEAST attacks)
//   - Weak cipher suites
//   - Optional certificate verification disabling
func NewInsecureHTTPClient(timeout time.Duration, skipVerify bool) *http.Client {
	tlsConfig := config.InsecureTLSConfig()
	if skipVerify {
		tlsConfig = tlsConfig.WithInsecureSkipVerify(true)
	}
	tlsConfig = tlsConfig.WithAllowInsecure(true)

	transportConfig := DefaultHTTPTransportConfig()
	transportConfig.TLSConfig = &tlsConfig

	return NewSecureHTTPClient(timeout, &tlsConfig, &transportConfig, "")
}
