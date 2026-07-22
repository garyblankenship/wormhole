package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// CacheControlType identifies a provider cache-control mode.
type CacheControlType string

const (
	// CacheControlTypeEphemeral enables Anthropic's ephemeral prompt cache.
	CacheControlTypeEphemeral CacheControlType = "ephemeral"
)

// CacheTTL identifies the lifetime of an Anthropic cache entry.
type CacheTTL string

const (
	// CacheTTLDefault omits the TTL and uses Anthropic's default lifetime.
	CacheTTLDefault CacheTTL = ""
	// CacheTTL1Hour requests Anthropic's one-hour cache lifetime.
	CacheTTL1Hour CacheTTL = "1h"
)

// CacheControl marks a native Anthropic tool-definition cache boundary.
type CacheControl struct {
	Type CacheControlType `json:"type"`
	TTL  CacheTTL         `json:"ttl,omitempty"`
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
	Type         string         `json:"type,omitempty"` // For OpenAI compatibility ("function")
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	Function     *ToolFunction  `json:"function,omitempty"` // For OpenAI compatibility
	CacheControl *CacheControl  `json:"cache_control,omitempty"`
}

// ToolFunction represents the function definition for OpenAI tools
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents a tool call made by the model
type ToolCall struct {
	Index     int               `json:"-"`              // OpenAI stream index (internal; not serialized)
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

// NormalizeToolCall reconciles the provider-neutral and OpenAI-compatible
// representations of a tool call. The provider-neutral Name and Arguments
// fields are authoritative when present; the nested Function fills missing
// values. Both representations are populated on success.
func NormalizeToolCall(src ToolCall) (ToolCall, error) {
	dst := CloneToolCall(src)
	if dst.ArgsInvalid {
		return ToolCall{}, fmt.Errorf("tool call %q has malformed arguments: %s", dst.ID, dst.ArgsParseError)
	}

	name := dst.Name
	if name == "" && dst.Function != nil {
		name = dst.Function.Name
	}

	topArguments := dst.Arguments
	var nestedArguments map[string]any
	if dst.Function != nil && dst.Function.Arguments != "" {
		decoder := json.NewDecoder(bytes.NewBufferString(dst.Function.Arguments))
		if err := decoder.Decode(&nestedArguments); err != nil {
			return ToolCall{}, fmt.Errorf("tool call %q has malformed function arguments: %w", dst.ID, err)
		}
		if nestedArguments == nil {
			return ToolCall{}, fmt.Errorf("tool call %q function arguments must be a JSON object", dst.ID)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return ToolCall{}, fmt.Errorf("tool call %q has trailing function arguments data", dst.ID)
		}
	}

	switch {
	case topArguments != nil && nestedArguments != nil:
		topJSON, err := json.Marshal(topArguments)
		if err != nil {
			return ToolCall{}, fmt.Errorf("tool call %q has malformed arguments: %w", dst.ID, err)
		}
		nestedJSON, err := json.Marshal(nestedArguments)
		if err != nil {
			return ToolCall{}, fmt.Errorf("tool call %q has malformed function arguments: %w", dst.ID, err)
		}
		if !bytes.Equal(topJSON, nestedJSON) {
			return ToolCall{}, fmt.Errorf("tool call %q has conflicting argument representations", dst.ID)
		}
	case topArguments == nil && nestedArguments != nil:
		topArguments = nestedArguments
	case topArguments == nil:
		topArguments = map[string]any{}
	}

	argumentsJSON, err := json.Marshal(topArguments)
	if err != nil {
		return ToolCall{}, fmt.Errorf("tool call %q has malformed arguments: %w", dst.ID, err)
	}
	dst.Name = name
	dst.Arguments = CloneMap(topArguments)
	dst.Function = &ToolCallFunction{Name: name, Arguments: string(argumentsJSON)}
	return dst, nil
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name,omitempty"`
	Result     any    `json:"result"`
	Error      string `json:"error,omitempty"`
}

// NewTool creates a new tool definition
func NewTool(name, description string, inputSchema map[string]any) *Tool {
	return &Tool{
		Type:        "function", // OpenAI compatibility
		Name:        name,
		Description: description,
		InputSchema: CloneMap(inputSchema),
		Function: &ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  CloneMap(inputSchema),
		},
	}
}
