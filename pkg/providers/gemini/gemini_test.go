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

func TestGeminiProvider_New(t *testing.T) {
	testCases := []struct {
		name        string
		apiKey      string
		config      types.ProviderConfig
		expectedURL string
	}{
		{
			name:        "default configuration",
			apiKey:      "test-api-key",
			config:      types.ProviderConfig{},
			expectedURL: "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			name:   "custom base URL",
			apiKey: "custom-key",
			config: types.ProviderConfig{
				BaseURL: "https://custom.gemini.ai/v1",
			},
			expectedURL: "https://custom.gemini.ai/v1",
		},
		{
			name:   "with custom headers and timeouts",
			apiKey: "test-key",
			config: types.ProviderConfig{
				Headers: map[string]string{
					"Custom-Header": "custom-value",
				},
				Timeout:    60,
				MaxRetries: func() *int { i := 3; return &i }(),
				RetryDelay: func() *time.Duration { d := 100 * time.Millisecond; return &d }(),
			},
			expectedURL: "https://generativelanguage.googleapis.com/v1beta",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := gemini.New(tc.apiKey, tc.config)

			require.NotNil(t, provider)
			assert.Equal(t, "gemini", provider.Name())
			assert.Equal(t, tc.expectedURL, provider.GetBaseURL())
			
			// Verify that API key is not set in headers (Gemini uses it in URL)
			assert.Empty(t, provider.Config.APIKey)
		})
	}
}

