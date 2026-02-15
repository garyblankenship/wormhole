package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/internal/pool"
	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// requestBodyPool pools byte slices for request bodies to reduce allocations.
// Stores *[]byte so sync.Pool.Put receives a pointer type (SA6002).
var requestBodyPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 1024)
		return &buf
	},
}

// responseBodyPool pools byte slices for response bodies to reduce allocations.
// Stores *[]byte so sync.Pool.Put receives a pointer type (SA6002).
var responseBodyPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 4096)
		return &buf
	},
}

// pooledBytesReader is an io.Reader that returns its underlying byte slice to the pool after reading
type pooledBytesReader struct {
	bytes    []byte
	pos      int
	returned bool
}

func (r *pooledBytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.bytes) {
		if !r.returned {
			// Return slice to pool, resetting length to 0 but keeping capacity
			buf := r.bytes[:0]
			requestBodyPool.Put(&buf)
			r.returned = true
		}
		return 0, io.EOF
	}
	n = copy(p, r.bytes[r.pos:])
	r.pos += n
	return n, nil
}

// jsonPooledReader is an io.Reader that returns its underlying byte slice to the JSON buffer pool after reading
type jsonPooledReader struct {
	bytes    []byte
	pos      int
	returned bool
}

func (r *jsonPooledReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.bytes) {
		if !r.returned {
			// Return slice to JSON buffer pool
			pool.Return(r.bytes)
			r.returned = true
		}
		return 0, io.EOF
	}
	n = copy(p, r.bytes[r.pos:])
	r.pos += n
	return n, nil
}

