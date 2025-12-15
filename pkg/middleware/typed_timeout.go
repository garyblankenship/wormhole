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

// withTimeout executes a function with timeout enforcement using generics
// This eliminates the duplicate timeout pattern across all Apply* methods
func withTimeout[Req any, Resp any](
	ctx context.Context,
	timeout time.Duration,
	request Req,
	fn func(context.Context, Req) (Resp, error),
) (Resp, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		resp Resp
		err  error
	}

	done := make(chan result, 1)

	go func() {
		resp, err := fn(ctx, request)
		done <- result{resp, err}
	}()

	select {
	case <-ctx.Done():
		var zero Resp
		return zero, ctx.Err()
	case res := <-done:
		return res.resp, res.err
	}
}

// ApplyText wraps text generation calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withTimeout(ctx, m.timeout, request, next)
	}
}

// ApplyStream wraps streaming calls with timeout enforcement
// Note: Streaming requires special handling to maintain timeout during the stream
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
			cancel()
			return nil, ctx.Err()
		case res := <-done:
			if res.err != nil {
				cancel()
				return res.stream, res.err
			}

			// Wrap stream to handle timeout during streaming
			wrappedStream := make(chan types.StreamChunk)
			go func() {
				defer close(wrappedStream)
				defer cancel()

				if res.stream == nil {
					return
				}

				for {
					select {
					case chunk, ok := <-res.stream:
						if !ok {
							return
						}
						select {
						case wrappedStream <- chunk:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
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
		return withTimeout(ctx, m.timeout, request, next)
	}
}

// ApplyEmbeddings wraps embeddings calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withTimeout(ctx, m.timeout, request, next)
	}
}

// ApplyAudio wraps audio calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withTimeout(ctx, m.timeout, request, next)
	}
}

// ApplyImage wraps image generation calls with timeout enforcement
func (m *TypedTimeoutMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withTimeout(ctx, m.timeout, request, next)
	}
}
