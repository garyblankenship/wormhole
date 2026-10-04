package openai

import (
	"context"
	"errors"

	"github.com/garyblankenship/wormhole/v3/types"
)

type accumulatedToolCall struct {
	id   string
	typ  string
	name string
	args []byte // accumulated raw argument fragments
}

// accumulatingStream wraps a raw OpenAI chunk channel and stitches streaming
// tool-call argument fragments. It is the sole closer of its output channel and
// guards every send with ctx so it exits when the consumer stops reading.
// On the terminal chunk (FinishReason set), assembled tool calls are attached
// to that chunk's ToolCalls before it is forwarded.
func (p *Provider) accumulatingStream(ctx context.Context, in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		acc := newStreamFragmentAccumulator()
		flushed := false
		for {
			var chunk types.TextChunk
			select {
			case <-ctx.Done():
				return
			case next, ok := <-in:
				if !ok {
					return
				}
				chunk = next
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
		// A supported end marker can close the parser without a finish chunk.
		// Buffered tool calls still require explicit completion; do not lose them
		// silently or expose them as successful calls when that event is absent.
		if ctx.Err() == nil {
			if calls := acc.finish(); len(calls) > 0 {
				select {
				case out <- types.TextChunk{Error: errors.New("stream ended before tool-call completion"), ToolCalls: calls}:
				case <-ctx.Done():
				}
			}
		}
	}()
	return out
}

// streamFragmentAccumulator stitches []types.ToolCall fragments (as emitted by
// convertToolCalls) into complete tool calls. OpenAI opens each tool call with a
// fragment carrying id+name (and index 0,1,2...); subsequent fragments for that
// call carry empty id and only an argument substring on Function.Arguments.
// Fragments are merged by their stream index so interleaved tool-call deltas
// route to the correct call.
type streamFragmentAccumulator struct {
	calls map[int]*accumulatedToolCall // keyed by stream index
	order []int                        // first-seen index ordering
}

func newStreamFragmentAccumulator() *streamFragmentAccumulator {
	return &streamFragmentAccumulator{
		calls: make(map[int]*accumulatedToolCall),
	}
}

func (s *streamFragmentAccumulator) add(frags []types.ToolCall) {
	for _, f := range frags {
		raw := ""
		if f.Function != nil {
			raw = f.Function.Arguments
		}
		acc, ok := s.calls[f.Index]
		if !ok {
			acc = &accumulatedToolCall{}
			s.calls[f.Index] = acc
			s.order = append(s.order, f.Index)
		}
		if f.ID != "" {
			acc.id = f.ID
		}
		if f.Type != "" {
			acc.typ = f.Type
		}
		if f.Name != "" {
			acc.name = f.Name
		}
		acc.args = append(acc.args, raw...)
	}
}

func (s *streamFragmentAccumulator) finish() []types.ToolCall {
	if len(s.order) == 0 {
		return nil
	}
	out := make([]types.ToolCall, 0, len(s.order))
	for _, idx := range s.order {
		acc := s.calls[idx]
		argsMap, parseErrMsg := types.ParseToolArgs(string(acc.args), map[string]any{})
		toolCall := types.ToolCall{
			Index:     idx,
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
	// A later usage, finish, or error event must not emit these calls again.
	clear(s.calls)
	s.order = nil
	return out
}