// readAllPooled reads all data from r into a pooled byte slice.
// The caller MUST call returnResponseBuf after using the slice.
func readAllPooled(r io.Reader) ([]byte, error) {
	// Get initial buffer from pool
	bufPtr := responseBodyPool.Get().(*[]byte)
	buf := (*bufPtr)[:0] // reset length

	// Temporary scratch buffer for reading chunks
	scratch := make([]byte, 4096)

	for {
		n, err := r.Read(scratch)
		if n > 0 {
			// Ensure we have enough capacity
			if cap(buf)-len(buf) < n {
				// Need to grow buffer
				newCap := cap(buf) * 2
				if newCap == 0 {
					newCap = 4096
				}
				// Ensure new capacity can hold existing data + new data
				if newCap < len(buf)+n {
					newCap = len(buf) + n
				}
				newBuf := make([]byte, len(buf), newCap)
				copy(newBuf, buf)
				// Return old buffer to pool
				old := buf[:0]
				responseBodyPool.Put(&old)
				buf = newBuf
			}
			buf = append(buf, scratch[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			// On error, return buffer to pool
			errBuf := buf[:0]
			responseBodyPool.Put(&errBuf)
			return nil, err
		}
	}
	return buf, nil
}

// returnResponseBuf returns a response buffer to the pool.
func returnResponseBuf(buf []byte) {
	buf = buf[:0]
	responseBodyPool.Put(&buf)
}

// AuthStrategy defines the interface for authentication strategies
type AuthStrategy interface {
	// Apply adds authentication to the request
	Apply(req *http.Request, config types.ProviderConfig) error
	// Name returns the name of the authentication strategy
	Name() string
}

// BaseProvider provides common functionality for all providers
// Embeds the types.BaseProvider for default method implementations
// and adds HTTP functionality for making requests
type BaseProvider struct {
	*types.BaseProvider
	Config       types.ProviderConfig
	tlsConfig    *config.TLSConfig
	httpClient   *http.Client
	retryClient  *utils.RetryableHTTPClient
	authStrategy AuthStrategy
}

// GetHTTPTimeout returns the configured HTTP timeout for this provider
// - 0 = unlimited timeout (no timeout)
// - >0 = timeout in seconds
// - default = configured default timeout
func (p *BaseProvider) GetHTTPTimeout() time.Duration {
	if p.Config.Timeout == 0 {
		return 0 // Unlimited timeout
	} else if p.Config.Timeout > 0 {
		return time.Duration(p.Config.Timeout) * time.Second
	}
	return config.GetDefaultHTTPTimeout()
}

// GetHTTPClient returns an HTTP client with the configured timeout and TLS settings
// Reuses the existing httpClient if available, otherwise creates a new one
func (p *BaseProvider) GetHTTPClient() *http.Client {
	if p.httpClient != nil {
		return p.httpClient
	}

	// Create HTTP client with TLS configuration
	return NewSecureHTTPClient(p.GetHTTPTimeout(), p.tlsConfig, nil, "")
}

// NewBaseProvider creates a new base provider with default secure TLS configuration
func NewBaseProvider(name string, providerConfig types.ProviderConfig) *BaseProvider {
	return NewBaseProviderWithAuth(name, providerConfig, nil, nil)
}

// NewBaseProviderWithTLS creates a new base provider with custom TLS configuration
// If tlsConfig is nil, extracts TLS configuration from ProviderConfig.Params if available,
// otherwise uses DefaultTLSConfig() for secure defaults
func NewBaseProviderWithTLS(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig) *BaseProvider {
	return NewBaseProviderWithAuth(name, providerConfig, tlsConfig, nil)
}

// NewBaseProviderWithAuth creates a new base provider with custom TLS and auth configuration
func NewBaseProviderWithAuth(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig, authStrategy AuthStrategy) *BaseProvider {
	// Extract TLS configuration from ProviderConfig if not explicitly provided
	if tlsConfig == nil {
		tlsConfig = ExtractTLSConfigFromProviderConfig(providerConfig)
	}

	// Use BearerAuthStrategy if no auth strategy provided
	if authStrategy == nil {
		authStrategy = &BearerAuthStrategy{}
	}

	bp := &BaseProvider{
		BaseProvider: types.NewBaseProvider(name),
		Config:       providerConfig,
		tlsConfig:    tlsConfig,
		authStrategy: authStrategy,
	}

	// Create HTTP client with configured timeout and TLS settings
	// Include base URL for connection pooling across providers with same host
	bp.httpClient = NewSecureHTTPClient(bp.GetHTTPTimeout(), tlsConfig, nil, providerConfig.BaseURL)

	// Configure retry logic based on per-provider settings
	retryConfig := utils.DefaultRetryConfig() // Start with global defaults

	// Override with provider-specific settings if provided
	if providerConfig.MaxRetries != nil {
		retryConfig.MaxRetries = *providerConfig.MaxRetries
	}
	if providerConfig.RetryDelay != nil {
		retryConfig.InitialDelay = *providerConfig.RetryDelay
	}
	if providerConfig.RetryMaxDelay != nil {
		retryConfig.MaxDelay = *providerConfig.RetryMaxDelay
	}

	bp.retryClient = utils.NewRetryableHTTPClient(bp.httpClient, retryConfig)

	return bp
}

// NewInsecureBaseProvider creates a new base provider with insecure TLS configuration
// WARNING: This should only be used for testing or legacy compatibility
// The provider will allow TLS 1.0 and weak cipher suites
func NewInsecureBaseProvider(name string, providerConfig types.ProviderConfig, skipVerify bool) *BaseProvider {
	insecureTLS := config.InsecureTLSConfig()
	if skipVerify {
		insecureTLS = insecureTLS.WithInsecureSkipVerify(true)
	}

	return NewBaseProviderWithAuth(name, providerConfig, &insecureTLS, nil)
}

// DoRequest performs an HTTP request with common error handling
func (p *BaseProvider) DoRequest(ctx context.Context, method, url string, body any, result any) error {
	// Build and execute request
	req, err := p.buildRequest(ctx, method, url, body)
	if err != nil {
		return err
	}

	// Execute with retry logic
	resp, err := p.retryClient.Do(req)
	if err != nil {
		return p.handleRequestError(ctx, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()

	// Read response using pooled buffer
	respBody, err := readAllPooled(resp.Body)
	if err != nil {
		return types.Errorf("read response body", err)
	}
	defer returnResponseBuf(respBody)

	// Handle error responses
	if resp.StatusCode >= 400 {
		return p.buildErrorResponse(resp.StatusCode, resp.Status, url, respBody)
	}

	// Parse successful response
	return p.parseResponse(respBody, result)
}

// buildRequest creates an HTTP request with headers and body
func (p *BaseProvider) buildRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	// Marshal request body if provided
	reqBody, err := p.marshalRequestBody(body)
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, types.Errorf("create request", err)
	}

	// Set headers
	if err := p.setRequestHeaders(req); err != nil {
		return nil, err
	}

	return req, nil
}

// marshalRequestBody converts request body to io.Reader using pooled byte slices
func (p *BaseProvider) marshalRequestBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}

	// Use pooled JSON marshaling
	bytes, err := pool.Marshal(body)
	if err != nil {
		return nil, types.Errorf("marshal request body", err)
	}

	// Create pooled reader that will return slice to pool after reading
	// Note: pooledBytesReader uses requestBodyPool, but we want to use pool.Return
	// So we need a custom reader that calls pool.Return
	return &jsonPooledReader{bytes: bytes}, nil
}

