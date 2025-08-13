package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// BaseProvider provides common functionality for all providers
type BaseProvider struct {
	Config      types.ProviderConfig
	httpClient  *http.Client
	retryClient *utils.RetryableHTTPClient
	name        string
}

// NewBaseProvider creates a new base provider
func NewBaseProvider(name string, config types.ProviderConfig) *BaseProvider {
	timeout := 30 * time.Second
	if config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}

	// Configure retry logic
	retryConfig := utils.DefaultRetryConfig()
	if config.MaxRetries > 0 {
		retryConfig.MaxRetries = config.MaxRetries
	}
	if config.RetryDelay > 0 {
		retryConfig.InitialDelay = time.Duration(config.RetryDelay) * time.Millisecond
	}

	retryClient := utils.NewRetryableHTTPClient(httpClient, retryConfig)

	return &BaseProvider{
		name:        name,
		Config:      config,
		httpClient:  httpClient,
		retryClient: retryClient,
	}
}

// Name returns the provider name
func (p *BaseProvider) Name() string {
	return p.name
}

// DoRequest performs an HTTP request with common error handling
func (p *BaseProvider) DoRequest(ctx context.Context, method, url string, body interface{}, result interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Config.APIKey)

	// Set custom headers
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.retryClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Enhanced error reporting with HTTP details as requested by user
		errorCode := p.mapHTTPStatusToErrorCode(resp.StatusCode)
		wormholeErr := types.NewWormholeError(
			errorCode,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status),
			p.isRetryableStatus(resp.StatusCode),
		).WithDetails(fmt.Sprintf("URL: %s\nResponse: %s", url, string(respBody)))

		wormholeErr.StatusCode = resp.StatusCode
		return wormholeErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
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
func (p *BaseProvider) StreamRequest(ctx context.Context, method, url string, body interface{}) (io.ReadCloser, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set common headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

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
	return fmt.Errorf("%s provider does not support %s", p.name, method)
}
