package wormhole

import (
	"sync"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Memory pools for frequently allocated objects
var (
	textRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.TextRequest{
				Messages: make([]types.Message, 0, 4), // Pre-allocate capacity for 4 messages
			}
		},
	}

	structuredRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.StructuredRequest{
				Messages: make([]types.Message, 0, 4), // Pre-allocate capacity for 4 messages
			}
		},
	}

	embeddingsRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.EmbeddingsRequest{
				Input: make([]string, 0, 2), // Pre-allocate capacity for 2 inputs
			}
		},
	}

	imageRequestPool = sync.Pool{
		New: func() interface{} {
			return &types.ImageRequest{}
		},
	}

	messageSlicePool = sync.Pool{
		New: func() interface{} {
			return make([]types.Message, 0, 8) // Pre-allocate capacity for 8 messages
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

// putTextRequest returns a TextRequest to the pool
func putTextRequest(req *types.TextRequest) {
	if req != nil {
		textRequestPool.Put(req)
	}
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

// putStructuredRequest returns a StructuredRequest to the pool
func putStructuredRequest(req *types.StructuredRequest) {
	if req != nil {
		structuredRequestPool.Put(req)
	}
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

// putImageRequest returns an ImageRequest to the pool
func putImageRequest(req *types.ImageRequest) {
	if req != nil {
		imageRequestPool.Put(req)
	}
}

// getMessageSlice gets a message slice from the pool
func getMessageSlice() []types.Message {
	slice := messageSlicePool.Get().([]types.Message)
	return slice[:0] // Reset length but keep capacity
}

// putMessageSlice returns a message slice to the pool
func putMessageSlice(slice []types.Message) {
	if slice != nil && cap(slice) <= 32 { // Don't pool extremely large slices
		messageSlicePool.Put(slice)
	}
}