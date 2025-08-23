package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
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

// CreateCompletion creates a completion using the Wormhole provider
func (a *WormholeToOrchestrationAdapter) CreateCompletion(ctx context.Context, req OrchestrationCompletionRequest) (*OrchestrationCompletionResponse, error) {
	start := time.Now()

	// Convert orchestration request to Wormhole request
	temp := float32(req.Temperature)
	wormholeReq := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:       req.Model,
			MaxTokens:   &req.MaxTokens,
			Temperature: &temp,
		},
		Messages: []types.Message{types.NewUserMessage(req.Prompt)},
	}

	// If no model specified, use default
	if wormholeReq.BaseRequest.Model == "" {
		wormholeReq.BaseRequest.Model = a.model
	}

	// Check if provider supports text capability
	textProvider, ok := types.GetTextCapability(a.provider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support text generation", a.provider.Name())
	}

	// Call Wormhole provider
	resp, err := textProvider.Text(ctx, wormholeReq)
	if err != nil {
		return nil, err
	}

	// Convert response
	return &OrchestrationCompletionResponse{
		Content:    resp.Text,
		TokensUsed: resp.Usage.TotalTokens,
		Cost:       a.estimateCostFromUsage(*resp.Usage),
		Provider:   a.name,
		Model:      wormholeReq.BaseRequest.Model,
		Duration:   time.Since(start),
	}, nil
}

// CreateStreamingCompletion creates a streaming completion using the Wormhole provider
func (a *WormholeToOrchestrationAdapter) CreateStreamingCompletion(ctx context.Context, req OrchestrationCompletionRequest, callback OrchestrationStreamCallback) (*OrchestrationCompletionResponse, error) {
	start := time.Now()

	// Convert orchestration request to Wormhole request
	temp := float32(req.Temperature)
	wormholeReq := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:       req.Model,
			MaxTokens:   &req.MaxTokens,
			Temperature: &temp,
		},
		Messages: []types.Message{types.NewUserMessage(req.Prompt)},
	}

	// If no model specified, use default
	if wormholeReq.BaseRequest.Model == "" {
		wormholeReq.BaseRequest.Model = a.model
	}

	// Check if provider supports streaming capability
	streamProvider, ok := types.GetStreamCapability(a.provider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support streaming", a.provider.Name())
	}

	// Call Wormhole provider streaming
	stream, err := streamProvider.Stream(ctx, wormholeReq)
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
		Model:      wormholeReq.BaseRequest.Model,
		Duration:   time.Since(start),
	}, nil
}

// EstimateCost estimates the cost of a request
func (a *WormholeToOrchestrationAdapter) EstimateCost(req OrchestrationCompletionRequest) float64 {
	// Basic cost estimation - can be enhanced with actual pricing data
	model := req.Model
	if model == "" {
		model = a.model
	}

	// Rough estimates based on typical pricing
	var costPer1kTokens float64
	switch a.name {
	case "openai":
		switch model {
		case "gpt-4":
			costPer1kTokens = 0.03
		case "gpt-3.5-turbo":
			costPer1kTokens = 0.002
		default:
			costPer1kTokens = 0.002
		}
	case "anthropic":
		switch model {
		case "claude-3-opus":
			costPer1kTokens = 0.015
		case "claude-3-sonnet":
			costPer1kTokens = 0.003
		default:
			costPer1kTokens = 0.003
		}
	default:
		costPer1kTokens = 0.001 // Default low cost for unknown providers
	}

	// Estimate tokens (rough: 4 chars = 1 token)
	estimatedTokens := len(req.Prompt) / 4
	if req.MaxTokens > 0 {
		estimatedTokens += req.MaxTokens
	} else {
		estimatedTokens += 500 // Default response size
	}

	return float64(estimatedTokens) * costPer1kTokens / 1000
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

	// Check if provider supports text capability
	textProvider, ok := types.GetTextCapability(a.provider)
	if !ok {
		return fmt.Errorf("provider %s does not support text generation", a.provider.Name())
	}

	_, err := textProvider.Text(ctx, req)
	return err
}

// estimateCostFromUsage calculates cost from usage stats
func (a *WormholeToOrchestrationAdapter) estimateCostFromUsage(usage types.Usage) float64 {
	// This would ideally use actual pricing data
	// For now, use rough estimates
	var costPer1kTokens float64

	switch a.name {
	case "openai":
		costPer1kTokens = 0.002
	case "anthropic":
		costPer1kTokens = 0.003
	default:
		costPer1kTokens = 0.001
	}

	return float64(usage.TotalTokens) * costPer1kTokens / 1000
}