func TestGeminiProvider_Text(t *testing.T) {
	testCases := []struct {
		name                string
		request             types.TextRequest
		mockResponse        map[string]any
		expectedError       string
		expectedText        string
		expectedFinish      types.FinishReason
		expectedUsage       *types.Usage
		verifyRequestFormat bool
	}{
		{
			name: "basic text generation",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Hello, how are you?"),
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "I'm doing well, thank you for asking!"},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":     10,
					"candidatesTokenCount": 12,
					"totalTokenCount":      22,
				},
			},
			expectedText:   "I'm doing well, thank you for asking!",
			expectedFinish: types.FinishReasonStop,
			expectedUsage: &types.Usage{
				PromptTokens:     10,
				CompletionTokens: 12,
				TotalTokens:      22,
			},
			verifyRequestFormat: true,
		},
		{
			name: "with system prompt and parameters",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model:       "gemini-pro",
					MaxTokens:   func(i int) *int { return &i }(100),
					Temperature: func(f float32) *float32 { return &f }(0.7),
					TopP:        func(f float32) *float32 { return &f }(0.9),
					Stop:        []string{"END", "STOP"},
				},
				Messages: []types.Message{
					types.NewUserMessage("Write a short story"),
				},
				SystemPrompt: "You are a creative writer.",
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "Once upon a time..."},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedText:        "Once upon a time...",
			expectedFinish:      types.FinishReasonStop,
			verifyRequestFormat: true,
		},
		{
			name: "with tool calls",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("What's the weather like?"),
				},
				Tools: []types.Tool{
					*types.NewTool(
						"get_weather",
						"Get current weather information",
						map[string]any{
							"type": "object",
							"properties": map[string]any{
								"location": map[string]any{
									"type":        "string",
									"description": "City name",
								},
							},
							"required": []string{"location"},
						},
					),
				},
				ToolChoice: &types.ToolChoice{
					Type: types.ToolChoiceTypeAuto,
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{
									"functionCall": map[string]any{
										"name": "get_weather",
										"args": map[string]any{
											"location": "New York",
										},
									},
								},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedText:        "",
			expectedFinish:      types.FinishReasonStop,
			verifyRequestFormat: true,
		},
		{
			name: "max tokens finish reason",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Write a long essay"),
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "This is a truncated response..."},
							},
							"role": "model",
						},
						"finishReason": "MAX_TOKENS",
					},
				},
			},
			expectedText:   "This is a truncated response...",
			expectedFinish: types.FinishReasonLength,
		},
		{
			name: "safety filter finish reason",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Inappropriate content"),
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": ""},
							},
							"role": "model",
						},
						"finishReason": "SAFETY",
					},
				},
			},
			expectedText:   "",
			expectedFinish: types.FinishReasonContentFilter,
		},
		{
			name: "API error response",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "invalid-model",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test message"),
				},
			},
			mockResponse: map[string]any{
				"error": map[string]any{
					"code":    400,
					"message": "Invalid model specified",
					"status":  "INVALID_ARGUMENT",
				},
			},
			expectedError: "Invalid model specified",
		},
		{
			name: "empty candidates response",
			request: types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Test"),
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{},
			},
			expectedError: "400",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedRequest map[string]any
			var capturedURL string

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedURL = r.URL.String()
				
				// Verify request method and headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				
				// Capture request body
				if tc.verifyRequestFormat {
					var reqBody map[string]any
					err := json.NewDecoder(r.Body).Decode(&reqBody)
					require.NoError(t, err)
					capturedRequest = reqBody
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				if tc.expectedError != "" {
					w.WriteHeader(http.StatusBadRequest)
				} else {
					w.WriteHeader(http.StatusOK)
				}
				
				err := json.NewEncoder(w).Encode(tc.mockResponse)
				require.NoError(t, err)
			}))
			defer server.Close()

			// Create provider with mock server URL
			config := types.ProviderConfig{
				BaseURL: server.URL,
			}
			provider := gemini.New("test-api-key", config)

			// Execute request
			ctx := context.Background()
			response, err := provider.Text(ctx, tc.request)

			// Verify error cases
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				return
			}

			// Verify successful cases
			require.NoError(t, err)
			require.NotNil(t, response)

			assert.Equal(t, tc.expectedText, response.Text)
			assert.Equal(t, tc.expectedFinish, response.FinishReason)

			if tc.expectedUsage != nil {
				require.NotNil(t, response.Usage)
				assert.Equal(t, tc.expectedUsage.PromptTokens, response.Usage.PromptTokens)
				assert.Equal(t, tc.expectedUsage.CompletionTokens, response.Usage.CompletionTokens)
				assert.Equal(t, tc.expectedUsage.TotalTokens, response.Usage.TotalTokens)
			}

			// Verify metadata
			assert.Equal(t, "gemini", response.Metadata["provider"])

			// Verify URL format includes API key
			assert.Contains(t, capturedURL, "key=test-api-key")
			assert.Contains(t, capturedURL, fmt.Sprintf("models/%s:generateContent", tc.request.Model))

			// Verify request format
			if tc.verifyRequestFormat {
				require.NotNil(t, capturedRequest)
				
				// Check contents
				contents, ok := capturedRequest["contents"].([]any)
				require.True(t, ok)
				require.Len(t, contents, len(tc.request.Messages))

				// Check system instruction
				if tc.request.SystemPrompt != "" {
					systemInstr, ok := capturedRequest["systemInstruction"].(map[string]any)
					require.True(t, ok)
					parts, ok := systemInstr["parts"].([]any)
					require.True(t, ok)
					require.Len(t, parts, 1)
					part := parts[0].(map[string]any)
					assert.Equal(t, tc.request.SystemPrompt, part["text"])
				}

				// Check generation config
				if tc.request.MaxTokens != nil || tc.request.Temperature != nil || tc.request.TopP != nil || len(tc.request.Stop) > 0 {
					genConfig, ok := capturedRequest["generationConfig"].(map[string]any)
					require.True(t, ok)
					
					if tc.request.MaxTokens != nil {
						assert.Equal(t, float64(*tc.request.MaxTokens), genConfig["maxOutputTokens"])
					}
					if tc.request.Temperature != nil {
						assert.InDelta(t, float64(*tc.request.Temperature), genConfig["temperature"], 0.001)
					}
					if tc.request.TopP != nil {
						assert.InDelta(t, float64(*tc.request.TopP), genConfig["topP"], 0.001)
					}
					if len(tc.request.Stop) > 0 {
						stopSeqs := genConfig["stopSequences"].([]any)
						assert.Len(t, stopSeqs, len(tc.request.Stop))
					}
				}

				// Check tools
				if len(tc.request.Tools) > 0 {
					tools, ok := capturedRequest["tools"].([]any)
					require.True(t, ok)
					require.Len(t, tools, 1)
					
					tool := tools[0].(map[string]any)
					funcDecls, ok := tool["functionDeclarations"].([]any)
					require.True(t, ok)
					require.Len(t, funcDecls, len(tc.request.Tools))
					
					funcDecl := funcDecls[0].(map[string]any)
					assert.Equal(t, tc.request.Tools[0].Name, funcDecl["name"])
					assert.Equal(t, tc.request.Tools[0].Description, funcDecl["description"])
				}

				// Check tool config
				if tc.request.ToolChoice != nil {
					toolConfig, ok := capturedRequest["toolConfig"].(map[string]any)
					require.True(t, ok)
					funcConfig, ok := toolConfig["functionCallingConfig"].(map[string]any)
					require.True(t, ok)
					assert.Equal(t, "AUTO", funcConfig["mode"])
				}
			}
		})
	}
}

