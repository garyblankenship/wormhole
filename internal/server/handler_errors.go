package server

import (
	"net/http"

	"github.com/garyblankenship/wormhole/v2/types"
)

// responseFormatUnsupported reports whether the proxy must reject response_format
// for a provider rather than pass it through. Anthropic and Gemini never read
// ResponseFormat on the text path (they drive structured output through separate
// mechanisms), and native Ollama's text path only accepts a narrow shape — so a
// raw passthrough would silently yield unstructured output. OpenAI and all
// OpenAI-Chat-compatible providers handle it correctly.
func responseFormatUnsupported(provider string) bool {
	switch provider {
	case "anthropic", "gemini", "ollama":
		return true
	default:
		return false
	}
}

// upstreamErrorStatus maps a provider error to an OpenAI-style HTTP status and
// error type. When err carries a *types.WormholeError (via errors.As), its
// StatusCode and Code drive the response so clients can distinguish a 429 rate
// limit from a 400 bad request from a 401 auth failure. Falls back to 502
// (bad gateway) + "api_error" when no structured error is present.
func upstreamErrorStatus(err error) (int, string, string) {
	whErr, ok := types.AsWormholeError(err)
	if !ok {
		return http.StatusBadGateway, "api_error", "upstream provider error"
	}

	errType := wormholeErrorType(whErr.Code)
	if whErr.StatusCode != 0 {
		return whErr.StatusCode, errType, upstreamClientMessage(errType)
	}
	// No upstream status (SDK-internal error). Map by code to the semantically
	// correct HTTP status instead of defaulting everything to 502 bad gateway.
	switch whErr.Code {
	case types.ErrorCodeAuth:
		return http.StatusUnauthorized, errType, upstreamClientMessage(errType)
	case types.ErrorCodeRateLimit:
		return http.StatusTooManyRequests, errType, upstreamClientMessage(errType)
	case types.ErrorCodeTimeout:
		return http.StatusGatewayTimeout, errType, upstreamClientMessage(errType)
	case types.ErrorCodeModel, types.ErrorCodeRequest, types.ErrorCodeValidation:
		return http.StatusBadRequest, errType, actionableInvalidRequestMessage(whErr)
	default:
		return http.StatusBadGateway, errType, upstreamClientMessage(errType)
	}
}

func upstreamClientMessage(errType string) string {
	switch errType {
	case "authentication_error":
		return "upstream authentication failed"
	case "rate_limit_error":
		return "upstream rate limit exceeded"
	case "invalid_request_error":
		return "upstream request rejected"
	default:
		return "upstream provider error"
	}
}

// wormholeErrorType maps a WormholeError code to an OpenAI-style error type string.
func wormholeErrorType(code types.ErrorCode) string {
	switch code {
	case types.ErrorCodeAuth:
		return "authentication_error"
	case types.ErrorCodeRateLimit:
		return "rate_limit_error"
	case types.ErrorCodeModel, types.ErrorCodeRequest, types.ErrorCodeValidation:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

func actionableInvalidRequestMessage(err *types.WormholeError) string {
	if err == nil {
		return "upstream request rejected"
	}
	switch {
	case err.Message == "" && err.Details == "":
		return "upstream request rejected"
	case err.Message == "":
		return err.Details
	case err.Details == "":
		return err.Message
	default:
		return err.Message + ": " + err.Details
	}
}
