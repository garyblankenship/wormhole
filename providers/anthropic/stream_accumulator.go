package anthropic

import (
	"context"
	"encoding/json"
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

// mergeUsage folds cumulative usage counts into the accumulator and attaches
// the merged snapshot to usage and terminal chunks.
func (s *streamFragmentAccumulator) mergeUsage(chunk *types.StreamChunk) {
	if chunk.Usage != nil {
		if s.usage == nil {
			copy := *chunk.Usage
			s.usage = &copy
		} else {
			// Usage events contain cumulative counts, not increments.
			// Preserve input/cache counts omitted by later output updates.
			s.usage.PromptTokens = max(s.usage.PromptTokens, chunk.Usage.PromptTokens)
			s.usage.CompletionTokens = max(s.usage.CompletionTokens, chunk.Usage.CompletionTokens)
			s.usage.CacheReadTokens = max(s.usage.CacheReadTokens, chunk.Usage.CacheReadTokens)
			s.usage.CacheWriteTokens = max(s.usage.CacheWriteTokens, chunk.Usage.CacheWriteTokens)
			if len(chunk.Usage.CacheWriteTokensByTTL) > 0 {
				s.usage.CacheWriteTokensByTTL = cloneTTL(chunk.Usage.CacheWriteTokensByTTL)
			}
			s.usage.TotalTokens = s.usage.PromptTokens + s.usage.CompletionTokens
		}
	}
	if s.usage != nil && (chunk.Usage != nil || chunk.IsDone()) {
		copy := *s.usage
		copy.CacheWriteTokensByTTL = cloneTTL(s.usage.CacheWriteTokensByTTL)
		chunk.Usage = &copy
	}
}

// collectProviderUsage merges provider-native usage entries for the terminal snapshot.
func collectProviderUsage(chunk types.StreamChunk, providerUsage map[string]any) {
	if raw, ok := chunk.Metadata["provider_usage"].(map[string]any); ok {
		for k, v := range raw {
			providerUsage[k] = v
		}
	}
}

// attachNewCitations appends only the citations newly produced by this frame.
func attachNewCitations(raw json.RawMessage, refs *citationAccumulator, chunk *types.StreamChunk) {
	beforeSources, beforeCitations, beforeWarnings := len(refs.sources), len(refs.citations), len(refs.warnings)
	refs.add(raw)
	chunk.Sources = append([]types.Source(nil), refs.sources[beforeSources:]...)
	chunk.Citations = append([]types.Citation(nil), refs.citations[beforeCitations:]...)
	chunk.Warnings = append([]types.ResponseWarning(nil), refs.warnings[beforeWarnings:]...)
	delete(chunk.Metadata, "anthropic_citation")
}

// attachTerminalSnapshot publishes the full citation and provider-usage state
// on the terminal chunk.
func attachTerminalSnapshot(chunk *types.StreamChunk, refs *citationAccumulator, providerUsage map[string]any) {
	chunk.Sources = append([]types.Source(nil), refs.sources...)
	chunk.Citations = append([]types.Citation(nil), refs.citations...)
	chunk.Warnings = append([]types.ResponseWarning(nil), refs.warnings...)
	if len(providerUsage) > 0 {
		if chunk.Metadata == nil {
			chunk.Metadata = make(map[string]any)
		}
		copyUsage := make(map[string]any, len(providerUsage))
		for k, v := range providerUsage {
			copyUsage[k] = v
		}
		chunk.Metadata["provider_usage"] = copyUsage
	}
}

// pendingFragments selects inbound tool-call fragments, preferring the delta
// contract over compatibility aliases on non-terminal chunks.
func pendingFragments(chunk *types.StreamChunk) []types.ToolCall {
	var fragments []types.ToolCall
	if chunk.Delta != nil {
		fragments = chunk.Delta.ToolCalls
	}
	if len(fragments) > 0 || chunk.IsDone() || chunk.Error != nil {
		return fragments
	}
	if len(chunk.ToolCalls) > 0 {
		return chunk.ToolCalls
	}
	if chunk.ToolCall != nil {
		return []types.ToolCall{*chunk.ToolCall}
	}
	return nil
}

// settleToolCalls consumes inbound fragments and, on the terminal or error
// chunk, swaps in the accumulated calls, retaining parser terminal calls when
// nothing was accumulated.
func settleToolCalls(chunk *types.StreamChunk, acc *streamFragmentAccumulator, flushed *bool) {
	if chunk.Delta != nil {
		chunk.Delta.ToolCalls = nil
	}
	// Fragments are consumed here; only completed calls leave the accumulator.
	terminalCalls := chunk.ToolCalls
	chunk.ToolCall = nil
	chunk.ToolCalls = nil
	if *flushed || (!chunk.IsDone() && chunk.Error == nil) {
		return
	}
	chunk.ToolCalls = acc.finish()
	if len(chunk.ToolCalls) == 0 {
		chunk.ToolCalls = terminalCalls
	}
	*flushed = true
}

// emitChunk forwards one chunk, letting cancellation win deterministically
// over a waiting receiver.
func emitChunk(ctx context.Context, out chan<- types.StreamChunk, chunk types.StreamChunk) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}
	select {
	case out <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
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
		refs := newCitationAccumulator()
		providerUsage := make(map[string]any)
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
			acc.mergeUsage(&chunk)
			collectProviderUsage(chunk, providerUsage)
			if raw, ok := chunk.Metadata["anthropic_citation"].(json.RawMessage); ok {
				attachNewCitations(raw, refs, &chunk)
			}
			if chunk.IsDone() {
				attachTerminalSnapshot(&chunk, refs, providerUsage)
			}
			fragments := pendingFragments(&chunk)
			if !flushed {
				acc.add(fragments)
			}
			settleToolCalls(&chunk, acc, &flushed)
			if !emitChunk(ctx, out, chunk) {
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