func TestGeminiProvider_Structured(t *testing.T) {
	testCases := []struct {
		name             string
		request          types.StructuredRequest
		mockResponse     map[string]any
		expectedError    string
		expectedData     any
		verifySchema     bool
	}{
		{
			name: "basic structured output",
			request: types.StructuredRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Generate a person object"),
				},
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
						"age":  map[string]any{"type": "number"},
					},
					"required": []string{"name", "age"},
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": `{"name": "John Doe", "age": 30}`},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedData: map[string]any{
				"name": "John Doe",
				"age":  float64(30),
			},
			verifySchema: true,
		},
		{
			name: "invalid JSON response",
			request: types.StructuredRequest{
				BaseRequest: types.BaseRequest{
					Model: "gemini-pro",
				},
				Messages: []types.Message{
					types.NewUserMessage("Generate invalid JSON"),
				},
				Schema: map[string]any{
					"type": "object",
				},
			},
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "invalid json {"},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedError: "failed to parse structured response",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedRequest map[string]any

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Capture request body
				if tc.verifySchema {
					var reqBody map[string]any
					err := json.NewDecoder(r.Body).Decode(&reqBody)
					require.NoError(t, err)
					capturedRequest = reqBody
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				
				err := json.NewEncoder(w).Encode(tc.mockResponse)
				require.NoError(t, err)
			}))
			defer server.Close()

			// Create provider
			config := types.ProviderConfig{
				BaseURL: server.URL,
			}
			provider := gemini.New("test-api-key", config)

			// Execute request
			ctx := context.Background()
			response, err := provider.Structured(ctx, tc.request)

			// Verify error cases
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				return
			}

			// Verify successful cases
			require.NoError(t, err)
			require.NotNil(t, response)

			assert.Equal(t, tc.expectedData, response.Data)

			// Verify schema setup in request
			if tc.verifySchema {
				require.NotNil(t, capturedRequest)
				
				genConfig, ok := capturedRequest["generationConfig"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "application/json", genConfig["responseMimeType"])
				
				responseSchema := genConfig["responseSchema"]
				assert.NotNil(t, responseSchema)
			}
		})
	}
}

