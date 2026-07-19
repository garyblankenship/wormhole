package providers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

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
