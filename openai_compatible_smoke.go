package wormhole

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
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

	// ValidateResponse optionally validates the parsed chat response after
	// the transport-level chat check passes. It receives the same parsed
	// *types.TextResponse the smoke inspects and reports semantic failures
	// (JSON shape, domain rules) as an error. The smoke records it as a
	// separate "response_validation" check, wraps its error so
	// errors.Is matches the callback's error, and skips the optional
	// streaming and embeddings probes when it fails. No default validator
	// is installed; nil keeps the historical behavior exactly.
	ValidateResponse func(*types.TextResponse) error
}

// OpenAICompatibleSmokeResult contains the parsed response and endpoint facts
// from RunOpenAICompatibleSmoke.
type OpenAICompatibleSmokeResult struct {
	BaseURL string
	Model   string
	Text    string
	Checks  []OpenAICompatibleSmokeCheck

	// RequestedModel is the model the smoke asked for. Model above stays
	// the requested-model compatibility field for existing callers.
	RequestedModel string
	// ReportedModel is the model identity parsed from the chat response
	// body. It is diagnostic only: an upstream alias or mismatch never
	// fails a check. When the upstream body omits the model field, the
	// OpenAI-compatible provider backfills the requested model, so a
	// missing upstream identity is reported as the requested model and
	// cannot be distinguished from an exact echo by this smoke.
	ReportedModel string
	// FinishReason is the parsed stop reason from the chat response.
	FinishReason types.FinishReason
	// Usage carries SDK-parsed token accounting. The adapter supplies a
	// zero-valued Usage when upstream usage is absent, so a nonnil value
	// is not proof that the upstream reported accounting.
	Usage *types.Usage
	// Thinking carries the parsed reasoning block, or nil when absent.
	Thinking *types.Thinking
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
		BaseURL:        cfg.BaseURL,
		Model:          cfg.Model,
		RequestedModel: cfg.Model,
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
	result.ReportedModel = resp.Model
	result.FinishReason = resp.FinishReason
	result.Usage = resp.Usage
	result.Thinking = resp.Thinking
	result.addCheck("chat", nil)

	// Caller-defined semantic validation stays distinct from the transport
	// chat check above: protocol parsing and nonempty text have already
	// passed, and the diagnostics above survive a validation failure.
	if cfg.ValidateResponse != nil {
		if err := cfg.ValidateResponse(resp); err != nil {
			result.addCheck("response_validation", err)
			return result, fmt.Errorf("openai-compatible smoke response validation failed for %s model %q: %w", cfg.BaseURL, cfg.Model, err)
		}
		result.addCheck("response_validation", nil)
	}

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
