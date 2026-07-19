package gemini

import (
	"net/url"
	"strings"

	"github.com/garyblankenship/wormhole/v2/providers"
	"github.com/garyblankenship/wormhole/v2/types"
)

// buildTextPayload builds the request payload for text generation
func (g *Gemini) buildTextPayload(request types.TextRequest) (map[string]any, error) {
	if request.ParallelToolCalls != nil {
		return nil, g.ValidationError("parallel_tool_calls is not supported by Gemini")
	}
	prepared, _, prepareErr := providers.PrepareMessages(request.Messages)
	if prepareErr != nil {
		return nil, prepareErr
	}
	contents, err := g.transformMessages(prepared, request.Model)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"contents": contents,
	}

	if systemText := mergeSystemInstruction(request.SystemPrompt, request.Messages); systemText != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{
				{"text": systemText},
			},
		}
	}

	// Add generation config using shared utility
	generationConfig := map[string]any{}
	// Use shared utility for common parameters, then map to Gemini field names
	stdConfig := map[string]any{}
	g.requestBuilder.AddGenerationParams(stdConfig, request.Temperature, request.TopP, request.MaxTokens, request.Stop)

	// Map standard field names to Gemini-specific names
	if maxTokens, ok := stdConfig["max_tokens"]; ok {
		generationConfig["maxOutputTokens"] = maxTokens
	}
	if temp, ok := stdConfig["temperature"]; ok {
		generationConfig["temperature"] = temp
	}
	if topP, ok := stdConfig["top_p"]; ok {
		generationConfig["topP"] = topP
	}
	if stop, ok := stdConfig["stop"]; ok {
		generationConfig["stopSequences"] = stop
	}
	if request.FrequencyPenalty != nil {
		generationConfig["frequencyPenalty"] = *request.FrequencyPenalty
	}
	if request.PresencePenalty != nil {
		generationConfig["presencePenalty"] = *request.PresencePenalty
	}
	if request.Seed != nil {
		generationConfig["seed"] = *request.Seed
	}
	if thinking := geminiThinkingConfig(request.Reasoning); len(thinking) > 0 {
		generationConfig["thinkingConfig"] = thinking
	}

	if len(generationConfig) > 0 {
		payload["generationConfig"] = generationConfig
	}

	// Add tools if provided
	if len(request.Tools) > 0 {
		tools := g.transformTools(request.Tools)
		payload["tools"] = tools

		// Add tool config if specified
		if request.ToolChoice != nil {
			payload["toolConfig"] = g.transformToolChoice(request.ToolChoice)
		}
	}

	for k, v := range g.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		if k == "generationConfig" {
			if opts, ok := v.(map[string]any); ok {
				if existing, ok := payload["generationConfig"].(map[string]any); ok {
					for optKey, optValue := range opts {
						existing[optKey] = optValue
					}
				} else {
					payload["generationConfig"] = opts
				}
				continue
			}
		}
		payload[k] = v
	}

	return payload, nil
}

func geminiThinkingConfig(reasoning *types.Reasoning) map[string]any {
	if reasoning == nil {
		return nil
	}
	out := make(map[string]any, 2)
	if reasoning.MaxTokens > 0 {
		out["thinkingBudget"] = reasoning.MaxTokens
	}
	if reasoning.Enabled != nil {
		out["includeThoughts"] = *reasoning.Enabled
	}
	return out
}

// buildStructuredPayload builds the request payload for structured generation
func (g *Gemini) buildStructuredPayload(request types.StructuredRequest) (map[string]any, error) {
	// For Gemini, we use response schema in generation config
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	payload, err := g.buildTextPayload(textRequest)
	if err != nil {
		return nil, err
	}

	// Add response schema to generation config
	if generationConfig, ok := payload["generationConfig"].(map[string]any); ok {
		generationConfig["responseMimeType"] = "application/json"
		generationConfig["responseSchema"] = g.transformSchema(request.Schema)
	} else {
		payload["generationConfig"] = map[string]any{
			"responseMimeType": "application/json",
			"responseSchema":   g.transformSchema(request.Schema),
		}
	}

	return payload, nil
}

func normalizeModelResource(model string) string {
	model = strings.TrimPrefix(model, "google/")
	model = strings.TrimPrefix(model, "models/")
	// The result is interpolated directly into a URL path segment
	// (see Text/Structured/Images/StreamText endpoint construction), so
	// metacharacters (/, ?, #, ..) must be percent-escaped here — the
	// single call site that all 4 endpoint builders route through.
	return url.PathEscape(model)
}
