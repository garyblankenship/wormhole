package openai

import (
	"context"

	"github.com/garyblankenship/wormhole/v2/types"
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
		for chunk := range in {
			// Fold any tool-call fragments out of the delta; they are buffered,
			// not forwarded mid-stream (a partial fragment is not a usable call).
			if chunk.Delta != nil && len(chunk.Delta.ToolCalls) > 0 {
				acc.add(chunk.Delta.ToolCalls)
				chunk.Delta.ToolCalls = nil
			}
			if len(chunk.ToolCalls) > 0 {
				acc.add(chunk.ToolCalls)
				chunk.ToolCalls = nil
			}
			// The default transformer path also stamps the singular ToolCall
			// pointer with the same per-fragment call. The plural slice above
			// already fed the accumulator, so drop the singular to stop the raw
			// fragment from leaking into MergeTextChunks (which folds singular
			// ToolCall from every chunk). Without this, fragments surface as N
			// separate tool calls instead of the one accumulated call.
			chunk.ToolCall = nil
			// On the terminal chunk, attach the assembled, parsed tool calls.
			// Also flush on an error chunk so buffered fragments are not silently
			// dropped when a stream ends prematurely.
			if chunk.IsDone() || chunk.Error != nil {
				if calls := acc.finish(); len(calls) > 0 {
					chunk.ToolCalls = calls
				}
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
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
	return out
}
