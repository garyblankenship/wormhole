package wormhole

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

func buildToolResultMessage(result types.ToolResult) *types.ToolResultMessage {
	// For most providers, tool results are sent as a special message with role "tool"
	// The content depends on whether there are errors

	content := ""
	if result.Error != "" {
		content = fmt.Sprintf("Tool %s failed: %s", result.ToolCallID, result.Error)
	} else if resultJSON, err := json.Marshal(result.Result); err != nil {
		content = fmt.Sprintf("Tool %s failed to serialize: %v", result.ToolCallID, err)
	} else {
		content = fmt.Sprintf("Tool %s result: %s", result.ToolCallID, resultJSON)
	}

	return &types.ToolResultMessage{
		Content:      content,
		ToolCallID:   result.ToolCallID,
		FunctionName: result.Name,
	}
}

// BuildToolResultMessages creates one ToolResultMessage per tool result.
// Providers correlate tool results by ToolCallID, so parallel calls must not be
// collapsed into a single message associated with only the first call.
func (e *ToolExecutor) BuildToolResultMessages(toolResults []types.ToolResult) []*types.ToolResultMessage {
	messages := make([]*types.ToolResultMessage, 0, len(toolResults))
	for _, result := range toolResults {
		messages = append(messages, buildToolResultMessage(result))
	}
	return messages
}

// Stop stops any background goroutines used by the tool executor
func (e *ToolExecutor) Stop() {
	if e.ownsAdmission {
		e.admission.Stop()
	}
}
