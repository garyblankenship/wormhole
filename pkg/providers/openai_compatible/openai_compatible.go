package openai_compatible

import (
	"context"
	"fmt"

	"github.com/prism-php/prism-go/pkg/providers/openai"
	"github.com/prism-php/prism-go/pkg/types"
)

const (
	defaultBaseURL = "http://localhost:1234/v1" // LMStudio default
)

// Provider implements an OpenAI-compatible API provider
// This works with LMStudio, vLLM, Ollama's OpenAI API, and other compatible services
type Provider struct {
	*openai.Provider
	name string
}

// New creates a new OpenAI-compatible provider
func New(name string, config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	// Use the OpenAI provider as the base since the API is compatible
	config.APIKey = "" // Most local services don't need API keys
	openaiProvider := openai.New(config)

	return &Provider{
		Provider: openaiProvider,
		name:     name,
	}
}

// NewLMStudio creates a new LMStudio provider with default configuration
func NewLMStudio(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:1234/v1"
	}
	return New("lmstudio", config)
}

// NewVLLM creates a new vLLM provider
func NewVLLM(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:8000/v1"
	}
	return New("vllm", config)
}

// NewOllamaOpenAI creates a new Ollama OpenAI-compatible provider
func NewOllamaOpenAI(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434/v1"
	}
	return New("ollama-openai", config)
}

// NewGeneric creates a generic OpenAI-compatible provider
func NewGeneric(name string, baseURL string, config types.ProviderConfig) *Provider {
	config.BaseURL = baseURL
	return New(name, config)
}

// Name returns the provider name
func (p *Provider) Name() string {
	return p.name
}

// Text generates text using the OpenAI-compatible API
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	response, err := p.Provider.Text(ctx, request)
	if err != nil {
		return nil, err
	}
	
	// Update metadata to reflect the actual provider
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}
	response.Metadata["provider"] = p.name
	
	return response, nil
}

// Stream generates streaming text using the OpenAI-compatible API
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	stream, err := p.Provider.Stream(ctx, request)
	if err != nil {
		return nil, err
	}

	// Create a new channel to modify metadata
	ch := make(chan types.TextChunk)
	
	go func() {
		defer close(ch)
		for chunk := range stream {
			chunk.Model = p.name // Update model to provider name for clarity
			ch <- chunk
		}
	}()

	return ch, nil
}

// Structured generates structured output using the OpenAI-compatible API
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	response, err := p.Provider.Structured(ctx, request)
	if err != nil {
		return nil, err
	}
	
	// Update metadata
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}
	response.Metadata["provider"] = p.name
	
	return response, nil
}

// Embeddings generates embeddings using the OpenAI-compatible API
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	response, err := p.Provider.Embeddings(ctx, request)
	if err != nil {
		return nil, err
	}
	
	// Update metadata
	if response.Metadata == nil {
		response.Metadata = make(map[string]interface{})
	}
	response.Metadata["provider"] = p.name
	
	return response, nil
}

// Audio handles audio requests using the OpenAI-compatible API
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	// Most OpenAI-compatible APIs don't support audio, but we'll try anyway
	return p.Provider.Audio(ctx, request)
}

// Images generates images using the OpenAI-compatible API
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	// Most OpenAI-compatible APIs don't support image generation, but we'll try anyway
	return p.Provider.Images(ctx, request)
}

// SpeechToText handles speech-to-text requests
func (p *Provider) SpeechToText(ctx context.Context, request types.SpeechToTextRequest) (*types.SpeechToTextResponse, error) {
	// Convert to AudioRequest and use the Audio method
	audioReq := types.AudioRequest{
		Type:        types.AudioRequestTypeSTT,
		Model:       request.Model,
		Input:       request.Audio,
		Language:    request.Language,
		Prompt:      request.Prompt,
		Temperature: request.Temperature,
	}
	
	audioResp, err := p.Provider.Audio(ctx, audioReq)
	if err != nil {
		return nil, err
	}
	
	return &types.SpeechToTextResponse{
		Text: audioResp.Text,
	}, nil
}

// TextToSpeech handles text-to-speech requests
func (p *Provider) TextToSpeech(ctx context.Context, request types.TextToSpeechRequest) (*types.TextToSpeechResponse, error) {
	// Convert to AudioRequest and use the Audio method
	audioReq := types.AudioRequest{
		Type:           types.AudioRequestTypeTTS,
		Model:          request.Model,
		Input:          request.Input,
		Voice:          request.Voice,
		Speed:          request.Speed,
		ResponseFormat: request.ResponseFormat,
	}
	
	audioResp, err := p.Provider.Audio(ctx, audioReq)
	if err != nil {
		return nil, err
	}
	
	return &types.TextToSpeechResponse{
		Audio:  audioResp.Audio,
		Format: audioResp.Format,
	}, nil
}

// GenerateImage generates images
func (p *Provider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	// Use the Images method
	return p.Provider.Images(ctx, request)
}

// Custom methods for OpenAI-compatible providers

// ListModels lists available models from the OpenAI-compatible API
func (p *Provider) ListModels(ctx context.Context) (*ModelsResponse, error) {
	var response ModelsResponse
	
	endpoint := fmt.Sprintf("%s/models", p.GetBaseURL())
	if err := p.DoRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	
	return &response, nil
}

// GetModel retrieves information about a specific model
func (p *Provider) GetModel(ctx context.Context, modelID string) (*ModelInfo, error) {
	var response ModelInfo
	
	endpoint := fmt.Sprintf("%s/models/%s", p.GetBaseURL(), modelID)
	if err := p.DoRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	
	return &response, nil
}

// Health checks if the OpenAI-compatible API is healthy
func (p *Provider) Health(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/models", p.GetBaseURL())
	
	// Simple health check by trying to list models
	var response ModelsResponse
	if err := p.DoRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	
	return nil
}

// ModelsResponse represents the response from /models endpoint
type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// ModelInfo represents information about a model
type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	Root    string `json:"root,omitempty"`
	Parent  string `json:"parent,omitempty"`
}