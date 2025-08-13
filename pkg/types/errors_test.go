package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWormholeError_Creation(t *testing.T) {
	t.Run("basic creation", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "test message", true)

		assert.Equal(t, ErrorCodeAuth, err.Code)
		assert.Equal(t, "test message", err.Message)
		assert.True(t, err.Retryable)
		assert.Equal(t, "", err.Provider)
		assert.Equal(t, "", err.Model)
		assert.Equal(t, "", err.Details)
		assert.Equal(t, 0, err.StatusCode)
		assert.Nil(t, err.Cause)
	})

	t.Run("with cause", func(t *testing.T) {
		originalErr := errors.New("original error")
		err := WrapError(ErrorCodeNetwork, "wrapped message", true, originalErr)

		assert.Equal(t, ErrorCodeNetwork, err.Code)
		assert.Equal(t, "wrapped message", err.Message)
		assert.True(t, err.Retryable)
		assert.Equal(t, originalErr, err.Cause)
	})
}

func TestWormholeError_ErrorInterface(t *testing.T) {
	t.Run("basic error message", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "authentication failed", false)
		expected := "AUTH_ERROR: authentication failed"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("error message with details", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "authentication failed", false).
			WithDetails("invalid API key format")
		expected := "AUTH_ERROR: authentication failed (invalid API key format)"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("unwrap returns cause", func(t *testing.T) {
		originalErr := errors.New("network timeout")
		err := WrapError(ErrorCodeNetwork, "request failed", true, originalErr)

		assert.Equal(t, originalErr, err.Unwrap())
	})

	t.Run("unwrap returns nil when no cause", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "auth failed", false)
		assert.Nil(t, err.Unwrap())
	})
}

func TestWormholeError_Chaining(t *testing.T) {
	t.Run("with provider", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeModel, "model error", false).
			WithProvider("openai")

		assert.Equal(t, "openai", err.Provider)
		assert.Equal(t, ErrorCodeModel, err.Code) // Original error unchanged
	})

	t.Run("with model", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeModel, "model error", false).
			WithModel("gpt-5")

		assert.Equal(t, "gpt-5", err.Model)
	})

	t.Run("with details", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeRequest, "bad request", false).
			WithDetails("missing required field 'model'")

		assert.Equal(t, "missing required field 'model'", err.Details)
	})

	t.Run("with status code", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "unauthorized", false).
			WithStatusCode(401)

		assert.Equal(t, 401, err.StatusCode)
	})

	t.Run("with cause", func(t *testing.T) {
		originalErr := errors.New("connection refused")
		err := NewWormholeError(ErrorCodeNetwork, "network error", true).
			WithCause(originalErr)

		assert.Equal(t, originalErr, err.Cause)
	})

	t.Run("chaining multiple methods", func(t *testing.T) {
		originalErr := errors.New("timeout")
		err := NewWormholeError(ErrorCodeTimeout, "request timeout", true).
			WithProvider("anthropic").
			WithModel("claude-3-opus").
			WithStatusCode(408).
			WithDetails("request took longer than 30s").
			WithCause(originalErr)

		assert.Equal(t, ErrorCodeTimeout, err.Code)
		assert.Equal(t, "anthropic", err.Provider)
		assert.Equal(t, "claude-3-opus", err.Model)
		assert.Equal(t, 408, err.StatusCode)
		assert.Equal(t, "request took longer than 30s", err.Details)
		assert.Equal(t, originalErr, err.Cause)
		assert.True(t, err.Retryable)
	})

	t.Run("chaining creates new instances", func(t *testing.T) {
		original := NewWormholeError(ErrorCodeAuth, "auth error", false)
		modified := original.WithProvider("openai")

		// Original should be unchanged
		assert.Equal(t, "", original.Provider)
		// Modified should have the new value
		assert.Equal(t, "openai", modified.Provider)
		// They should be different instances
		assert.NotSame(t, original, modified)
	})
}

func TestWormholeError_IsRetryable(t *testing.T) {
	t.Run("retryable error", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeRateLimit, "rate limited", true)
		assert.True(t, err.IsRetryable())
	})

	t.Run("non-retryable error", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "invalid key", false)
		assert.False(t, err.IsRetryable())
	})
}

