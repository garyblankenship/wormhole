package gemini_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiProvider_Stream(t *testing.T) {
	testCases := []struct {
		name           string
		request        types.TextRequest
		mockStreamData string
		expectedChunks int
		expectedTexts  []string
		expectedError  string
		expectedFinish *types.FinishReason
		verifyURL      bool
	}{
		{
			name: "basic streaming response",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Tell me a story"),
				},
			},
			mockStreamData: `data: {"candidates":[{"content":{"parts":[{"text":"Once"}],"role":"model"},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{"text":" upon"}],"role":"model"},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{"text":" a time"}],"role":"model"},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{"text":"..."}],"role":"model"},"finishReason":"STOP"}]}

`,
			expectedChunks: 5, // 4 text chunks + 1 finish reason
			expectedTexts:  []string{"Once", " upon", " a time", "..."},
			expectedFinish: func() *types.FinishReason { fr := types.FinishReasonStop; return &fr }(),
			verifyURL:      true,
		},
		{
			name: "streaming with tool calls",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("What's the weather?"),
				},
			},
			mockStreamData: `data: {"candidates":[{"content":{"parts":[{"text":"I'll check the weather for you."}],"role":"model"},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"location":"New York"}}}],"role":"model"},"finishReason":"STOP"}]}

`,
			expectedChunks: 3, // 1 text chunk + 1 tool call chunk + 1 finish reason
			expectedTexts:  []string{"I'll check the weather for you."},
			expectedFinish: func() *types.FinishReason { fr := types.FinishReasonStop; return &fr }(),
		},
		{
			name: "streaming with empty chunks",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test"),
				},
			},
			mockStreamData: `data: 

data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":""}]}

data: 

data: {"candidates":[{"content":{"parts":[{"text":" World"}],"role":"model"},"finishReason":"STOP"}]}

`,
			expectedChunks: 3, // 2 text chunks + 1 finish reason (empty data lines ignored)
			expectedTexts:  []string{"Hello", " World"},
			expectedFinish: func() *types.FinishReason { fr := types.FinishReasonStop; return &fr }(),
		},
		{
			name: "streaming with [DONE] message",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Short response"),
				},
			},
			mockStreamData: `data: {"candidates":[{"content":{"parts":[{"text":"Done"}],"role":"model"},"finishReason":"STOP"}]}

data: [DONE]

`,
			expectedChunks: 2, // 1 text chunk + 1 finish reason ([DONE] stops processing)
			expectedTexts:  []string{"Done"},
			expectedFinish: func() *types.FinishReason { fr := types.FinishReasonStop; return &fr }(),
		},
		{
			name: "streaming error response",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "invalid-model",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test"),
				},
			},
			mockStreamData: `data: {"error":{"code":400,"message":"Invalid model specified","status":"INVALID_ARGUMENT"}}

`,
			expectedChunks: 1, // 1 error chunk
			expectedError:  "Invalid model specified",
		},
		{
			name: "streaming with invalid JSON",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test"),
				},
			},
			mockStreamData: `data: {invalid json}

`,
			expectedChunks: 1, // 1 error chunk
			expectedError:  "invalid character",
		},
		{
			name: "streaming with max tokens finish",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model:     "gemini-pro",
					MaxTokens: func(i int) *int { return &i }(10),
				},
				Messages: []types.Message{
					types.NewUserMessage("Write a long story"),
				},
			},
			mockStreamData: `data: {"candidates":[{"content":{"parts":[{"text":"This is a story that gets cut off"}],"role":"model"},"finishReason":"MAX_TOKENS"}]}

`,
			expectedChunks: 2, // 1 text chunk + 1 finish reason
			expectedTexts:  []string{"This is a story that gets cut off"},
			expectedFinish: func() *types.FinishReason { fr := types.FinishReasonLength; return &fr }(),
		},
		{
			name: "streaming with safety filter",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Inappropriate content"),
				},
			},
			mockStreamData: `data: {"candidates":[{"content":{"parts":[{"text":""}],"role":"model"},"finishReason":"SAFETY"}]}

`,
			expectedChunks: 1, // 1 finish reason (empty text not sent)
			expectedTexts:  []string{},
			expectedFinish: func() *types.FinishReason { fr := types.FinishReasonContentFilter; return &fr }(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedURL string

			// Create mock streaming server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedURL = r.URL.String()

				// Verify request method and headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))
				assert.Equal(t, "no-cache", r.Header.Get("Cache-Control"))

				// Set SSE headers
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")

				// Write the mock stream data
				flusher, ok := w.(http.Flusher)
				require.True(t, ok, "ResponseWriter should support flushing")

				// Split by lines and write with small delays
				lines := strings.Split(tc.mockStreamData, "\n")
				for _, line := range lines {
					fmt.Fprintf(w, "%s\n", line)
					flusher.Flush()
					time.Sleep(1 * time.Millisecond) // Small delay to simulate streaming
				}
			}))
			defer server.Close()

			// Create provider with mock server URL
			config := types.ProviderConfig{
				BaseURL: server.URL,
			}
			provider := gemini.New("test-api-key", config)

			// Execute streaming request
			ctx := context.Background()
			stream, err := provider.Stream(ctx, tc.request)

			require.NoError(t, err)
			require.NotNil(t, stream)

			// Collect all chunks
			var chunks []types.TextChunk
			var textChunks []string
			var toolCalls []types.ToolCall
			var finalFinishReason *types.FinishReason
			var streamError error

			for chunk := range stream {
				chunks = append(chunks, chunk)

				// Handle different chunk types
				if chunk.Error != nil {
					streamError = chunk.Error
					break
				}

				if chunk.Text != "" {
					textChunks = append(textChunks, chunk.Text)
				}

				if chunk.ToolCall != nil {
					toolCalls = append(toolCalls, *chunk.ToolCall)
				}
				_ = toolCalls // collected for potential future assertions

				if chunk.FinishReason != nil {
					finalFinishReason = chunk.FinishReason
				}

				// Verify chunk model
				if chunk.Text != "" || chunk.ToolCall != nil || chunk.FinishReason != nil {
					assert.Equal(t, "gemini", chunk.Model)
				}
			}

			// Verify error cases
			if tc.expectedError != "" {
				require.Error(t, streamError)
				assert.Contains(t, streamError.Error(), tc.expectedError)
				return
			}

			// Verify successful cases
			require.NoError(t, streamError)
			assert.Equal(t, tc.expectedChunks, len(chunks))

			// Verify text chunks
			assert.Equal(t, len(tc.expectedTexts), len(textChunks))
			for i, expectedText := range tc.expectedTexts {
				assert.Equal(t, expectedText, textChunks[i])
			}

			// Verify finish reason
			if tc.expectedFinish != nil {
				require.NotNil(t, finalFinishReason)
				assert.Equal(t, *tc.expectedFinish, *finalFinishReason)
			}

			// Verify URL format
			if tc.verifyURL {
				assert.Contains(t, capturedURL, "key=test-api-key")
				assert.Contains(t, capturedURL, fmt.Sprintf("models/%s:streamGenerateContent", tc.request.Model))
			}
		})
	}
}

