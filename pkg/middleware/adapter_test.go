package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func markerMiddleware(t *testing.T, wantModel string) Middleware {
	t.Helper()
	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			switch typed := req.(type) {
			case *types.TextRequest:
				assert.Equal(t, wantModel, typed.Model)
			case *types.StructuredRequest:
				assert.Equal(t, wantModel, typed.Model)
			case *types.EmbeddingsRequest:
				assert.Equal(t, wantModel, typed.Model)
			case *types.AudioRequest:
				assert.Equal(t, wantModel, typed.Model)
			case *types.ImageRequest:
				assert.Equal(t, wantModel, typed.Model)
			default:
				t.Fatalf("unexpected request type %T", req)
			}
			return next(ctx, req)
		}
	}
}

func TestLegacyAdapterApplyMethods(t *testing.T) {
	t.Parallel()
	adapter := NewLegacyAdapter(markerMiddleware(t, "test-model"))
	ctx := context.Background()

	textResp, err := adapter.ApplyText(func(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Model: req.Model, Text: "ok"}, nil
	})(ctx, types.TextRequest{BaseRequest: types.BaseRequest{Model: "test-model"}})
	require.NoError(t, err)
	assert.Equal(t, "ok", textResp.Text)

	stream := make(chan types.StreamChunk, 1)
	stream <- types.StreamChunk{Text: "chunk"}
	close(stream)
	streamResp, err := adapter.ApplyStream(func(ctx context.Context, req types.TextRequest) (<-chan types.StreamChunk, error) {
		return stream, nil
	})(ctx, types.TextRequest{BaseRequest: types.BaseRequest{Model: "test-model"}})
	require.NoError(t, err)
	streamChunk := <-streamResp
	assert.Equal(t, "chunk", streamChunk.Content())

	structuredResp, err := adapter.ApplyStructured(func(ctx context.Context, req types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{Model: req.Model, Data: map[string]any{"ok": true}}, nil
	})(ctx, types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "test-model"}})
	require.NoError(t, err)
	assert.Equal(t, true, structuredResp.Data.(map[string]any)["ok"])

	embeddingResp, err := adapter.ApplyEmbeddings(func(ctx context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{Model: req.Model, Embeddings: []types.Embedding{{Index: 0}}}, nil
	})(ctx, types.EmbeddingsRequest{Model: "test-model"})
	require.NoError(t, err)
	assert.Len(t, embeddingResp.Embeddings, 1)

	audioResp, err := adapter.ApplyAudio(func(ctx context.Context, req types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{Model: req.Model, Text: "audio"}, nil
	})(ctx, types.AudioRequest{Model: "test-model"})
	require.NoError(t, err)
	assert.Equal(t, "audio", audioResp.Text)

	imageResp, err := adapter.ApplyImage(func(ctx context.Context, req types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{Model: req.Model, Images: []types.GeneratedImage{{URL: "https://example.test/image.png"}}}, nil
	})(ctx, types.ImageRequest{Model: "test-model"})
	require.NoError(t, err)
	assert.Len(t, imageResp.Images, 1)
}

func TestLegacyAdapterReturnsMiddlewareError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("middleware failed")
	adapter := NewLegacyAdapter(func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			return nil, wantErr
		}
	})

	resp, err := adapter.ApplyText(func(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
		t.Fatal("next should not be called")
		return nil, nil
	})(context.Background(), types.TextRequest{})

	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, resp)
}

func TestCircuitBreaker(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(2, time.Millisecond)
	require.Equal(t, StateClosed, cb.GetState())

	_, err := cb.Execute(context.Background(), func() (any, error) {
		return "first", errors.New("boom")
	})
	require.Error(t, err)
	assert.Equal(t, StateClosed, cb.GetState())

	_, err = cb.Execute(context.Background(), func() (any, error) {
		return "second", errors.New("boom")
	})
	require.Error(t, err)
	assert.Equal(t, StateOpen, cb.GetState())

	_, err = cb.Execute(context.Background(), func() (any, error) {
		t.Fatal("open circuit should not call fn")
		return nil, nil
	})
	require.Error(t, err)
	assert.Equal(t, StateOpen, cb.GetState())

	time.Sleep(2 * time.Millisecond)
	result, err := cb.Execute(context.Background(), func() (any, error) {
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, StateClosed, cb.GetState())
	require.NoError(t, cb.Close())
}

func TestCircuitBreakerMiddleware(t *testing.T) {
	t.Parallel()
	mw := CircuitBreakerMiddleware(1, time.Hour)
	handler := mw(func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("failed")
	})

	_, err := handler(context.Background(), "request")
	require.Error(t, err)

	_, err = handler(context.Background(), "request")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
}
