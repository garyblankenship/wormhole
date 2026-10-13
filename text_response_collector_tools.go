package wormhole

import "github.com/garyblankenship/wormhole/v3/types"

func (c *textResponseCollector) addTools(chunk types.TextChunk) {
	calls := chunk.ToolCalls
	if len(calls) == 0 && chunk.ToolCall != nil {
		calls = []types.ToolCall{*chunk.ToolCall}
	}
	if len(chunk.ToolCalls) > 0 && chunk.IsDone() {
		c.response.ToolCalls = types.CloneToolCalls(chunk.ToolCalls)
		c.pending = nil
		c.toolSnapshot = true
		return
	}
	for _, call := range calls {
		c.mergeCompleted(call)
	}
	// Top-level completed calls take precedence over their raw compatibility alias.
	if len(calls) == 0 && chunk.Delta != nil && !c.toolSnapshot {
		for _, call := range chunk.Delta.ToolCalls {
			c.mergeFragment(call)
		}
	}
}

func (c *textResponseCollector) mergeCompleted(call types.ToolCall) {
	call = types.CloneToolCall(call)
	if call.ID != "" {
		for i, previous := range c.response.ToolCalls {
			if previous.ID == call.ID {
				c.response.ToolCalls[i] = call
				return
			}
		}
	}
	c.response.ToolCalls = append(c.response.ToolCalls, call)
}

func (c *textResponseCollector) mergeFragment(fragment types.ToolCall) {
	for i := range c.pending {
		if c.pending[i].Index == fragment.Index {
			appendToolFragment(&c.pending[i], fragment)
			return
		}
	}
	c.pending = append(c.pending, types.CloneToolCall(fragment))
}

func appendToolFragment(call *types.ToolCall, fragment types.ToolCall) {
	if fragment.ID != "" {
		call.ID = fragment.ID
	}
	if fragment.Type != "" {
		call.Type = fragment.Type
	}
	if fragment.Name != "" {
		call.Name = fragment.Name
	}
	if fragment.ThoughtSignature != "" {
		call.ThoughtSignature = fragment.ThoughtSignature
	}
	if fragment.Arguments != nil {
		call.Arguments = types.CloneMap(fragment.Arguments)
	}
	if fragment.ArgsInvalid {
		call.ArgsInvalid = true
		call.ArgsParseError = fragment.ArgsParseError
	}
	if fragment.Function != nil {
		if call.Function == nil {
			call.Function = &types.ToolCallFunction{}
		}
		if fragment.Function.Name != "" {
			call.Function.Name = fragment.Function.Name
		}
		call.Function.Arguments += fragment.Function.Arguments
	}
}

func (c *textResponseCollector) finishTools() {
	for _, call := range c.pending {
		if c.hasCompleted(call) {
			continue
		}
		if call.Name == "" && call.Function != nil {
			call.Name = call.Function.Name
		}
		if !call.ArgsInvalid {
			normalized, err := types.NormalizeToolCall(call)
			if err != nil {
				call.ArgsInvalid = true
				call.ArgsParseError = err.Error()
			} else {
				call = normalized
			}
		}
		c.response.ToolCalls = append(c.response.ToolCalls, call)
	}
}

func (c *textResponseCollector) hasCompleted(call types.ToolCall) bool {
	for _, completed := range c.response.ToolCalls {
		if call.ID != "" && call.ID == completed.ID {
			return true
		}
	}
	return false
}