// setRequestHeaders sets common and custom headers
func (p *BaseProvider) setRequestHeaders(req *http.Request) error {
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)

	// Apply authentication strategy
	if err := p.authStrategy.Apply(req, p.Config); err != nil {
		return err
	}

	// Set custom headers
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}

	return nil
}

// handleRequestError processes errors from HTTP request execution
func (p *BaseProvider) handleRequestError(ctx context.Context, err error) error {
	// Guard: Check context cancellation first
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Guard: Check for timeout errors
	if p.isTimeoutError(err) {
		wormholeErr := types.NewWormholeError(types.ErrorCodeTimeout, "request timeout", true)
		wormholeErr.Provider = p.Name()
		return wormholeErr
	}

	return p.WrapError(types.ErrorCodeNetwork, "request failed", err)
}

// buildErrorResponse creates a detailed error response for HTTP errors
func (p *BaseProvider) buildErrorResponse(statusCode int, status, url string, respBody []byte) error {
	errorCode := p.mapHTTPStatusToErrorCode(statusCode)
	errorMessage := p.extractErrorMessage(statusCode, status, respBody)

	wormholeErr := types.NewWormholeError(
		errorCode,
		errorMessage,
		p.isRetryableStatus(statusCode),
	).WithDetails(fmt.Sprintf("URL: %s\nResponse: %s", p.maskAPIKeyInURL(url), string(respBody)))

	wormholeErr.StatusCode = statusCode
	wormholeErr.Provider = p.Name()
	return wormholeErr
}

// extractErrorMessage parses provider-specific error message from response
func (p *BaseProvider) extractErrorMessage(statusCode int, status string, respBody []byte) string {
	// Default message
	errorMessage := fmt.Sprintf("HTTP %d: %s", statusCode, status)

	// Guard: No body to parse
	if len(respBody) == 0 {
		return errorMessage
	}

	// Try to parse provider-specific error
	var errorResp map[string]any
	if err := json.Unmarshal(respBody, &errorResp); err != nil {
		return errorMessage
	}

	// Extract nested error message if available
	if errorObj, ok := errorResp["error"].(map[string]any); ok {
		if msg, ok := errorObj["message"].(string); ok && msg != "" {
			return msg
		}
	}

	return errorMessage
}

// parseResponse unmarshals response body into result
func (p *BaseProvider) parseResponse(respBody []byte, result any) error {
	// Guard: No result to parse into
	if result == nil {
		return nil
	}

	// Guard: Empty response body
	if len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return types.Errorf("unmarshal response", err)
	}

	return nil
}

// mapHTTPStatusToErrorCode maps HTTP status codes to Wormhole error codes
func (p *BaseProvider) mapHTTPStatusToErrorCode(statusCode int) types.ErrorCode {
	switch statusCode {
	case 401, 403:
		return types.ErrorCodeAuth
	case 404:
		return types.ErrorCodeModel
	case 429:
		return types.ErrorCodeRateLimit
	case 400, 422:
		return types.ErrorCodeRequest
	case 408, 504:
		return types.ErrorCodeTimeout
	case 500, 502, 503:
		return types.ErrorCodeProvider
	default:
		return types.ErrorCodeNetwork
	}
}

// isRetryableStatus determines if an HTTP status code indicates a retryable error
func (p *BaseProvider) isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504, 408:
		return true
	default:
		return false
	}
}

// StreamRequest performs a streaming HTTP request
func (p *BaseProvider) StreamRequest(ctx context.Context, method, url string, body any) (io.ReadCloser, error) {
	var reqBody io.Reader
	if body != nil {
		var err error
		reqBody, err = p.marshalRequestBody(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, types.Errorf("create request", err)
	}

	// Set common headers
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)
	req.Header.Set(types.HeaderAccept, types.ContentTypeEventStream)
	req.Header.Set(types.HeaderCacheControl, "no-cache")

	// Apply authentication strategy
	if err := p.authStrategy.Apply(req, p.Config); err != nil {
		return nil, err
	}

	// Set custom headers
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, p.WrapError(types.ErrorCodeNetwork, "request failed", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBody, _ := readAllPooled(resp.Body)
		defer returnResponseBuf(respBody)
		return nil, p.buildErrorResponse(resp.StatusCode, resp.Status, url, respBody)
	}

	return resp.Body, nil
}

