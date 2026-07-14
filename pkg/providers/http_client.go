package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/internal/pool"
	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// HTTPClient is the request-execution boundary used by providers.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// responseBodyPool pools byte slices for response bodies to reduce allocations.
// Stores *[]byte so sync.Pool.Put receives a pointer type (SA6002).
var responseBodyPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 4096)
		return &buf
	},
}

const maxProviderResponseBodyBytes = 32 << 20

func readResponseBodyLimited(r io.Reader) ([]byte, error) {
	respBody, err := readAllPooled(io.LimitReader(r, maxProviderResponseBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(respBody) > maxProviderResponseBodyBytes {
		returnResponseBuf(respBody)
		return nil, types.ErrRequestTooLarge.WithDetails(
			fmt.Sprintf("provider response body exceeded %d bytes", maxProviderResponseBodyBytes),
		)
	}
	return respBody, nil
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

// keyPool provides thread-safe stateful selection over a set of API keys.
// maxKeyCooldown caps how long a key rotation cooldown can be, regardless of
// what a provider's Retry-After header requests. Without a cap, a bogus or
// malicious large header value (e.g. 10h) would bench a key for that long.
const maxKeyCooldown = 5 * time.Minute

type keyPool struct {
	mu       sync.Mutex
	keys     []string
	current  int
	limited  map[int]time.Time
	cooldown time.Duration
}

func newKeyPool(keys []string, cooldown time.Duration) *keyPool {
	if cooldown <= 0 {
		cooldown = time.Second
	}
	return &keyPool{
		keys:     append([]string(nil), keys...),
		limited:  make(map[int]time.Time),
		cooldown: cooldown,
	}
}

func (kp *keyPool) currentKey(now time.Time) string {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	kp.expireLocked(now)
	if !kp.isLimitedLocked(kp.current, now) {
		return kp.keys[kp.current]
	}
	for offset := 1; offset < len(kp.keys); offset++ {
		next := (kp.current + offset) % len(kp.keys)
		if !kp.isLimitedLocked(next, now) {
			kp.current = next
			return kp.keys[kp.current]
		}
	}
	return kp.keys[kp.current]
}

func (kp *keyPool) rotateAfterRateLimit(failedKey string, retryAfter time.Duration, now time.Time) string {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	kp.expireLocked(now)

	failedIdx := kp.indexOfLocked(failedKey)
	if failedIdx >= 0 {
		cooldown := kp.cooldown
		if retryAfter > 0 {
			cooldown = retryAfter
		}
		if cooldown > maxKeyCooldown {
			cooldown = maxKeyCooldown
		}
		kp.limited[failedIdx] = now.Add(cooldown)
	}

	// Avoid double-advancing: only move the cursor when the request that saw
	// the 429 used the currently selected key.
	if failedIdx == kp.current {
		for offset := 1; offset < len(kp.keys); offset++ {
			next := (kp.current + offset) % len(kp.keys)
			if !kp.isLimitedLocked(next, now) {
				kp.current = next
				break
			}
		}
	}

	if kp.isLimitedLocked(kp.current, now) {
		for idx := range kp.keys {
			if !kp.isLimitedLocked(idx, now) {
				kp.current = idx
				break
			}
		}
	}
	return kp.keys[kp.current]
}

func (kp *keyPool) indexOfLocked(key string) int {
	for idx, existing := range kp.keys {
		if existing == key {
			return idx
		}
	}
	return -1
}

func (kp *keyPool) expireLocked(now time.Time) {
	for idx, until := range kp.limited {
		if !until.After(now) {
			delete(kp.limited, idx)
		}
	}
}

func (kp *keyPool) isLimitedLocked(idx int, now time.Time) bool {
	until, ok := kp.limited[idx]
	return ok && until.After(now)
}

type HTTPClientWrapper struct {
	providerName   string
	Config         types.ProviderConfig
	tlsConfig      *config.TLSConfig
	httpClient     *http.Client
	retryClient    *retryableHTTPClient
	authStrategy   AuthStrategy
	keyPool        *keyPool
	transportCache *TransportCache
}

// NewHTTPClientWrapper creates a new HTTPClientWrapper.
// Pass a non-nil httpClient to inject a custom HTTP client (useful for testing).
// Pass nil to use the default secure HTTP client.
func NewHTTPClientWrapper(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig, authStrategy AuthStrategy, httpClient HTTPClient) *HTTPClientWrapper {
	// Seed the first-attempt key from APIKeys[0] when only APIKeys is set, so the
	// first request's auth uses APIKeys[0] and the pool's next() returns APIKeys[1]
	// on the first 429.
	if providerConfig.APIKey == "" {
		providerConfig.APIKey = providerConfig.EffectiveAPIKey()
	}

	w := &HTTPClientWrapper{
		providerName:   name,
		Config:         providerConfig,
		tlsConfig:      tlsConfig,
		authStrategy:   authStrategy,
		transportCache: NewTransportCache(),
	}

	// Use injected client if provided, otherwise create default
	if httpClient != nil {
		// Type assertion to get the concrete *http.Client if possible
		if hc, ok := httpClient.(*http.Client); ok {
			w.httpClient = hc
		} else {
			// For non-standard HTTPClient implementations, create a concrete client for GetHTTPClient()
			w.httpClient = w.transportCache.newSecureHTTPClient(0, tlsConfig, nil, providerConfig.BaseURL)
		}
	} else {
		w.httpClient = w.transportCache.newSecureHTTPClient(0, tlsConfig, nil, providerConfig.BaseURL)
	}

	retryConfig := defaultRetryConfig()
	if providerConfig.MaxRetries != nil {
		retryConfig.MaxRetries = *providerConfig.MaxRetries
	}
	if providerConfig.RetryDelay != nil {
		retryConfig.InitialDelay = *providerConfig.RetryDelay
	}
	if providerConfig.RetryMaxDelay != nil {
		retryConfig.MaxDelay = *providerConfig.RetryMaxDelay
	}
	if len(providerConfig.APIKeys) > 1 {
		w.keyPool = newKeyPool(providerConfig.APIKeys, retryConfig.InitialDelay)
	}

	// Use injected client for retry wrapper if provided, otherwise use the concrete httpClient
	if httpClient != nil {
		w.retryClient = newRetryableHTTPClient(httpClient, retryConfig)
	} else {
		w.retryClient = newRetryableHTTPClient(w.httpClient, retryConfig)
	}

	// Stateful key rotation: only rotate after a retryable rate-limit response.
	if w.keyPool != nil {
		pool := w.keyPool
		auth := authStrategy
		baseCfg := providerConfig
		w.retryClient.OnRetry = func(reqClone *http.Request, _ int, retryErr *retryableError, previousRequest *http.Request) {
			cfg := baseCfg
			now := time.Now()
			if retryErr != nil && retryErr.StatusCode == http.StatusTooManyRequests {
				cfg.APIKey = pool.rotateAfterRateLimit(auth.ExtractKey(previousRequest), retryErr.RetryAfter, now)
			} else {
				cfg.APIKey = pool.currentKey(now)
			}
			if err := auth.Apply(reqClone, cfg); err != nil {
				slog.Warn("failed to re-apply auth on retry", "provider", w.providerName, "error", err)
			}
		}
	}

	return w
}

func (w *HTTPClientWrapper) GetHTTPTimeout() time.Duration {
	if w.Config.HTTPTimeout != nil {
		return *w.Config.HTTPTimeout
	}
	if w.Config.Timeout == 0 {
		return 0
	}
	if w.Config.Timeout > 0 {
		return time.Duration(w.Config.Timeout) * time.Second
	}
	return config.GetDefaultHTTPTimeout()
}

func (w *HTTPClientWrapper) GetHTTPClient() *http.Client {
	if w.httpClient != nil {
		return w.httpClient
	}
	return w.transportCache.newSecureHTTPClient(0, w.tlsConfig, nil, "")
}

func (w *HTTPClientWrapper) DoRequest(ctx context.Context, method, url string, body any, result any) error {
	reqCtx, cancel := w.requestContext(ctx)
	defer cancel()

	req, err := w.buildRequest(reqCtx, method, url, body)
	if err != nil {
		return err
	}

	resp, err := w.retryClient.Do(req)
	if err != nil {
		return w.handleRequestError(ctx, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("failed to close response body", "error", err)
		}
	}()

	respBody, err := readResponseBodyLimited(resp.Body)
	if err != nil {
		return types.Errorf("read response body", err)
	}
	defer returnResponseBuf(respBody)

	if resp.StatusCode >= 400 {
		return w.buildErrorResponse(resp.StatusCode, resp.Status, url, resp.Header, respBody)
	}

	return w.parseResponse(respBody, result)
}

func (w *HTTPClientWrapper) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := w.GetHTTPTimeout()
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// RequestContext applies the wrapper's configured per-request timeout to ctx.
// The returned cancel function must be called when the request body is fully
// consumed or the request fails.
func (w *HTTPClientWrapper) RequestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return w.requestContext(ctx)
}

func (w *HTTPClientWrapper) StreamRequest(ctx context.Context, method, url string, body any) (io.ReadCloser, error) {
	reqCtx, cancel := w.requestContext(ctx)
	req, err := w.buildRequest(reqCtx, method, url, body)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set(types.HeaderAccept, types.ContentTypeEventStream)
	req.Header.Set(types.HeaderCacheControl, "no-cache")

	resp, err := w.retryClient.Do(req)
	if err != nil {
		cancel()
		return nil, w.handleRequestError(ctx, err)
	}

	if resp.StatusCode >= 400 {
		defer cancel()
		defer func() { _ = resp.Body.Close() }()
		respBody, err := readResponseBodyLimited(resp.Body)
		if err != nil {
			return nil, types.Errorf("read response body", err)
		}
		defer returnResponseBuf(respBody)
		return nil, w.buildErrorResponse(resp.StatusCode, resp.Status, url, resp.Header, respBody)
	}

	return &cancelOnCloseReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
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

	if err := w.authStrategy.Apply(req, w.authConfig()); err != nil {
		return err
	}

	for k, v := range w.Config.Headers {
		req.Header.Set(k, v)
	}

	return nil
}

func (w *HTTPClientWrapper) authConfig() types.ProviderConfig {
	cfg := w.Config
	if w.keyPool != nil {
		cfg.APIKey = w.keyPool.currentKey(time.Now())
	}
	return cfg
}

func (w *HTTPClientWrapper) handleRequestError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var retryErr *retryableError
	if errors.As(err, &retryErr) && retryErr.StatusCode > 0 {
		details := err.Error()
		// If the retry layer preserved the provider error body, fold its structured
		// type/code and raw payload into Details so ClassifyError can distinguish
		// e.g. insufficient_quota / RESOURCE_EXHAUSTED from a generic rate limit
		// even after retries are exhausted (the body is dropped otherwise).
		if len(retryErr.Body) > 0 {
			if typeCode := extractErrorTypeCode(retryErr.Body); typeCode != "" {
				details = typeCode + "\n" + details
			}
			details = details + "\nResponse: " + string(retryErr.Body)
		}
		wormholeErr := types.NewWormholeError(
			w.mapHTTPStatusToErrorCode(retryErr.StatusCode),
			fmt.Sprintf("HTTP %d after retries", retryErr.StatusCode),
			retryErr.ShouldRetry,
		).WithDetails(details)
		wormholeErr.StatusCode = retryErr.StatusCode
		wormholeErr.Provider = w.providerName
		if retryErr.RetryAfter > 0 {
			wormholeErr = wormholeErr.WithRetryAfter(retryErr.RetryAfter)
		}
		return wormholeErr
	}

	if w.isTimeoutError(err) {
		wormholeErr := types.NewWormholeError(types.ErrorCodeTimeout, "request timeout", true)
		wormholeErr.Provider = w.providerName
		return wormholeErr
	}

	return types.WrapProviderError(w.providerName, types.ErrorCodeNetwork, "request failed", err)
}

func (w *HTTPClientWrapper) buildErrorResponse(statusCode int, status, url string, header http.Header, respBody []byte) error {
	errorCode := w.mapHTTPStatusToErrorCode(statusCode)
	errorMessage := w.extractErrorMessage(statusCode, status, respBody)

	details := fmt.Sprintf("URL: %s\nResponse: %s", w.maskAPIKeyInURL(url), string(respBody))
	if typeCode := extractErrorTypeCode(respBody); typeCode != "" {
		details = typeCode + "\n" + details
	}

	wormholeErr := types.NewWormholeError(
		errorCode,
		errorMessage,
		isRetryableStatusCode(statusCode),
	).WithDetails(details)

	wormholeErr.StatusCode = statusCode
	wormholeErr.Provider = w.providerName
	if d := types.ParseRetryAfterHeader(header, time.Now()); d > 0 {
		wormholeErr = wormholeErr.WithRetryAfter(d)
	}
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

// extractErrorTypeCode pulls the provider's structured error type/code/status
// from the error body so the classifier (ClassifyError) can distinguish e.g.
// an OpenAI 429 "insufficient_quota" (quota cap, non-retryable) from a plain
// rate-limit 429. Handles the three provider shapes:
//
//	OpenAI:    {"error":{"type":...,"code":...}}
//	Anthropic: {"type":"error","error":{"type":...}}
//	Gemini:    {"error":{"code":...,"status":...}}
//
// Returns "" when nothing structured is present.
func extractErrorTypeCode(respBody []byte) string {
	if len(respBody) == 0 {
		return ""
	}
	var errorResp map[string]any
	if err := json.Unmarshal(respBody, &errorResp); err != nil {
		return ""
	}

	var parts []string
	add := func(label string, v any) {
		switch s := v.(type) {
		case string:
			if s != "" {
				parts = append(parts, label+"="+s)
			}
		case float64:
			parts = append(parts, fmt.Sprintf("%s=%v", label, s))
		}
	}

	if errorObj, ok := errorResp["error"].(map[string]any); ok {
		add("type", errorObj["type"])
		add("code", errorObj["code"])
		add("status", errorObj["status"])
	}
	// Anthropic carries a top-level "type":"error"; only surface it when the
	// nested error object did not already provide a type.
	if !strings.Contains(strings.Join(parts, " "), "type=") {
		if topType, ok := errorResp["type"].(string); ok && topType != "" && topType != "error" {
			parts = append(parts, "type="+topType)
		}
	}

	return strings.Join(parts, " ")
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
