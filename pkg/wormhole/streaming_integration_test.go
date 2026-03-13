package wormhole_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/internal/testutil"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIIntegration_StreamingGeneration(t *testing.T) {
	// Register test models in the global model registry
	testutil.SetupTestModels(t)

	t.Run("streaming text generation", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify streaming request
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			assert.Equal(t, true, req["stream"])

			// Send SSE streaming response
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			require.True(t, ok)

			// Send streaming chunks
			chunks := []string{
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			}

			for _, chunk := range chunks {
				_, err := w.Write([]byte(chunk + "\n\n"))
				require.NoError(t, err)
				flusher.Flush()
				time.Sleep(10 * time.Millisecond) // Simulate streaming delay
			}
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		stream, err := client.Text().
			Model("gpt-5").
			Prompt("Say hello").
			Stream(context.Background())

		require.NoError(t, err)

		// Collect streaming chunks
		chunks := make([]types.TextChunk, 0, 10)
		for chunk := range stream {
			chunks = append(chunks, chunk)
			// Check for error in chunk
			if chunk.Error != nil {
				require.NoError(t, chunk.Error)
			}
		}

		// Verify we received the expected chunks
		require.Len(t, chunks, 4) // 3 content chunks + 1 finish chunk

		assert.Equal(t, "Hello", chunks[0].Text)
		assert.Equal(t, " there", chunks[1].Text)
		assert.Equal(t, "!", chunks[2].Text)

		// Check finish reason (should be pointer in chunk)
		require.NotNil(t, chunks[3].FinishReason)
		assert.Equal(t, types.FinishReasonStop, *chunks[3].FinishReason)

		// Verify full text concatenation
		var builder strings.Builder
		for _, chunk := range chunks[:3] { // Exclude finish chunk
			builder.WriteString(chunk.Text)
		}
		assert.Equal(t, "Hello there!", builder.String())
	})
}
