package utils

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		config   ErrorSanitizerConfig
		expected string
	}{
		{
			name: "none level - no sanitization",
			err:  errors.New("error with sk-1234567890abcdef"),
			config: ErrorSanitizerConfig{
				Level: SanitizeNone,
			},
			expected: "error with sk-1234567890abcdef",
		},
		{
			name: "basic level - masks API key",
			err:  errors.New("error with sk-1234567890abcdef"),
			config: ErrorSanitizerConfig{
				Level: SanitizeBasic,
			},
			expected: "error with sk-12****cdef",
		},
		{
			name: "basic level - masks bearer token",
			err:  errors.New("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"),
			config: ErrorSanitizerConfig{
				Level: SanitizeBasic,
			},
			expected: "Bearer eyJh****VCJ9",
		},
		{
			name: "basic level - masks URL with API key",
			err:  errors.New("https://api.openai.com/v1/chat/completions?api_key=sk-1234567890abcdef"),
			config: ErrorSanitizerConfig{
				Level: SanitizeBasic,
			},
			expected: "https://api.openai.com/v1/chat/completions?api_key=sk-12****cdef",
		},
		{
			name: "basic level - masks internal IP",
			err:  errors.New("connection to 10.0.0.1:8080 failed"),
			config: ErrorSanitizerConfig{
				Level: SanitizeBasic,
			},
			expected: "connection to **** failed",
		},
		{
			name: "strict level - generic message for WormholeError",
			err: types.NewWormholeError(
				types.ErrorCodeAuth,
				"Invalid API key: sk-1234567890abcdef",
				false,
			).WithDetails("Failed to authenticate with https://api.openai.com?key=sk-1234567890abcdef"),
			config: ErrorSanitizerConfig{
				Level: SanitizeStrict,
			},
			expected: "AUTH_ERROR: authentication failed",
		},
		{
			name: "strict level - generic message for generic error",
			err: errors.New("detailed error: sk-1234567890abcdef at 10.0.0.1"),
			config: ErrorSanitizerConfig{
				Level: SanitizeStrict,
			},
			expected: "an error occurred",
		},
		{
			name: "basic level - WormholeError with details",
			err: types.NewWormholeError(
				types.ErrorCodeAuth,
				"Invalid API key",
				false,
			).WithDetails("Key: sk-1234567890abcdef, URL: http://10.0.0.1/api"),
			config: ErrorSanitizerConfig{
				Level: SanitizeBasic,
			},
			expected: "AUTH_ERROR: Invalid API key (Key: sk-12****cdef, URL: ****/api)",
		},
		{
			name: "basic level - preserves error chain",
			err: types.WrapError(
				types.ErrorCodeNetwork,
				"Connection failed to http://10.0.0.1",
				true,
				errors.New("dial tcp 10.0.0.1:443: connect: connection refused"),
			),
			config: ErrorSanitizerConfig{
				Level: SanitizeBasic,
			},
			expected: "NETWORK_ERROR: Connection failed to ****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized := SanitizeError(tt.err, tt.config)
			assert.Equal(t, tt.expected, sanitized.Error())
		})
	}
}

func TestMaskAPIKeyInString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "OpenAI API key",
			input:    "sk-1234567890abcdef1234567890abcdef",
			expected: "sk-12****cdef",
		},
		{
			name:     "Bearer token",
			input:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: "Bearer eyJh****sw5c",
		},
		{
			name:     "API key in query param",
			input:    "apikey=sk-1234567890abcdef",
			expected: "apikey=sk-12****cdef",
		},
		{
			name:     "Token in query param",
			input:    "token=abc123def456",
			expected: "token=abc1****f456",
		},
		{
			name:     "Mixed content",
			input:    "Error with key=sk-1234567890abcdef and token=xyz789",
			expected: "Error with key=sk-12****cdef and token=****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskAPIKeyInString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaskURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with API key",
			input:    "https://api.openai.com/v1/chat/completions?api_key=sk-1234567890abcdef&model=gpt-4",
			expected: "https://api.openai.com/v1/chat/completions?api_key=sk-12****cdef&model=gpt-4",
		},
		{
			name:     "URL with token",
			input:    "https://api.example.com/auth?token=abc123def456&action=login",
			expected: "https://api.example.com/auth?token=abc1****f456&action=login",
		},
		{
			name:     "Internal URL",
			input:    "http://10.0.0.1:8080/api/v1/users",
			expected: "****/api/v1/users",
		},
		{
			name:     "Localhost URL",
			input:    "https://localhost:3000/api",
			expected: "****/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeErrorWithDefaults(t *testing.T) {
	t.Run("default config is none", func(t *testing.T) {
		err := errors.New("error with sk-1234567890abcdef")
		sanitized := SanitizeErrorWithDefaults(err)
		assert.Equal(t, "error with sk-1234567890abcdef", sanitized.Error())
	})
}

