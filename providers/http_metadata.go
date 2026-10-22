package providers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

var requestIDHeaders = []string{"x-request-id", "request-id", "anthropic-request-id", "x-goog-request-id", "x-amzn-requestid", "x-amz-request-id"}

// SafeResponseHeaders filters headers to the fixed public proxy allowlist.
// SDK-configured additional headers are deliberately excluded.
func SafeResponseHeaders(header http.Header) http.Header {
	return FilterResponseHeaders(header, nil)
}

// FilterResponseHeaders returns detached safe response headers. Extra names are
// matched case-insensitively; credentials and hop-by-hop fields remain excluded.
func FilterResponseHeaders(header http.Header, additional []string) http.Header {
	allowed := make(map[string]bool)
	for _, name := range requestIDHeaders {
		allowed[name] = true
	}
	allowed["retry-after"] = true
	for _, prefix := range []string{"x-ratelimit-", "anthropic-ratelimit-"} {
		for _, resource := range []string{"requests", "tokens", "input-tokens", "output-tokens"} {
			for _, measure := range []string{"limit", "remaining", "reset"} {
				if prefix == "x-ratelimit-" {
					allowed[prefix+measure+"-"+resource] = true
				} else {
					allowed[prefix+resource+"-"+measure] = true
				}
			}
		}
	}
	for _, name := range additional {
		allowed[strings.ToLower(strings.TrimSpace(name))] = true
	}
	blocked := map[string]bool{"connection": true, "proxy-connection": true, "keep-alive": true, "te": true, "trailer": true, "transfer-encoding": true, "upgrade": true}
	for name, values := range header {
		if strings.EqualFold(name, "connection") {
			for _, value := range values {
				for token := range strings.SplitSeq(value, ",") {
					blocked[strings.ToLower(strings.TrimSpace(token))] = true
				}
			}
		}
	}
	result := make(http.Header)
	for name, values := range header {
		lower := strings.ToLower(name)
		if !allowed[lower] || blocked[lower] || credentialHeader(lower) {
			continue
		}
		canonical := http.CanonicalHeaderKey(name)
		result[canonical] = append(result[canonical], slices.Clone(values)...)
	}
	return result
}

func credentialHeader(name string) bool {
	return strings.Contains(name, "authorization") || strings.Contains(name, "cookie") ||
		strings.Contains(name, "api-key") || strings.Contains(name, "apikey") || strings.Contains(name, "secret") ||
		name == "x-auth-token" || name == "x-access-token" || name == "authentication-info" || name == "proxy-authentication-info"
}

func (w *HTTPClientWrapper) responseMetadata(status int, header http.Header) *types.ResponseMetadata {
	metadata := &types.ResponseMetadata{Provider: w.providerName, StatusCode: status, Headers: FilterResponseHeaders(header, w.Config.ResponseHeaderNames)}
	for _, name := range requestIDHeaders {
		if value := http.Header(metadata.Headers).Get(name); value != "" {
			metadata.RequestID = value
			break
		}
	}
	return metadata
}

// WithResponseMetadata attaches a detached response snapshot without mutating an
// existing error. It preserves errors.Is/errors.As through the original cause.
func WithResponseMetadata(err error, metadata *types.ResponseMetadata) error {
	if err == nil || metadata == nil {
		return err
	}
	var original *types.WormholeError
	if errors.As(err, &original) {
		copy := *original
		if copy.StatusCode < http.StatusBadRequest || copy.StatusCode > 599 {
			copy.StatusCode = 0
		}
		copy.ResponseMetadata = types.CloneResponseMetadata(metadata)
		if err != original {
			copy.Cause = err
		}
		return &copy
	}
	wrapped := types.WrapError(types.ErrorCodeProvider, err.Error(), false, err)
	wrapped.Provider = metadata.Provider
	if metadata.StatusCode >= http.StatusBadRequest && metadata.StatusCode <= 599 {
		wrapped.StatusCode = metadata.StatusCode
	}
	wrapped.ResponseMetadata = types.CloneResponseMetadata(metadata)
	return wrapped
}

func (w *HTTPClientWrapper) requestErrorWithMetadata(ctx context.Context, err error) (*types.ResponseMetadata, error) {
	var retryErr *retryableError
	var metadata *types.ResponseMetadata
	if errors.As(err, &retryErr) && retryErr.StatusCode > 0 {
		metadata = w.responseMetadata(retryErr.StatusCode, retryErr.Headers)
	}
	return metadata, WithResponseMetadata(w.handleRequestError(ctx, err), metadata)
}
