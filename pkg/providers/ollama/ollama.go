package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// No default base URLs - Ollama must be configured with explicit URL

// Provider implements the Ollama provider
type Provider struct {
	*providers.BaseProvider
}

// New creates a new Ollama provider
func New(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		panic("Ollama BaseURL is required: provide via config.BaseURL or environment variable")
	}

	return &Provider{
		BaseProvider: providers.NewBaseProvider("ollama", config),
	}
}

// Text generates a text response using Ollama's chat API
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload := p.buildChatPayload(&request)

	url := p.GetBaseURL() + "/api/chat"

	var response chatResponse
	err := p.doOllamaRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformTextResponse(&response), nil
}

// Stream generates a streaming text response using Ollama's streaming chat API
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload := p.buildChatPayload(&request)
	payload.Stream = true

	url := p.GetBaseURL() + "/api/chat"

	body, err := p.streamOllamaRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return utils.ProcessStream(body, p.parseStreamChunk, 100), nil
}

// Structured generates a structured response using JSON mode
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Convert to text request with JSON mode
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	// Use JSON format for structured output
	if request.Mode == types.StructuredModeJSON {
		textRequest.ResponseFormat = map[string]string{"type": "json_object"}
	} else {
		// Ollama doesn't support function calling, fallback to JSON mode
		textRequest.ResponseFormat = map[string]string{"type": "json_object"}

		// Add schema instruction to system prompt or last user message
		schemaBytes, err := json.Marshal(request.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal schema: %w", err)
		}

		schemaInstruction := fmt.Sprintf("Please respond with valid JSON that conforms to this schema: %s", string(schemaBytes))

		if textRequest.SystemPrompt != "" {
			textRequest.SystemPrompt += "\n\n" + schemaInstruction
		} else {
			// Add to last user message
			if len(textRequest.Messages) > 0 {
				lastMsg := textRequest.Messages[len(textRequest.Messages)-1]
				if userMsg, ok := lastMsg.(*types.UserMessage); ok {
					userMsg.Content = userMsg.Content + "\n\n" + schemaInstruction
				}
			}
		}
	}

	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var data any
	err = json.Unmarshal([]byte(response.Text), &data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse structured response: %w", err)
	}

	return &types.StructuredResponse{
		ID:      response.ID,
		Model:   response.Model,
		Data:    data,
		Usage:   response.Usage,
		Created: response.Created,
	}, nil
}

// Embeddings generates embeddings using Ollama's embeddings API
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	// Ollama embeddings API processes one input at a time
	// For multiple inputs, we process them concurrently for better performance
	if len(request.Input) == 0 {
		return nil, fmt.Errorf("no input provided for embeddings")
	}

	// For small batches, process sequentially to avoid overwhelming local Ollama instance
	if len(request.Input) <= 3 {
		return p.processEmbeddingsSequentially(ctx, request)
	}

	// For larger batches, use concurrent processing
	return p.processEmbeddingsConcurrently(ctx, request)
}

// processEmbeddingsSequentially handles small batches sequentially
func (p *Provider) processEmbeddingsSequentially(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	embeddings := make([]types.Embedding, 0, len(request.Input))

	for i, input := range request.Input {
		payload := &embeddingsRequest{
			Model:  request.Model,
			Prompt: input,
		}

		url := p.GetBaseURL() + "/api/embeddings"

		var response embeddingsResponse
		err := p.doOllamaRequest(ctx, http.MethodPost, url, payload, &response)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding for input %d: %w", i, err)
		}

		embeddings = append(embeddings, types.Embedding{
			Index:     i,
			Embedding: response.Embedding,
		})
	}

	return &types.EmbeddingsResponse{
		Model:      request.Model,
		Embeddings: embeddings,
		Usage:      nil, // Ollama doesn't provide usage info for embeddings
		Created:    time.Now(),
	}, nil
}

