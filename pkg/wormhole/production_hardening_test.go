package wormhole_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorCloser struct {
	err error
}

func (c errorCloser) Close() error { return c.err }

// TestGracefulShutdownBasic tests basic graceful shutdown functionality
func TestGracefulShutdownBasic(t *testing.T) {
	t.Parallel()
	client := wormhole.New(wormhole.WithDefaultProvider("openai"))

	// Shutdown should complete with no active requests
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Shutdown(ctx)
	require.NoError(t, err, "Shutdown should succeed with no active requests")

	// Shutdown should be idempotent
	err = client.Shutdown(ctx)
	require.NoError(t, err, "Multiple shutdown calls should be idempotent")
}

func TestShutdownPersistsCleanupError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("close failed")
	client := wormhole.New(func(cfg *wormhole.Config) {
		cfg.Closers = append(cfg.Closers, io.Closer(errorCloser{err: wantErr}))
	})

	firstErr := client.Shutdown(context.Background())
	require.ErrorIs(t, firstErr, wantErr)

	secondErr := client.Shutdown(context.Background())
	require.ErrorIs(t, secondErr, wantErr)
}

// TestIdempotencyKeyBasic tests basic idempotency key functionality
func TestIdempotencyKeyBasic(t *testing.T) {
	t.Parallel()
	// Create client with idempotency key
	client := wormhole.New(
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithIdempotencyKey("test-key-123", 5*time.Minute),
	)

	require.NotNil(t, client, "Client should be created with idempotency key")
}

// TestErrorHandlingBasic tests that factory methods return errors instead of panic
func TestErrorHandlingBasic(t *testing.T) {
	// Test Quick functions return errors
	_, err := wormhole.QuickOllama("") // Empty base URL
	require.Error(t, err, "QuickOllama without base URL should return error")
	assert.Contains(t, err.Error(), "base URL", "Error should mention base URL")

	t.Setenv("OPENROUTER_API_KEY", "")
	_, err = wormhole.QuickOpenRouter("") // Empty API key
	require.Error(t, err, "QuickOpenRouter without API key should return error")
	assert.Contains(t, err.Error(), "API key", "Error should mention API key")
}

// TestToolSafetyConfig tests tool safety config structure
func TestToolSafetyConfig(t *testing.T) {
	t.Parallel()
	config := wormhole.DefaultToolSafetyConfig()

	// Verify default values for new security fields
	assert.Equal(t, 0, config.MaxMemoryMB, "Default max memory should be 0 (unlimited)")
	assert.Equal(t, 0*time.Second, config.MaxCPUTime, "Default max CPU time should be 0 (unlimited)")
	assert.True(t, config.EnableInputValidation, "Input validation should be enabled by default")
	assert.False(t, config.EnableResourceIsolation, "Resource isolation should be disabled by default")
	assert.Equal(t, 10*1024*1024, config.MaxToolOutputSize, "Default max output size should be 10MB")

	// Test validation
	err := config.Validate()
	require.NoError(t, err, "Default config should be valid")
}
