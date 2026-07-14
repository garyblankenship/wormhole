// Wire-conformance replay tests: feed real-shaped provider SSE payloads (testdata/*.sse) through the actual parse path and assert the mapped wormhole types. Guards against wire-protocol regressions that synthetic struct-based tests miss.
package gemini

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestGeminiStreamWireConformance(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/stream_tool_call_thinking.sse")
	require.NoError(t, err)

	g := New("test-key", types.ProviderConfig{})
	ctx := context.Background()
	ch := g.handleStream(ctx, io.NopCloser(strings.NewReader(string(data))))

	var chunks []types.TextChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
		assert.Nil(t, chunk.Error)
	}

	var toolCalls []types.ToolCall
	var thinkingParts []string
	var answerParts []string
	var usage *types.Usage
	var finishReason *types.FinishReason

	for _, chunk := range chunks {
		if chunk.ToolCall != nil {
			toolCalls = append(toolCalls, *chunk.ToolCall)
		}
		if chunk.Thinking != nil {
			thinkingParts = append(thinkingParts, chunk.Thinking.Content)
		}
		if chunk.Text != "" {
			answerParts = append(answerParts, chunk.Text)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.FinishReason != nil {
			finishReason = chunk.FinishReason
		}
	}

	assert.Len(t, toolCalls, 1, "expected one tool-call chunk in replay")
	assert.Equal(t, "get_weather", toolCalls[0].Name)
	city, ok := toolCalls[0].Arguments["city"].(string)
	assert.True(t, ok)
	assert.Equal(t, "SF", city)
	assert.Equal(t, "sig-abc-123", toolCalls[0].ThoughtSignature)
	assert.Contains(t, toolCalls[0].ID, "get_weather")

	assert.Equal(t, "Let me check the weather for you.", strings.Join(thinkingParts, ""))
	assert.Equal(t, "Looking that up.", strings.Join(answerParts, ""))
	assert.NotContains(t, strings.Join(answerParts, ""), "Let me check the weather for you.")

	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, 15, usage.TotalTokens)
	assert.Equal(t, 2, usage.CacheReadTokens)

	require.NotNil(t, finishReason)
	assert.Equal(t, types.FinishReasonStop, *finishReason)
}