func TestGeminiProvider_Embeddings(t *testing.T) {
	testCases := []struct {
		name           string
		request        types.EmbeddingsRequest
		mockResponse   map[string]any
		expectedError  string
		expectedEmbeds []types.Embedding
	}{
		{
			name: "basic embeddings",
			request: types.EmbeddingsRequest{
				Model: "text-embedding-004",
				Input: []string{
					"Hello world",
					"Goodbye world",
				},
			},
			mockResponse: map[string]any{
				"embeddings": []map[string]any{
					{"values": []float64{0.1, 0.2, 0.3}},
					{"values": []float64{0.4, 0.5, 0.6}},
				},
			},
			expectedEmbeds: []types.Embedding{
				{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
				{Index: 1, Embedding: []float64{0.4, 0.5, 0.6}},
			},
		},
		{
			name: "non-embedding model error",
			request: types.EmbeddingsRequest{
				Model: "gemini-pro",
				Input: []string{"test"},
			},
			expectedError: "model must be an embedding model",
		},
		{
			name: "with provider options",
			request: types.EmbeddingsRequest{
				Model: "text-embedding-004",
				Input: []string{"Document text"},
				ProviderOptions: map[string]any{
					"taskType": "SEMANTIC_SIMILARITY",
					"title":    "Document Title",
				},
			},
			mockResponse: map[string]any{
				"embeddings": []map[string]any{
					{"values": []float64{0.1, 0.2, 0.3}},
				},
			},
			expectedEmbeds: []types.Embedding{
				{Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Skip server creation for model validation errors
			if tc.expectedError == "model must be an embedding model" {
				config := types.ProviderConfig{}
				provider := gemini.New("test-api-key", config)
				
				ctx := context.Background()
				_, err := provider.Embeddings(ctx, tc.request)
				
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				return
			}

			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify URL format
				assert.Contains(t, r.URL.String(), "batchEmbedContents")
				assert.Contains(t, r.URL.String(), "key=test-api-key")

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				
				err := json.NewEncoder(w).Encode(tc.mockResponse)
				require.NoError(t, err)
			}))
			defer server.Close()

			// Create provider
			config := types.ProviderConfig{
				BaseURL: server.URL,
			}
			provider := gemini.New("test-api-key", config)

			// Execute request
			ctx := context.Background()
			response, err := provider.Embeddings(ctx, tc.request)

			// Verify successful cases
			require.NoError(t, err)
			require.NotNil(t, response)

			assert.Len(t, response.Embeddings, len(tc.expectedEmbeds))
			for i, expected := range tc.expectedEmbeds {
				actual := response.Embeddings[i]
				assert.Equal(t, expected.Index, actual.Index)
				assert.Equal(t, expected.Embedding, actual.Embedding)
			}

			// Verify metadata
			assert.Equal(t, "gemini", response.Metadata["provider"])
		})
	}
}

