package anthropic

import (
	"context"
	"errors"

	"github.com/garyblankenship/wormhole/v3/types"
)

// accumulatedToolCall buffers one in-flight Anthropic tool_use block.
type accumulatedToolCall struct {
	id   string
	typ  string
	name string
	args []byte // accumulated partial_json fragments
}

// streamFragmentAccumulator correlates Anthropic fragments by their wire
// content-block index, which may include gaps occupied by text or thinking.
type streamFragmentAccumulator struct {
	calls     map[int]*accumulatedToolCall
	order     []int
	usage     *types.Usage
	ambiguous bool // wire indices are not distinguishable; follow recency
	synthetic int  // next synthetic slot key for collided indices
}

func newStreamFragmentAccumulator() *streamFragmentAccumulator {
	return &streamFragmentAccumulator{calls: make(map[int]*accumulatedToolCall)}
}

func (s *streamFragmentAccumulator) add(frags []types.ToolCall) {
	for _, fragment := range frags {
		index := fragment.Index
		call, exists := s.calls[index]
		if fragment.ID != "" && exists && call.id != "" && call.id != fragment.ID {
			// A second distinct tool_use block reuses this index: the wire omits
			// distinguishable indices. Separate calls by identity from here on.
			s.ambiguous = true
			s.synthetic--
			index = s.synthetic
			call, exists = nil, false
		} else if fragment.ID == "" && s.ambiguous {
			// Continuations after a detected collision follow the most recent call.
			if len(s.order) > 0 {
				index = s.order[len(s.order)-1]
				call, exists = s.calls[index], true
			}
		}
		if !exists {
			call = &accumulatedToolCall{}
			s.calls[index] = call
			s.order = append(s.order, index)
		}
		if fragment.ID != "" {
			call.id = fragment.ID
		}
		if fragment.Type != "" {
			call.typ = fragment.Type
		}
		if fragment.Name != "" {
			call.name = fragment.Name
		}
		if fragment.Function != nil {
			if fragment.Function.Name != "" && call.name == "" {
				call.name = fragment.Function.Name
			}
			call.args = append(call.args, fragment.Function.Arguments...)
		}
	}
}

func (s *streamFragmentAccumulator) finish() []types.ToolCall {
	if len(s.calls) == 0 {
		return nil
	}
	out := make([]types.ToolCall, 0, len(s.calls))
	for position, index := range s.order {
		acc := s.calls[index]
		if index < 0 {
			// Synthetic slots come from streams without distinguishable indices;
			// emit sequential positions for them.
			index = position
		}
		argsMap, parseErrMsg := types.ParseToolArgs(string(acc.args), map[string]any{})
		toolCall := types.ToolCall{
			Index:     index,
			ID:        acc.id,
			Type:      acc.typ,
			Name:      acc.name,
			Arguments: argsMap,
			Function: &types.ToolCallFunction{
				Name:      acc.name,
				Arguments: string(acc.args),
			},
		}
		toolCall.MarkArgsError(parseErrMsg)
		out = append(out, toolCall)
	}
	s.calls = nil
	return out
}

// accumulatingStream wraps a raw Anthropic chunk channel and stitches streaming
// tool-call fragments. Sole closer of out; every send is ctx-guarded so the
// goroutine exits if the consumer stops reading. On the terminal chunk
// (FinishReason set via message_delta), assembled tool calls are attached.
func (p *Provider) accumulatingStream(ctx context.Context, in <-chan types.StreamChunk) <-chan types.StreamChunk {
	out := make(chan types.StreamChunk)
	go func() {
		defer close(out)
		acc := newStreamFragmentAccumulator()
		flushed := false
	loop:
		for {
			var chunk types.StreamChunk
			select {
			case <-ctx.Done():
				return
			case next, ok := <-in:
				if !ok {
					// Upstream closed without a terminal chunk; flush below.
					break loop
				}
				chunk = next
			}
			if chunk.Usage != nil {
				if acc.usage == nil {
					copy := *chunk.Usage
					acc.usage = &copy
				} else {
					// Usage events contain cumulative counts, not increments.
					// Preserve input/cache counts omitted by later output updates.
					acc.usage.PromptTokens = max(acc.usage.PromptTokens, chunk.Usage.PromptTokens)
					acc.usage.CompletionTokens = max(acc.usage.CompletionTokens, chunk.Usage.CompletionTokens)
					acc.usage.CacheReadTokens = max(acc.usage.CacheReadTokens, chunk.Usage.CacheReadTokens)
					acc.usage.CacheWriteTokens = max(acc.usage.CacheWriteTokens, chunk.Usage.CacheWriteTokens)
					acc.usage.TotalTokens = acc.usage.PromptTokens + acc.usage.CompletionTokens
				}
			}
			if acc.usage != nil && (chunk.Usage != nil || chunk.IsDone()) {
				copy := *acc.usage
				chunk.Usage = &copy
			}
			// Delta is authoritative when the parser also exposes compatibility aliases.
			var fragments []types.ToolCall
			if chunk.Delta != nil {
				fragments = chunk.Delta.ToolCalls
			}
			if len(fragments) == 0 && !chunk.IsDone() && chunk.Error == nil {
				fragments = chunk.ToolCalls
				if len(fragments) == 0 && chunk.ToolCall != nil {
					fragments = []types.ToolCall{*chunk.ToolCall}
				}
				if len(fragments) > 0 {
					delta := types.ChunkDelta{}
					if chunk.Delta != nil {
						delta = *chunk.Delta
					}
					delta.ToolCalls = fragments
					chunk.Delta = &delta
				}
			}
			if !flushed {
				acc.add(fragments)
			}
			// Top-level calls are complete terminal results, never live fragments.
			chunk.ToolCall = nil
			terminalCalls := chunk.ToolCalls
			chunk.ToolCalls = nil
			if (chunk.IsDone() || chunk.Error != nil) && !flushed {
				chunk.ToolCalls = acc.finish()
				if len(chunk.ToolCalls) == 0 {
					chunk.ToolCalls = terminalCalls
				}
				flushed = true
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		if calls := acc.finish(); len(calls) > 0 {
			chunk := types.StreamChunk{Error: errors.New("stream ended before tool-call completion"), ToolCalls: calls}
			select {
			case out <- chunk:
			case <-ctx.Done():
			}
		}
	}()
	return out
}
