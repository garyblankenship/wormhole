package providers

import (
	"bytes"
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

// HTTPClient is the interface for HTTP clients (alias for utils.HTTPClient).
type HTTPClient = utils.HTTPClient

// responseBodyPool pools byte slices for response bodies to reduce allocations.
// Stores *[]byte so sync.Pool.Put receives a pointer type (SA6002).
var responseBodyPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 4096)
		return &buf
	},
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

type HTTPClientWrapper struct {
    providerName string
	Config       types.ProviderConfig
	tlsConfig    *config.TLSConfig
	httpClient   *http.Client
	retryClient  *utils.RetryableHTTPClient
	authStrategy AuthStrategy
}

// NewHTTPClientWrapper creates a new HTTPClientWrapper.
// Pass a non-nil httpClient to inject a custom HTTP client (useful for testing).
// Pass nil to use the default secure HTTP client.
func NewHTTPClientWrapper(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig, authStrategy AuthStrategy, httpClient HTTPClient) *HTTPClientWrapper {
	w := &HTTPClientWrapper{
		providerName: name,
		Config:       providerConfig,
		tlsConfig:    tlsConfig,
		authStrategy: authStrategy,
	}

	// Use injected client if provided, otherwise create default
	if httpClient != nil {
		// Type assertion to get the concrete *http.Client if possible
		if hc, ok := httpClient.(*http.Client); ok {
			w.httpClient = hc
		} else {
			// For non-standard HTTPClient implementations, create a concrete client for GetHTTPClient()
			w.httpClient = NewSecureHTTPClient(w.GetHTTPTimeout(), tlsConfig, nil, providerConfig.BaseURL)
		}
	} else {
		w.httpClient = NewSecureHTTPClient(w.GetHTTPTimeout(), tlsConfig, nil, providerConfig.BaseURL)
	}

	retryConfig := utils.DefaultRetryConfig()
	if providerConfig.MaxRetries != nil {
		retryConfig.MaxRetries = *providerConfig.MaxRetries
	}
	if providerConfig.RetryDelay != nil {
		retryConfig.InitialDelay = *providerConfig.RetryDelay
	}
	if providerConfig.RetryMaxDelay != nil {
		retryConfig.MaxDelay = *providerConfig.RetryMaxDelay
	}

	// Use injected client for retry wrapper if provided, otherwise use the concrete httpClient
	if httpClient != nil {
		w.retryClient = utils.NewRetryableHTTPClient(httpClient, retryConfig)
	} else {
		w.retryClient = utils.NewRetryableHTTPClient(w.httpClient, retryConfig)
	}

	return w
}

func (w *HTTPClientWrapper) GetHTTPTimeout() time.Duration {
	if w.Config.Timeout == 0 {
		return 0 // Unlimited timeout
	} else if w.Config.Timeout > 0 {
		return time.Duration(w.Config.Timeout) * time.Second
	}
	return config.GetDefaultHTTPTimeout()
}

func (w *HTTPClientWrapper) GetHTTPClient() *http.Client {
	if w.httpClient != nil {
		return w.httpClient
	}
	return NewSecureHTTPClient(w.GetHTTPTimeout(), w.tlsConfig, nil, "")
}


func (w *HTTPClientWrapper) DoRequest(ctx context.Context, method, url string, body any, result any) error {
	req, err := w.buildRequest(ctx, method, url, body)
	if err != nil {
		return err
	}

	resp, err := w.retryClient.Do(req)
	if err != nil {
		return w.handleRequestError(ctx, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()

	respBody, err := readAllPooled(resp.Body)
	if err != nil {
		return types.Errorf("read response body", err)
	}
	defer returnResponseBuf(respBody)

	if resp.StatusCode >= 400 {
		return w.buildErrorResponse(resp.StatusCode, resp.Status, url, respBody)
	}

	return w.parseResponse(respBody, result)
}

func (w *HTTPClientWrapper) StreamRequest(ctx context.Context, method, url string, body any) (io.ReadCloser, error) {
	var reqBody io.Reader
	if body != nil {
		payload, err := w.marshalRequestBody(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, types.Errorf("create request", err)
	}

	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)
	req.Header.Set(types.HeaderAccept, types.ContentTypeEventStream)
	req.Header.Set(types.HeaderCacheControl, "no-cache")

	if err := w.authStrategy.Apply(req, w.Config); err != nil {
		return nil, err
	}

	for k, v := range w.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, w.handleRequestError(ctx, err)
	}

	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		respBody, _ := readAllPooled(resp.Body)
		defer returnResponseBuf(respBody)
		return nil, w.buildErrorResponse(resp.StatusCode, resp.Status, url, respBody)
	}

	return resp.Body, nil
}


