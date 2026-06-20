package types

import (
	"encoding/json"
)

// ToolChoiceType represents the type of tool choice
type ToolChoiceType string

const (
	ToolChoiceTypeAuto     ToolChoiceType = "auto"
	ToolChoiceTypeNone     ToolChoiceType = "none"
	ToolChoiceTypeAny      ToolChoiceType = "any"
	ToolChoiceTypeSpecific ToolChoiceType = "specific"
)

// ToolChoice represents how the model should use tools
type ToolChoice struct {
	Type     ToolChoiceType `json:"type"`
	ToolName string         `json:"tool_name,omitempty"`
}

func (tc *ToolChoice) MarshalJSON() ([]byte, error) {
	// If it's a simple type without a specific tool name, serialize as a string
	if tc.ToolName == "" && (tc.Type == ToolChoiceTypeAuto || tc.Type == ToolChoiceTypeNone || tc.Type == ToolChoiceTypeAny) {
		return json.Marshal(string(tc.Type))
	}
	// Otherwise serialize as an object
	return json.Marshal(struct {
		Type     ToolChoiceType `json:"type"`
		ToolName string         `json:"tool_name,omitempty"`
	}{
		Type:     tc.Type,
		ToolName: tc.ToolName,
	})
}

// Tool represents a function that can be called by the model
type Tool struct {
	Type        string         `json:"type,omitempty"` // For OpenAI compatibility ("function")
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Function    *ToolFunction  `json:"function,omitempty"` // For OpenAI compatibility
}

// ToolFunction represents the function definition for OpenAI tools
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents a tool call made by the model
type ToolCall struct {
	Type      string            `json:"type,omitempty"` // For OpenAI compatibility ("function")
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Arguments map[string]any    `json:"arguments"`
	Function  *ToolCallFunction `json:"function,omitempty"` // For OpenAI compatibility
	// ThoughtSignature carries the opaque base64 token Gemini thinking models attach to functionCall parts; must be echoed verbatim on the next turn. Empty for all other providers.
	ThoughtSignature string `json:"thought_signature,omitempty"`
	// ArgsInvalid is true when the provider's tool-call arguments failed to parse as JSON
	// (e.g. a truncated streaming tool_call). When set, Arguments is nil/empty and the raw
	// fragment is retained in Function.Arguments. Empty/false for well-formed calls.
	ArgsInvalid bool `json:"args_invalid,omitempty"`
	// ArgsParseError holds the JSON parse error message when ArgsInvalid is true; empty otherwise.
	ArgsParseError string `json:"args_parse_error,omitempty"`
}

// ToolCallFunction represents the function call details for OpenAI
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Result     any    `json:"result"`
	Error      string `json:"error,omitempty"`
}

// NewTool creates a new tool definition
func NewTool(name, description string, inputSchema map[string]any) *Tool {
	return &Tool{
		Type:        "function", // OpenAI compatibility
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Function: &ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  inputSchema,
		},
	}
}