func TestPredefinedErrors(t *testing.T) {
	testCases := []struct {
		name      string
		err       *WormholeError
		code      ErrorCode
		retryable bool
	}{
		{"ErrInvalidAPIKey", ErrInvalidAPIKey, ErrorCodeAuth, true},
		{"ErrMissingAPIKey", ErrMissingAPIKey, ErrorCodeAuth, false},
		{"ErrModelNotFound", ErrModelNotFound, ErrorCodeModel, false},
		{"ErrModelNotSupported", ErrModelNotSupported, ErrorCodeModel, false},
		{"ErrInvalidModel", ErrInvalidModel, ErrorCodeModel, false},
		{"ErrRateLimited", ErrRateLimited, ErrorCodeRateLimit, true},
		{"ErrQuotaExceeded", ErrQuotaExceeded, ErrorCodeRateLimit, false},
		{"ErrInvalidRequest", ErrInvalidRequest, ErrorCodeRequest, false},
		{"ErrRequestTooLarge", ErrRequestTooLarge, ErrorCodeRequest, false},
		{"ErrTimeout", ErrTimeout, ErrorCodeTimeout, true},
		{"ErrProviderNotFound", ErrProviderNotFound, ErrorCodeProvider, false},
		{"ErrProviderUnavailable", ErrProviderUnavailable, ErrorCodeProvider, true},
		{"ErrProviderConstraintError", ErrProviderConstraintError, ErrorCodeProvider, false},
		{"ErrNetworkError", ErrNetworkError, ErrorCodeNetwork, true},
		{"ErrServiceUnavailable", ErrServiceUnavailable, ErrorCodeNetwork, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, tc.err.Code)
			assert.Equal(t, tc.retryable, tc.err.Retryable)
			assert.NotEmpty(t, tc.err.Message)
		})
	}
}

func TestHTTPStatusToError(t *testing.T) {
	testCases := []struct {
		status       int
		expectedCode ErrorCode
		retryable    bool
	}{
		{http.StatusUnauthorized, ErrorCodeAuth, true},
		{http.StatusForbidden, ErrorCodeRateLimit, false},
		{http.StatusNotFound, ErrorCodeModel, false},
		{http.StatusTooManyRequests, ErrorCodeRateLimit, true},
		{http.StatusBadRequest, ErrorCodeRequest, false},
		{http.StatusRequestEntityTooLarge, ErrorCodeRequest, false},
		{http.StatusRequestTimeout, ErrorCodeTimeout, true},
		{http.StatusInternalServerError, ErrorCodeNetwork, true},
		{http.StatusBadGateway, ErrorCodeNetwork, true},
		{http.StatusServiceUnavailable, ErrorCodeNetwork, true},
		{http.StatusGatewayTimeout, ErrorCodeNetwork, true},
		{418, ErrorCodeUnknown, false}, // I'm a teapot
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			body := "error response body"
			err := HTTPStatusToError(tc.status, body)

			assert.Equal(t, tc.expectedCode, err.Code)
			assert.Equal(t, tc.retryable, err.Retryable)
			assert.Equal(t, tc.status, err.StatusCode)
			assert.Equal(t, body, err.Details)
		})
	}
}

func TestErrorTypeChecking(t *testing.T) {
	t.Run("IsWormholeError", func(t *testing.T) {
		wormholeErr := NewWormholeError(ErrorCodeAuth, "auth error", false)
		regularErr := errors.New("regular error")

		assert.True(t, IsWormholeError(wormholeErr))
		assert.False(t, IsWormholeError(regularErr))
		assert.False(t, IsWormholeError(nil))
	})

	t.Run("AsWormholeError", func(t *testing.T) {
		wormholeErr := NewWormholeError(ErrorCodeAuth, "auth error", false)
		regularErr := errors.New("regular error")

		// Should extract WormholeError
		extracted, ok := AsWormholeError(wormholeErr)
		assert.True(t, ok)
		assert.Equal(t, wormholeErr, extracted)

		// Should fail for regular error
		extracted, ok = AsWormholeError(regularErr)
		assert.False(t, ok)
		assert.Nil(t, extracted)

		// Should fail for nil
		extracted, ok = AsWormholeError(nil)
		assert.False(t, ok)
		assert.Nil(t, extracted)
	})

	t.Run("IsRetryableError", func(t *testing.T) {
		retryableErr := NewWormholeError(ErrorCodeRateLimit, "rate limited", true)
		nonRetryableErr := NewWormholeError(ErrorCodeAuth, "auth failed", false)
		regularErr := errors.New("regular error")

		assert.True(t, IsRetryableError(retryableErr))
		assert.False(t, IsRetryableError(nonRetryableErr))
		assert.False(t, IsRetryableError(regularErr))
		assert.False(t, IsRetryableError(nil))
	})
}