func (w *HTTPClientWrapper) buildRequest(ctx context.Context, method, url string, body any) (*http.Request, error) {
	payload, err := w.marshalRequestBody(body)
	if err != nil {
		return nil, err
	}

	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, types.Errorf("create request", err)
	}
	if payload != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(payload)), nil
		}
		req.ContentLength = int64(len(payload))
	}

	if err := w.setRequestHeaders(req); err != nil {
		return nil, err
	}

	return req, nil
}

func (w *HTTPClientWrapper) marshalRequestBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	pooledBytes, err := pool.Marshal(body)
	if err != nil {
		return nil, types.Errorf("marshal request body", err)
	}
	defer pool.Return(pooledBytes)

	owned := make([]byte, len(pooledBytes))
	copy(owned, pooledBytes)
	return owned, nil
}

func (w *HTTPClientWrapper) setRequestHeaders(req *http.Request) error {
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)

	if err := w.authStrategy.Apply(req, w.Config); err != nil {
		return err
	}

	for k, v := range w.Config.Headers {
		req.Header.Set(k, v)
	}

	return nil
}

func (w *HTTPClientWrapper) handleRequestError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if w.isTimeoutError(err) {
		wormholeErr := types.NewWormholeError(types.ErrorCodeTimeout, "request timeout", true)
		wormholeErr.Provider = w.providerName
		return wormholeErr
	}

	return types.WrapProviderError(w.providerName, types.ErrorCodeNetwork, "request failed", err)
}

func (w *HTTPClientWrapper) buildErrorResponse(statusCode int, status, url string, respBody []byte) error {
	errorCode := w.mapHTTPStatusToErrorCode(statusCode)
	errorMessage := w.extractErrorMessage(statusCode, status, respBody)

	wormholeErr := types.NewWormholeError(
		errorCode,
		errorMessage,
		utils.IsRetryableStatusCode(statusCode),
	).WithDetails(fmt.Sprintf("URL: %s\nResponse: %s", w.maskAPIKeyInURL(url), string(respBody)))

	wormholeErr.StatusCode = statusCode
	wormholeErr.Provider = w.providerName
	return wormholeErr
}

func (w *HTTPClientWrapper) extractErrorMessage(statusCode int, status string, respBody []byte) string {
	errorMessage := fmt.Sprintf("HTTP %d: %s", statusCode, status)

	if len(respBody) == 0 {
		return errorMessage
	}

	var errorResp map[string]any
	if err := json.Unmarshal(respBody, &errorResp); err != nil {
		return errorMessage
	}

	if errorObj, ok := errorResp["error"].(map[string]any); ok {
		if msg, ok := errorObj["message"].(string); ok && msg != "" {
			return msg
		}
	}

	return errorMessage
}

func (w *HTTPClientWrapper) parseResponse(respBody []byte, result any) error {
	if result == nil {
		return nil
	}

	if len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, result); err != nil {
		return types.Errorf("unmarshal response", err)
	}

	return nil
}

func (w *HTTPClientWrapper) mapHTTPStatusToErrorCode(statusCode int) types.ErrorCode {
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



func (w *HTTPClientWrapper) maskAPIKeyInURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

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

func (w *HTTPClientWrapper) isTimeoutError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return true
	}

	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "deadline exceeded") ||
		strings.Contains(errMsg, "context deadline exceeded")
}

func (w *HTTPClientWrapper) Close() error {
	if w.httpClient != nil && w.httpClient.Transport != nil {
		if transport, ok := w.httpClient.Transport.(interface{ CloseIdleConnections() }); ok {
			transport.CloseIdleConnections()
		}
	}
	return nil
}
