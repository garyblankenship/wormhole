package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

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

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, types.Errorf("marshal request body", err)
	}
	return payload, nil
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
