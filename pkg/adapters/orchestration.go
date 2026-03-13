package adapters

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Provider name constants
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
)

// OrchestrationProvider interface - matches what orchestration package expects
type OrchestrationProvider interface {
	Name() string
	CreateCompletion(ctx context.Context, req OrchestrationCompletionRequest) (*OrchestrationCompletionResponse, error)
	CreateStreamingCompletion(ctx context.Context, req OrchestrationCompletionRequest, callback OrchestrationStreamCallback) (*OrchestrationCompletionResponse, error)
	EstimateCost(req OrchestrationCompletionRequest) float64
	HealthCheck(ctx context.Context) error
}

// OrchestrationCompletionRequest represents a request to an AI provider
type OrchestrationCompletionRequest struct {
	Prompt      string
	Model       string
	MaxTokens   int
	Temperature float64
	Variables   map[string]any
}

// OrchestrationCompletionResponse represents a response from an AI provider
type OrchestrationCompletionResponse struct {
	Content    string
	TokensUsed int
	Cost       float64
	Provider   string
	Model      string
	Duration   time.Duration
}

// OrchestrationStreamCallback for handling streaming responses
type OrchestrationStreamCallback func(chunk string, done bool) error

// WormholeToOrchestrationAdapter adapts a Wormhole provider to work with orchestration package
type WormholeToOrchestrationAdapter struct {
	provider types.Provider
	name     string
	model    string
}

// NewWormholeToOrchestrationAdapter creates a new adapter
func NewWormholeToOrchestrationAdapter(provider types.Provider, name string, defaultModel string) *WormholeToOrchestrationAdapter {
	return &WormholeToOrchestrationAdapter{
		provider: provider,
		name:     name,
		model:    defaultModel,
	}
}

// Name returns the provider name
func (a *WormholeToOrchestrationAdapter) Name() string {
	return a.name
}

// buildWormholeRequest converts an orchestration request to a Wormhole TextRequest.
func (a *WormholeToOrchestrationAdapter) buildWormholeRequest(req OrchestrationCompletionRequest) types.TextRequest {
	temp := float32(req.Temperature)
	wormholeReq := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:       req.Model,
			MaxTokens:   &req.MaxTokens,
			Temperature: &temp,
		},
		Messages: []types.Message{types.NewUserMessage(req.Prompt)},
	}

	if wormholeReq.Model == "" {
		wormholeReq.Model = a.model
	}

	return wormholeReq
}

// CreateCompletion creates a completion using the Wormhole provider
func (a *WormholeToOrchestrationAdapter) CreateCompletion(ctx context.Context, req OrchestrationCompletionRequest) (*OrchestrationCompletionResponse, error) {
	start := time.Now()
	wormholeReq := a.buildWormholeRequest(req)

	// Call Wormhole provider
	resp, err := a.provider.Text(ctx, wormholeReq)
	if err != nil {
		return nil, err
	}

	// Convert response
	return &OrchestrationCompletionResponse{
		Content:    resp.Text,
		TokensUsed: resp.Usage.TotalTokens,
		Cost:       a.estimateCostFromUsage(*resp.Usage),
		Provider:   a.name,
		Model:      wormholeReq.Model,
		Duration:   time.Since(start),
	}, nil
}

// CreateStreamingCompletion creates a streaming completion using the Wormhole provider
func (a *WormholeToOrchestrationAdapter) CreateStreamingCompletion(ctx context.Context, req OrchestrationCompletionRequest, callback OrchestrationStreamCallback) (*OrchestrationCompletionResponse, error) {
	start := time.Now()
	wormholeReq := a.buildWormholeRequest(req)

	// Call Wormhole provider streaming
	stream, err := a.provider.Stream(ctx, wormholeReq)
	if err != nil {
		return nil, err
	}

	var fullContent string
	var totalTokens int

	// Process stream
	for chunk := range stream {
		if chunk.Error != nil {
			return nil, chunk.Error
		}

		if chunk.Delta != nil {
			fullContent += chunk.Delta.Content
		}
		totalTokens++

		// Call the callback
		var content string
		if chunk.Delta != nil {
			content = chunk.Delta.Content
		}
		// Check if we're done (no more content and finish reason is set)
		done := chunk.FinishReason != nil
		if err := callback(content, done); err != nil {
			return nil, err
		}
	}

	// Return final response
	return &OrchestrationCompletionResponse{
		Content:    fullContent,
		TokensUsed: totalTokens,
		Cost:       a.EstimateCost(req),
		Provider:   a.name,
		Model:      wormholeReq.Model,
		Duration:   time.Since(start),
	}, nil
}

// costPer1kTokens returns the cost per 1K tokens for a provider/model combination.
func (a *WormholeToOrchestrationAdapter) costPer1kTokens(model string) float64 {
	switch a.name {
	case ProviderOpenAI:
		switch model {
		case "gpt-5":
			return 0.0125
		case "gpt-5-mini":
			return 0.0001
		default:
			return 0.002
		}
	case ProviderAnthropic:
		return 0.003
	default:
		return 0.001
	}
}

func (a *WormholeToOrchestrationAdapter) EstimateCost(req OrchestrationCompletionRequest) float64 {
	model := req.Model
	if model == "" {
		model = a.model
	}

	rate := a.costPer1kTokens(model)

	// Estimate tokens (rough: 4 chars = 1 token)
	estimatedTokens := len(req.Prompt) / 4
	if req.MaxTokens > 0 {
		estimatedTokens += req.MaxTokens
	} else {
		estimatedTokens += 500 // Default response size
	}

	return float64(estimatedTokens) * rate / 1000
}

// HealthCheck performs a health check on the provider
func (a *WormholeToOrchestrationAdapter) HealthCheck(ctx context.Context) error {
	// Simple health check - try a minimal request
	maxTokens := 1
	req := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     a.model,
			MaxTokens: &maxTokens,
		},
		Messages: []types.Message{types.NewUserMessage("test")},
	}

	_, err := a.provider.Text(ctx, req)
	return err
}

// estimateCostFromUsage calculates cost from usage stats
func (a *WormholeToOrchestrationAdapter) estimateCostFromUsage(usage types.Usage) float64 {
	return float64(usage.TotalTokens) * a.costPer1kTokens("") / 1000
}
