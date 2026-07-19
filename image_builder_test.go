package wormhole_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
	mocktesting "github.com/garyblankenship/wormhole/v2/wormholetest"
)

func TestImageRequestBuilder(t *testing.T) {
	t.Parallel()

	mockProvider := mocktesting.NewMockProvider("mock")
	client := wormhole.New(
		wormhole.WithDefaultProvider("mock"),
		wormhole.WithCustomProvider("mock", mocktesting.MockProviderFactory(mockProvider)),
		wormhole.WithProviderConfig("mock", types.ProviderConfig{}),
	)

	ctx := context.Background()

	t.Run("validation error - missing prompt", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Image().
			Model("dall-e-3").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no prompt provided")
	})

	t.Run("validation error - missing model", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Image().
			Prompt("A photo of a cat").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no model specified")
	})

	t.Run("successful generation with builder parameters", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Image().
			Using("mock").
			BaseURL("https://custom.api.com").
			Model("dall-e-3").
			Prompt("A vivid sunrise over mountains").
			Size("1024x1024").
			Quality("hd").
			Style("vivid").
			N(2).
			ResponseFormat("url").
			ProviderOptions(map[string]any{"user": "test-user"}).
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "dall-e-3", resp.Model)
		assert.Equal(t, "mock-image", resp.ID)
		assert.NotEmpty(t, resp.Images)
		assert.Equal(t, "https://example.com/generated-image.png", resp.Images[0].URL)
	})

	t.Run("default N setting", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Image().
			Model("dall-e-3").
			Prompt("A cool cybernetic futuristic city").
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("provider error", func(t *testing.T) {
		t.Parallel()
		errProvider := mocktesting.NewMockProvider("err-provider").WithError("image generation failed")
		errClient := wormhole.New(
			wormhole.WithDefaultProvider("err-provider"),
			wormhole.WithCustomProvider("err-provider", mocktesting.MockProviderFactory(errProvider)),
			wormhole.WithProviderConfig("err-provider", types.ProviderConfig{}),
		)

		resp, err := errClient.Image().
			Model("dall-e-3").
			Prompt("A landscape").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "image generation failed")
	})
}
