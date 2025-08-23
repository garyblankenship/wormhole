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

// TestGeminiProvider_IntegrationTextGeneration tests the complete text generation flow
func TestGeminiProvider_IntegrationTextGeneration(t *testing.T) {
	testCases := []struct {
		name          string
		model         string
		maxTokens     int
		systemMsg     string
		userMsg       string
		checkHeaders  bool
		mockResponse  map[string]any
		expectedText  string
		expectedUsage *types.Usage
	}{
		{
			name:         "Gemini Pro basic generation",
			model:        "gemini-pro",
			maxTokens:    100,
			systemMsg:    "You are a helpful assistant.",
			userMsg:      "Hello, how are you?",
			checkHeaders: true,
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "I'm doing well, thank you for asking! How can I help you today?"},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":     15,
					"candidatesTokenCount": 18,
					"totalTokenCount":      33,
				},
			},
			expectedText: "I'm doing well, thank you for asking! How can I help you today?",
			expectedUsage: &types.Usage{
				PromptTokens:     15,
				CompletionTokens: 18,
				TotalTokens:      33,
			},
		},
		{
			name:         "Gemini Pro Vision with system prompt",
			model:        "gemini-pro-vision",
			maxTokens:    200,
			systemMsg:    "You are an expert image analyzer.",
			userMsg:      "Describe this image in detail.",
			checkHeaders: true,
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "I can see an image that shows..."},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedText: "I can see an image that shows...",
		},
		{
			name:         "Gemini Flash minimal tokens",
			model:        "gemini-1.5-flash",
			maxTokens:    50,
			systemMsg:    "",
			userMsg:      "Hi!",
			checkHeaders: false,
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "Hello!"},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedText: "Hello!",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Track the actual request sent to the API
			var capturedHeaders http.Header
			var capturedURL string

			// Create a mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedURL = r.URL.String()
				capturedHeaders = r.Header.Clone()

				// Verify request method and basic headers
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Capture and verify the request body
				var reqBody map[string]any
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)

				// Verify model parameter in URL
				assert.Contains(t, r.URL.Path, fmt.Sprintf("models/%s:generateContent", tc.model))
				assert.Contains(t, r.URL.RawQuery, "key=test-api-key")

				// Verify request payload structure
				assert.Contains(t, reqBody, "contents")
				contents := reqBody["contents"].([]any)
				require.Len(t, contents, 1)

				userContent := contents[0].(map[string]any)
				assert.Equal(t, "user", userContent["role"])

				parts := userContent["parts"].([]any)
				require.Len(t, parts, 1)
				textPart := parts[0].(map[string]any)
				assert.Equal(t, tc.userMsg, textPart["text"])

				// Verify system prompt handling
				if tc.systemMsg != "" {
					assert.Contains(t, reqBody, "systemInstruction")
					systemInstr := reqBody["systemInstruction"].(map[string]any)
					sysParts := systemInstr["parts"].([]any)
					require.Len(t, sysParts, 1)
					sysPart := sysParts[0].(map[string]any)
					assert.Equal(t, tc.systemMsg, sysPart["text"])
				} else {
					assert.NotContains(t, reqBody, "systemInstruction")
				}

				// Verify generation config
				if tc.maxTokens > 0 {
					assert.Contains(t, reqBody, "generationConfig")
					genConfig := reqBody["generationConfig"].(map[string]any)
					assert.Equal(t, float64(tc.maxTokens), genConfig["maxOutputTokens"])
				}

				// Return mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				err = json.NewEncoder(w).Encode(tc.mockResponse)
				require.NoError(t, err)
			}))
			defer server.Close()

			// Create provider with mock server URL
			config := types.ProviderConfig{
				BaseURL: server.URL,
			}
			provider := gemini.New("test-api-key", config)

			// Create request
			maxTokens := tc.maxTokens
			request := types.TextRequest{
				BaseRequest: types.BaseRequest{
					Model:     tc.model,
					MaxTokens: func() *int { if maxTokens > 0 { return &maxTokens }; return nil }(),
				},
				Messages:     []types.Message{types.NewUserMessage(tc.userMsg)},
				SystemPrompt: tc.systemMsg,
			}

			// Execute request
			ctx := context.Background()
			response, err := provider.Text(ctx, request)

			// Verify response
			require.NoError(t, err)
			require.NotNil(t, response)

			assert.Equal(t, tc.expectedText, response.Text)
			assert.Equal(t, types.FinishReasonStop, response.FinishReason)

			// Verify usage if expected
			if tc.expectedUsage != nil {
				require.NotNil(t, response.Usage)
				assert.Equal(t, tc.expectedUsage.PromptTokens, response.Usage.PromptTokens)
				assert.Equal(t, tc.expectedUsage.CompletionTokens, response.Usage.CompletionTokens)
				assert.Equal(t, tc.expectedUsage.TotalTokens, response.Usage.TotalTokens)
			}

			// Verify metadata
			assert.Equal(t, "gemini", response.Metadata["provider"])

			// Verify URL format
			assert.Contains(t, capturedURL, "key=test-api-key")
			assert.Contains(t, capturedURL, fmt.Sprintf("models/%s:generateContent", tc.model))

			// Additional header checks if required
			if tc.checkHeaders {
				assert.Equal(t, "application/json", capturedHeaders.Get("Content-Type"))
				// Note: Gemini doesn't use Authorization header, API key is in URL
				// The base provider sets it by default, but Gemini ignores it
				assert.Contains(t, capturedHeaders.Get("Authorization"), "Bearer")
			}
		})
	}
}