// GetBaseURL returns the base URL for the provider
func (p *BaseProvider) GetBaseURL() string {
	if p.Config.BaseURL != "" {
		return p.Config.BaseURL
	}
	// Default URLs will be set by specific providers
	return ""
}

// NotImplementedError returns a standard not implemented error
func (p *BaseProvider) NotImplementedError(method string) error {
	return p.ProviderErrorf("%s provider does not support %s", p.Name(), method)
}

// ValidationError returns a WormholeError with ErrorCodeValidation
func (p *BaseProvider) ValidationError(message string, details ...string) error {
	err := types.NewWormholeError(types.ErrorCodeValidation, message, false)
	err.Provider = p.Name()
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// ValidationErrorf formats a validation error
func (p *BaseProvider) ValidationErrorf(format string, args ...any) error {
	return p.ValidationError(fmt.Sprintf(format, args...))
}

// ProviderError returns a WormholeError with ErrorCodeProvider
func (p *BaseProvider) ProviderError(message string, details ...string) error {
	err := types.NewWormholeError(types.ErrorCodeProvider, message, true) // provider errors are retryable
	err.Provider = p.Name()
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// ProviderErrorf formats a provider error
func (p *BaseProvider) ProviderErrorf(format string, args ...any) error {
	return p.ProviderError(fmt.Sprintf(format, args...))
}

// RequestError wraps a cause with ErrorCodeRequest
func (p *BaseProvider) RequestError(message string, cause error) error {
	err := types.NewWormholeError(types.ErrorCodeRequest, message, false)
	err.Provider = p.Name()
	err.Cause = cause
	return err
}

// ModelError returns a WormholeError with ErrorCodeModel
func (p *BaseProvider) ModelError(message string, details ...string) error {
	err := types.NewWormholeError(types.ErrorCodeModel, message, false)
	err.Provider = p.Name()
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// ModelErrorf formats a model error
func (p *BaseProvider) ModelErrorf(format string, args ...any) error {
	return p.ModelError(fmt.Sprintf(format, args...))
}

// AuthError returns a WormholeError with ErrorCodeAuth
func (p *BaseProvider) AuthError(message string, details ...string) error {
	err := types.NewWormholeError(types.ErrorCodeAuth, message, true) // auth errors often retryable
	err.Provider = p.Name()
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}

// AuthErrorf formats an auth error
func (p *BaseProvider) AuthErrorf(format string, args ...any) error {
	return p.AuthError(fmt.Sprintf(format, args...))
}

// WrapError wraps an error with Wormhole error context
func (p *BaseProvider) WrapError(code types.ErrorCode, message string, cause error) error {
	err := types.NewWormholeError(code, message, p.isRetryableCode(code))
	err.Provider = p.Name()
	err.Cause = cause
	return err
}

// Helper to determine retryability from error code (similar to isRetryableStatus)
func (p *BaseProvider) isRetryableCode(code types.ErrorCode) bool {
	switch code {
	case types.ErrorCodeAuth, types.ErrorCodeRateLimit, types.ErrorCodeTimeout,
		types.ErrorCodeProvider, types.ErrorCodeNetwork:
		return true
	default:
		return false
	}
}

// maskAPIKeyInURL masks API keys in URLs for security in error messages
func (p *BaseProvider) maskAPIKeyInURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL // Return original if parsing fails
	}

	// Mask API keys in query parameters
	query := parsed.Query()
	for key, values := range query {
		if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
			for i, value := range values {
				if len(value) > 8 {
					values[i] = value[:4] + "****" + value[len(value)-4:]
				} else if len(value) > 0 {
					values[i] = "****"
				}
			}
		}
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

// isTimeoutError checks if an error is a timeout error
func (p *BaseProvider) isTimeoutError(err error) bool {
	// Check for net.Error with Timeout() method
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check for url.Error with timeout in message
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}

	// Check for common timeout error messages
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "deadline exceeded") ||
		strings.Contains(errMsg, "context deadline exceeded")
}

// Close implements io.Closer interface for BaseProvider
func (p *BaseProvider) Close() error {
	// No resources to clean up in BaseProvider
	// httpClient is created as a pointer to http.Client, which has no Close() method
	// retryClient doesn't need cleanup as it wraps http.Client
	return nil
}