func TestModelConstraintError(t *testing.T) {
	t.Run("creation and properties", func(t *testing.T) {
		err := NewModelConstraintError("gpt-5", "temperature", 1.0, 0.7)

		assert.Equal(t, ErrorCodeProvider, err.Code)
		assert.Equal(t, "gpt-5", err.Model)
		assert.Equal(t, "temperature", err.Constraint)
		assert.Equal(t, 1.0, err.Expected)
		assert.Equal(t, 0.7, err.Actual)
		assert.False(t, err.Retryable)
		assert.Contains(t, err.Details, "constraint 'temperature' violated")
		assert.Contains(t, err.Details, "expected 1")
		assert.Contains(t, err.Details, "got 0.7")
	})

	t.Run("error message", func(t *testing.T) {
		err := NewModelConstraintError("gpt-5", "max_tokens", 4096, 8192)
		errMsg := err.Error()

		assert.Contains(t, errMsg, "PROVIDER_ERROR")
		assert.Contains(t, errMsg, "constraint violation")
		assert.Contains(t, errMsg, "max_tokens")
	})

	t.Run("is WormholeError", func(t *testing.T) {
		err := NewModelConstraintError("gpt-5", "temperature", 1.0, 0.5)

		assert.True(t, IsWormholeError(err))

		extracted, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.NotNil(t, extracted)
	})
}

func TestWormholeError_JSONSerialization(t *testing.T) {
	t.Run("marshal to JSON", func(t *testing.T) {
		err := NewWormholeError(ErrorCodeAuth, "authentication failed", true).
			WithProvider("openai").
			WithModel("gpt-5").
			WithStatusCode(401).
			WithDetails("invalid API key")

		data, jsonErr := json.Marshal(err)
		require.NoError(t, jsonErr)

		var result map[string]interface{}
		jsonErr = json.Unmarshal(data, &result)
		require.NoError(t, jsonErr)

		assert.Equal(t, "AUTH_ERROR", result["code"])
		assert.Equal(t, "authentication failed", result["message"])
		assert.Equal(t, true, result["retryable"])
		assert.Equal(t, "openai", result["provider"])
		assert.Equal(t, "gpt-5", result["model"])
		assert.Equal(t, "invalid API key", result["details"])
		assert.Equal(t, float64(401), result["status_code"])

		// Cause should not be in JSON (json:"-" tag)
		_, exists := result["Cause"]
		assert.False(t, exists)
	})

	t.Run("unmarshal from JSON", func(t *testing.T) {
		jsonData := `{
			"code": "RATE_LIMIT_ERROR",
			"message": "rate limit exceeded",
			"retryable": true,
			"provider": "anthropic",
			"model": "claude-3-opus",
			"details": "60 requests per minute exceeded",
			"status_code": 429
		}`

		var err WormholeError
		jsonErr := json.Unmarshal([]byte(jsonData), &err)
		require.NoError(t, jsonErr)

		assert.Equal(t, ErrorCodeRateLimit, err.Code)
		assert.Equal(t, "rate limit exceeded", err.Message)
		assert.True(t, err.Retryable)
		assert.Equal(t, "anthropic", err.Provider)
		assert.Equal(t, "claude-3-opus", err.Model)
		assert.Equal(t, "60 requests per minute exceeded", err.Details)
		assert.Equal(t, 429, err.StatusCode)
		assert.Nil(t, err.Cause) // Should not be deserialized
	})

	t.Run("round trip JSON", func(t *testing.T) {
		original := NewWormholeError(ErrorCodeTimeout, "request timeout", true).
			WithProvider("groq").
			WithModel("llama-3").
			WithStatusCode(408)

		// Marshal
		data, err := json.Marshal(original)
		require.NoError(t, err)

		// Unmarshal
		var restored WormholeError
		err = json.Unmarshal(data, &restored)
		require.NoError(t, err)

		// Compare (excluding Cause which is not serialized)
		assert.Equal(t, original.Code, restored.Code)
		assert.Equal(t, original.Message, restored.Message)
		assert.Equal(t, original.Retryable, restored.Retryable)
		assert.Equal(t, original.Provider, restored.Provider)
		assert.Equal(t, original.Model, restored.Model)
		assert.Equal(t, original.Details, restored.Details)
		assert.Equal(t, original.StatusCode, restored.StatusCode)
	})
}

func TestErrorCodes(t *testing.T) {
	expectedCodes := []ErrorCode{
		ErrorCodeAuth,
		ErrorCodeModel,
		ErrorCodeRateLimit,
		ErrorCodeRequest,
		ErrorCodeTimeout,
		ErrorCodeProvider,
		ErrorCodeNetwork,
		ErrorCodeUnknown,
	}

	for _, code := range expectedCodes {
		t.Run(string(code), func(t *testing.T) {
			assert.NotEmpty(t, string(code))
			assert.Contains(t, string(code), "_ERROR")
		})
	}
}
