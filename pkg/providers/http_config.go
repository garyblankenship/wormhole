// Package providers provides HTTP client configuration with secure TLS defaults.
// This file contains utilities for creating secure HTTP clients with proper
// TLS configuration, connection pooling, and timeout management.

package providers

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// HTTPTransportConfig holds configuration for HTTP transport settings.
// This includes connection pooling, timeouts, and TLS configuration.
type HTTPTransportConfig struct {
	// TLSConfig specifies TLS settings for secure connections.
	// If nil, uses DefaultTLSConfig().
	TLSConfig *config.TLSConfig

	// Connection pooling settings
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration

	// Timeout settings
	DialTimeout           time.Duration
	DialKeepAlive         time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ResponseHeaderTimeout time.Duration

	// Proxy settings (optional)
	Proxy func(*http.Request) (*url.URL, error)
}

// DefaultHTTPTransportConfig returns a secure HTTP transport configuration
// with optimized connection pooling and secure TLS defaults.
func DefaultHTTPTransportConfig() HTTPTransportConfig {
	defaultTLS := config.DefaultTLSConfig()
	return HTTPTransportConfig{
		TLSConfig:             &defaultTLS,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       0, // No limit
		IdleConnTimeout:       90 * time.Second,
		DialTimeout:           30 * time.Second,
		DialKeepAlive:         30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 0, // No timeout
		Proxy:                 http.ProxyFromEnvironment,
	}
}

// Fingerprint returns a string that uniquely identifies the HTTP transport configuration.
// Used for caching transports based on configuration settings.
func (c HTTPTransportConfig) Fingerprint() string {
	var b strings.Builder
	if c.TLSConfig != nil {
		fmt.Fprintf(&b, "tls:%s|", c.TLSConfig.Fingerprint())
	} else {
		b.WriteString("tls:nil|")
	}
	fmt.Fprintf(&b, "maxidle:%d|maxidlehost:%d|maxconns:%d|idletimeout:%s|dialtimeout:%s|dialkeepalive:%s|tlshandshake:%s|expectcontinue:%s|responseheader:%s",
		c.MaxIdleConns, c.MaxIdleConnsPerHost, c.MaxConnsPerHost,
		c.IdleConnTimeout, c.DialTimeout, c.DialKeepAlive,
		c.TLSHandshakeTimeout, c.ExpectContinueTimeout, c.ResponseHeaderTimeout)
	fmt.Fprintf(&b, "|proxy:%s", proxyFingerprint(c.Proxy))
	return b.String()
}

func proxyFingerprint(proxy func(*http.Request) (*url.URL, error)) string {
	if proxy == nil {
		return "nil"
	}
	pc := reflect.ValueOf(proxy).Pointer()
	name := ""
	if fn := runtime.FuncForPC(pc); fn != nil {
		name = fn.Name()
	}
	return fmt.Sprintf("%s@%x", name, pc)
}

// CacheKey returns a cache key that includes both base URL and transport configuration fingerprint.
// This enables connection pooling across providers with the same base URL and identical transport configuration.
func (c HTTPTransportConfig) CacheKey(baseURL string) string {
	if baseURL == "" {
		return c.Fingerprint()
	}
	// Extract host from base URL for grouping
	host := extractHostFromBaseURL(baseURL)
	if host == "" {
		return c.Fingerprint()
	}
	return host + "|" + c.Fingerprint()
}

