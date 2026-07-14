// Wire-conformance replay tests: feed real-shaped provider SSE payloads (testdata/*.sse) through the actual parse + accumulate path and assert the mapped wormhole types. Guards wire-protocol regressions that synthetic struct tests miss.
package anthropic

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	providerstream "github.com/garyblankenship/wormhole/v2/providers/internal/stream"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestAnthropicStreamWireConformance(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/stream_tool_use_thinking.sse")
	require.NoError(t, err)

	p := &Provider{}
	ctx := context.Background()
	in := providerstream.ProcessSSE(ctx, io.NopCloser(strings.NewReader(string(data))), p.parseStreamChunk, 100)
	out := p.accumulatingStream(ctx, in)

	chunks := []types.StreamChunk{}
	for chunk := range out {
		assert.NoError(t, chunk.Error)
		chunks = append(chunks, chunk)
	}

	var terminalChunk types.StreamChunk
	var terminalFound bool
	var toolCallChunkCount int
	var promptUsageSeen bool
	var completionUsageSeen bool
	var thinkingContentSeen bool
	var thinkingSignatureSeen bool
	var signatureProviderSeen bool

	var thinkingBuffer bytes.Buffer

	for _, chunk := range chunks {
		if len(chunk.ToolCalls) > 0 {
			toolCallChunkCount++
			if chunk.IsDone() {
				terminalChunk = chunk
				terminalFound = true
			}
		}

		if chunk.Thinking != nil {
			if chunk.Thinking.Content == "Let me look up the weather." {
				thinkingContentSeen = true
			}
			if chunk.Thinking.Content != "" {
				thinkingBuffer.WriteString(chunk.Thinking.Content)
			}
			if chunk.Thinking.Signature == "sig-xyz-789" {
				thinkingSignatureSeen = true
				signatureProviderSeen = chunk.Thinking.Provider == "anthropic"
			}
		}

		if chunk.Usage != nil {
			if chunk.Usage.PromptTokens == 10 {
				promptUsageSeen = true
			}
			if chunk.Usage.CompletionTokens == 5 {
				completionUsageSeen = true
			}
		}
	}

	assert.Equal(t, 1, toolCallChunkCount)
	require.True(t, terminalFound)
	require.Len(t, terminalChunk.ToolCalls, 1)

	toolCall := terminalChunk.ToolCalls[0]
	assert.Equal(t, "toolu_abc", toolCall.ID)
	assert.Equal(t, "get_weather", toolCall.Name)
	location, ok := toolCall.Arguments["location"].(string)
	require.True(t, ok)
	assert.Equal(t, "Paris", location)

	assert.Equal(t, "Let me look up the weather.", thinkingBuffer.String())
	assert.True(t, thinkingContentSeen)
	assert.True(t, thinkingSignatureSeen)
	assert.True(t, signatureProviderSeen)

	assert.True(t, promptUsageSeen)
	assert.True(t, completionUsageSeen)

	require.NotNil(t, terminalChunk.FinishReason)
	expectedReason := p.mapStopReason("tool_use")
	assert.Equal(t, expectedReason, *terminalChunk.FinishReason)
}