// processEmbeddingsConcurrently handles larger batches with controlled concurrency
func (p *Provider) processEmbeddingsConcurrently(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	type result struct {
		index     int
		embedding types.Embedding
		err       error
	}

	// Limit concurrency to avoid overwhelming local Ollama instance
	const maxConcurrency = 3
	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan result, len(request.Input))

	// Start concurrent workers
	for i, input := range request.Input {
		go func(idx int, txt string) {
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			payload := &embeddingsRequest{
				Model:  request.Model,
				Prompt: txt,
			}

			url := p.GetBaseURL() + "/api/embeddings"

			var response embeddingsResponse
			err := p.doOllamaRequest(ctx, http.MethodPost, url, payload, &response)

			if err != nil {
				results <- result{index: idx, err: fmt.Errorf("failed to get embedding for input %d: %w", idx, err)}
			} else {
				results <- result{
					index: idx,
					embedding: types.Embedding{
						Index:     idx,
						Embedding: response.Embedding,
					},
				}
			}
		}(i, input)
	}

	// Collect results
	embeddings := make([]types.Embedding, len(request.Input))
	for i := 0; i < len(request.Input); i++ {
		res := <-results
		if res.err != nil {
			return nil, res.err
		}
		embeddings[res.index] = res.embedding
	}

	return &types.EmbeddingsResponse{
		Model:      request.Model,
		Embeddings: embeddings,
		Usage:      nil, // Ollama doesn't provide usage info for embeddings
		Created:    time.Now(),
	}, nil
}

// Images generates images - Ollama doesn't support image generation natively
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, p.NotImplementedError("Images - Ollama does not support image generation")
}

// Audio handles both speech-to-text and text-to-speech
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if request.Type == types.AudioRequestTypeSTT {
		return p.handleSpeechToText(ctx, request)
	}

	// Ollama doesn't support TTS
	return nil, p.NotImplementedError("TextToSpeech - Ollama does not support text-to-speech")
}

// handleSpeechToText handles speech-to-text requests
func (p *Provider) handleSpeechToText(_ context.Context, _ types.AudioRequest) (*types.AudioResponse, error) {
	// Ollama doesn't support speech-to-text directly
	return nil, p.NotImplementedError("SpeechToText - Ollama does not support speech-to-text")
}

// SpeechToText handles speech-to-text conversion - not supported by Ollama
func (p *Provider) SpeechToText(ctx context.Context, request types.SpeechToTextRequest) (*types.SpeechToTextResponse, error) {
	return nil, p.NotImplementedError("SpeechToText - Ollama does not support speech-to-text")
}

// TextToSpeech handles text-to-speech conversion - not supported by Ollama
func (p *Provider) TextToSpeech(ctx context.Context, request types.TextToSpeechRequest) (*types.TextToSpeechResponse, error) {
	return nil, p.NotImplementedError("TextToSpeech - Ollama does not support text-to-speech")
}

// GenerateImage generates an image - not supported by Ollama
func (p *Provider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return nil, p.NotImplementedError("GenerateImage - Ollama does not support image generation")
}

// ListModels lists available Ollama models
func (p *Provider) ListModels(ctx context.Context) (*modelsResponse, error) {
	url := p.GetBaseURL() + "/api/tags"

	var response modelsResponse
	err := p.doOllamaRequest(ctx, http.MethodGet, url, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// PullModel pulls a model from Ollama registry
func (p *Provider) PullModel(ctx context.Context, model string) error {
	payload := map[string]any{
		"name": model,
	}

	url := p.GetBaseURL() + "/api/pull"

	// This is a streaming endpoint but we'll treat it as regular request for simplicity
	var response map[string]any // Ollama returns various status messages
	err := p.doOllamaRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return fmt.Errorf("failed to pull model %s: %w", model, err)
	}

	return nil
}

// ShowModel shows information about a model
func (p *Provider) ShowModel(ctx context.Context, model string) (map[string]any, error) {
	payload := map[string]any{
		"name": model,
	}

	url := p.GetBaseURL() + "/api/show"

	var response map[string]any
	err := p.doOllamaRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to show model %s: %w", model, err)
	}

	return response, nil
}

// DeleteModel deletes a model from Ollama
func (p *Provider) DeleteModel(ctx context.Context, model string) error {
	payload := map[string]any{
		"name": model,
	}

	url := p.GetBaseURL() + "/api/delete"

	var response map[string]any
	err := p.doOllamaRequest(ctx, http.MethodDelete, url, payload, &response)
	if err != nil {
		return fmt.Errorf("failed to delete model %s: %w", model, err)
	}

	return nil
}

// doOllamaRequest performs HTTP requests without Bearer authentication
func (p *Provider) doOllamaRequest(ctx context.Context, method, url string, body any, result any) error {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers - Ollama doesn't require authentication by default
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)

	// Set custom headers from config
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Ollama returns simple error messages, not structured like other APIs
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// streamOllamaRequest performs streaming HTTP requests without Bearer authentication
func (p *Provider) streamOllamaRequest(ctx context.Context, method, url string, body any) (io.ReadCloser, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)
	req.Header.Set(types.HeaderAccept, types.ContentTypeEventStream)
	req.Header.Set(types.HeaderCacheControl, "no-cache")

	// Set custom headers from config
	for k, v := range p.Config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return resp.Body, nil
}
