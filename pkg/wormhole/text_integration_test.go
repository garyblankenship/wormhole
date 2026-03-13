package wormhole_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/internal/testutil"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIIntegration_TextGeneration(t *testing.T) {
	// Register test models in the global model registry
	testutil.SetupTestModels(t)

	t.Run("successful text generation", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify request method and path
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/chat/completions", r.URL.Path)

			// Verify headers
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Parse and verify request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			assert.Equal(t, "gpt-5", req["model"])
			assert.Equal(t, 0.7, req["temperature"])
			assert.Equal(t, float64(100), req["max_completion_tokens"]) // GPT-5 uses max_completion_tokens

			// Verify messages structure
			messages, ok := req["messages"].([]any)
			require.True(t, ok)
			require.Len(t, messages, 1)

			message := messages[0].(map[string]any)
			assert.Equal(t, "user", message["role"])
			assert.Equal(t, "Hello, how are you?", message["content"])

			// Send mock response
			response := map[string]any{
				"id":      "chatcmpl-test123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "I'm doing well, thank you for asking!",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 12,
					"total_tokens":      22,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		// Create Wormhole client with mock server
		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		// Execute request
		response, err := client.Text().
			Model("gpt-5").
			Prompt("Hello, how are you?").
			Temperature(0.7).
			MaxTokens(100).
			Generate(context.Background())

		// Verify response
		require.NoError(t, err)
		assert.Equal(t, "chatcmpl-test123", response.ID)
		assert.Equal(t, "gpt-5", response.Model)
		assert.Equal(t, "I'm doing well, thank you for asking!", response.Text)
		assert.Equal(t, types.FinishReasonStop, response.FinishReason)
		assert.Equal(t, 10, response.Usage.PromptTokens)
		assert.Equal(t, 12, response.Usage.CompletionTokens)
		assert.Equal(t, 22, response.Usage.TotalTokens)
	})

	t.Run("with system message", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify messages include system message
			messages := req["messages"].([]any)
			require.Len(t, messages, 2)

			systemMsg := messages[0].(map[string]any)
			assert.Equal(t, "system", systemMsg["role"])
			assert.Equal(t, "You are a helpful assistant.", systemMsg["content"])

			userMsg := messages[1].(map[string]any)
			assert.Equal(t, "user", userMsg["role"])
			assert.Equal(t, "What's 2+2?", userMsg["content"])

			// Mock response
			response := map[string]any{
				"id":      "chatcmpl-math123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "2 + 2 = 4",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     15,
					"completion_tokens": 8,
					"total_tokens":      23,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		response, err := client.Text().
			Model("gpt-5").
			SystemPrompt("You are a helpful assistant.").
			Prompt("What's 2+2?").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "2 + 2 = 4", response.Text)
	})

	t.Run("with multiple messages", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify conversation history
			messages := req["messages"].([]any)
			require.Len(t, messages, 3)

			assert.Equal(t, "user", messages[0].(map[string]any)["role"])
			assert.Equal(t, "Hello", messages[0].(map[string]any)["content"])

			assert.Equal(t, "assistant", messages[1].(map[string]any)["role"])
			assert.Equal(t, "Hi there!", messages[1].(map[string]any)["content"])

			assert.Equal(t, "user", messages[2].(map[string]any)["role"])
			assert.Equal(t, "How are you?", messages[2].(map[string]any)["content"])

			// Mock response
			response := map[string]any{
				"id":      "chatcmpl-conv123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "I'm doing great, thanks for asking!",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     20,
					"completion_tokens": 10,
					"total_tokens":      30,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		messages := []types.Message{
			types.NewUserMessage("Hello"),
			types.NewAssistantMessage("Hi there!"),
			types.NewUserMessage("How are you?"),
		}

		response, err := client.Text().
			Model("gpt-5").
			Messages(messages...).
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "I'm doing great, thanks for asking!", response.Text)
		assert.Equal(t, 30, response.Usage.TotalTokens)
	})
}

func TestOpenAIIntegration_FunctionCalling(t *testing.T) {
	// Register test models in the global model registry
	testutil.SetupTestModels(t)

	t.Run("function call request and response", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify tools in request
			tools, ok := req["tools"].([]any)
			require.True(t, ok)
			require.Len(t, tools, 1)

			tool := tools[0].(map[string]any)
			assert.Equal(t, "function", tool["type"])

			function := tool["function"].(map[string]any)
			assert.Equal(t, "get_weather", function["name"])
			assert.Equal(t, "Get current weather for a location", function["description"])

			// Verify tool_choice
			assert.Equal(t, "auto", req["tool_choice"])

			// Mock function call response
			response := map[string]any{
				"id":      "chatcmpl-func123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"tool_calls": []map[string]any{
								{
									"id":   "call-123",
									"type": "function",
									"function": map[string]any{
										"name":      "get_weather",
										"arguments": `{"location": "San Francisco, CA"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     25,
					"completion_tokens": 15,
					"total_tokens":      40,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		tool := types.NewTool(
			"get_weather",
			"Get current weather for a location",
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]string{
						"type":        "string",
						"description": "The city and state, e.g. San Francisco, CA",
					},
				},
				"required": []string{"location"},
			},
		)

		response, err := client.Text().
			Model("gpt-5").
			Prompt("What's the weather like in San Francisco?").
			Tools(*tool).
			ToolChoice("auto").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, types.FinishReasonToolCalls, response.FinishReason)
		require.Len(t, response.ToolCalls, 1)

		toolCall := response.ToolCalls[0]
		assert.Equal(t, "call-123", toolCall.ID)
		assert.Equal(t, "get_weather", toolCall.Function.Name)
		assert.Contains(t, toolCall.Function.Arguments, "San Francisco, CA")
	})
}
