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
			deltas := chunk.ToolCalls
			if len(deltas) == 0 && chunk.ToolCall != nil {
				deltas = []types.ToolCall{*chunk.ToolCall}
			}
			if len(deltas) == 0 && chunk.Delta != nil {
				deltas = chunk.Delta.ToolCalls
			}
			visible := make([]types.ToolCall, 0, len(deltas))
			for _, delta := range deltas {
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
				name := delta.Name
				if name == "" && delta.Function != nil {
					name = delta.Function.Name
				}
				if name != "" {
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
					send(types.TextChunk{Error: fmt.Errorf("structured result exceeds streaming limit")})
					return
				}
				if call.application {
					visible = append(visible, delta)
				} else {
					call.pending = append(call.pending, types.CloneToolCalls([]types.ToolCall{delta})[0])
					if call.name != "" && !strings.HasPrefix(ResultToolName, call.name) {
						call.application = true
						visible = append(visible, call.pending...)
						call.pending = nil
					} else if call.name == ResultToolName {
						// The reserved name is complete; its arguments stay private.
						call.pending = nil
					}
				}
			}
			chunk.ToolCall = nil
			chunk.ToolCalls = visible
			if chunk.Delta != nil {
				detached := *chunk.Delta
				detached.ToolCalls = visible
				chunk.Delta = &detached
			}
			if chunk.FinishReason != nil {
				var result *streamedCall
				applications := 0
				for _, call := range calls {
					if call.name == ResultToolName {
						if result != nil {
							chunk.Error = fmt.Errorf("multiple structured result calls")
						}
						result = call
					} else {
						applications++
						visible = append(visible, call.pending...)
						call.pending = nil
					}
				}
				if chunk.Error == nil && result != nil {
					if applications > 0 {
						chunk.Error = fmt.Errorf("application and final result calls were mixed")
					} else {
						var value any
						if result.args != "" {
							chunk.Error = json.Unmarshal([]byte(result.args), &value)
						} else {
							value = result.arguments
						}
						if chunk.Error == nil {
							chunk.Error = Validate(schema, value)
						}
						if chunk.Error == nil {
							raw, err := json.Marshal(value)
							chunk.Error = err
							chunk.Text = string(raw)
							if chunk.Delta != nil {
								chunk.Delta.Content = chunk.Text
							}
							finish := types.FinishReasonStop
							chunk.FinishReason = &finish
						}
					}
				} else if chunk.Error == nil && applications == 0 {
					chunk.Error = fmt.Errorf("missing structured result tool call")
				}
				if chunk.Error != nil {
					chunk.FinishReason = nil
					chunk.Text = ""
					if chunk.Delta != nil {
						chunk.Delta.Content = ""
					}
				}
				completed = true
			}
			chunk.ToolCalls = visible
			if chunk.Delta != nil {
				chunk.Delta.ToolCalls = visible
			}
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
