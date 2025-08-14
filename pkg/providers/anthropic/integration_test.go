package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/providers/anthropic"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnthropicProvider_IntegrationTextGeneration tests the complete text generation flow
func TestAnthropicProvider_IntegrationTextGeneration(t *testing.T) {
	testCases := []struct {
		name         string
		model        string
		maxTokens    int
		systemMsg    string
		checkHeaders bool
	}{
		{
			name:         "Claude-3 Opus basic generation",
			model:        "claude-3-opus-20240229",
			maxTokens:    100,
			systemMsg:    "You are a helpful assistant.",
			checkHeaders: true,
		},
		{
			name:         "Claude-3 Sonnet with system prompt",
			model:        "claude-3-sonnet-20240229",
			maxTokens:    200,
			systemMsg:    "Be concise and direct.",
			checkHeaders: true,
		},
		{
			name:         "Claude-3 Haiku minimal tokens",
			model:        "claude-3-haiku-20240307",
			maxTokens:    50,
			systemMsg:    "",
			checkHeaders: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Track the actual request sent to the API
			var capturedRequest map[string]interface{}
			var capturedHeaders http.Header

			// Create a mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
				assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

				capturedHeaders = r.Header

				// Capture and verify the request body
				var reqBody map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				capturedRequest = reqBody

				// Verify model and max tokens parameter
				assert.Equal(t, tc.model, reqBody["model"])
				assert.Equal(t, float64(tc.maxTokens), reqBody["max_tokens"])

				// Verify system prompt handling
				if tc.systemMsg != "" {
					assert.Equal(t, tc.systemMsg, reqBody["system"])
				} else {
					assert.NotContains(t, reqBody, "system")
				}

				// Verify messages format
				messages, ok := reqBody["messages"].([]interface{})
				require.True(t, ok, "messages should be an array")
				require.Len(t, messages, 1)

				msg := messages[0].(map[string]interface{})
				assert.Equal(t, "user", msg["role"])

				content := msg["content"].([]interface{})
				require.Len(t, content, 1)
				contentPart := content[0].(map[string]interface{})
				assert.Equal(t, "text", contentPart["type"])
				assert.Equal(t, "Hello, world!", contentPart["text"])

				// Return a mock response
				response := map[string]interface{}{
					"id":          "msg_test123",
					"type":        "message",
					"role":        "assistant",
					"model":       tc.model,
					"stop_reason": "end_turn",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": "Hello! This is a test response from Claude.",
						},
					},
					"usage": map[string]interface{}{
						"input_tokens":  10,
						"output_tokens": 8,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create provider with mock server URL
			provider := anthropic.New(types.ProviderConfig{
				APIKey:  "test-api-key",
				BaseURL: server.URL,
			})

			// Create test request
			maxTokens := tc.maxTokens
			request := &types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model:       tc.model,
					MaxTokens:   &maxTokens,
					Temperature: &[]float32{0.7}[0],
				},
				Messages:     []types.Message{types.NewUserMessage("Hello, world!")},
				SystemPrompt: tc.systemMsg,
			}

			// Execute the request
			ctx := context.Background()
			response, err := provider.Text(ctx, *request)

			// Verify the response
			require.NoError(t, err)
			assert.NotNil(t, response)
			assert.Equal(t, "msg_test123", response.ID)
			assert.Equal(t, tc.model, response.Model)
			assert.Equal(t, "Hello! This is a test response from Claude.", response.Text)
			assert.Equal(t, types.FinishReasonStop, response.FinishReason)

			// Verify usage
			require.NotNil(t, response.Usage)
			assert.Equal(t, 10, response.Usage.PromptTokens)
			assert.Equal(t, 8, response.Usage.CompletionTokens)
			assert.Equal(t, 18, response.Usage.TotalTokens)

			// Verify the request was captured correctly
			assert.NotNil(t, capturedRequest)
			assert.Equal(t, tc.model, capturedRequest["model"])
			assert.Equal(t, float64(0.7), capturedRequest["temperature"])

			if tc.checkHeaders {
				assert.Equal(t, "test-api-key", capturedHeaders.Get("x-api-key"))
				assert.Equal(t, "2023-06-01", capturedHeaders.Get("anthropic-version"))
			}
		})
	}
}

