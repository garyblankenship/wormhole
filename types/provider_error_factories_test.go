package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthError_NotRetryable(t *testing.T) {
	t.Parallel()
	err := AuthError("openai", "x")
	we, ok := AsWormholeError(err)
	assert.True(t, ok, "AuthError should be a WormholeError")
	assert.False(t, we.IsRetryable(), "auth errors must not be retryable")

	errFmt := AuthErrorf("openai", "invalid key %s", "key-123")
	weFmt, okFmt := AsWormholeError(errFmt)
	assert.True(t, okFmt)
	assert.Equal(t, "invalid key key-123", weFmt.Message)
}

func TestProviderWrapperError(t *testing.T) {
	t.Parallel()
	err := NewProviderWrapperError("feature missing", "anthropic")
	require.NotNil(t, err)
	assert.Equal(t, "feature missing (provider: anthropic)", err.Error())
	assert.True(t, IsProviderWrapperError(err))
	assert.False(t, IsProviderWrapperError(errors.New("generic error")))
}

func TestWrapProviderError(t *testing.T) {
	t.Parallel()
	cause := errors.New("underlying socket closed")

	// Retryable code
	err1 := WrapProviderError("openai", ErrorCodeRateLimit, "rate limit exceeded", cause)
	we1, ok1 := AsWormholeError(err1)
	require.True(t, ok1)
	assert.Equal(t, "openai", we1.Provider)
	assert.True(t, we1.IsRetryable())
	assert.Equal(t, cause, we1.Cause)
	assert.Equal(t, "underlying socket closed", we1.Details)

	// Non-retryable code
	err2 := WrapProviderError("openai", ErrorCodeValidation, "invalid body", nil)
	we2, ok2 := AsWormholeError(err2)
	require.True(t, ok2)
	assert.False(t, we2.IsRetryable())
	assert.Nil(t, we2.Cause)
}

func TestNotImplementedError(t *testing.T) {
	t.Parallel()
	err := NotImplementedError("ollama", "Images")
	we, ok := AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeProvider, we.Code)
	assert.Contains(t, we.Message, "ollama provider does not support Images")
}

func TestProviderValidationErrors(t *testing.T) {
	t.Parallel()
	err := NewProviderValidationError("gemini", "invalid param", "param 'top_k' out of bounds")
	we, ok := AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeValidation, we.Code)
	assert.Equal(t, "param 'top_k' out of bounds", we.Details)

	errFmt := ValidationErrorf("gemini", "field %s is required", "prompt")
	weFmt, okFmt := AsWormholeError(errFmt)
	require.True(t, okFmt)
	assert.Equal(t, "field prompt is required", weFmt.Message)
}

func TestProviderErrorAndErrorf(t *testing.T) {
	t.Parallel()
	err := ProviderError("groq", "upstream 500", "server crashed")
	we, ok := AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeProvider, we.Code)
	assert.True(t, we.IsRetryable())
	assert.Equal(t, "server crashed", we.Details)

	errFmt := ProviderErrorf("groq", "error status %d", 503)
	weFmt, okFmt := AsWormholeError(errFmt)
	require.True(t, okFmt)
	assert.Equal(t, "error status 503", weFmt.Message)
}

func TestRequestError(t *testing.T) {
	t.Parallel()
	cause := errors.New("bad JSON body")
	err := RequestError("mistral", "failed to marshal", cause)
	we, ok := AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeRequest, we.Code)
	assert.False(t, we.IsRetryable())
	assert.Equal(t, cause, we.Cause)
	assert.Equal(t, "bad JSON body", we.Details)
}

func TestModelErrorAndErrorf(t *testing.T) {
	t.Parallel()
	err := ModelError("openrouter", "unknown model", "model 'gpt-6' not found")
	we, ok := AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeModel, we.Code)
	assert.False(t, we.IsRetryable())
	assert.Equal(t, "model 'gpt-6' not found", we.Details)

	errFmt := ModelErrorf("openrouter", "model %s unsupported", "foo-bar")
	weFmt, okFmt := AsWormholeError(errFmt)
	require.True(t, okFmt)
	assert.Equal(t, "model foo-bar unsupported", weFmt.Message)
}
