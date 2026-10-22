package structured

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

type streamedCall struct {
	id, name, args string
	arguments      map[string]any
	pending        []types.ToolCall
	application    bool
}

// chunkDeltas normalizes the chunk's tool-call representations into one list.
func chunkDeltas(chunk types.TextChunk) []types.ToolCall {
	deltas := chunk.ToolCalls
	if len(deltas) == 0 && chunk.ToolCall != nil {
		deltas = []types.ToolCall{*chunk.ToolCall}
	}
	if len(deltas) == 0 && chunk.Delta != nil {
		deltas = chunk.Delta.ToolCalls
	}
	return deltas
}

// reconcileCall returns the streamed call a delta belongs to. Out-of-order or
// reindexed carriers are matched by identifier before a new slot is allocated.
func reconcileCall(calls map[int]*streamedCall, delta types.ToolCall) *streamedCall {
	index := delta.Index
	call := calls[index]
	if delta.ID != "" && call != nil && call.id != "" && call.id != delta.ID {
		call = nil
		for existingIndex, existing := range calls {
			if existing.id == delta.ID {
				index = existingIndex
				call = existing
				break
			}
		}
		if call == nil {
			index = len(calls)
			for calls[index] != nil {
				index++
			}
		}
	}
	if call == nil {
		call = &streamedCall{}
		calls[index] = call
	}
	if delta.ID != "" {
		call.id = delta.ID
	}
	return call
}

func deltaName(delta types.ToolCall) string {
	if delta.Name != "" {
		return delta.Name
	}
	if delta.Function != nil {
		return delta.Function.Name
	}
	return ""
}

// absorb folds one delta into the accumulated call state and enforces the
// streaming size budget.
func (call *streamedCall) absorb(delta types.ToolCall) error {
	if name := deltaName(delta); name != "" {
		if call.name == "" || strings.HasPrefix(name, call.name) {
			call.name = name
		} else if name != call.name {
			call.name += name
		}
	}
	if delta.Function != nil {
		call.args += delta.Function.Arguments
	}
	if delta.Arguments != nil {
		call.arguments = types.CloneMap(delta.Arguments)
	}
	if len(call.args)+len(call.name) > 10<<20 || len(call.pending) >= maxSchemaWork {
		return fmt.Errorf("structured result exceeds streaming limit")
	}
	return nil
}

// reveal forwards application-tool deltas and keeps synthetic result deltas
// private until the buffered arguments are validated at the terminal chunk.
func (call *streamedCall) reveal(visible []types.ToolCall, delta types.ToolCall) []types.ToolCall {
	if call.application {
		return append(visible, delta)
	}
	call.pending = append(call.pending, types.CloneToolCalls([]types.ToolCall{delta})[0])
	if call.name != "" && !strings.HasPrefix(ResultToolName, call.name) {
		call.application = true
		visible = append(visible, call.pending...)
		call.pending = nil
	} else if call.name == ResultToolName {
		// The reserved name is complete; its arguments stay private.
		call.pending = nil
	}
	return visible
}

// projectDeltas folds every delta into tracker state and returns the calls that
// are forwardable so far.
func projectDeltas(calls map[int]*streamedCall, deltas, visible []types.ToolCall) ([]types.ToolCall, error) {
	for _, delta := range deltas {
		call := reconcileCall(calls, delta)
		if err := call.absorb(delta); err != nil {
			return visible, err
		}
		visible = call.reveal(visible, delta)
	}
	return visible, nil
}

// finalizeCalls flushes buffered application calls at the terminal chunk and
// selects the single structured result call.
func finalizeCalls(calls map[int]*streamedCall, visible []types.ToolCall) (*streamedCall, []types.ToolCall, error) {
	var result *streamedCall
	var failure error
	applications := 0
	for _, call := range calls {
		if call.name == ResultToolName {
			if result != nil {
				failure = fmt.Errorf("multiple structured result calls")
			}
			result = call
			continue
		}
		applications++
		visible = append(visible, call.pending...)
		call.pending = nil
	}
	if failure == nil && result != nil {
		if applications > 0 {
			failure = fmt.Errorf("application and final result calls were mixed")
		}
	} else if failure == nil && applications == 0 {
		failure = fmt.Errorf("missing structured result tool call")
	}
	return result, visible, failure
}