// extractHostFromBaseURL extracts the host (hostname:port) from a base URL.
// Returns empty string if parsing fails.
func extractHostFromBaseURL(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

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

// NewStrictHTTPClient creates an HTTP client with the strictest TLS configuration.
// Suitable for high-security environments:
//   - TLS 1.3 only
//   - TLS 1.3 cipher suites only
//   - Certificate verification always enabled
func NewStrictHTTPClient(timeout time.Duration) *http.Client {
	tlsConfig := config.StrictTLSConfig()
	transportConfig := DefaultHTTPTransportConfig()
	transportConfig.TLSConfig = &tlsConfig

	return NewSecureHTTPClient(timeout, &tlsConfig, &transportConfig, "")
}

// WithTLSConfig returns a copy of HTTPTransportConfig with the specified TLS configuration.
func (c HTTPTransportConfig) WithTLSConfig(tlsConfig *config.TLSConfig) HTTPTransportConfig {
	c.TLSConfig = tlsConfig
	return c
}

// WithConnectionPooling returns a copy of HTTPTransportConfig with custom connection pooling.
func (c HTTPTransportConfig) WithConnectionPooling(maxIdleConns, maxIdleConnsPerHost, maxConnsPerHost int, idleTimeout time.Duration) HTTPTransportConfig {
	c.MaxIdleConns = maxIdleConns
	c.MaxIdleConnsPerHost = maxIdleConnsPerHost
	c.MaxConnsPerHost = maxConnsPerHost
	c.IdleConnTimeout = idleTimeout
	return c
}

// WithTimeouts returns a copy of HTTPTransportConfig with custom timeout settings.
func (c HTTPTransportConfig) WithTimeouts(dialTimeout, dialKeepAlive, tlsHandshakeTimeout, expectContinueTimeout, responseHeaderTimeout time.Duration) HTTPTransportConfig {
	c.DialTimeout = dialTimeout
	c.DialKeepAlive = dialKeepAlive
	c.TLSHandshakeTimeout = tlsHandshakeTimeout
	c.ExpectContinueTimeout = expectContinueTimeout
	c.ResponseHeaderTimeout = responseHeaderTimeout
	return c
}

// WithProxy returns a copy of HTTPTransportConfig with a custom proxy function.
func (c HTTPTransportConfig) WithProxy(proxy func(*http.Request) (*url.URL, error)) HTTPTransportConfig {
	c.Proxy = proxy
	return c
}

// validateNonNegativeInt returns a validation error if val is negative.
func validateNonNegativeInt(name string, val int) error {
	if val < 0 {
		err := types.NewWormholeError(types.ErrorCodeValidation, name+" cannot be negative", false)
		err.Details = fmt.Sprintf("%d", val)
		return err
	}
	return nil
}

// validateNonNegativeDuration returns a validation error if val is negative.
func validateNonNegativeDuration(name string, val time.Duration) error {
	if val < 0 {
		err := types.NewWormholeError(types.ErrorCodeValidation, name+" cannot be negative", false)
		err.Details = fmt.Sprintf("%v", val)
		return err
	}
	return nil
}

// Validate checks if the HTTP transport configuration is valid.
// Returns an error if any setting is invalid.
func (c HTTPTransportConfig) Validate() error {
	if c.TLSConfig != nil && !c.TLSConfig.IsSecure() && !c.TLSConfig.AllowInsecure {
		err := types.NewWormholeError(types.ErrorCodeValidation, "TLS configuration is not secure", false)
		err.Details = fmt.Sprintf("MinVersion=%d, InsecureSkipVerify=%v, AllowInsecure=%v", c.TLSConfig.MinVersion, c.TLSConfig.InsecureSkipVerify, c.TLSConfig.AllowInsecure)
		return err
	}

	if err := validateNonNegativeInt("MaxIdleConns", c.MaxIdleConns); err != nil {
		return err
	}
	if err := validateNonNegativeInt("MaxIdleConnsPerHost", c.MaxIdleConnsPerHost); err != nil {
		return err
	}
	if err := validateNonNegativeInt("MaxConnsPerHost", c.MaxConnsPerHost); err != nil {
		return err
	}

	if err := validateNonNegativeDuration("IdleConnTimeout", c.IdleConnTimeout); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("DialTimeout", c.DialTimeout); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("TLSHandshakeTimeout", c.TLSHandshakeTimeout); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("ExpectContinueTimeout", c.ExpectContinueTimeout); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("ResponseHeaderTimeout", c.ResponseHeaderTimeout); err != nil {
		return err
	}

	return nil
}