func TestGeminiProvider_IntegrationToolCalling(t *testing.T) {
	// Create a mock server that handles tool calling scenario
	var capturedRequests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		capturedRequests = append(capturedRequests, reqBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Simulate a tool call response
		response := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{
								"functionCall": map[string]any{
									"name": "get_weather",
									"args": map[string]any{
										"location": "San Francisco",
										"units":    "celsius",
									},
								},
							},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     25,
				"candidatesTokenCount": 5,
				"totalTokenCount":      30,
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	config := types.ProviderConfig{BaseURL: server.URL}
	provider := gemini.New("test-api-key", config)

	// Create request with tools
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-pro",
		},
		Messages: []types.Message{
			types.NewUserMessage("What's the weather like in San Francisco?"),
		},
		Tools: []types.Tool{
			*types.NewTool(
				"get_weather",
				"Get current weather information for a location",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The city and state, e.g. San Francisco, CA",
						},
						"units": map[string]any{
							"type":        "string",
							"enum":        []string{"celsius", "fahrenheit"},
							"description": "The units for temperature",
						},
					},
					"required": []string{"location"},
				},
			),
		},
		ToolChoice: &types.ToolChoice{
			Type: types.ToolChoiceTypeAuto,
		},
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Text(ctx, request)

	// Verify successful response
	require.NoError(t, err)
	require.NotNil(t, response)

	// Should have no text content but tool calls
	assert.Empty(t, response.Text)
	assert.Len(t, response.ToolCalls, 1)

	// Verify tool call details
	toolCall := response.ToolCalls[0]
	assert.Equal(t, "get_weather", toolCall.ID)   // Gemini uses function name as ID
	assert.Equal(t, "get_weather", toolCall.Name)
	
	assert.Equal(t, "San Francisco", toolCall.Arguments["location"])
	assert.Equal(t, "celsius", toolCall.Arguments["units"])

	// Verify usage
	require.NotNil(t, response.Usage)
	assert.Equal(t, 25, response.Usage.PromptTokens)
	assert.Equal(t, 5, response.Usage.CompletionTokens)
	assert.Equal(t, 30, response.Usage.TotalTokens)

	// Verify request format
	require.Len(t, capturedRequests, 1)
	req := capturedRequests[0]

	// Check tools format
	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)

	tool := tools[0].(map[string]any)
	funcDecls := tool["functionDeclarations"].([]any)
	require.Len(t, funcDecls, 1)

	funcDecl := funcDecls[0].(map[string]any)
	assert.Equal(t, "get_weather", funcDecl["name"])
	assert.Equal(t, "Get current weather information for a location", funcDecl["description"])

	params := funcDecl["parameters"].(map[string]any)
	assert.Equal(t, "object", params["type"])

	// Check tool config
	toolConfig, ok := req["toolConfig"].(map[string]any)
	require.True(t, ok)
	funcConfig := toolConfig["functionCallingConfig"].(map[string]any)
	assert.Equal(t, "AUTO", funcConfig["mode"])
}

