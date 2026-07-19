package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