func TestGeminiProvider_UnsupportedMethods(t *testing.T) {
	config := types.ProviderConfig{}
	provider := gemini.New("test-api-key", config)

	ctx := context.Background()

	t.Run("Audio not supported", func(t *testing.T) {
		audioReq := types.AudioRequest{
			Model: "gemini-pro",
		}
		
		_, err := provider.Audio(ctx, audioReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gemini provider does not support Audio")
	})

	t.Run("Images not supported", func(t *testing.T) {
		imagesReq := types.ImagesRequest{
			Model: "gemini-pro",
		}
		
		_, err := provider.Images(ctx, imagesReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gemini provider does not support images")
	})
}

func TestGeminiProvider_MessageTransformation(t *testing.T) {
	testCases := []struct {
		name            string
		messages        []types.Message
		expectedContents int
		expectedRoles   []string
	}{
		{
			name: "user and assistant messages",
			messages: []types.Message{
				types.NewUserMessage("Hello"),
				types.NewAssistantMessage("Hi there!"),
				types.NewUserMessage("How are you?"),
			},
			expectedContents: 3,
			expectedRoles:    []string{"user", "model", "user"},
		},
		{
			name: "system message mapped to model",
			messages: []types.Message{
				types.NewSystemMessage("You are helpful"),
			},
			expectedContents: 1,
			expectedRoles:    []string{"model"},
		},
		{
			name: "tool result message",
			messages: []types.Message{
				&types.ToolResultMessage{
					ToolCallID: "tool_123",
					Content:    "Weather result",
				},
			},
			expectedContents: 1,
			expectedRoles:    []string{"function"},
		},
		{
			name: "assistant with tool calls",
			messages: []types.Message{
				&types.AssistantMessage{
					Content: "I'll check the weather",
					ToolCalls: []types.ToolCall{
						{
							ID:   "call_123",
							Name: "get_weather",
							Arguments: map[string]any{
								"location": "NYC",
							},
						},
					},
				},
			},
			expectedContents: 1,
			expectedRoles:    []string{"model"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedRequest map[string]any

			// Create mock server that captures request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var reqBody map[string]any
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				capturedRequest = reqBody

				// Return minimal valid response
				response := map[string]any{
					"candidates": []map[string]any{
						{
							"content": map[string]any{
								"parts": []map[string]any{
									{"text": "response"},
								},
								"role": "model",
							},
							"finishReason": "STOP",
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create provider
			config := types.ProviderConfig{BaseURL: server.URL}
			provider := gemini.New("test-api-key", config)

			// Make request
			request := types.TextRequest{
				BaseRequest: types.BaseRequest{Model: "gemini-pro"},
				Messages:    tc.messages,
			}

			ctx := context.Background()
			_, err := provider.Text(ctx, request)
			require.NoError(t, err)

			// Verify message transformation
			require.NotNil(t, capturedRequest)
			
			contents, ok := capturedRequest["contents"].([]any)
			require.True(t, ok)
			assert.Len(t, contents, tc.expectedContents)

			// Verify roles
			for i, expectedRole := range tc.expectedRoles {
				content := contents[i].(map[string]any)
				assert.Equal(t, expectedRole, content["role"])
			}
		})
	}
}

func TestGeminiProvider_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedError  string
	}{
		{
			name: "HTTP 401 Unauthorized",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    401,
						"message": "Invalid API key",
						"status":  "UNAUTHENTICATED",
					},
				})
			},
			expectedError: "Invalid API key",
		},
		{
			name: "HTTP 429 Rate Limit",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    429,
						"message": "Rate limit exceeded",
						"status":  "RESOURCE_EXHAUSTED",
					},
				})
			},
			expectedError: "Rate limit exceeded",
		},
		{
			name: "HTTP 500 Internal Server Error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    500,
						"message": "Internal server error",
						"status":  "INTERNAL",
					},
				})
			},
			expectedError: "Internal server error",
		},
		{
			name: "Network timeout simulation",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond) // Simulate delay
				w.WriteHeader(http.StatusOK)
			},
			expectedError: "", // Will be tested separately with context timeout
		},
	}

	for _, tc := range testCases {
		if tc.name == "Network timeout simulation" {
			continue // Skip timeout test in this loop
		}

		t.Run(tc.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer server.Close()

			// Create provider
			config := types.ProviderConfig{BaseURL: server.URL}
			provider := gemini.New("test-api-key", config)

			// Make request
			request := types.TextRequest{
				BaseRequest: types.BaseRequest{Model: "gemini-pro"},
				Messages:    []types.Message{types.NewUserMessage("test")},
			}

			ctx := context.Background()
			_, err := provider.Text(ctx, request)

			require.Error(t, err)
			if tc.expectedError != "" {
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}

	// Test context timeout separately
	t.Run("Context timeout", func(t *testing.T) {
		// Create mock server that delays response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": "response"}},
							"role":  "model",
						},
						"finishReason": "STOP",
					},
				},
			})
		}))
		defer server.Close()

		// Create provider
		config := types.ProviderConfig{BaseURL: server.URL}
		provider := gemini.New("test-api-key", config)

		// Create context with short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// Make request
		request := types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-pro"},
			Messages:    []types.Message{types.NewUserMessage("test")},
		}

		_, err := provider.Text(ctx, request)
		require.Error(t, err)
		assert.True(t, 
			strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "timeout"),
		)
	})
}

func TestGeminiProvider_FinishReasonMapping(t *testing.T) {
	testCases := []struct {
		geminiReason string
		expected     types.FinishReason
	}{
		{"STOP", types.FinishReasonStop},
		{"MAX_TOKENS", types.FinishReasonLength},
		{"SAFETY", types.FinishReasonContentFilter},
		{"RECITATION", types.FinishReasonContentFilter},
		{"OTHER", types.FinishReasonOther},
		{"FINISH_REASON_UNSPECIFIED", types.FinishReasonOther},
		{"UNKNOWN_REASON", types.FinishReasonStop}, // Fallback
	}

	for _, tc := range testCases {
		t.Run(tc.geminiReason, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := map[string]any{
					"candidates": []map[string]any{
						{
							"content": map[string]any{
								"parts": []map[string]any{
									{"text": "test response"},
								},
								"role": "model",
							},
							"finishReason": tc.geminiReason,
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			// Create provider
			config := types.ProviderConfig{BaseURL: server.URL}
			provider := gemini.New("test-api-key", config)

			// Make request
			request := types.TextRequest{
				BaseRequest: types.BaseRequest{Model: "gemini-pro"},
				Messages:    []types.Message{types.NewUserMessage("test")},
			}

			ctx := context.Background()
			response, err := provider.Text(ctx, request)

			require.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, tc.expected, response.FinishReason)
		})
	}
}