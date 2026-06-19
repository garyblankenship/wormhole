package wormhole

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// OpenAICompatibleSmokeConfig controls a one-request smoke check for an
// OpenAI-compatible chat-completions endpoint.
type OpenAICompatibleSmokeConfig struct {
	BaseURL string
	Model   string
	Prompt  string
	APIKey  string
	NoAuth  bool
	Timeout time.Duration
}

// OpenAICompatibleSmokeResult contains the parsed response and endpoint facts
// from RunOpenAICompatibleSmoke.
type OpenAICompatibleSmokeResult struct {
	BaseURL string
	Model   string
	Text    string
}

// RunOpenAICompatibleSmoke sends one chat completion through the same adapter
// Wormhole uses for OpenAI-compatible providers. It validates request
// serialization and response parsing by making the real call; callers can use
// httptest or a local server to assert exact path/body/header behavior.
func RunOpenAICompatibleSmoke(ctx context.Context, cfg OpenAICompatibleSmokeConfig) (*OpenAICompatibleSmokeResult, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai-compatible smoke: base URL is required, usually http://host:port/v1")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("openai-compatible smoke: model is required")
	}
	if cfg.Prompt == "" {
		cfg.Prompt = "hello"
	}
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	providerConfig := types.ProviderConfig{
		APIKey:        cfg.APIKey,
		NoAuth:        cfg.NoAuth || cfg.APIKey == "",
		DynamicModels: true,
	}.WithNoRetries()

	client := New(
		WithOpenAICompatible("openai-compatible-smoke", cfg.BaseURL, providerConfig),
		WithDefaultProvider("openai-compatible-smoke"),
		WithDiscovery(false),
	)
	defer func() { _ = client.Close() }()

	resp, err := client.Text().
		Model(cfg.Model).
		Prompt(cfg.Prompt).
		Generate(ctx)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible smoke failed for %s model %q: %w", cfg.BaseURL, cfg.Model, err)
	}
	if resp == nil || resp.Text == "" {
		return nil, fmt.Errorf("openai-compatible smoke failed for %s model %q: empty parsed response", cfg.BaseURL, cfg.Model)
	}
	return &OpenAICompatibleSmokeResult{
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Text:    resp.Text,
	}, nil
}
