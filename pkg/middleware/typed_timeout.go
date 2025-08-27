package middleware

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TypedTimeoutMiddleware implements the ProviderMiddleware interface with type-safe timeout enforcement
type TypedTimeoutMiddleware struct {
	timeout time.Duration
}

// NewTypedTimeoutMiddleware creates a new type-safe timeout middleware
func NewTypedTimeoutMiddleware(timeout time.Duration) *TypedTimeoutMiddleware {
	return &TypedTimeoutMiddleware{
		timeout: timeout,
	}
}

// ApplyText wraps text generation calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		type result struct {
			resp *types.TextResponse
			err  error
		}

		done := make(chan result, 1)

		go func() {
			resp, err := next(ctx, request)
			done <- result{resp, err}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			return res.resp, res.err
		}
	}
}

// ApplyStream wraps streaming calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		
		type result struct {
			stream <-chan types.StreamChunk
			err    error
		}

		done := make(chan result, 1)

		go func() {
			stream, err := next(ctx, request)
			done <- result{stream, err}
		}()

		select {
		case <-ctx.Done():
			cancel() // Clean up the timeout context
			return nil, ctx.Err()
		case res := <-done:
			// For streaming, we keep the timeout context alive for the duration of the stream
			// The context will be cancelled when the stream completes or times out
			if res.err != nil {
				cancel()
				return res.stream, res.err
			}
			
			// Wrap the stream to handle timeout during streaming
			wrappedStream := make(chan types.StreamChunk)
			go func() {
				defer close(wrappedStream)
				defer cancel() // Clean up when stream completes
				
				if res.stream == nil {
					return
				}
				
				for {
					select {
					case chunk, ok := <-res.stream:
						if !ok {
							return // Stream closed normally
						}
						select {
						case wrappedStream <- chunk:
						case <-ctx.Done():
							return // Context timeout during streaming
						}
					case <-ctx.Done():
						return // Context timeout while waiting for chunk
					}
				}
			}()
			
			return wrappedStream, nil
		}
	}
}

// ApplyStructured wraps structured output calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		type result struct {
			resp *types.StructuredResponse
			err  error
		}

		done := make(chan result, 1)

		go func() {
			resp, err := next(ctx, request)
			done <- result{resp, err}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			return res.resp, res.err
		}
	}
}

// ApplyEmbeddings wraps embeddings calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		type result struct {
			resp *types.EmbeddingsResponse
			err  error
		}

		done := make(chan result, 1)

		go func() {
			resp, err := next(ctx, request)
			done <- result{resp, err}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			return res.resp, res.err
		}
	}
}

// ApplyAudio wraps audio calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		type result struct {
			resp *types.AudioResponse
			err  error
		}

		done := make(chan result, 1)

		go func() {
			resp, err := next(ctx, request)
			done <- result{resp, err}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			return res.resp, res.err
		}
	}
}

// ApplyImage wraps image generation calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		defer cancel()

		type result struct {
			resp *types.ImageResponse
			err  error
		}

		done := make(chan result, 1)

		go func() {
			resp, err := next(ctx, request)
			done <- result{resp, err}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-done:
			return res.resp, res.err
		}
	}
}