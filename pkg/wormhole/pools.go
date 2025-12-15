package wormhole

import (
	"sync"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Memory pools for frequently allocated objects
var (
	textRequestPool = sync.Pool{
		New: func() any {
			return &types.TextRequest{
				Messages: make([]types.Message, 0, 4), // Pre-allocate capacity for 4 messages
			}
		},
	}

	structuredRequestPool = sync.Pool{
		New: func() any {
			return &types.StructuredRequest{
				Messages: make([]types.Message, 0, 4), // Pre-allocate capacity for 4 messages
			}
		},
	}

	embeddingsRequestPool = sync.Pool{
		New: func() any {
			return &types.EmbeddingsRequest{
				Input: make([]string, 0, 2), // Pre-allocate capacity for 2 inputs
			}
		},
	}

	imageRequestPool = sync.Pool{
		New: func() any {
			return &types.ImageRequest{}
		},
	}
)

// getTextRequest gets a TextRequest from the pool
func getTextRequest() *types.TextRequest {
	req := textRequestPool.Get().(*types.TextRequest)
	// Reset the request but keep the underlying slice capacity
	req.Model = ""
	req.Messages = req.Messages[:0]
	req.SystemPrompt = ""
	req.Temperature = nil
	req.MaxTokens = nil
	req.TopP = nil
	req.Stop = nil
	req.Tools = nil
	req.ToolChoice = nil
	req.ResponseFormat = nil
	req.ProviderOptions = nil
	return req
}

// getStructuredRequest gets a StructuredRequest from the pool
func getStructuredRequest() *types.StructuredRequest {
	req := structuredRequestPool.Get().(*types.StructuredRequest)
	// Reset the request but keep the underlying slice capacity
	req.Model = ""
	req.Messages = req.Messages[:0]
	req.SystemPrompt = ""
	req.Temperature = nil
	req.MaxTokens = nil
	req.TopP = nil
	req.Stop = nil
	req.Schema = nil
	req.SchemaName = ""
	req.ProviderOptions = nil
	return req
}

// getEmbeddingsRequest gets an EmbeddingsRequest from the pool
func getEmbeddingsRequest() *types.EmbeddingsRequest {
	req := embeddingsRequestPool.Get().(*types.EmbeddingsRequest)
	// Reset the request but keep the underlying slice capacity
	req.Input = req.Input[:0]
	req.Model = ""
	req.Dimensions = nil
	req.ProviderOptions = nil
	return req
}

// putEmbeddingsRequest returns an EmbeddingsRequest to the pool
func putEmbeddingsRequest(req *types.EmbeddingsRequest) {
	if req != nil {
		embeddingsRequestPool.Put(req)
	}
}

// getImageRequest gets an ImageRequest from the pool
func getImageRequest() *types.ImageRequest {
	req := imageRequestPool.Get().(*types.ImageRequest)
	// Reset the request
	req.Prompt = ""
	req.Model = ""
	req.N = 0
	req.Size = ""
	req.Quality = ""
	req.Style = ""
	req.ResponseFormat = ""
	req.ProviderOptions = nil
	return req
}
