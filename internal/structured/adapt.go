// Package structured shares schema and application-tool adaptation between providers.
package structured

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

const ResultToolName = "__wormhole_structured_output"

// Prepare adapts a structured request without executing application tools.
func Prepare(r types.StructuredRequest) (types.TextRequest, error) {
	t := types.TextRequest{BaseRequest: r.BaseRequest, Messages: types.CloneMessages(r.Messages), SystemPrompt: r.SystemPrompt, Tools: types.CloneTools(r.Tools)}
	if r.ToolChoice != nil {
		c := *r.ToolChoice
		t.ToolChoice = &c
	}
	if _, err := schemaMap(r.Schema); err != nil {
		return t, err
	}
	switch r.Mode {
	case types.StructuredModeJSON:
		t.ResponseFormat = map[string]any{"type": "json_object"}
	case types.StructuredModeStrict:
		schema, err := schemaMap(r.Schema)
		if err != nil {
			return t, err
		}
		name := r.SchemaName
		if name == "" {
			name = "structured_output"
		}
		t.ResponseFormat = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": name, "strict": true, "schema": schema}}
	default:
		return PrepareTools(t, r.Schema)
	}
	return t, nil
}

// PrepareText selects the explicit schema-bearing text/proxy tool fallback.
func PrepareText(t types.TextRequest) (types.TextRequest, types.Schema, error) {
	if t.ProviderOptions["structured_mode"] != "tools" {
		return t, nil, nil
	}
	b, err := json.Marshal(t.ResponseFormat)
	if err != nil {
		return t, nil, err
	}
	var format struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Schema json.RawMessage `json:"schema"`
			Strict bool            `json:"strict"`
		} `json:"json_schema"`
	}
	if err = json.Unmarshal(b, &format); err != nil {
		return t, nil, err
	}
	if format.Type != "json_schema" || len(format.JSONSchema.Schema) == 0 {
		return t, nil, fmt.Errorf("structured tools mode requires a JSON schema")
	}
	if format.JSONSchema.Strict {
		return t, nil, fmt.Errorf("strict native output cannot use structured tools mode")
	}
	schema := types.Schema(format.JSONSchema.Schema)
	adapted, err := PrepareTools(t, schema)
	return adapted, schema, err
}

func PrepareTools(t types.TextRequest, schema types.Schema) (types.TextRequest, error) {
	for _, tool := range t.Tools {
		name := tool.Name
		if name == "" && tool.Function != nil {
			name = tool.Function.Name
		}
		if name == ResultToolName {
			return t, fmt.Errorf("reserved structured result tool name: %s", name)
		}
	}
	definition, err := schemaMap(schema)
	if err != nil {
		return t, err
	}
	t.Tools = types.CloneTools(t.Tools)
	result := *types.NewTool(ResultToolName, "Return the final structured result", definition)
	if t.ToolChoice != nil && t.ToolChoice.Type == types.ToolChoiceTypeNone {
		t.Tools = nil
	}
	t.Tools = append(t.Tools, result)
	if len(t.Tools) == 1 || (t.ToolChoice != nil && t.ToolChoice.Type == types.ToolChoiceTypeNone) {
		t.ToolChoice = &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: ResultToolName}
	}
	// Required applies to application tools, never to the synthetic final result.
	if t.ToolChoice != nil && t.ToolChoice.Type == types.ToolChoiceTypeAny && len(t.Tools) > 1 {
		t.Tools = t.Tools[:len(t.Tools)-1]
	}
	t.ResponseFormat = nil
	t.ProviderOptions = types.CloneMap(t.ProviderOptions)
	delete(t.ProviderOptions, "structured_mode")
	return t, nil
}

func schemaMap(schema types.Schema) (map[string]any, error) {
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	if len(b) > 10<<20 {
		return nil, fmt.Errorf("structured schema exceeds size limit")
	}
	var m map[string]any
	err = json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("structured schema must be an object")
	}
	if err := checkDefinition(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ResponseError retains an invalid or mixed provider response for inspection.
type ResponseError struct {
	Response *types.TextResponse
	Err      error
}

func (e *ResponseError) Error() string { return "invalid structured response: " + e.Err.Error() }
func (e *ResponseError) Unwrap() error { return e.Err }

// Convert preserves tool turns and validates final values.
func Convert(r types.StructuredRequest, response *types.TextResponse) (*types.StructuredResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("nil structured response")
	}
	original := response
	var err error
	if r.Mode != types.StructuredModeJSON && r.Mode != types.StructuredModeStrict {
		response, err = FinishTools(r.Schema, response)
		if err != nil {
			return nil, err
		}
	}
	out := &types.StructuredResponse{ID: response.ID, Provider: response.Provider, Model: response.Model, Usage: response.Usage, Created: response.Created, Metadata: response.Metadata, ResponseMetadata: response.ResponseMetadata, Warnings: response.Warnings, FinishReason: response.FinishReason, Sources: response.Sources, Citations: response.Citations}
	if len(response.ToolCalls) > 0 {
		out.ToolCalls = types.CloneToolCalls(response.ToolCalls)
		return out, nil
	}
	var data any
	if err = json.Unmarshal([]byte(response.Text), &data); err != nil {
		return nil, &ResponseError{original, err}
	}
	if err = Validate(r.Schema, data); err != nil {
		return nil, &ResponseError{original, err}
	}
	out.Data = data
	out.Raw = response.Text
	return out, nil
}

// FinishTools turns only the reserved result call into validated final JSON.
func FinishTools(schema types.Schema, response *types.TextResponse) (*types.TextResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("nil structured response")
	}
	var result *types.ToolCall
	applications := 0
	for _, raw := range response.ToolCalls {
		call, err := types.NormalizeToolCall(raw)
		if err != nil {
			return nil, &ResponseError{response, err}
		}
		if call.Name == ResultToolName {
			if result != nil {
				return nil, &ResponseError{response, fmt.Errorf("multiple structured result calls")}
			}
			result = &call
		} else {
			applications++
		}
	}
	if result == nil {
		if applications > 0 {
			return response, nil
		}
		return nil, &ResponseError{response, fmt.Errorf("missing structured result tool call")}
	}
	if applications > 0 {
		return nil, &ResponseError{response, fmt.Errorf("application and final result calls were mixed")}
	}
	if err := Validate(schema, result.Arguments); err != nil {
		return nil, &ResponseError{response, err}
	}
	b, err := json.Marshal(result.Arguments)
	if err != nil {
		return nil, &ResponseError{response, err}
	}
	out := *response
	out.Text = string(b)
	out.ToolCalls = nil
	out.FinishReason = types.FinishReasonStop
	return &out, nil
}