func TestGeminiProvider_IntegrationMultimodalMessage(t *testing.T) {
	var capturedRequest map[string]any

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		capturedRequest = reqBody

		// Return mock response
		response := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "I can see a beautiful image of a sunset over mountains."},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	config := types.ProviderConfig{BaseURL: server.URL}
	provider := gemini.New("test-api-key", config)

	// Create multimodal message
	imageData := []byte("fake image data for testing")
	userMessage := &types.UserMessage{
		Content: "What do you see in this image?",
		Media: []types.Media{
			&types.ImageMedia{
				MimeType: "image/jpeg",
				Data:     imageData,
			},
		},
	}

	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-pro-vision",
		},
		Messages:     []types.Message{userMessage},
		SystemPrompt: "You are an expert image analyst.",
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Text(ctx, request)

	// Verify successful response
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "I can see a beautiful image of a sunset over mountains.", response.Text)

	// Verify request format
	require.NotNil(t, capturedRequest)

	// Check contents structure
	contents := capturedRequest["contents"].([]any)
	require.Len(t, contents, 1)

	userContent := contents[0].(map[string]any)
	assert.Equal(t, "user", userContent["role"])

	parts := userContent["parts"].([]any)
	require.Len(t, parts, 2) // Text + image

	// Check text part
	textPart := parts[0].(map[string]any)
	assert.Equal(t, "What do you see in this image?", textPart["text"])

	// Check image part
	imagePart := parts[1].(map[string]any)
	require.Contains(t, imagePart, "inlineData")
	
	inlineData := imagePart["inlineData"].(map[string]any)
	assert.Equal(t, "image/jpeg", inlineData["mimeType"])
	assert.NotEmpty(t, inlineData["data"]) // Should be base64 encoded
}

func TestGeminiProvider_IntegrationStructuredOutput(t *testing.T) {
	var capturedRequest map[string]any

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		capturedRequest = reqBody

		// Return structured JSON response
		response := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": `{"name": "John Doe", "age": 30, "occupation": "Software Engineer", "skills": ["Go", "Python", "JavaScript"]}`},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     20,
				"candidatesTokenCount": 25,
				"totalTokenCount":      45,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	config := types.ProviderConfig{BaseURL: server.URL}
	provider := gemini.New("test-api-key", config)

	// Create structured request
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Person's full name",
			},
			"age": map[string]any{
				"type":        "number",
				"description": "Person's age in years",
			},
			"occupation": map[string]any{
				"type":        "string",
				"description": "Person's job title",
			},
			"skills": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "List of technical skills",
			},
		},
		"required": []string{"name", "age", "occupation"},
	}

	request := types.StructuredRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-pro",
		},
		Messages: []types.Message{
			types.NewUserMessage("Generate a person profile with technical skills"),
		},
		Schema: schema,
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Structured(ctx, request)

	// Verify successful response
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify structured data
	expectedData := map[string]any{
		"name":       "John Doe",
		"age":        float64(30),
		"occupation": "Software Engineer",
		"skills":     []any{"Go", "Python", "JavaScript"},
	}
	assert.Equal(t, expectedData, response.Data)

	// Verify usage
	require.NotNil(t, response.Usage)
	assert.Equal(t, 20, response.Usage.PromptTokens)
	assert.Equal(t, 25, response.Usage.CompletionTokens)
	assert.Equal(t, 45, response.Usage.TotalTokens)

	// Verify request format
	require.NotNil(t, capturedRequest)

	// Check generation config has structured output settings
	genConfig := capturedRequest["generationConfig"].(map[string]any)
	assert.Equal(t, "application/json", genConfig["responseMimeType"])

	responseSchema := genConfig["responseSchema"]
	assert.NotNil(t, responseSchema)
	// Schema transformation is tested in transform_test.go
}

