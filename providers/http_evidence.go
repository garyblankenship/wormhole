package providers

import (
	"bytes"
	"context"
	"net/http"

	"github.com/garyblankenship/wormhole/v3/types"
)

type injectedRoundTripper struct{ client HTTPClient }

func (r injectedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.client.Do(req)
}

// DoRequestOnceWithBody performs one client invocation, without provider retries,
// returning an owned, bounded, complete body and response metadata. A read error
// or overflow returns no body. HTTP status errors may accompany a complete body.
func (w *HTTPClientWrapper) DoRequestOnceWithBody(ctx context.Context, method, url string, body any) ([]byte, *types.ResponseMetadata, error) {
	reqCtx, cancel := w.requestContext(ctx)
	defer cancel()
	req, err := w.buildRequest(reqCtx, method, url, body)
	if err != nil {
		return nil, nil, err
	}
	resp, err := w.client.Do(req)
	if err != nil {
		metadata, requestErr := w.requestErrorWithMetadata(ctx, err)
		return nil, metadata, requestErr
	}
	defer func() { _ = resp.Body.Close() }()
	metadata := w.responseMetadata(resp.StatusCode, resp.Header)
	data, err := readResponseBodyLimited(resp.Body)
	if err != nil {
		return nil, metadata, WithResponseMetadata(types.Errorf("read response body", err), metadata)
	}
	owned := bytes.Clone(data)
	returnResponseBuf(data)
	if resp.StatusCode >= 400 {
		return owned, metadata, WithResponseMetadata(w.buildErrorResponse(resp.StatusCode, resp.Status, url, resp.Header, owned), metadata)
	}
	return owned, metadata, nil
}
