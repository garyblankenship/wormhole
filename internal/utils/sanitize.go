package utils

import (
	"errors"
	"regexp"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// SanitizationLevel defines the strictness of error sanitization
type SanitizationLevel string

const (
	// SanitizeNone performs no sanitization (default)
	SanitizeNone SanitizationLevel = "none"
	// SanitizeBasic masks API keys, tokens, and internal URLs in error messages
	SanitizeBasic SanitizationLevel = "basic"
	// SanitizeStrict also replaces detailed error messages with generic ones
	// and removes all potentially sensitive information
	SanitizeStrict SanitizationLevel = "strict"
)

// ErrorSanitizerConfig holds configuration for error sanitization
type ErrorSanitizerConfig struct {
	// Level determines how aggressive the sanitization should be
	Level SanitizationLevel
	// CustomPatterns allows adding custom regex patterns for sensitive data
	CustomPatterns []*regexp.Regexp
}

// DefaultErrorSanitizerConfig returns the default sanitization configuration
func DefaultErrorSanitizerConfig() ErrorSanitizerConfig {
	return ErrorSanitizerConfig{
		Level: SanitizeNone,
	}
}

// patternReplacement defines a regex pattern and how to replace matches
type patternReplacement struct {
	pattern *regexp.Regexp
	// replaceFunc takes the match and returns the replacement
	replaceFunc func(string) string
}

// sensitivePatterns defines regex patterns for sensitive data with replacement logic
var sensitivePatterns = []patternReplacement{
	// API keys - preserve prefix like "sk-", mask the key part
	{
		regexp.MustCompile(`(sk-|pk-)([A-Za-z0-9\-_]{10,})`),
		func(match string) string {
			// For "sk-1234567890abcdef", preserve "sk-" and mask the rest
			if len(match) <= 7 { // "sk-" + at least 4 chars to mask
				return "****"
			}
			// Keep first 3 chars ("sk-") + first 2 of key + **** + last 4
			// This gives "sk-12****cdef" for a typical key
			prefix := match[:3] // "sk-"
			keyPart := match[3:]
			if len(keyPart) <= 8 {
				return prefix + "****"
			}
			return prefix + keyPart[:2] + "****" + keyPart[len(keyPart)-4:]
		},
	},
	// Bearer tokens - preserve "Bearer " prefix, mask token
	{
		regexp.MustCompile(`(Bearer\s+)([A-Za-z0-9\-_\.=+/:]{10,})`),
		func(match string) string {
			// Find the "Bearer " part
			idx := strings.Index(match, "Bearer ")
			if idx == -1 {
				return maskSensitiveData(match)
			}
			prefix := match[:idx+7] // "Bearer "
			token := match[idx+7:]
			return prefix + maskSensitiveData(token)
		},
	},
	// URL query parameters - preserve param name, mask value
	{
		regexp.MustCompile(`(api[_-]?key|access[_-]?token|token|key|secret)=([^&\s]+)`),
		func(match string) string {
			// Split on "="
			parts := strings.SplitN(match, "=", 2)
			if len(parts) != 2 {
				return maskSensitiveData(match)
			}
			paramName, value := parts[0], parts[1]

			// Check if value looks like an API key
			if strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "pk-") {
				// Use API key masking logic
				if len(value) <= 7 {
					return paramName + "=****"
				}
				prefix := value[:3] // "sk-"
				keyPart := value[3:]
				if len(keyPart) <= 8 {
					return paramName + "=" + prefix + "****"
				}
				return paramName + "=" + prefix + keyPart[:2] + "****" + keyPart[len(keyPart)-4:]
			}

			// Default masking
			return paramName + "=" + maskSensitiveData(value)
		},
	},
	// Internal IPs - mask the entire IP/host with ****
	{
		regexp.MustCompile(`((https?://)?)(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|127\.|localhost)([^/\s]*)`),
		func(match string) string {
			return "****"
		},
	},
	// Email addresses - mask local part
	{
		regexp.MustCompile(`([a-zA-Z0-9._%+-]+)@([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`),
		func(match string) string {
			parts := strings.Split(match, "@")
			if len(parts) != 2 {
				return maskSensitiveData(match)
			}
			return maskSensitiveData(parts[0]) + "@" + parts[1]
		},
	},
}

// SanitizeError sanitizes sensitive information from an error based on the configuration
// It preserves the error chain and returns a new error with sanitized messages
func SanitizeError(err error, config ErrorSanitizerConfig) error {
	if config.Level == SanitizeNone || err == nil {
		return err
	}

	// Handle WormholeError specifically
	if wormholeErr, ok := types.AsWormholeError(err); ok {
		return sanitizeWormholeError(wormholeErr, config)
	}

	// Handle generic errors
	return sanitizeGenericError(err, config)
}