func TestGeminiProvider_IntegrationEmbeddings(t *testing.T) {
	var capturedRequest map[string]any

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		capturedRequest = reqBody

		// Verify URL
		assert.Contains(t, r.URL.Path, "batchEmbedContents")

		// Return mock embeddings
		response := map[string]any{
			"embeddings": []map[string]any{
				{"values": []float64{0.1, 0.2, 0.3, 0.4, 0.5}},
				{"values": []float64{0.6, 0.7, 0.8, 0.9, 1.0}},
				{"values": []float64{1.1, 1.2, 1.3, 1.4, 1.5}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	config := types.ProviderConfig{BaseURL: server.URL}
	provider := gemini.New("test-api-key", config)

	// Create embeddings request
	request := types.EmbeddingsRequest{
		Model: "text-embedding-004",
		Input: []string{
			"Hello world",
			"Natural language processing",
			"Machine learning embeddings",
		},
		ProviderOptions: map[string]any{
			"taskType": "SEMANTIC_SIMILARITY",
			"title":    "Document similarity analysis",
		},
	}

	// Execute request
	ctx := context.Background()
	response, err := provider.Embeddings(ctx, request)

	// Verify successful response
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify embeddings
	require.Len(t, response.Embeddings, 3)
	
	for i, embedding := range response.Embeddings {
		assert.Equal(t, i, embedding.Index)
		assert.Len(t, embedding.Embedding, 5)
		
		// Verify embedding values
		expectedStart := float64(i)*0.5 + 0.1
		assert.Equal(t, expectedStart, embedding.Embedding[0])
	}

	// Verify metadata
	assert.Equal(t, "gemini", response.Metadata["provider"])

	// Verify request format
	require.NotNil(t, capturedRequest)

	requests := capturedRequest["requests"].([]any)
	require.Len(t, requests, 3)

	// Check first request
	firstReq := requests[0].(map[string]any)
	content := firstReq["content"].(map[string]any)
	parts := content["parts"].([]any)
	require.Len(t, parts, 1)
	
	textPart := parts[0].(map[string]any)
	assert.Equal(t, "Hello world", textPart["text"])

	// Check provider options
	assert.Equal(t, "SEMANTIC_SIMILARITY", firstReq["taskType"])
	assert.Equal(t, "Document similarity analysis", firstReq["title"])
}

func TestGeminiProvider_IntegrationErrorHandling(t *testing.T) {
	testCases := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedError  string
		expectStatus   int
	}{
		{
			name: "API Key Invalid",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				response := map[string]any{
					"error": map[string]any{
						"code":    401,
						"message": "API key not valid. Please pass a valid API key.",
						"status":  "UNAUTHENTICATED",
					},
				}
				json.NewEncoder(w).Encode(response)
			},
			expectedError: "API key not valid",
			expectStatus:  401,
		},
		{
			name: "Rate Limit Exceeded",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				response := map[string]any{
					"error": map[string]any{
						"code":    429,
						"message": "Rate limit exceeded. Please try again later.",
						"status":  "RESOURCE_EXHAUSTED",
					},
				}
				json.NewEncoder(w).Encode(response)
			},
			expectedError: "Rate limit exceeded",
			expectStatus:  429,
		},
		{
			name: "Invalid Model",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				response := map[string]any{
					"error": map[string]any{
						"code":    404,
						"message": "Model 'invalid-model' not found.",
						"status":  "NOT_FOUND",
					},
				}
				json.NewEncoder(w).Encode(response)
			},
			expectedError: "Model 'invalid-model' not found",
			expectStatus:  404,
		},
		{
			name: "Request Too Large",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				response := map[string]any{
					"error": map[string]any{
						"code":    400,
						"message": "Request payload too large.",
						"status":  "INVALID_ARGUMENT",
					},
				}
				json.NewEncoder(w).Encode(response)
			},
			expectedError: "Request payload too large",
			expectStatus:  400,
		},
		{
			name: "Server Error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				response := map[string]any{
					"error": map[string]any{
						"code":    500,
						"message": "Internal server error.",
						"status":  "INTERNAL",
					},
				}
				json.NewEncoder(w).Encode(response)
			},
			expectedError: "Internal server error",
			expectStatus:  500,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer server.Close()

			// Create provider
			config := types.ProviderConfig{BaseURL: server.URL}
			provider := gemini.New("test-api-key", config)

			// Test different methods
			ctx := context.Background()

			t.Run("Text method", func(t *testing.T) {
				request := types.TextRequest{
					BaseRequest: types.BaseRequest{Model: "gemini-pro"},
					Messages:    []types.Message{types.NewUserMessage("test")},
				}

				_, err := provider.Text(ctx, request)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)

				// Check if it's a WormholeError with correct status
				if wormholeErr, ok := err.(*types.WormholeError); ok {
					assert.Equal(t, tc.expectStatus, wormholeErr.StatusCode)
					assert.Equal(t, "gemini", wormholeErr.Provider)
				}
			})

			t.Run("Structured method", func(t *testing.T) {
				request := types.StructuredRequest{
					BaseRequest: types.BaseRequest{Model: "gemini-pro"},
					Messages:    []types.Message{types.NewUserMessage("test")},
					Schema:      map[string]any{"type": "object"},
				}

				_, err := provider.Structured(ctx, request)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			})

			t.Run("Embeddings method", func(t *testing.T) {
				request := types.EmbeddingsRequest{
					Model: "text-embedding-004",
					Input: []string{"test"},
				}

				_, err := provider.Embeddings(ctx, request)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			})
		})
	}
}

