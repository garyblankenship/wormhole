package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/providers/openai"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenAIProvider_IntegrationTextGeneration tests the complete text generation flow
func TestOpenAIProvider_IntegrationTextGeneration(t *testing.T) {
	testCases := []struct {
		name         string
		model        string
		maxTokens    int
		expectedAPI  string // "max_tokens" or "max_completion_tokens"
	}{
		{
			name:        "GPT-4 uses deprecated max_tokens",
			model:       "gpt-4",
			maxTokens:   100,
			expectedAPI: "max_tokens",
		},
		{
			name:        "GPT-5 uses new max_completion_tokens",
			model:       "gpt-5",
			maxTokens:   100,
			expectedAPI: "max_completion_tokens",
		},
		{
			name:        "GPT-5-mini uses new max_completion_tokens",
			model:       "gpt-5-mini",
			maxTokens:   100,
			expectedAPI: "max_completion_tokens",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Track the actual request sent to the API
			var capturedRequest map[string]interface{}

			// Create a mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

				// Capture and verify the request body
				var reqBody map[string]interface{}
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				capturedRequest = reqBody

				// Verify model and max tokens parameter
				assert.Equal(t, tc.model, reqBody["model"])
				
				if tc.expectedAPI == "max_completion_tokens" {
					assert.Contains(t, reqBody, "max_completion_tokens")
					assert.NotContains(t, reqBody, "max_tokens")
					assert.Equal(t, float64(tc.maxTokens), reqBody["max_completion_tokens"])
				} else {
					assert.Contains(t, reqBody, "max_tokens")
					assert.NotContains(t, reqBody, "max_completion_tokens")
					assert.Equal(t, float64(tc.maxTokens), reqBody["max_tokens"])
				}

				// Return a mock response
				response := map[string]interface{}{
					"id":      "chatcmpl-test123",
					"object":  "chat.completion",
					"created": time.Now().Unix(),
					"model":   tc.model,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":    "assistant",
								"content": "Hello! This is a test response.",
							},
							"finish_reason": "stop",
						},
					},
					"usage": map[string]interface{}{
						"prompt_tokens":     10,
						"completion_tokens": 8,
						"total_tokens":      18,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create provider with mock server URL
			provider := openai.New(types.ProviderConfig{
				APIKey:  "test-api-key",
				BaseURL: server.URL + "/v1",
			})

			// Create test request
			maxTokens := tc.maxTokens
			request := &types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model:       tc.model,
					MaxTokens:   &maxTokens,
					Temperature: &[]float32{0.7}[0],
				},
				Messages: []types.Message{
					types.NewUserMessage("Hello, world!"),
				},
			}

			// Execute the request
			ctx := context.Background()
			response, err := provider.Text(ctx, *request)

			// Verify the response
			require.NoError(t, err)
			assert.NotNil(t, response)
			assert.Equal(t, "chatcmpl-test123", response.ID)
			assert.Equal(t, tc.model, response.Model)
			assert.Equal(t, "Hello! This is a test response.", response.Text)
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
		})
	}
}

