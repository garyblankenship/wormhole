package wormhole

import (
	"sync"

	"github.com/garyblankenship/wormhole/v2/types"
)

var embeddingsRequestPool = sync.Pool{
	New: func() any {
		return &types.EmbeddingsRequest{
			Input: make([]string, 0, 2),
		}
	},
}

// getEmbeddingsRequest gets an EmbeddingsRequest from the pool
func getEmbeddingsRequest() *types.EmbeddingsRequest {
	req := embeddingsRequestPool.Get().(*types.EmbeddingsRequest)
	// Reset the request but keep the underlying slice capacity
	req.Input = req.Input[:0]
	req.Model = ""
	req.Dimensions = nil
	req.EncodingFormat = ""
	req.ProviderOptions = nil
	return req
}

// putEmbeddingsRequest returns an EmbeddingsRequest to the pool
func putEmbeddingsRequest(req *types.EmbeddingsRequest) {
	if req != nil {
		embeddingsRequestPool.Put(req)
	}
}