// TestAnthropicProvider_IntegrationStreaming tests streaming functionality
func TestAnthropicProvider_IntegrationStreaming(t *testing.T) {
	// Create a mock server that returns streaming responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify streaming request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		assert.Equal(t, true, reqBody["stream"])

		// Set up SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send streaming chunks in Anthropic format
		chunks := []string{
			// Message start event
			`event: message_start` + "\n" +
				`data: {"type":"message_start","message":{"id":"msg_stream123","type":"message","role":"assistant","model":"claude-3-sonnet-20240229","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n",

			// Content block start
			`event: content_block_start` + "\n" +
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n",

			// Content deltas
			`event: content_block_delta` + "\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n\n",

			`event: content_block_delta` + "\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}` + "\n\n",

			`event: content_block_delta` + "\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}` + "\n\n",

			// Content block stop
			`event: content_block_stop` + "\n" +
				`data: {"type":"content_block_stop","index":0}` + "\n\n",

			// Message delta with final usage
			`event: message_delta` + "\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","usage":{"output_tokens":3}}}` + "\n\n",

			// Message stop
			`event: message_stop` + "\n" +
				`data: {"type":"message_stop"}` + "\n\n",
		}

		for _, chunk := range chunks {
			fmt.Fprint(w, chunk)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	// Create provider
	provider := anthropic.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	// Create streaming request
	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "claude-3-sonnet-20240229",
		},
		Messages: []types.Message{
			types.NewUserMessage("Hello"),
		},
	}

	// Execute streaming request
	ctx := context.Background()
	stream, err := provider.Stream(ctx, *request)
	require.NoError(t, err)

	// Collect streaming chunks
	var chunks []types.StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	// Verify streaming results
	require.GreaterOrEqual(t, len(chunks), 3, "Should receive multiple chunks")

	// Verify that we got text content
	var textChunks []types.StreamChunk
	for _, chunk := range chunks {
		if chunk.Text != "" || (chunk.Delta != nil && chunk.Delta.Content != "") {
			textChunks = append(textChunks, chunk)
		}
	}

	require.GreaterOrEqual(t, len(textChunks), 3, "Should receive text chunks")

	// Verify final chunk has finish reason
	var finalChunk *types.StreamChunk
	for i := len(chunks) - 1; i >= 0; i-- {
		if chunks[i].FinishReason != nil {
			finalChunk = &chunks[i]
			break
		}
	}

	if finalChunk != nil {
		assert.Equal(t, types.FinishReasonStop, *finalChunk.FinishReason)
	}
}

// TestAnthropicProvider_IntegrationStructuredOutput tests structured output functionality
func TestAnthropicProvider_IntegrationStructuredOutput(t *testing.T) {
	// Create mock server that returns tool call response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools in request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		tools, ok := reqBody["tools"].([]interface{})
		require.True(t, ok, "Request should include tools")
		require.Len(t, tools, 1)

		// Verify tool choice
		toolChoice, ok := reqBody["tool_choice"]
		require.True(t, ok, "Request should include tool_choice")
		assert.NotNil(t, toolChoice)

		// Return tool call response
		response := map[string]interface{}{
			"id":          "msg_tool123",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-3-sonnet-20240229",
			"stop_reason": "tool_use",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "I'll extract the structured data for you.",
				},
				{
					"type": "tool_use",
					"id":   "tool_call_123",
					"name": "extract_user_info",
					"input": map[string]interface{}{
						"name": "John Doe",
						"age":  30,
						"city": "New York",
					},
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  20,
				"output_tokens": 15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	provider := anthropic.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	// Create structured request
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]string{"type": "string"},
			"age":  map[string]string{"type": "integer"},
			"city": map[string]string{"type": "string"},
		},
		"required": []string{"name", "age", "city"},
	}

	request := &types.StructuredRequest{
		BaseRequest: types.BaseRequest{
			Model: "claude-3-sonnet-20240229",
		},
		Messages: []types.Message{
			types.NewUserMessage("Extract: John Doe, 30 years old, lives in New York"),
		},
		Schema:     schema,
		SchemaName: "extract_user_info",
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Structured(ctx, *request)

	require.NoError(t, err)
	assert.Equal(t, "msg_tool123", response.ID)
	assert.Equal(t, "claude-3-sonnet-20240229", response.Model)

	// Verify structured data
	require.NotNil(t, response.Data)
	data, ok := response.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "John Doe", data["name"])
	assert.Equal(t, float64(30), data["age"]) // JSON numbers are float64
	assert.Equal(t, "New York", data["city"])

	// Verify usage
	require.NotNil(t, response.Usage)
	assert.Equal(t, 20, response.Usage.PromptTokens)
	assert.Equal(t, 15, response.Usage.CompletionTokens)
	assert.Equal(t, 35, response.Usage.TotalTokens)
}

// TestAnthropicProvider_ErrorHandling tests various error scenarios
func TestAnthropicProvider_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name         string
		statusCode   int
		responseBody string
		checkError   func(t *testing.T, err error)
	}{
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			responseBody: `{
				"type": "error",
				"message": "Invalid API key"
			}`,
			checkError: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "Invalid API key")
			},
		},
		{
			name:       "400 Bad Request",
			statusCode: 400,
			responseBody: `{
				"type": "error", 
				"message": "Invalid request format"
			}`,
			checkError: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "Invalid request format")
			},
		},
		{
			name:       "429 Rate Limit",
			statusCode: 429,
			responseBody: `{
				"type": "error",
				"message": "Rate limit exceeded"
			}`,
			checkError: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "Rate limit exceeded")
			},
		},
		{
			name:       "500 Server Error",
			statusCode: 500,
			responseBody: `{
				"type": "error",
				"message": "Internal server error"
			}`,
			checkError: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "Internal server error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock server that returns error
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			// Create provider
			provider := anthropic.New(types.ProviderConfig{
				APIKey:  "test-api-key",
				BaseURL: server.URL,
			})

			// Create request
			request := &types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "claude-3-sonnet-20240229",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test"),
				},
			}

			// Execute request and verify error
			ctx := context.Background()
			_, err := provider.Text(ctx, *request)

			require.Error(t, err)
			tc.checkError(t, err)
		})
	}
}

// TestAnthropicProvider_Authentication tests API key validation
func TestAnthropicProvider_Authentication(t *testing.T) {
	t.Run("missing API key", func(t *testing.T) {
		// Create provider without API key
		provider := anthropic.New(types.ProviderConfig{
			BaseURL: "https://api.anthropic.com/v1",
		})

		request := &types.TextRequest{
			BaseRequest: types.BaseRequest{
				Model: "claude-3-sonnet-20240229",
			},
			Messages: []types.Message{
				types.NewUserMessage("Test"),
			},
		}

		ctx := context.Background()
		_, err := provider.Text(ctx, *request)

		require.Error(t, err)
		// The error will occur during HTTP request with empty API key
		assert.NotNil(t, err)
	})

	t.Run("API key and headers in request", func(t *testing.T) {
		// Track headers
		var authHeader, versionHeader string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader = r.Header.Get("x-api-key")
			versionHeader = r.Header.Get("anthropic-version")

			response := map[string]interface{}{
				"id":          "test",
				"type":        "message",
				"role":        "assistant",
				"model":       "claude-3-sonnet-20240229",
				"stop_reason": "end_turn",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "Test response",
					},
				},
				"usage": map[string]interface{}{
					"input_tokens":  5,
					"output_tokens": 2,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		provider := anthropic.New(types.ProviderConfig{
			APIKey:  "sk-ant-test-key-123",
			BaseURL: server.URL,
		})

		request := &types.TextRequest{
			BaseRequest: types.BaseRequest{
				Model: "claude-3-sonnet-20240229",
			},
			Messages: []types.Message{
				types.NewUserMessage("Test"),
			},
		}

		ctx := context.Background()
		_, err := provider.Text(ctx, *request)

		require.NoError(t, err)
		assert.Equal(t, "sk-ant-test-key-123", authHeader)
		assert.Equal(t, "2023-06-01", versionHeader)
	})
}

// TestAnthropicProvider_ToolCalling tests function calling functionality
func TestAnthropicProvider_ToolCalling(t *testing.T) {
	// Create mock server that returns tool call response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools in request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		tools, ok := reqBody["tools"].([]interface{})
		require.True(t, ok, "Request should include tools")
		require.Len(t, tools, 1)

		tool := tools[0].(map[string]interface{})
		assert.Equal(t, "get_weather", tool["name"])
		assert.Equal(t, "Get current weather for a location", tool["description"])

		// Return tool call response
		response := map[string]interface{}{
			"id":          "msg_tool123",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-3-sonnet-20240229",
			"stop_reason": "tool_use",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "I'll get the weather for you.",
				},
				{
					"type": "tool_use",
					"id":   "tool_call_123",
					"name": "get_weather",
					"input": map[string]interface{}{
						"location": "San Francisco",
						"unit":     "celsius",
					},
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  15,
				"output_tokens": 10,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	provider := anthropic.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	// Create request with tools
	weatherTool := types.NewTool(
		"get_weather",
		"Get current weather for a location",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]string{"type": "string"},
				"unit":     map[string]string{"type": "string"},
			},
			"required": []string{"location"},
		},
	)

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "claude-3-sonnet-20240229",
		},
		Messages: []types.Message{
			types.NewUserMessage("What's the weather in San Francisco?"),
		},
		Tools: []types.Tool{*weatherTool},
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Text(ctx, *request)

	require.NoError(t, err)
	assert.Equal(t, types.FinishReasonToolCalls, response.FinishReason)
	require.Len(t, response.ToolCalls, 1)

	toolCall := response.ToolCalls[0]
	assert.Equal(t, "tool_call_123", toolCall.ID)
	assert.Equal(t, "function", toolCall.Type)

	// Check arguments contain expected data - Arguments should be parsed from Function.Arguments
	require.NotNil(t, toolCall.Function)
	var args map[string]interface{}
	err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "San Francisco", args["location"])
	assert.Equal(t, "celsius", args["unit"])
}

// TestAnthropicProvider_MultimodalMessages tests image input functionality
func TestAnthropicProvider_MultimodalMessages(t *testing.T) {
	// Create mock server that handles image messages
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify multimodal content in request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		messages, ok := reqBody["messages"].([]interface{})
		require.True(t, ok)
		require.Len(t, messages, 1)

		msg := messages[0].(map[string]interface{})
		content := msg["content"].([]interface{})
		require.Len(t, content, 2) // Text + image

		// Check text part
		textPart := content[0].(map[string]interface{})
		assert.Equal(t, "text", textPart["type"])
		assert.Equal(t, "What's in this image?", textPart["text"])

		// Check image part
		imagePart := content[1].(map[string]interface{})
		assert.Equal(t, "image", imagePart["type"])

		response := map[string]interface{}{
			"id":          "msg_vision123",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-3-sonnet-20240229",
			"stop_reason": "end_turn",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "I can see a beautiful landscape with mountains and a lake.",
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  25,
				"output_tokens": 12,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	provider := anthropic.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	// Create multimodal message
	parts := []types.MessagePart{
		types.TextPart("What's in this image?"),
		types.ImagePart(map[string]interface{}{
			"type":       "base64",
			"media_type": "image/jpeg",
			"data":       "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		}),
	}

	// Create a user message with multimodal content using BaseMessage
	multimodalMsg := &types.BaseMessage{
		Role:    types.RoleUser,
		Content: parts,
	}

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "claude-3-sonnet-20240229",
		},
		Messages: []types.Message{multimodalMsg},
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Text(ctx, *request)

	require.NoError(t, err)
	assert.Equal(t, "msg_vision123", response.ID)
	assert.Equal(t, "I can see a beautiful landscape with mountains and a lake.", response.Text)
	assert.Equal(t, types.FinishReasonStop, response.FinishReason)
}

// TestAnthropicProvider_UnsupportedFeatures tests that unsupported features return appropriate errors
func TestAnthropicProvider_UnsupportedFeatures(t *testing.T) {
	provider := anthropic.New(types.ProviderConfig{
		APIKey: "test-api-key",
	})

	ctx := context.Background()

	t.Run("embeddings not supported", func(t *testing.T) {
		request := &types.EmbeddingsRequest{
			Model: "claude-3-sonnet-20240229",
			Input: []string{"test text"},
		}

		_, err := provider.Embeddings(ctx, *request)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "does not support")
	})

	t.Run("audio not supported", func(t *testing.T) {
		request := &types.AudioRequest{
			Model: "claude-3-sonnet-20240229",
			Input: "test audio input",
		}

		_, err := provider.Audio(ctx, *request)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "does not support")
	})

	t.Run("images not supported", func(t *testing.T) {
		request := &types.ImagesRequest{
			Model:  "claude-3-sonnet-20240229",
			Prompt: "test image prompt",
		}

		_, err := provider.Images(ctx, *request)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "does not support")
	})
}
