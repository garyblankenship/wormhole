package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingsPayloadDimensions(t *testing.T) {
	t.Parallel()
	dimensions := 2
	for _, tc := range []struct {
		name       string
		dimensions *int
	}{
		{name: "requested", dimensions: &dimensions},
		{name: "omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := New("fixture-key", types.ProviderConfig{})
			t.Cleanup(func() { require.NoError(t, provider.Close()) })
			request := types.EmbeddingsRequest{
				Model:           "models/gemini-embedding-fixture",
				ProviderOptions: map[string]any{"taskType": "RETRIEVAL_DOCUMENT", "title": "ledger fixture"},
				Input:           []string{"ledger alpha", "ledger beta"},
				Dimensions:      tc.dimensions,
			}
			payload := provider.buildEmbeddingsPayload(request)
			requests, ok := payload["requests"].([]map[string]any)
			require.True(t, ok)
			require.Len(t, requests, len(request.Input))
			for i, item := range requests {
				dimension, ok := item["outputDimensionality"]
				if tc.dimensions == nil {
					require.False(t, ok)
				} else {
					require.True(t, ok)
					require.Equal(t, *tc.dimensions, dimension)
				}
				require.Equal(t, "models/gemini-embedding-fixture", item["model"])
				require.Equal(t, "RETRIEVAL_DOCUMENT", item["taskType"])
				require.Equal(t, "ledger fixture", item["title"])
				require.Equal(t, map[string]any{"parts": []map[string]any{{"text": request.Input[i]}}}, item["content"])
			}
		})
	}
}