// TestOpenAIProvider_IntegrationStreaming tests streaming functionality
func TestOpenAIProvider_IntegrationStreaming(t *testing.T) {
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

		// Send streaming chunks
		chunks := []string{
			`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-4","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n",
			`data: [DONE]` + "\n\n",
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
	provider := openai.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL + "/v1",
	})

	// Create streaming request
	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-4",
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
	var chunks []types.TextChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	// Verify streaming results
	require.GreaterOrEqual(t, len(chunks), 3, "Should receive multiple chunks")
	
	// Verify first chunk
	assert.Equal(t, "chatcmpl-stream123", chunks[0].ID)
	assert.Equal(t, "gpt-4", chunks[0].Model)
	require.NotNil(t, chunks[0].Delta)
	assert.Equal(t, "Hello", chunks[0].Delta.Content)
	assert.Nil(t, chunks[0].Error)

	// Verify subsequent chunks contain text
	require.NotNil(t, chunks[1].Delta)
	assert.Equal(t, " there", chunks[1].Delta.Content)
	require.NotNil(t, chunks[2].Delta)
	assert.Equal(t, "!", chunks[2].Delta.Content)

	// Verify final chunk has finish reason
	finalChunk := chunks[len(chunks)-1]
	assert.NotNil(t, finalChunk.FinishReason)
	assert.Equal(t, types.FinishReasonStop, *finalChunk.FinishReason)
}

// TestOpenAIProvider_ErrorHandling tests various error scenarios
func TestOpenAIProvider_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedError  types.ErrorCode
	}{
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			responseBody: `{
				"error": {
					"message": "Invalid API key",
					"type": "invalid_request_error",
					"code": "invalid_api_key"
				}
			}`,
			expectedError: types.ErrorCodeAuth,
		},
		{
			name:       "429 Rate Limit",
			statusCode: 429,
			responseBody: `{
				"error": {
					"message": "Rate limit exceeded",
					"type": "rate_limit_error",
					"code": "rate_limit_exceeded"
				}
			}`,
			expectedError: types.ErrorCodeTimeout, // Will timeout due to retries
		},
		{
			name:       "404 Model Not Found",
			statusCode: 404,
			responseBody: `{
				"error": {
					"message": "Model not found",
					"type": "invalid_request_error",
					"code": "model_not_found"
				}
			}`,
			expectedError: types.ErrorCodeModel,
		},
		{
			name:       "500 Server Error",
			statusCode: 500,
			responseBody: `{
				"error": {
					"message": "Internal server error",
					"type": "server_error"
				}
			}`,
			expectedError: types.ErrorCodeTimeout, // Will timeout due to retries
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
			provider := openai.New(types.ProviderConfig{
				APIKey:  "test-api-key", 
				BaseURL: server.URL + "/v1",
			})

			// Create request
			request := &types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gpt-4",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test"),
				},
			}

			// Execute request and verify error with short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := provider.Text(ctx, *request)
			
			require.Error(t, err)
			
			// Check if it's a WormholeError and verify the error code
			var wormholeErr *types.WormholeError
			if errors.As(err, &wormholeErr) {
				assert.Equal(t, tc.expectedError, wormholeErr.Code)
				assert.NotEmpty(t, wormholeErr.Message)
			} else {
				// For retryable errors that timeout, just verify we get an error
				assert.NotEmpty(t, err.Error())
				// These should be timeout errors due to retry behavior
				if tc.statusCode == 429 || tc.statusCode == 500 {
					assert.Contains(t, err.Error(), "deadline exceeded")
				}
			}
		})
	}
}

// TestOpenAIProvider_Authentication tests API key validation
func TestOpenAIProvider_Authentication(t *testing.T) {
	t.Run("missing API key", func(t *testing.T) {
		// Create provider without API key
		provider := openai.New(types.ProviderConfig{
			BaseURL: "https://api.openai.com/v1",
		})

		request := &types.TextRequest{
			BaseRequest: types.BaseRequest{
				Model: "gpt-4",
			},
			Messages: []types.Message{
				types.NewUserMessage("Test"),
			},
		}

		ctx := context.Background()
		_, err := provider.Text(ctx, *request)
		
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "api key")
	})

	t.Run("API key in headers", func(t *testing.T) {
		// Track authorization header
		var authHeader string
		
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader = r.Header.Get("Authorization")
			
			response := map[string]interface{}{
				"id":      "test",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Test response",
						},
						"finish_reason": "stop",
					},
				},
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		provider := openai.New(types.ProviderConfig{
			APIKey:  "sk-test-key-123",
			BaseURL: server.URL + "/v1",
		})

		request := &types.TextRequest{
			BaseRequest: types.BaseRequest{
				Model: "gpt-4",
			},
			Messages: []types.Message{
				types.NewUserMessage("Test"),
			},
		}

		ctx := context.Background()
		_, err := provider.Text(ctx, *request)
		
		require.NoError(t, err)
		assert.Equal(t, "Bearer sk-test-key-123", authHeader)
	})
}

// TestOpenAIProvider_ToolCalling tests function calling functionality
func TestOpenAIProvider_ToolCalling(t *testing.T) {
	// Create mock server that returns tool call response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify tools in request
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		
		tools, ok := reqBody["tools"].([]interface{})
		require.True(t, ok, "Request should include tools")
		require.Len(t, tools, 1)

		// Return tool call response
		response := map[string]interface{}{
			"id":      "chatcmpl-tool123",
			"object":  "chat.completion", 
			"created": time.Now().Unix(),
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_weather",
									"arguments": `{"location": "San Francisco"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	provider := openai.New(types.ProviderConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL + "/v1",
	})

	// Create request with tools
	weatherTool := types.NewTool(
		"get_weather",
		"Get current weather for a location",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]string{"type": "string"},
			},
			"required": []string{"location"},
		},
	)

	request := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-4",
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
	assert.Equal(t, "call_123", toolCall.ID)
	assert.Equal(t, "function", toolCall.Type)
	assert.Equal(t, "get_weather", toolCall.Function.Name)
	assert.Contains(t, toolCall.Function.Arguments, "San Francisco")
}