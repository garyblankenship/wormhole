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

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// requestBodyPool pools byte slices for request bodies to reduce allocations
var requestBodyPool = sync.Pool{
	New: func() any {
		// Start with 1KB buffer, will grow as needed
		return make([]byte, 0, 1024)
	},
}

// responseBodyPool pools byte slices for response bodies to reduce allocations
var responseBodyPool = sync.Pool{
	New: func() any {
		// Start with 4KB buffer, typical response size
		return make([]byte, 0, 4096)
	},
}

// pooledBytesReader is an io.Reader that returns its underlying byte slice to the pool after reading
type pooledBytesReader struct {
	bytes   []byte
	pos     int
	returned bool
}

func (r *pooledBytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.bytes) {
		if !r.returned {
			// Return slice to pool, resetting length to 0 but keeping capacity
			requestBodyPool.Put(r.bytes[:0])
			r.returned = true
		}
		return 0, io.EOF
	}
	n = copy(p, r.bytes[r.pos:])
	r.pos += n
	return n, nil
}


// readAllPooled reads all data from r into a pooled byte slice.
// The caller MUST call responseBodyPool.Put(buf[:0]) after using the slice.
func readAllPooled(r io.Reader) ([]byte, error) {
	// Get initial buffer from pool
	buf := responseBodyPool.Get().([]byte)
	buf = buf[:0] // reset length

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
				responseBodyPool.Put(buf[:0])
				buf = newBuf
			}
			buf = append(buf, scratch[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			// On error, return buffer to pool
			responseBodyPool.Put(buf[:0])
			return nil, err
		}
	}
	return buf, nil
}

// BaseProvider provides common functionality for all providers
// Embeds the types.BaseProvider for default method implementations
// and adds HTTP functionality for making requests
type BaseProvider struct {
	*types.BaseProvider
	Config      types.ProviderConfig
	tlsConfig   *config.TLSConfig
	httpClient  *http.Client
	retryClient *utils.RetryableHTTPClient
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
	return NewSecureHTTPClient(p.GetHTTPTimeout(), p.tlsConfig, nil)
}

// NewBaseProvider creates a new base provider with default secure TLS configuration
func NewBaseProvider(name string, providerConfig types.ProviderConfig) *BaseProvider {
	return NewBaseProviderWithTLS(name, providerConfig, nil)
}

// NewBaseProviderWithTLS creates a new base provider with custom TLS configuration
// If tlsConfig is nil, extracts TLS configuration from ProviderConfig.Params if available,
// otherwise uses DefaultTLSConfig() for secure defaults
func NewBaseProviderWithTLS(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig) *BaseProvider {
	// Extract TLS configuration from ProviderConfig if not explicitly provided
	if tlsConfig == nil {
		tlsConfig = ExtractTLSConfigFromProviderConfig(providerConfig)
	}

	bp := &BaseProvider{
		BaseProvider: types.NewBaseProvider(name),
		Config:       providerConfig,
		tlsConfig:    tlsConfig,
	}

	// Create HTTP client with configured timeout and TLS settings
	bp.httpClient = NewSecureHTTPClient(bp.GetHTTPTimeout(), tlsConfig, nil)

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

	return NewBaseProviderWithTLS(name, providerConfig, &insecureTLS)
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
	defer responseBodyPool.Put(respBody[:0])

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
	p.setRequestHeaders(req)

	return req, nil
}

// marshalRequestBody converts request body to io.Reader using pooled byte slices
func (p *BaseProvider) marshalRequestBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}

	// Marshal to temporary slice to determine size
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, types.Errorf("marshal request body", err)
	}

	// Get pooled slice with sufficient capacity
	bytes := requestBodyPool.Get().([]byte)
	if cap(bytes) < len(jsonBody) {
		// Pooled slice too small, allocate new one
		bytes = make([]byte, 0, len(jsonBody))
	}
	bytes = bytes[:0] // Reset length
	bytes = append(bytes, jsonBody...) // Copy data

	// Create pooled reader that will return slice to pool after reading
	return &pooledBytesReader{bytes: bytes}, nil
}

// setRequestHeaders sets common and custom headers
func (p *BaseProvider) setRequestHeaders(req *http.Request) {
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)
	req.Header.Set(types.HeaderAuthorization, "Bearer "+p.Config.APIKey)

	// Set custom headers
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}
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

	return fmt.Errorf("request failed: %w", err)
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
	req.Header.Set(types.HeaderAuthorization, "Bearer "+p.Config.APIKey)
	req.Header.Set(types.HeaderAccept, types.ContentTypeEventStream)
	req.Header.Set(types.HeaderCacheControl, "no-cache")

	// Set custom headers
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()
		respBody, _ := readAllPooled(resp.Body)
		defer responseBodyPool.Put(respBody[:0])
		var apiError types.WormholeProviderError
		if err := json.Unmarshal(respBody, &apiError); err != nil {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, apiError
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
	return fmt.Errorf("%s provider does not support %s", p.Name(), method)
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
