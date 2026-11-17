package wormhole

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

type snapshotMutationProvider struct {
	*types.BaseProvider
	t     *testing.T
	calls int
}

func (p *snapshotMutationProvider) Stream(_ context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	p.calls++
	expected := snapshotRequestFixture()
	expected.Model = request.Model
	expected.Messages = append([]types.Message{types.NewSystemMessage(expected.SystemPrompt)}, expected.Messages...)
	expected.SystemPrompt = ""
	expected.Messages[2].(*types.AssistantMessage).Thinking.Provider = "mock"
	if !assert.Equal(p.t, *expected, request) {
		return nil, errors.New("request snapshot changed")
	}
	if request.Model == "primary" {
		request.Messages[1].(*types.UserMessage).Media[0].(*types.ImageMedia).Data[0] = 99
		request.Messages[2].(*types.AssistantMessage).ToolCalls[0].Arguments["items"].([]any)[0] = "changed"
		request.Messages[2].(*types.AssistantMessage).ToolCalls[0].Function.Arguments = "changed"
		request.Messages[2].(*types.AssistantMessage).Thinking.Signature = "changed"
		request.Messages[3].(*types.BaseMessage).Content.([]types.MessagePart)[0].Data.(map[string]any)["items"].([]any)[0] = "changed"
		request.Tools[0].InputSchema["items"].([]any)[0] = "changed"
		request.Tools[0].Function.Parameters["items"].([]any)[0] = "changed"
		request.Tools[0].CacheControl.TTL = types.CacheTTL1Hour
		request.ProviderOptions["nested"].(map[string]any)["items"].([]any)[0] = "changed"
		request.ResponseFormat.(map[string]any)["items"].([]any)[0] = "changed"
		*request.Reasoning.Enabled = false
		*request.Temperature = 1
		request.Stop[0] = "changed"
		request.ToolChoice.ToolName = "changed"
		return nil, errors.New("try fallback")
	}
	stop := types.FinishReasonStop
	return streamChunks(types.TextChunk{Text: "ok", FinishReason: &stop})()
}

func snapshotRequestFixture() *types.TextRequest {
	shared := map[string]any{"items": []any{"original"}}
	enabled, temperature := true, float32(0.5)
	return &types.TextRequest{
		SystemPrompt: "system",
		BaseRequest:  types.BaseRequest{Model: "primary", Temperature: &temperature, Stop: []string{"stop"}, Reasoning: &types.Reasoning{Enabled: &enabled}, ProviderOptions: map[string]any{"nested": shared}},
		Messages: []types.Message{
			&types.UserMessage{Content: "question", Media: []types.Media{&types.ImageMedia{Data: []byte{1}, MimeType: "image/png"}}},
			&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "call", Arguments: shared, Function: &types.ToolCallFunction{Arguments: `{"items":["original"]}`}}}, Thinking: &types.Thinking{Signature: "signed"}},
			&types.BaseMessage{Role: types.RoleUser, Content: []types.MessagePart{{Type: "image", Data: shared}}},
		},
		Tools:      []types.Tool{{Name: "tool", InputSchema: shared, Function: &types.ToolFunction{Parameters: shared}, CacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral}}},
		ToolChoice: &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "tool"}, ResponseFormat: shared,
	}
}

func TestStreamRequestSnapshotSurvivesProviderMutation(t *testing.T) {
	request := snapshotRequestFixture()
	original := snapshotRequestFixture()
	provider := &snapshotMutationProvider{BaseProvider: types.NewBaseProvider("mock"), t: t}
	client := New(WithDiscovery(false), WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), WithProviderConfig("mock", types.ProviderConfig{}))
	builder := client.Text().WithFallback("fallback")
	builder.request = request
	stream, err := builder.Stream(context.Background())
	require.NoError(t, err)
	chunks := collectStreamChunks(t, stream)
	require.Len(t, chunks, 1)
	require.Equal(t, "ok", chunks[0].Content())
	require.Equal(t, 2, provider.calls)
	require.Equal(t, original, request)
}