// applyFinalResult decodes, validates, and projects the buffered result
// arguments onto the terminal chunk.
func applyFinalResult(schema types.Schema, chunk *types.TextChunk, result *streamedCall) error {
	var value any
	if result.args != "" {
		if err := json.Unmarshal([]byte(result.args), &value); err != nil {
			return err
		}
	} else {
		value = result.arguments
	}
	if err := Validate(schema, value); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	chunk.Text = string(raw)
	if chunk.Delta != nil {
		chunk.Delta.Content = chunk.Text
	}
	finish := types.FinishReasonStop
	chunk.FinishReason = &finish
	return nil
}

// clearFinalOutputs hides unprojected result state from a failed terminal chunk.
func clearFinalOutputs(chunk *types.TextChunk) {
	chunk.FinishReason = nil
	chunk.Text = ""
	if chunk.Delta != nil {
		chunk.Delta.Content = ""
	}
}

// finalizeChunk projects terminal state for the structured result, if any, and
// reports that the stream has completed.
func finalizeChunk(schema types.Schema, chunk *types.TextChunk, calls map[int]*streamedCall, visible []types.ToolCall) []types.ToolCall {
	result, flushed, err := finalizeCalls(calls, visible)
	visible = flushed
	chunk.Error = err
	if chunk.Error == nil && result != nil {
		chunk.Error = applyFinalResult(schema, chunk, result)
	}
	if chunk.Error != nil {
		clearFinalOutputs(chunk)
	}
	return visible
}

// attachVisible publishes the forwardable calls on every carrier of the chunk.
func attachVisible(chunk *types.TextChunk, visible []types.ToolCall) {
	chunk.ToolCall = nil
	chunk.ToolCalls = visible
	if chunk.Delta != nil {
		detached := *chunk.Delta
		detached.ToolCalls = visible
		chunk.Delta = &detached
	}
}

// StreamTools forwards application tool deltas but buffers and validates the
// synthetic result arguments. Its goroutine exits on cancellation or input close.
func StreamTools(ctx context.Context, schema types.Schema, in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		calls := map[int]*streamedCall{}
		send := func(chunk types.TextChunk) bool {
			select {
			case out <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}
		completed := false
		var pendingMetadata *types.ResponseMetadata
		for {
			var chunk types.TextChunk
			var ok bool
			select {
			case <-ctx.Done():
				return
			case chunk, ok = <-in:
			}
			if !ok {
				if !completed {
					send(types.TextChunk{Error: fmt.Errorf("structured stream ended without a terminal result")})
				}
				return
			}
			if chunk.ResponseMetadata != nil && pendingMetadata == nil {
				pendingMetadata = chunk.ResponseMetadata
			}
			if chunk.Error != nil {
				send(chunk)
				return
			}
			deltas := chunkDeltas(chunk)
			visible, err := projectDeltas(calls, deltas, make([]types.ToolCall, 0, len(deltas)))
			if err != nil {
				send(types.TextChunk{Error: err})
				return
			}
			attachVisible(&chunk, visible)
			if chunk.FinishReason != nil {
				visible = finalizeChunk(schema, &chunk, calls, visible)
				completed = true
			}
			attachVisible(&chunk, visible)
			// Suppressed synthetic deltas never create metadata-only first chunks.
			if len(deltas) > 0 && len(visible) == 0 && chunk.FinishReason == nil && chunk.Error == nil && chunk.Content() == "" && chunk.Thinking == nil {
				continue
			}
			if pendingMetadata != nil && chunk.ResponseMetadata == nil {
				chunk.ResponseMetadata = pendingMetadata
			}
			pendingMetadata = nil
			if !send(chunk) {
				return
			}
			if chunk.Error != nil {
				return
			}
		}
	}()
	return out
}
