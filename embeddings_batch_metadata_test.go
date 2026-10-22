package wormhole

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestCloneEmbeddingBatchResponseMetadata(t *testing.T) {
	t.Parallel()
	source := &types.EmbeddingsResponse{
		Provider: "gemini",
		Model:    "embedding-fixture",
		ResponseMetadata: &types.ResponseMetadata{
			Provider:   "gemini",
			RequestID:  "first-batch",
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Request-Id": []string{"first-batch"}},
		},
	}
	cloned := cloneEmbeddingsResponseHeader(source)
	require.Equal(t, source.ResponseMetadata, cloned.ResponseMetadata)
	require.NotSame(t, source.ResponseMetadata, cloned.ResponseMetadata)
	cloned.ResponseMetadata.RequestID = "caller-change"
	cloned.ResponseMetadata.Headers["Request-Id"][0] = "caller-change"
	require.Equal(t, "first-batch", source.ResponseMetadata.RequestID)
	require.Equal(t, "first-batch", source.ResponseMetadata.Headers["Request-Id"][0])
}