func TestCustomPatterns(t *testing.T) {
	t.Run("custom pattern for masking phone numbers", func(t *testing.T) {
		phonePattern := regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`)
		config := ErrorSanitizerConfig{
			Level:          SanitizeBasic,
			CustomPatterns: []*regexp.Regexp{phonePattern},
		}

		err := errors.New("Contact support at 555-123-4567")
		sanitized := SanitizeError(err, config)
		assert.Equal(t, "Contact support at 555-****4567", sanitized.Error())
	})
}

func TestGenericErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		code     types.ErrorCode
		expected string
	}{
		{"auth error", types.ErrorCodeAuth, "authentication failed"},
		{"model error", types.ErrorCodeModel, "model error"},
		{"rate limit", types.ErrorCodeRateLimit, "rate limit exceeded"},
		{"request error", types.ErrorCodeRequest, "invalid request"},
		{"timeout", types.ErrorCodeTimeout, "request timeout"},
		{"provider error", types.ErrorCodeProvider, "provider error"},
		{"network error", types.ErrorCodeNetwork, "network error"},
		{"validation error", types.ErrorCodeValidation, "validation error"},
		{"middleware error", types.ErrorCodeMiddleware, "middleware error"},
		{"unknown error", types.ErrorCodeUnknown, "an error occurred"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := genericErrorMessage(tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorChainPreservation(t *testing.T) {
	t.Run("error chain is preserved after sanitization", func(t *testing.T) {
		originalErr := errors.New("original: sk-1234567890abcdef")
		wrappedErr := fmt.Errorf("wrapped: %w", originalErr)
		wormholeErr := types.WrapError(types.ErrorCodeAuth, "auth failed: %w", false, wrappedErr)

		config := ErrorSanitizerConfig{
			Level: SanitizeBasic,
		}

		sanitized := SanitizeError(wormholeErr, config)

		// Check that it's still a WormholeError
		wormholeSanitized, ok := types.AsWormholeError(sanitized)
		assert.True(t, ok)
		assert.Equal(t, types.ErrorCodeAuth, wormholeSanitized.Code)

		// Check that the message was sanitized
		assert.Contains(t, wormholeSanitized.Error(), "auth failed:")
		assert.NotContains(t, wormholeSanitized.Error(), "sk-1234567890abcdef")

		// The API key should be masked in the error chain
		sanitizedStr := sanitized.Error()
		// Check that original key is not present
		assert.NotContains(t, sanitizedStr, "sk-1234567890abcdef")
	})
}

func TestValidationErrorSanitization(t *testing.T) {
	t.Run("validation errors are properly sanitized", func(t *testing.T) {
		validationErr := types.NewValidationError(
			"api_key",
			"invalid",
			"sk-1234567890abcdef",
			"API key is invalid",
		)

		config := ErrorSanitizerConfig{
			Level: SanitizeBasic,
		}

		sanitized := SanitizeError(validationErr, config)
		sanitizedStr := sanitized.Error()

		assert.Contains(t, sanitizedStr, "VALIDATION_ERROR")
		assert.Contains(t, sanitizedStr, "validation failed")
		// Note: The API key value is in the Value field (any type), not in the error message
		// So we can't check for masked version in the string
		// But we should check the original key is not in the message
		assert.NotContains(t, sanitizedStr, "sk-1234567890abcdef")
	})
}