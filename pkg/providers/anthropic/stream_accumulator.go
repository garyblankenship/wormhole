package anthropic

import (
	"context"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// accumulatedToolCall buffers one in-flight Anthropic tool_use block.
type accumulatedToolCall struct {
	id   string
	typ  string
	name string
	args []byte // accumulated partial_json fragments
}

// streamFragmentAccumulator stitches tool-call fragments emitted by
// parseStreamChunk into complete tool calls. Anthropic opens each tool_use block
// with a content_block_start fragment carrying id+name; subsequent
// input_json_delta fragments carry empty id and only an argument substring on
// Function.Arguments. A fragment WITH a non-empty ID opens a new slot; a
// fragment with empty ID appends its raw args to the most-recently-opened slot.
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
			s.calls = append(s.calls, &accumulatedToolCall{
				id:   f.ID,
				typ:  f.Type,
				name: f.Name,
				args: append([]byte(nil), raw...),
			})
			continue
		}
		if len(s.calls) == 0 {
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

// accumulatingStream wraps a raw Anthropic chunk channel and stitches streaming
// tool-call fragments. Sole closer of out; every send is ctx-guarded so the
// goroutine exits if the consumer stops reading. On the terminal chunk
// (FinishReason set via message_delta), assembled tool calls are attached.
func (p *Provider) accumulatingStream(ctx context.Context, in <-chan types.StreamChunk) <-chan types.StreamChunk {
	out := make(chan types.StreamChunk)
	go func() {
		defer close(out)
		acc := newStreamFragmentAccumulator()
		for chunk := range in {
			if chunk.Delta != nil && len(chunk.Delta.ToolCalls) > 0 {
				acc.add(chunk.Delta.ToolCalls)
				chunk.Delta.ToolCalls = nil
			}
			if len(chunk.ToolCalls) > 0 {
				acc.add(chunk.ToolCalls)
				chunk.ToolCalls = nil
			}
			// On the terminal chunk, attach assembled tool calls. Also flush on
			// an error chunk so buffered fragments are not silently dropped when
			// a stream ends prematurely.
			if chunk.IsDone() || chunk.Error != nil {
				if calls := acc.finish(); len(calls) > 0 {
					chunk.ToolCalls = calls
				}
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
