package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// FIX 4: auth errors must be non-retryable — retrying wastes the retry budget
// and churns API keys.
func TestAuthError_NotRetryable(t *testing.T) {
	t.Parallel()
	err := AuthError("openai", "x")
	we, ok := AsWormholeError(err)
	assert.True(t, ok, "AuthError should be a WormholeError")
	assert.False(t, we.IsRetryable(), "auth errors must not be retryable")
}