func TestGeminiProvider_IntegrationTimeout(t *testing.T) {
	// Create server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		
		response := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{{"text": "response"}},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider with timeout
	config := types.ProviderConfig{
		BaseURL: server.URL,
		Timeout: 1, // 1 second timeout
	}
	provider := gemini.New("test-api-key", config)

	// Test with context timeout (shorter than server delay)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

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
}

func TestGeminiProvider_IntegrationConversation(t *testing.T) {
	// Simulate a multi-turn conversation with tool usage
	var requestCount int
	var capturedRequests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		capturedRequests = append(capturedRequests, reqBody)

		var response map[string]any

		switch requestCount {
		case 0:
			// First request: User asks about weather, AI responds with tool call
			response = map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{
									"functionCall": map[string]any{
										"name": "get_weather",
										"args": map[string]any{
											"location": "San Francisco",
										},
									},
								},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			}
		case 1:
			// Second request: After tool result, AI provides final answer
			response = map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "The weather in San Francisco is currently 22°C and sunny. It's a beautiful day!"},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			}
		}

		requestCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider
	config := types.ProviderConfig{BaseURL: server.URL}
	provider := gemini.New("test-api-key", config)

	ctx := context.Background()

	// First request: User asks about weather
	request1 := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-pro",
		},
		Messages: []types.Message{
			types.NewUserMessage("What's the weather like in San Francisco?"),
		},
		Tools: []types.Tool{
			*types.NewTool("get_weather", "Get weather info", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{"type": "string"},
				},
				"required": []string{"location"},
			}),
		},
		ToolChoice: &types.ToolChoice{Type: types.ToolChoiceTypeAuto},
	}

	response1, err := provider.Text(ctx, request1)
	require.NoError(t, err)
	require.Len(t, response1.ToolCalls, 1)
	assert.Equal(t, "get_weather", response1.ToolCalls[0].Name)

	// Second request: Provide tool result and continue conversation
	messages := []types.Message{
		types.NewUserMessage("What's the weather like in San Francisco?"),
		&types.AssistantMessage{
			Content:   "",
			ToolCalls: response1.ToolCalls,
		},
		&types.ToolResultMessage{
			ToolCallID: response1.ToolCalls[0].ID,
			Content:    "Temperature: 22°C, Conditions: Sunny",
		},
	}

	request2 := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-pro",
		},
		Messages: messages,
		Tools:    request1.Tools,
	}

	response2, err := provider.Text(ctx, request2)
	require.NoError(t, err)
	assert.Contains(t, response2.Text, "22°C")
	assert.Contains(t, response2.Text, "sunny")

	// Verify conversation structure in requests
	require.Len(t, capturedRequests, 2)

	// First request should have 1 message
	contents1 := capturedRequests[0]["contents"].([]any)
	assert.Len(t, contents1, 1)

	// Second request should have 3 messages (user + assistant + tool result)
	contents2 := capturedRequests[1]["contents"].([]any)
	assert.Len(t, contents2, 3)

	// Verify message roles
	assert.Equal(t, "user", contents2[0].(map[string]any)["role"])
	assert.Equal(t, "model", contents2[1].(map[string]any)["role"])
	assert.Equal(t, "function", contents2[2].(map[string]any)["role"])
}