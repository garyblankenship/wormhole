// Wire-conformance replay tests: feed realistic Ollama NDJSON payloads through
// the actual Stream path and assert the mapped wormhole types. Guards
// regressions where IsDone() misfires on intermediate chunks or done_reason
// is lost.
package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaStreamWireConformance(t *testing.T) {
	t.Parallel()

	t.Run("multi_chunk_only_terminal_is_done", func(t *testing.T) {
		t.Parallel()

		ndjson := strings.Join([]string{
			`{"model":"llama3","message":{"role":"assistant","content":"Hello"},"done":false}`,
			`{"model":"llama3","message":{"role":"assistant","content":" world"},"done":false}`,
			`{"model":"llama3","message":{"role":"assistant","content":"!"},"done":false}`,
			`{"model":"llama3","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":5}`,
		}, "\n")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(ndjson))
		}))
		defer server.Close()

		provider, err := New(types.ProviderConfig{BaseURL: server.URL})
		require.NoError(t, err)

		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "llama3"},
			Messages:    []types.Message{types.NewUserMessage("hi")},
		}

		stream, err := provider.Stream(context.Background(), request)
		require.NoError(t, err)

		var chunks []types.TextChunk
		for chunk := range stream {
			require.NoError(t, chunk.Error)
			chunks = append(chunks, chunk)
		}

		// ALL chunks must be delivered (prior bug truncated to 1).
		require.Greater(t, len(chunks), 1, "stream must deliver all chunks, not just the first")

		// Intermediate chunks: IsDone() must be false, FinishReason must be nil.
		for i, chunk := range chunks[:len(chunks)-1] {
			assert.False(t, chunk.IsDone(), "intermediate chunk %d must not be done", i)
			assert.Nil(t, chunk.FinishReason, "intermediate chunk %d must not have FinishReason", i)
		}

		// Terminal chunk: IsDone() must be true with FinishReasonStop.
		last := chunks[len(chunks)-1]
		assert.True(t, last.IsDone(), "terminal chunk must be done")
		require.NotNil(t, last.FinishReason, "terminal chunk must have FinishReason")
		assert.Equal(t, types.FinishReasonStop, *last.FinishReason)

		// Merged text sanity check.
		merged := utils.MergeTextChunks(chunks)
		assert.Equal(t, "Hello world!", merged.Text)
	})

	t.Run("done_reason_length_maps_correctly", func(t *testing.T) {
		t.Parallel()

		ndjson := strings.Join([]string{
			`{"model":"llama3","message":{"role":"assistant","content":"truncated"},"done":false}`,
			`{"model":"llama3","message":{"role":"assistant","content":""},"done":true,"done_reason":"length","prompt_eval_count":5,"eval_count":100}`,
		}, "\n")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(ndjson))
		}))
		defer server.Close()

		provider, err := New(types.ProviderConfig{BaseURL: server.URL})
		require.NoError(t, err)

		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "llama3"},
			Messages:    []types.Message{types.NewUserMessage("count to 100")},
		}

		stream, err := provider.Stream(context.Background(), request)
		require.NoError(t, err)

		var chunks []types.TextChunk
		for chunk := range stream {
			require.NoError(t, chunk.Error)
			chunks = append(chunks, chunk)
		}

		require.Len(t, chunks, 2, "both chunks must be delivered")
		assert.False(t, chunks[0].IsDone(), "intermediate chunk must not be done")
		assert.Nil(t, chunks[0].FinishReason)
		assert.True(t, chunks[1].IsDone(), "terminal chunk must be done")
		require.NotNil(t, chunks[1].FinishReason)
		assert.Equal(t, types.FinishReasonLength, *chunks[1].FinishReason)
	})
}
