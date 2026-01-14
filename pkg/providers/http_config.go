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
	"strings"
	"sync"
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
	// Proxy is a function pointer, cannot fingerprint; assume default proxy
	return b.String()
}

var (
	transportCache sync.RWMutex
	transports = make(map[string]*http.Transport)
)

func getCachedTransport(key string) *http.Transport {
	transportCache.RLock()
	defer transportCache.RUnlock()
	return transports[key]
}

func setCachedTransport(key string, transport *http.Transport) {
	transportCache.Lock()
	defer transportCache.Unlock()
	transports[key] = transport
}

// NewSecureHTTPClient creates a new HTTP client with secure TLS configuration
// and optimized transport settings.
//
// Parameters:
//   - timeout: Overall request timeout (0 for no timeout)
//   - tlsConfig: TLS configuration (nil for default secure configuration)
//   - transportConfig: HTTP transport configuration (nil for default)
//
// The client uses:
//   - Secure TLS 1.2+ by default
//   - Modern cipher suites only
//   - Connection pooling for performance
//   - Proper timeout handling
func NewSecureHTTPClient(timeout time.Duration, tlsConfig *config.TLSConfig, transportConfig *HTTPTransportConfig) *http.Client {
	// Use default TLS config if not provided
	if tlsConfig == nil {
		defaultTLS := config.DefaultTLSConfig()
		tlsConfig = &defaultTLS
	}

	// Use default transport config if not provided
	if transportConfig == nil {
		defaultTransport := DefaultHTTPTransportConfig()
		transportConfig = &defaultTransport
	}

	// Create TLS config
	var tlsClientConfig *tls.Config
	if tlsConfig != nil {
		tlsClientConfig = tlsConfig.ApplyToTLSConfig(nil)
	}

	// Compute cache key based on transport configuration
	key := transportConfig.Fingerprint()

	// Try to get cached transport
	transport := getCachedTransport(key)
	if transport == nil {
		// Create new transport with TLS config
		transport = &http.Transport{
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
		setCachedTransport(key, transport)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
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

	transportConfig := DefaultHTTPTransportConfig()
	transportConfig.TLSConfig = &tlsConfig

	return NewSecureHTTPClient(timeout, &tlsConfig, &transportConfig)
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

	return NewSecureHTTPClient(timeout, &tlsConfig, &transportConfig)
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

// Validate checks if the HTTP transport configuration is valid.
// Returns an error if any setting is invalid.
func (c HTTPTransportConfig) Validate() error {
	if c.TLSConfig != nil && !c.TLSConfig.IsSecure() {
		return fmt.Errorf("TLS configuration is not secure: MinVersion=%d, InsecureSkipVerify=%v",
			c.TLSConfig.MinVersion, c.TLSConfig.InsecureSkipVerify)
	}

	if c.MaxIdleConns < 0 {
		return fmt.Errorf("MaxIdleConns cannot be negative: %d", c.MaxIdleConns)
	}
	if c.MaxIdleConnsPerHost < 0 {
		return fmt.Errorf("MaxIdleConnsPerHost cannot be negative: %d", c.MaxIdleConnsPerHost)
	}
	if c.MaxConnsPerHost < 0 {
		return fmt.Errorf("MaxConnsPerHost cannot be negative: %d", c.MaxConnsPerHost)
	}

	if c.IdleConnTimeout < 0 {
		return fmt.Errorf("IdleConnTimeout cannot be negative: %v", c.IdleConnTimeout)
	}
	if c.DialTimeout < 0 {
		return fmt.Errorf("DialTimeout cannot be negative: %v", c.DialTimeout)
	}
	if c.TLSHandshakeTimeout < 0 {
		return fmt.Errorf("TLSHandshakeTimeout cannot be negative: %v", c.TLSHandshakeTimeout)
	}
	if c.ExpectContinueTimeout < 0 {
		return fmt.Errorf("ExpectContinueTimeout cannot be negative: %v", c.ExpectContinueTimeout)
	}
	if c.ResponseHeaderTimeout < 0 {
		return fmt.Errorf("ResponseHeaderTimeout cannot be negative: %v", c.ResponseHeaderTimeout)
	}

	return nil
}

// ==================== TLS Configuration Extraction from ProviderConfig ====================

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