func TestGeminiProvider_StreamContext(t *testing.T) {
	t.Run("Stream with context cancellation", func(t *testing.T) {
		// Create mock server that streams slowly
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			require.True(t, ok)

			// Send first chunk
			fmt.Fprintf(w, `data: {"candidates":[{"content":{"parts":[{"text":"First"}],"role":"model"},"finishReason":""}]}`+"\n\n")
			flusher.Flush()

			// Wait a bit before sending second chunk
			time.Sleep(100 * time.Millisecond)

			fmt.Fprintf(w, `data: {"candidates":[{"content":{"parts":[{"text":"Second"}],"role":"model"},"finishReason":"STOP"}]}`+"\n\n")
			flusher.Flush()
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Create context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// Start streaming
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-pro"},
			Messages:    []types.Message{types.NewUserMessage("test")},
		}

		stream, err := provider.Stream(ctx, request)
		require.NoError(t, err)

		// Collect chunks until context cancellation
		var chunks []types.TextChunk
		for chunk := range stream {
			chunks = append(chunks, chunk)
		}

		// Should receive at least the first chunk before cancellation
		assert.GreaterOrEqual(t, len(chunks), 1)
		if len(chunks) > 0 {
			assert.Equal(t, "First", chunks[0].Text)
		}
	})

	t.Run("Stream with immediate context cancellation", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, `data: {"candidates":[{"content":{"parts":[{"text":"Never received"}],"role":"model"},"finishReason":"STOP"}]}`+"\n\n")
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Create already-canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Start streaming
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-pro"},
			Messages:    []types.Message{types.NewUserMessage("test")},
		}

		stream, err := provider.Stream(ctx, request)
		// With immediate cancellation, we may get an error during request setup
		if err != nil {
			assert.Contains(t, err.Error(), "context canceled")
			return
		}

		// Collect chunks - should be empty or minimal due to immediate cancellation
		var chunks []types.TextChunk
		for chunk := range stream {
			chunks = append(chunks, chunk)
		}

		// Stream should close quickly with no or minimal chunks
		assert.LessOrEqual(t, len(chunks), 1)
	})
}

