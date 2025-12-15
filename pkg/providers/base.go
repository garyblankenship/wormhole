package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// BaseProvider provides common functionality for all providers
// Embeds the types.BaseProvider for default method implementations
// and adds HTTP functionality for making requests
type BaseProvider struct {
	*types.BaseProvider
	Config      types.ProviderConfig
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

// GetHTTPClient returns an HTTP client with the configured timeout
// Reuses the existing httpClient if available, otherwise creates a new one
func (p *BaseProvider) GetHTTPClient() *http.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return &http.Client{Timeout: p.GetHTTPTimeout()}
}

// NewBaseProvider creates a new base provider
func NewBaseProvider(name string, providerConfig types.ProviderConfig) *BaseProvider {
	bp := &BaseProvider{
		BaseProvider: types.NewBaseProvider(name),
		Config:       providerConfig,
	}

	// Create HTTP client with configured timeout
	bp.httpClient = &http.Client{
		Timeout: bp.GetHTTPTimeout(),
	}

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
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.Errorf("read response body", err)
	}

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

// marshalRequestBody converts request body to io.Reader
func (p *BaseProvider) marshalRequestBody(body any) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, types.Errorf("marshal request body", err)
	}

	return bytes.NewReader(jsonBody), nil
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
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, types.Errorf("marshal request body", err)
		}
		reqBody = bytes.NewReader(jsonBody)
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
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
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