// sanitizeWormholeError sanitizes a WormholeError
func sanitizeWormholeError(err *types.WormholeError, config ErrorSanitizerConfig) error {
	sanitizedErr := *err // Make a copy

	// Always sanitize details field if present
	if sanitizedErr.Details != "" {
		sanitizedErr.Details = sanitizeString(sanitizedErr.Details, config)
	}

	switch config.Level {
	case SanitizeBasic:
		// Basic level: sanitize message and details
		sanitizedErr.Message = sanitizeString(sanitizedErr.Message, config)

	case SanitizeStrict:
		// Strict level: replace detailed messages with generic ones
		sanitizedErr.Message = genericErrorMessage(sanitizedErr.Code)
		// Clear details in strict mode
		sanitizedErr.Details = ""
	}

	// Recursively sanitize the cause
	if sanitizedErr.Cause != nil {
		sanitizedErr.Cause = SanitizeError(sanitizedErr.Cause, config)
	}

	return &sanitizedErr
}

// sanitizeGenericError sanitizes a generic error
func sanitizeGenericError(err error, config ErrorSanitizerConfig) error {
	// Extract the error string
	errStr := err.Error()

	if config.Level == SanitizeStrict {
		// In strict mode, replace with a generic message
		return errors.New("an error occurred")
	}

	// In basic mode, sanitize the error string
	sanitizedStr := sanitizeString(errStr, config)
	return errors.New(sanitizedStr)
}

// sanitizeString applies sanitization patterns to a string
func sanitizeString(s string, config ErrorSanitizerConfig) string {
	result := s

	// Apply built-in patterns
	for _, pr := range sensitivePatterns {
		result = pr.pattern.ReplaceAllStringFunc(result, pr.replaceFunc)
	}

	// Apply custom patterns if provided
	for _, pattern := range config.CustomPatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			return maskSensitiveData(match)
		})
	}

	return result
}

// maskSensitiveData masks sensitive data with asterisks
func maskSensitiveData(data string) string {
	if len(data) <= 8 {
		return "****"
	}

	// For longer strings, show first 4 and last 4 characters
	return data[:4] + "****" + data[len(data)-4:]
}

// genericErrorMessage returns a generic error message for strict mode
func genericErrorMessage(code types.ErrorCode) string {
	switch code {
	case types.ErrorCodeAuth:
		return "authentication failed"
	case types.ErrorCodeModel:
		return "model error"
	case types.ErrorCodeRateLimit:
		return "rate limit exceeded"
	case types.ErrorCodeRequest:
		return "invalid request"
	case types.ErrorCodeTimeout:
		return "request timeout"
	case types.ErrorCodeProvider:
		return "provider error"
	case types.ErrorCodeNetwork:
		return "network error"
	case types.ErrorCodeValidation:
		return "validation error"
	case types.ErrorCodeMiddleware:
		return "middleware error"
	default:
		return "an error occurred"
	}
}

// MaskAPIKeyInString masks API keys in any string (not just URLs)
// This is a more general version of the provider-specific function
func MaskAPIKeyInString(s string) string {
	result := s

	// Use a simplified version of our sensitive patterns
	patterns := []patternReplacement{
		// API keys
		{
			regexp.MustCompile(`(sk-|pk-)([A-Za-z0-9\-_]{10,})`),
			func(match string) string {
				if len(match) <= 7 {
					return "****"
				}
				prefix := match[:3] // "sk-"
				keyPart := match[3:]
				if len(keyPart) <= 8 {
					return prefix + "****"
				}
				return prefix + keyPart[:2] + "****" + keyPart[len(keyPart)-4:]
			},
		},
		// Bearer tokens
		{
			regexp.MustCompile(`(Bearer\s+)([A-Za-z0-9\-_\.=+/:]{10,})`),
			func(match string) string {
				idx := strings.Index(match, "Bearer ")
				if idx == -1 {
					return maskSensitiveData(match)
				}
				prefix := match[:idx+7]
				token := match[idx+7:]
				return prefix + maskSensitiveData(token)
			},
		},
		// Query parameters
		{
			regexp.MustCompile(`(api[_-]?key|access[_-]?token|token|key)=([^"'\s&]+)`),
			func(match string) string {
				parts := strings.SplitN(match, "=", 2)
				if len(parts) != 2 {
					return maskSensitiveData(match)
				}
				paramName, value := parts[0], parts[1]

				// Check if value looks like an API key
				if strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "pk-") {
					if len(value) <= 7 {
						return paramName + "=****"
					}
					prefix := value[:3]
					keyPart := value[3:]
					if len(keyPart) <= 8 {
						return paramName + "=" + prefix + "****"
					}
					return paramName + "=" + prefix + keyPart[:2] + "****" + keyPart[len(keyPart)-4:]
				}

				return paramName + "=" + maskSensitiveData(value)
			},
		},
	}

	for _, pr := range patterns {
		result = pr.pattern.ReplaceAllStringFunc(result, pr.replaceFunc)
	}

	return result
}

// MaskURL masks sensitive information in URLs
func MaskURL(url string) string {
	result := MaskAPIKeyInString(url)

	// Also mask internal IPs/hosts
	internalIPPattern := regexp.MustCompile(`((https?://)?)(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|127\.|localhost)([^/\s]*)`)
	result = internalIPPattern.ReplaceAllStringFunc(result, func(match string) string {
		return "****"
	})

	return result
}

// SanitizeErrorWithDefaults sanitizes an error using default configuration
func SanitizeErrorWithDefaults(err error) error {
	return SanitizeError(err, DefaultErrorSanitizerConfig())
}