func TestGeminiProvider_StreamRequestFormat(t *testing.T) {
	t.Run("Stream request with all parameters", func(t *testing.T) {
		var capturedRequest map[string]any

		// Create mock server that captures request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Capture request body
			var reqBody map[string]any
			err := json.NewDecoder(r.Body).Decode(&reqBody)
			require.NoError(t, err)
			capturedRequest = reqBody

			// Return minimal response
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, `data: {"candidates":[{"content":{"parts":[{"text":"response"}],"role":"model"},"finishReason":"STOP"}]}`+"\n\n")
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Create comprehensive request
		maxTokens := 100
		temperature := float32(0.8)
		topP := float32(0.9)
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{
				Model:       "gemini-pro",
				MaxTokens:   &maxTokens,
				Temperature: &temperature,
				TopP:        &topP,
				Stop:        []string{"END", "STOP"},
			},
			Messages: []types.Message{
				types.NewUserMessage("Test streaming with parameters"),
			},
			SystemPrompt: "You are a helpful assistant",
			Tools: []types.Tool{
				*types.NewTool("test_tool", "Test tool", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"param": map[string]any{"type": "string"},
					},
				}),
			},
			ToolChoice: &types.ToolChoice{
				Type: types.ToolChoiceTypeAuto,
			},
		}

		// Execute request
		ctx := context.Background()
		stream, err := provider.Stream(ctx, request)
		require.NoError(t, err)

		// Consume stream
		for range stream {
			// Just consume chunks
		}

		// Verify request format matches text request format
		require.NotNil(t, capturedRequest)

		// Check system instruction
		systemInstr, ok := capturedRequest["systemInstruction"].(map[string]any)
		require.True(t, ok)
		parts := systemInstr["parts"].([]any)
		part := parts[0].(map[string]any)
		assert.Equal(t, "You are a helpful assistant", part["text"])

		// Check generation config
		genConfig, ok := capturedRequest["generationConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(100), genConfig["maxOutputTokens"])
		assert.Equal(t, 0.8, genConfig["temperature"])
		assert.Equal(t, 0.9, genConfig["topP"])

		stopSeqs := genConfig["stopSequences"].([]any)
		assert.Contains(t, stopSeqs, "END")
		assert.Contains(t, stopSeqs, "STOP")

		// Check tools
		tools, ok := capturedRequest["tools"].([]any)
		require.True(t, ok)
		require.Len(t, tools, 1)

		tool := tools[0].(map[string]any)
		funcDecls := tool["functionDeclarations"].([]any)
		require.Len(t, funcDecls, 1)

		funcDecl := funcDecls[0].(map[string]any)
		assert.Equal(t, "test_tool", funcDecl["name"])

		// Check tool config
		toolConfig, ok := capturedRequest["toolConfig"].(map[string]any)
		require.True(t, ok)
		funcConfig := toolConfig["functionCallingConfig"].(map[string]any)
		assert.Equal(t, "AUTO", funcConfig["mode"])
	})
}

func TestGeminiProvider_StreamErrorScenarios(t *testing.T) {
	t.Run("Server returns HTTP error", func(t *testing.T) {
		// Create server that returns error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error":{"code":500,"message":"Internal server error"}}`)
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Execute request
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-pro"},
			Messages:    []types.Message{types.NewUserMessage("test")},
		}

		ctx := context.Background()
		_, err := provider.Stream(ctx, request)

		require.Error(t, err)
		// Error should be caught during initial request, not in stream
	})

	t.Run("Server closes connection unexpectedly", func(t *testing.T) {
		// Create server that closes connection
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			require.True(t, ok)

			// Send partial response then close
			fmt.Fprintf(w, `data: {"candidates":[{"content":{"parts":[{"text":"Partial"}]`)
			flusher.Flush()

			// Abruptly close connection
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, err := hijacker.Hijack()
				if err == nil {
					conn.Close()
				}
			}
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Execute request
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-pro"},
			Messages:    []types.Message{types.NewUserMessage("test")},
		}

		ctx := context.Background()
		stream, err := provider.Stream(ctx, request)
		require.NoError(t, err)

		// Collect chunks - should get an error
		var chunks []types.TextChunk
		var streamError error

		for chunk := range stream {
			chunks = append(chunks, chunk)
			if chunk.Error != nil {
				streamError = chunk.Error
				break
			}
		}

		// Should have received an error due to connection closure/invalid JSON
		require.Error(t, streamError)
		_ = chunks // collected for debugging
	})

	t.Run("Empty stream response", func(t *testing.T) {
		// Create server that returns empty stream
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			// Return no data, just close
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Execute request
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-pro"},
			Messages:    []types.Message{types.NewUserMessage("test")},
		}

		ctx := context.Background()
		stream, err := provider.Stream(ctx, request)
		require.NoError(t, err)

		// Collect chunks - should be empty
		var chunks []types.TextChunk
		for chunk := range stream {
			chunks = append(chunks, chunk)
		}

		assert.Empty(t, chunks)
	})
}
