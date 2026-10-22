package anthropic

import (
	"encoding/json"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *Provider) validateReferences(messages []types.Message) error {
	for _, message := range messages {
		user, ok := message.(*types.UserMessage)
		if !ok {
			continue
		}
		for _, media := range user.Media {
			ref, ok := media.(*types.FileReferenceMedia)
			if !ok {
				continue
			}
			if err := ref.Validate(); err != nil {
				return p.ValidationError(err.Error())
			}
			if ref.Provider != p.Name() {
				return p.ValidationError("file reference belongs to another provider")
			}
			if ref.ID == "" || (ref.Kind != "image" && ref.Kind != "document") {
				return p.ValidationError("Anthropic requires image or document file IDs")
			}
		}
	}
	return nil
}

func (p *Provider) addResponseFormat(payload map[string]any, format any) error {
	if format == nil {
		return nil
	}
	data, err := json.Marshal(format)
	if err != nil {
		return p.ValidationError("invalid response format")
	}
	var value struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Schema any `json:"schema"`
		} `json:"json_schema"`
	}
	if err = json.Unmarshal(data, &value); err != nil {
		return p.ValidationError("invalid response format")
	}
	if value.Type != "json_schema" || value.JSONSchema.Schema == nil {
		return p.ValidationError("Anthropic structured output requires a JSON schema")
	}
	output, _ := payload["output_config"].(map[string]any)
	if output == nil {
		output = make(map[string]any)
	}
	output["format"] = map[string]any{"type": "json_schema", "schema": value.JSONSchema.Schema}
	payload["output_config"] = output
	return nil
}

func (p *Provider) validateCitationFormat(payload map[string]any) error {
	output, _ := payload["output_config"].(map[string]any)
	if output["format"] == nil {
		return nil
	}
	if hasEnabledCitations(payload["messages"]) {
		return p.ValidationError("Anthropic citations conflict with native structured output")
	}
	return nil
}

func hasEnabledCitations(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if citation, ok := v["citations"].(map[string]any); ok {
			if enabled, _ := citation["enabled"].(bool); enabled {
				return true
			}
		}
		for _, child := range v {
			if hasEnabledCitations(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range v {
			if hasEnabledCitations(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasEnabledCitations(child) {
				return true
			}
		}
	}
	return false
}

// Summary visibility cannot activate thinking by itself.
func visibilityWarnings(reasoning *types.Reasoning, payload map[string]any) []types.ResponseWarning {
	if reasoning == nil || reasoning.IncludeThoughts == nil {
		return nil
	}
	if thinking, ok := payload["thinking"].(map[string]any); ok && thinking["type"] != nil {
		return nil
	}
	return []types.ResponseWarning{{Code: types.WarningUnsupportedNormalization, Message: "Anthropic thought visibility requires an explicit thinking mode", Field: "reasoning.include_thoughts"}}
}
