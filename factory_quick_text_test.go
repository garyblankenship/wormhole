package wormhole_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2"
)

func TestFactoryQuickText(t *testing.T) {
	t.Parallel()

	t.Run("QuickText and QuickTextWithContext", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		resp, err := wormhole.QuickTextWithContext(ctx, "gpt-4o", "Hello", "fake-api-key")
		assert.Error(t, err)
		assert.Nil(t, resp)

		resp, err = wormhole.QuickText("gpt-4o", "Hello", "fake-api-key")
		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("QuickChat and QuickChatWithContext", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		resp, err := wormhole.QuickChatWithContext(ctx, "gpt-4o", "System instruction", "User message", "fake-api-key")
		assert.Error(t, err)
		assert.Nil(t, resp)

		resp, err = wormhole.QuickChat("gpt-4o", "System instruction", "User message", "fake-api-key")
		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("QuickStream and QuickStreamWithContext", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		stream, err := wormhole.QuickStreamWithContext(ctx, "gpt-4o", "Write a poem", "fake-api-key")
		// QuickStream initiates streaming.
		if err == nil {
			require.NotNil(t, stream)
			// Drain channel
			for chunk := range stream {
				_ = chunk
			}
		}

		stream2, err2 := wormhole.QuickStream("gpt-4o", "Write a poem", "fake-api-key")
		if err2 == nil {
			require.NotNil(t, stream2)
			for chunk := range stream2 {
				_ = chunk
			}
		}
	})
}
