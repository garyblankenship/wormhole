package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProviderConfigFluentOptions(t *testing.T) {
	t.Parallel()

	cfg := NewProviderConfig("my-key").
		WithBaseURL("https://api.example.com").
		WithNoAuth().
		WithHeaders(map[string]string{"X-Test": "1"}).
		WithHeader("X-Custom", "2").
		WithTimeout(30).
		WithTimeoutDuration(45 * time.Second).
		WithRetries(3, 200*time.Millisecond).
		WithNoRetries().
		WithMaxRetryDelay(5 * time.Second).
		WithHTTPTimeout(10 * time.Second).
		WithDynamicModels().
		WithParam("custom_param", "value1").
		WithParams(map[string]any{"param2": 123})

	assert.Equal(t, "my-key", cfg.APIKey)
	assert.Equal(t, "https://api.example.com", cfg.BaseURL)
	assert.True(t, cfg.NoAuth)
	assert.Equal(t, "1", cfg.Headers["X-Test"])
	assert.Equal(t, "2", cfg.Headers["X-Custom"])
	assert.Equal(t, 45, cfg.Timeout)
	assert.Equal(t, 0, *cfg.MaxRetries) // WithNoRetries set it to 0
	assert.Equal(t, 5*time.Second, *cfg.RetryMaxDelay)
	assert.Equal(t, 10*time.Second, *cfg.HTTPTimeout)
	assert.True(t, cfg.DynamicModels)
	assert.Equal(t, "value1", cfg.Params["custom_param"])
	assert.Equal(t, 123, cfg.Params["param2"])
}
