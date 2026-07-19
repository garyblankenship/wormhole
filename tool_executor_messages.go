package wormhole

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// BuildToolResultMessage creates one ToolResultMessage.
//
// Deprecated: pass exactly one result, or use BuildToolResultMessages. A single
// message cannot preserve ToolCallID correlation for multiple tool calls.
func (e *ToolExecutor) BuildToolResultMessage(toolResults []types.ToolResult) *types.ToolResultMessage {
	// For most providers, tool results are sent as a special message with role "tool"
	// The content depends on whether there are errors

	// Build content that includes all tool results using strings.Builder for efficiency
	var builder strings.Builder
	for i, result := range toolResults {
		if i > 0 {
			builder.WriteString("\n")
		}

		if result.Error != "" {
			fmt.Fprintf(&builder, "Tool %s failed: %s", result.ToolCallID, result.Error)
		} else {
			// Marshal result to JSON string for content
			resultJSON, err := json.Marshal(result.Result)
			if err != nil {
				fmt.Fprintf(&builder, "Tool %s failed to serialize: %v", result.ToolCallID, err)
			} else {
				fmt.Fprintf(&builder, "Tool %s result: %s", result.ToolCallID, string(resultJSON))
			}
		}
	}
	content := builder.String()

	// Use the first tool call ID/name (providers may handle multiple results differently)
	toolCallID := ""
	functionName := ""
	if len(toolResults) > 0 {
		toolCallID = toolResults[0].ToolCallID
		functionName = toolResults[0].Name
	}

	return &types.ToolResultMessage{
		Content:      content,
		ToolCallID:   toolCallID,
		FunctionName: functionName,
	}
}

// BuildToolResultMessages creates one ToolResultMessage per tool result.
// Providers correlate tool results by ToolCallID, so parallel calls must not be
// collapsed into a single message associated with only the first call.
func (e *ToolExecutor) BuildToolResultMessages(toolResults []types.ToolResult) []*types.ToolResultMessage {
	messages := make([]*types.ToolResultMessage, 0, len(toolResults))
	for _, result := range toolResults {
		messages = append(messages, e.BuildToolResultMessage([]types.ToolResult{result}))
	}
	return messages
}

// Stop stops any background goroutines used by the tool executor
func (e *ToolExecutor) Stop() {
	if e.adaptiveLimiter != nil {
		e.adaptiveLimiter.Stop()
	}
}
