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

func TestRerankBuilderValidate(t *testing.T) {
	t.Parallel()
	client := wormhole.New()

	// Missing all required fields.
	assert.Error(t, client.Rerank().Validate())
	// Missing documents.
	assert.Error(t, client.Rerank().Model("cohere/rerank-v3.5").Query("q").Validate())
	// Missing query.
	assert.Error(t, client.Rerank().Model("cohere/rerank-v3.5").Documents("doc1").Validate())
	// Missing model.
	assert.Error(t, client.Rerank().Query("q").Documents("doc1").Validate())
	// Complete request.
	assert.NoError(t, client.Rerank().Model("cohere/rerank-v3.5").Query("q").Documents("a", "b").Validate())
}

func TestRerankBuilderGenerate(t *testing.T) {
	t.Parallel()

	mockProvider := mocktesting.NewMockProvider("mock")
	client := wormhole.New(
		wormhole.WithDefaultProvider("mock"),
		wormhole.WithCustomProvider("mock", mocktesting.MockProviderFactory(mockProvider)),
		wormhole.WithProviderConfig("mock", types.ProviderConfig{}),
	)

	ctx := context.Background()

	t.Run("validation failure during Generate", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Rerank().Model("cohere/rerank-v3.5").Generate(ctx)
		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("successful rerank with all builder options", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Rerank().
			Using("mock").
			BaseURL("https://api.cohere.ai").
			Model("cohere/rerank-v3.5").
			Query("What is Go?").
			Documents("Go is a language", "Python is dynamic").
			AddDocument("Rust is memory safe").
			TopN(2).
			ProviderOptions(map[string]any{"rank_fields": []string{"text"}}).
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "cohere/rerank-v3.5", resp.Model)
		assert.Equal(t, "mock-rerank", resp.ID)
		assert.Len(t, resp.Results, 3)
	})

	t.Run("provider execution error", func(t *testing.T) {
		t.Parallel()
		errProvider := mocktesting.NewMockProvider("err-provider").WithError("rerank provider error")
		errClient := wormhole.New(
			wormhole.WithDefaultProvider("err-provider"),
			wormhole.WithCustomProvider("err-provider", mocktesting.MockProviderFactory(errProvider)),
			wormhole.WithProviderConfig("err-provider", types.ProviderConfig{}),
		)

		resp, err := errClient.Rerank().
			Model("cohere/rerank-v3.5").
			Query("test").
			Documents("doc1").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "rerank provider error")
	})
}
