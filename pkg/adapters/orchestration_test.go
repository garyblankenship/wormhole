package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	wmtest "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWormholeToOrchestrationAdapterCreateCompletion(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock").WithTextResponse(types.TextResponse{
		ID:      "resp-1",
		Model:   "mock-model",
		Text:    "completion text",
		Usage:   &types.Usage{PromptTokens: 4, CompletionTokens: 6, TotalTokens: 10},
		Created: time.Now(),
	})
	adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "default-model")

	resp, err := adapter.CreateCompletion(context.Background(), OrchestrationCompletionRequest{
		Prompt:      "hello",
		MaxTokens:   5,
		Temperature: 0.2,
	})

	require.NoError(t, err)
	assert.Equal(t, ProviderOpenAI, adapter.Name())
	assert.Equal(t, "completion text", resp.Content)
	assert.Equal(t, 10, resp.TokensUsed)
	assert.Equal(t, ProviderOpenAI, resp.Provider)
	assert.Equal(t, "default-model", resp.Model)
	assert.Greater(t, resp.Duration, time.Duration(0))
	assert.InEpsilon(t, 0.00002, resp.Cost, 0.000001)
}

func TestWormholeToOrchestrationAdapterCreateCompletionReturnsProviderError(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock").WithError("provider failed")
	adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "default-model")

	_, err := adapter.CreateCompletion(context.Background(), OrchestrationCompletionRequest{
		Prompt: "hello",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider failed")
}

func TestWormholeToOrchestrationAdapterCreateCompletionHandlesMissingUsage(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock").WithTextResponse(types.TextResponse{
		ID:    "resp-1",
		Model: "mock-model",
		Text:  "completion text",
	})
	adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "default-model")

	resp, err := adapter.CreateCompletion(context.Background(), OrchestrationCompletionRequest{
		Prompt:    "12345678",
		MaxTokens: 8,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, resp.TokensUsed)
	assert.InEpsilon(t, adapter.EstimateCost(OrchestrationCompletionRequest{Prompt: "12345678", MaxTokens: 8}), resp.Cost, 0.000001)
}

func TestWormholeToOrchestrationAdapterStreaming(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	mock := wmtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
		{Text: "hello "},
		{Delta: &types.ChunkDelta{Content: "world"}, FinishReason: &stop},
	})
	adapter := NewWormholeToOrchestrationAdapter(mock, ProviderAnthropic, "claude-test")

	var chunks []string
	var doneValues []bool
	resp, err := adapter.CreateStreamingCompletion(
		context.Background(),
		OrchestrationCompletionRequest{Prompt: "stream"},
		func(chunk string, done bool) error {
			chunks = append(chunks, chunk)
			doneValues = append(doneValues, done)
			return nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, "hello world", resp.Content)
	assert.Equal(t, 2, resp.TokensUsed)
	assert.Equal(t, []string{"hello ", "world"}, chunks)
	assert.Equal(t, []bool{false, true}, doneValues)
	assert.Equal(t, ProviderAnthropic, resp.Provider)
	assert.Equal(t, "claude-test", resp.Model)
	assert.Greater(t, resp.Cost, 0.0)
}

func TestWormholeToOrchestrationAdapterStreamingCallbackError(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
		{Delta: &types.ChunkDelta{Content: "hello"}},
	})
	adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "gpt-test")
	callbackErr := errors.New("stop streaming")

	_, err := adapter.CreateStreamingCompletion(
		context.Background(),
		OrchestrationCompletionRequest{Prompt: "stream"},
		func(string, bool) error { return callbackErr },
	)

	require.ErrorIs(t, err, callbackErr)
}

func TestWormholeToOrchestrationAdapterStreamingChunkError(t *testing.T) {
	t.Parallel()
	chunkErr := errors.New("chunk failed")
	mock := wmtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
		{Error: chunkErr},
	})
	adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "gpt-test")

	_, err := adapter.CreateStreamingCompletion(
		context.Background(),
		OrchestrationCompletionRequest{Prompt: "stream"},
		func(string, bool) error { return nil },
	)

	require.ErrorIs(t, err, chunkErr)
}

func TestWormholeToOrchestrationAdapterEstimateCost(t *testing.T) {
	t.Parallel()
	mock := wmtest.NewMockProvider("mock")

	tests := []struct {
		name      string
		provider  string
		model     string
		req       OrchestrationCompletionRequest
		wantCost  float64
		tolerance float64
	}{
		{
			name:     "openai gpt-5-mini",
			provider: ProviderOpenAI,
			model:    "gpt-5-mini",
			req: OrchestrationCompletionRequest{
				Prompt:    "12345678",
				MaxTokens: 8,
			},
			wantCost: 0.000001,
		},
		{
			name:     "anthropic default response tokens",
			provider: ProviderAnthropic,
			model:    "claude-test",
			req:      OrchestrationCompletionRequest{Prompt: "12345678"},
			wantCost: 0.001506,
		},
		{
			name:     "unknown provider default rate",
			provider: "custom",
			model:    "custom-model",
			req: OrchestrationCompletionRequest{
				Prompt:    "12345678",
				MaxTokens: 8,
			},
			wantCost: 0.00001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewWormholeToOrchestrationAdapter(mock, tt.provider, tt.model)
			assert.InEpsilon(t, tt.wantCost, adapter.EstimateCost(tt.req), 0.000001)
		})
	}
}

func TestWormholeToOrchestrationAdapterHealthCheck(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := wmtest.NewMockProvider("mock")
		adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "gpt-test")
		require.NoError(t, adapter.HealthCheck(context.Background()))
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		mock := wmtest.NewMockProvider("mock").WithError("unhealthy")
		adapter := NewWormholeToOrchestrationAdapter(mock, ProviderOpenAI, "gpt-test")
		err := adapter.HealthCheck(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unhealthy")
	})
}
