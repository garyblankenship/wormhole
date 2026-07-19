package providers

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

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
