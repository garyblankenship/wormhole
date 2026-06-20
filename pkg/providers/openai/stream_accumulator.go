package openai

import (
	"context"

	"github.com/garyblankenship/wormhole/pkg/types"
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
			if chunk.IsDone() {
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
// call carry empty id and only an argument substring on Function.Arguments. We
// resolve which in-flight call a fragment belongs to: a fragment WITH a
// non-empty ID opens/advances to a new slot; a fragment with empty ID appends
// its raw args to the most-recently-opened slot.
type streamFragmentAccumulator struct {
	calls []*accumulatedToolCall
}

func newStreamFragmentAccumulator() *streamFragmentAccumulator {
	return &streamFragmentAccumulator{}
}

func (s *streamFragmentAccumulator) add(frags []types.ToolCall) {
	for _, f := range frags {
		raw := ""
		if f.Function != nil {
			raw = f.Function.Arguments
		}
		if f.ID != "" {
			// New tool call opens.
			s.calls = append(s.calls, &accumulatedToolCall{
				id:   f.ID,
				typ:  f.Type,
				name: f.Name,
				args: append([]byte(nil), raw...),
			})
			continue
		}
		// Continuation fragment: append to the last open call.
		if len(s.calls) == 0 {
			// Defensive: a continuation with no opener; start a new slot.
			s.calls = append(s.calls, &accumulatedToolCall{
				typ:  f.Type,
				name: f.Name,
				args: append([]byte(nil), raw...),
			})
			continue
		}
		last := s.calls[len(s.calls)-1]
		if f.Name != "" && last.name == "" {
			last.name = f.Name
		}
		last.args = append(last.args, raw...)
	}
}

func (s *streamFragmentAccumulator) finish() []types.ToolCall {
	if len(s.calls) == 0 {
		return nil
	}
	out := make([]types.ToolCall, 0, len(s.calls))
	for _, acc := range s.calls {
		argsMap, parseErrMsg := types.ParseToolArgs(string(acc.args), map[string]any{})
		toolCall := types.ToolCall{
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
