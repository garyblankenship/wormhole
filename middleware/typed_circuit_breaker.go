package middleware

import (
	"context"
	"errors"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// TypedCircuitBreakerMiddleware isolates circuit state by provider identity
// and operation. A failure in one route therefore cannot block a fallback.
type TypedCircuitBreakerMiddleware struct {
	registry *circuitBreakerRegistry
}

// NewTypedCircuitBreakerMiddleware creates typed circuit-breaker middleware.
func NewTypedCircuitBreakerMiddleware(threshold int, timeoutDuration time.Duration) *TypedCircuitBreakerMiddleware {
	return &TypedCircuitBreakerMiddleware{registry: newCircuitBreakerRegistry(threshold, timeoutDuration)}
}

func typedCircuit[Req any, Resp any](m *TypedCircuitBreakerMiddleware, operation string, next func(context.Context, Req) (Resp, error)) func(context.Context, Req) (Resp, error) {
	return func(ctx context.Context, request Req) (Resp, error) {
		provider := providerIdentity(ctx)
		result, err := m.registry.breaker(provider, operation).Execute(ctx, func() (any, error) {
			return next(ctx, request)
		})
		if err != nil {
			var zero Resp
			return zero, err
		}
		return result.(Resp), nil
	}
}

func (m *TypedCircuitBreakerMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return typedCircuit(m, "text", next)
}

func (m *TypedCircuitBreakerMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
		breaker := m.registry.breaker(providerIdentity(ctx), "stream")
		if err := breaker.admit(); err != nil {
			return nil, err
		}
		stream, err := next(ctx, request)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
				breaker.recordCancellation()
				return stream, err
			}
			return stream, breaker.recordError(err)
		}
		if stream == nil {
			breaker.recordSuccess()
			return nil, nil
		}

		wrapped := make(chan types.StreamChunk, 1)
		go func() {
			defer close(wrapped)
			recordedOutcome := false
			for {
				select {
				case chunk, ok := <-stream:
					if !ok {
						if !recordedOutcome {
							breaker.recordSuccess()
						}
						return
					}
					if chunk.Error != nil && !recordedOutcome {
						if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(chunk.Error, ctxErr) {
							breaker.recordCancellation()
						} else {
							_ = breaker.recordError(chunk.Error)
						}
						recordedOutcome = true
					}
					select {
					case wrapped <- chunk:
					case <-ctx.Done():
						if !recordedOutcome {
							breaker.recordCancellation()
						}
						return
					}
				case <-ctx.Done():
					if !recordedOutcome {
						breaker.recordCancellation()
					}
					return
				}
			}
		}()
		return wrapped, nil
	}
}

func (m *TypedCircuitBreakerMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return typedCircuit(m, "structured", next)
}

func (m *TypedCircuitBreakerMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return typedCircuit(m, "embeddings", next)
}

func (m *TypedCircuitBreakerMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return typedCircuit(m, "rerank", next)
}

func (m *TypedCircuitBreakerMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return typedCircuit(m, "audio", next)
}

func (m *TypedCircuitBreakerMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return typedCircuit(m, "image", next)
}
