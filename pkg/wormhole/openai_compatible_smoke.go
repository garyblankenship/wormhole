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
	BaseURL         string
	Model           string
	EmbeddingsModel string
	Prompt          string
	APIKey          string
	NoAuth          bool
	Timeout         time.Duration
	ProviderOptions map[string]any
	CheckStreaming  bool
	CheckEmbeddings bool
}

// OpenAICompatibleSmokeResult contains the parsed response and endpoint facts
// from RunOpenAICompatibleSmoke.
type OpenAICompatibleSmokeResult struct {
	BaseURL string
	Model   string
	Text    string
	Checks  []OpenAICompatibleSmokeCheck
}

// OpenAICompatibleSmokeCheck describes one protocol surface checked by
// RunOpenAICompatibleSmoke.
type OpenAICompatibleSmokeCheck struct {
	Name   string
	Passed bool
	Error  string
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

	result := &OpenAICompatibleSmokeResult{
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	}

	resp, err := client.Text().
		Model(cfg.Model).
		Prompt(cfg.Prompt).
		ProviderOptions(cfg.ProviderOptions).
		Generate(ctx)
	if err != nil {
		result.addCheck("chat", err)
		return result, fmt.Errorf("openai-compatible smoke chat failed for %s model %q: %w", cfg.BaseURL, cfg.Model, err)
	}
	if resp == nil || resp.Text == "" {
		err := fmt.Errorf("empty parsed response")
		result.addCheck("chat", err)
		return result, fmt.Errorf("openai-compatible smoke chat failed for %s model %q: %w", cfg.BaseURL, cfg.Model, err)
	}
	result.Text = resp.Text
	result.addCheck("chat", nil)

	if cfg.CheckStreaming {
		if err := runOpenAICompatibleStreamCheck(ctx, client, cfg); err != nil {
			result.addCheck("stream", err)
			return result, fmt.Errorf("openai-compatible smoke stream failed for %s model %q: %w", cfg.BaseURL, cfg.Model, err)
		}
		result.addCheck("stream", nil)
	}

	if cfg.CheckEmbeddings {
		if err := runOpenAICompatibleEmbeddingsCheck(ctx, client, cfg); err != nil {
			result.addCheck("embeddings", err)
			return result, fmt.Errorf("openai-compatible smoke embeddings failed for %s model %q: %w", cfg.BaseURL, cfg.Model, err)
		}
		result.addCheck("embeddings", nil)
	}

	return result, nil
}

func (r *OpenAICompatibleSmokeResult) addCheck(name string, err error) {
	check := OpenAICompatibleSmokeCheck{Name: name, Passed: err == nil}
	if err != nil {
		check.Error = err.Error()
	}
	r.Checks = append(r.Checks, check)
}

func runOpenAICompatibleStreamCheck(ctx context.Context, client *Wormhole, cfg OpenAICompatibleSmokeConfig) error {
	stream, err := client.Text().
		Model(cfg.Model).
		Prompt(cfg.Prompt).
		ProviderOptions(cfg.ProviderOptions).
		Stream(ctx)
	if err != nil {
		return err
	}
	seenText := false
	for chunk := range stream {
		if chunk.Error != nil {
			return chunk.Error
		}
		if chunk.Content() != "" {
			seenText = true
		}
	}
	if !seenText {
		return fmt.Errorf("empty parsed stream")
	}
	return nil
}

func runOpenAICompatibleEmbeddingsCheck(ctx context.Context, client *Wormhole, cfg OpenAICompatibleSmokeConfig) error {
	model := cfg.EmbeddingsModel
	if model == "" {
		model = cfg.Model
	}
	resp, err := client.Embeddings().
		Model(model).
		Input("hello").
		ProviderOptions(cfg.ProviderOptions).
		Generate(ctx)
	if err != nil {
		return err
	}
	if resp == nil || len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Embedding) == 0 {
		return fmt.Errorf("empty parsed embeddings response")
	}
	return nil
}
