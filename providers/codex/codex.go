package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

const defaultBaseURL = "https://chatgpt.com/backend-api"

// Provider implements account-bound Codex text generation via Responses SSE.
// Other operations use BaseProvider's explicit unsupported errors.
type Provider struct {
	*providers.BaseProvider
	credential Credential
}

var _ types.Provider = (*Provider)(nil)

// New validates the explicit credential and refuses generic API-key auth.
func New(config types.ProviderConfig, credential Credential, client providers.HTTPClient) (*Provider, error) {
	if config.APIKey != "" || len(config.APIKeys) != 0 || config.NoAuth {
		return nil, errors.New("codex requires account-bound credentials, not generic API-key or no-auth configuration")
	}
	for key := range config.Headers {
		if strings.EqualFold(key, "authorization") || strings.EqualFold(key, "chatgpt-account-id") {
			return nil, errors.New("codex auth headers cannot be overridden")
		}
	}
	if config.ChatPath != "" || config.ResponsesPath != "" || config.UseResponsesAPI {
		return nil, errors.New("codex endpoint and transport cannot be overridden by generic response settings")
	}
	if err := validateOptions(config.DefaultProviderOptions); err != nil {
		return nil, err
	}
	for _, opts := range config.ProviderOptionsByModel {
		if err := validateOptions(opts); err != nil {
			return nil, err
		}
	}
	validated, err := ValidateCredential(credential, time.Now())
	if err != nil {
		return nil, err
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	// POST generation is not retried automatically. Callers may explicitly
	// configure MaxRetries, but must own replay/idempotency risk.
	if config.MaxRetries == nil {
		config = config.WithNoRetries()
	}
	config.Headers = cloneHeaders(config.Headers)
	config.Headers["chatgpt-account-id"] = validated.AccountID
	config.APIKey = validated.AccessToken
	return &Provider{BaseProvider: providers.NewBaseProviderWithAuth("codex", config, nil, nil, client), credential: validated}, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		out[k] = v
	}
	return out
}

func validateOptions(options map[string]any) error {
	for key := range options {
		switch strings.ToLower(key) {
		case "text":
			if key != "text" {
				return fmt.Errorf("codex provider option %q must use canonical spelling", key)
			}
			text, ok := options[key].(map[string]any)
			if !ok || len(text) != 1 {
				return errors.New("codex text option requires only verbosity")
			}
			verbosity, ok := text["verbosity"].(string)
			if !ok || (verbosity != "low" && verbosity != "medium" && verbosity != "high") {
				return errors.New("codex text verbosity must be low, medium, or high")
			}
		case "model", "input", "instructions", "stream", "store", "include", "max_output_tokens", "reasoning", "tools", "tool_choice", "service_tier", "temperature", "top_p":
			return fmt.Errorf("codex provider option %q conflicts with typed request or required wire contract", key)
		}
	}
	return nil
}

func (p *Provider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityStream}
}

func (p *Provider) payload(request types.TextRequest) (map[string]any, error) {
	if err := validateRequest(request); err != nil {
		return nil, err
	}
	input, instructions, err := requestInput(request)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"model": request.Model, "input": input, "store": false, "stream": true, "include": []string{"reasoning.encrypted_content"}}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if request.MaxTokens != nil {
		if *request.MaxTokens <= 0 {
			return nil, errors.New("codex max tokens must be positive")
		}
		payload["max_output_tokens"] = *request.MaxTokens
	}
	if request.Reasoning != nil {
		if request.Reasoning.Enabled != nil && !*request.Reasoning.Enabled {
			payload["reasoning"] = map[string]any{"effort": "none"}
		} else if request.Reasoning.Effort != "" {
			payload["reasoning"] = map[string]any{"effort": request.Reasoning.Effort}
		}
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	for key, value := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[key] = value
	}
	return payload, nil
}

func validateRequest(request types.TextRequest) error {
	if strings.TrimSpace(request.Model) == "" {
		return errors.New("codex model is required")
	}
	if len(request.Tools) != 0 || request.ToolChoice != nil || request.ResponseFormat != nil || request.ParallelToolCalls != nil || len(request.Stop) != 0 || request.Seed != nil || request.PresencePenalty != nil || request.FrequencyPenalty != nil {
		return errors.New("codex request contains unsupported tools, format, or sampling options")
	}
	if err := validateOptions(request.ProviderOptions); err != nil {
		return err
	}
	if request.Reasoning != nil && (request.Reasoning.MaxTokens != 0 || request.Reasoning.IncludeThoughts != nil) {
		return errors.New("codex does not support reasoning token budgets or exposed thoughts")
	}
	return nil
}

func requestInput(request types.TextRequest) ([]map[string]any, string, error) {
	input := make([]map[string]any, 0, len(request.Messages))
	instructions := request.SystemPrompt
	seenSystem := false
	for _, msg := range request.Messages {
		if msg == nil {
			return nil, "", errors.New("codex nil message")
		}
		role := msg.GetRole()
		if role == types.RoleSystem {
			if seenSystem || instructions != "" || len(input) != 0 {
				return nil, "", errors.New("codex conflicting or misplaced system instructions")
			}
			content, ok := msg.GetContent().(string)
			if !ok {
				return nil, "", errors.New("codex system instructions must be plain text")
			}
			instructions = content
			seenSystem = true
			continue
		}
		if role != types.RoleUser && role != types.RoleAssistant {
			return nil, "", errors.New("codex only supports user and assistant text messages; use SystemPrompt for instructions")
		}
		content, ok := msg.GetContent().(string)
		if !ok {
			return nil, "", errors.New("codex only supports plain text messages")
		}
		if user, ok := msg.(*types.UserMessage); ok && len(user.Media) != 0 {
			return nil, "", errors.New("codex media is unsupported")
		}
		input = append(input, map[string]any{"type": "message", "role": role, "content": []map[string]any{{"type": "input_text", "text": content}}})
	}
	if len(input) == 0 {
		return nil, "", errors.New("codex requires at least one message")
	}
	return input, instructions, nil
}

func (p *Provider) open(ctx context.Context, request types.TextRequest) (body io.ReadCloser, err error) {
	if _, err = ValidateCredential(p.credential, time.Now()); err != nil {
		return nil, err
	}
	payload, err := p.payload(request)
	if err != nil {
		return nil, err
	}
	return p.StreamRequest(ctx, http.MethodPost, strings.TrimRight(p.GetBaseURL(), "/")+"/codex/responses", payload)
}

func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	body, err := p.open(ctx, request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	final, err := consume(ctx, body, nil)
	if err != nil {
		return nil, err
	}
	return &types.TextResponse{ID: final.ID, Provider: p.Name(), Model: final.Model, Text: final.text(), FinishReason: types.FinishReasonStop, Usage: final.usage()}, nil
}

func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	body, err := p.open(ctx, request)
	if err != nil {
		return nil, err
	}
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		defer func() { _ = body.Close() }()
		send := func(chunk types.TextChunk) bool {
			select {
			case out <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}
		final, err := consume(ctx, body, func(delta string) bool {
			return send(types.TextChunk{Provider: p.Name(), Model: request.Model, Text: delta})
		})
		if err != nil {
			// A caller who abandons the stream must cancel ctx; otherwise errors
			// remain observable as a final chunk.
			send(types.TextChunk{Provider: p.Name(), Error: err})
			return
		}
		stop := types.FinishReasonStop
		send(types.TextChunk{Provider: p.Name(), ID: final.ID, Model: final.Model, FinishReason: &stop, Usage: final.usage()})
	}()
	return out, nil
}
