package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// toWormholeTools maps OpenAI-format request tools to wormhole tool definitions.
func toWormholeTools(in []ChatTool) ([]types.Tool, error) {
	out := make([]types.Tool, 0, len(in))
	for _, t := range in {
		if t.Type != "function" {
			return nil, fmt.Errorf("unsupported tool type %q", t.Type)
		}
		if strings.TrimSpace(t.Function.Name) == "" {
			return nil, fmt.Errorf("function tool name is required")
		}
		out = append(out, types.Tool{
			Type:        "function",
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return out, nil
}

// parseToolChoice maps an OpenAI tool_choice (string or object form) to a
// wormhole ToolChoice. Returns nil when absent or unrecognized.
func parseToolChoice(raw json.RawMessage) (*types.ToolChoice, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return &types.ToolChoice{Type: types.ToolChoiceTypeAuto}, nil
		case "none":
			return &types.ToolChoice{Type: types.ToolChoiceTypeNone}, nil
		case "required":
			return &types.ToolChoice{Type: types.ToolChoiceTypeAny}, nil
		default:
			return nil, fmt.Errorf("unsupported tool_choice %q", s)
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}
	if obj.Type != "function" {
		return nil, fmt.Errorf("unsupported tool_choice type %q", obj.Type)
	}
	if strings.TrimSpace(obj.Function.Name) == "" {
		return nil, fmt.Errorf("function tool_choice name is required")
	}
	return &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: obj.Function.Name}, nil
}

// toWormholeToolCalls maps inbound OpenAI assistant tool_calls to wormhole tool calls.
func toWormholeToolCalls(in []ChatToolCall) ([]types.ToolCall, error) {
	out := make([]types.ToolCall, 0, len(in))
	for _, c := range in {
		if c.Type != "function" {
			return nil, fmt.Errorf("unsupported assistant tool call type %q", c.Type)
		}
		if strings.TrimSpace(c.Function.Name) == "" {
			return nil, fmt.Errorf("assistant tool call name is required")
		}
		rawArguments := c.Function.Arguments
		if strings.TrimSpace(rawArguments) == "" {
			rawArguments = "{}"
		}
		var args map[string]any
		if err := json.Unmarshal([]byte(rawArguments), &args); err != nil {
			return nil, fmt.Errorf("assistant tool call %q arguments must be a JSON object: %w", c.Function.Name, err)
		}
		if args == nil {
			return nil, fmt.Errorf("assistant tool call %q arguments must be a JSON object", c.Function.Name)
		}
		out = append(out, types.ToolCall{
			Type:      "function",
			ID:        c.ID,
			Name:      c.Function.Name,
			Arguments: args,
			Function: &types.ToolCallFunction{
				Name:      c.Function.Name,
				Arguments: rawArguments,
			},
		})
	}
	return out, nil
}

// fromWormholeToolCalls maps wormhole tool calls to OpenAI-format tool calls for
// a response. Arguments are emitted as a JSON string per the OpenAI contract.
func fromWormholeToolCalls(in []types.ToolCall) []ChatToolCall {
	out := make([]ChatToolCall, 0, len(in))
	for _, c := range in {
		args := "{}"
		name := c.Name
		switch {
		case c.Function != nil && c.Function.Arguments != "":
			args = c.Function.Arguments
		case len(c.Arguments) > 0:
			if b, err := json.Marshal(c.Arguments); err == nil {
				args = string(b)
			}
		}
		if name == "" && c.Function != nil {
			name = c.Function.Name
		}
		out = append(out, ChatToolCall{
			ID:   c.ID,
			Type: "function",
			Function: ChatToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})
	}
	return out
}

// chunkToolFragments extracts tool-call fragments from a stream chunk, preferring
// the OpenAI-compat Delta carrier, then the chunk-level slices, then a single
// ToolCall. Providers differ in which field they populate.
func chunkToolFragments(chunk types.TextChunk) []types.ToolCall {
	if chunk.Delta != nil && len(chunk.Delta.ToolCalls) > 0 {
		return chunk.Delta.ToolCalls
	}
	if len(chunk.ToolCalls) > 0 {
		return chunk.ToolCalls
	}
	if chunk.ToolCall != nil {
		return []types.ToolCall{*chunk.ToolCall}
	}
	return nil
}

// streamToolState maps streamed tool-call fragments to OpenAI tool_call deltas
// with stable indices. Providers emit an opener fragment (non-empty ID + name)
// followed by argument continuations; some repeat the ID on continuations
// (OpenAI), others send an empty ID (Anthropic). id+name+type are emitted only
// on a slot's first delta; later deltas carry only the index and argument substring.
type streamToolState struct {
	index  map[string]int // tool-call ID -> OpenAI tool_call index
	opened map[int]bool   // indices that have already emitted id+name
	last   int            // last index touched, for empty-ID continuations
	next   int            // next index to assign
}

func newStreamToolState() *streamToolState {
	return &streamToolState{index: map[string]int{}, opened: map[int]bool{}, last: -1, next: 0}
}

func (s *streamToolState) indexFor(id string) int {
	if id == "" {
		if s.last < 0 {
			s.last = s.next
			s.next++
		}
		return s.last
	}
	if i, ok := s.index[id]; ok {
		s.last = i
		return i
	}
	i := s.next
	s.next++
	s.index[id] = i
	s.last = i
	return i
}

// delta maps a chunk's tool-call fragments to OpenAI streaming tool_call deltas.
func (s *streamToolState) delta(chunk types.TextChunk) []ChatToolCall {
	frags := chunkToolFragments(chunk)
	if len(frags) == 0 {
		return nil
	}
	out := make([]ChatToolCall, 0, len(frags))
	for _, f := range frags {
		idx := s.indexFor(f.ID)
		i := idx
		var args string
		if f.Function != nil {
			args = f.Function.Arguments
		}
		tc := ChatToolCall{Index: &i, Function: ChatToolCallFunction{Arguments: args}}
		if !s.opened[idx] {
			s.opened[idx] = true
			tc.ID = f.ID
			tc.Type = "function"
			tc.Function.Name = f.Name
		}
		out = append(out, tc)
	}
	return out
}
