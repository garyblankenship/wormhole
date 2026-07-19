// Package providers provides HTTP client configuration with secure TLS defaults.
// This file contains utilities for creating secure HTTP clients with proper
// TLS configuration, connection pooling, and timeout management.

package providers

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
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
