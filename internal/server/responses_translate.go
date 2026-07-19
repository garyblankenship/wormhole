package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

func translateResponsesTools(input []responsesTool, selection responsesToolChoiceSelection) ([]types.Tool, map[string]bool, error) {
	tools := make([]types.Tool, 0, len(input))
	customTools := make(map[string]bool)
	available := make(map[string]bool)
	for _, tool := range input {
		if tool.Type != "function" && tool.Type != "custom" {
			return nil, nil, fmt.Errorf("unsupported tool type %q with name %q", tool.Type, tool.Name)
		}
		if tool.Name == "" {
			return nil, nil, fmt.Errorf("%s tool name is required", tool.Type)
		}
		if selection.allowedTools != nil && !selection.allowedTools[tool.Name] {
			continue
		}
		schema := tool.Parameters
		if tool.Type == "custom" {
			customTools[tool.Name] = true
			schema = map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"input": map[string]any{"type": "string", "description": "Raw custom tool input"}},
				"required":             []string{"input"},
				"additionalProperties": false,
			}
		}
		tools = append(tools, *types.NewTool(tool.Name, tool.Description, schema))
		available[tool.Name] = true
	}
	for name := range selection.allowedTools {
		if !available[name] {
			return nil, nil, fmt.Errorf("allowed tool %q has no Chat Completions equivalent", name)
		}
	}
	if choice := selection.choice; choice != nil {
		if choice.Type == types.ToolChoiceTypeAny && len(available) == 0 {
			return nil, nil, fmt.Errorf("tool_choice requires a tool, but no portable tools remain after translation")
		}
		if choice.Type == types.ToolChoiceTypeSpecific && !available[choice.ToolName] {
			return nil, nil, fmt.Errorf("selected tool %q has no Chat Completions equivalent", choice.ToolName)
		}
	}
	return tools, customTools, nil
}

func responsesMessages(req responsesRequest) ([]types.Message, error) {
	messages := make([]types.Message, 0, len(req.Input.Items)+2)
	if req.Instructions != "" {
		messages = append(messages, types.NewSystemMessage(req.Instructions))
	}
	if req.Input.Text != "" {
		return append(messages, types.NewUserMessage(req.Input.Text)), nil
	}
	for _, item := range req.Input.Items {
		switch item.Type {
		case "message":
			text, media, err := responsesContent(item.Content)
			if err != nil {
				return nil, err
			}
			switch item.Role {
			case "developer", "system":
				if len(media) > 0 {
					return nil, fmt.Errorf("image content is only supported on user messages")
				}
				messages = append(messages, types.NewSystemMessage(text))
			case "user":
				messages = append(messages, &types.UserMessage{Content: text, Media: media})
			case "assistant":
				if len(media) > 0 {
					return nil, fmt.Errorf("image content is only supported on user messages")
				}
				messages = append(messages, types.NewAssistantMessage(text))
			default:
				return nil, fmt.Errorf("unsupported message role %q", item.Role)
			}
		case "function_call":
			if item.CallID == "" || item.Name == "" {
				return nil, fmt.Errorf("function_call requires call_id and name")
			}
			var appendErr error
			messages, appendErr = appendAssistantToolCall(messages, ChatToolCall{
				ID: item.CallID, Type: "function", Function: ChatToolCallFunction{Name: item.Name, Arguments: item.Arguments},
			})
			if appendErr != nil {
				return nil, appendErr
			}
		case "custom_tool_call":
			if item.CallID == "" || item.Name == "" {
				return nil, fmt.Errorf("custom_tool_call requires call_id and name")
			}
			arguments, err := json.Marshal(map[string]string{"input": item.CustomInput})
			if err != nil {
				return nil, fmt.Errorf("encode custom tool input: %w", err)
			}
			messages, err = appendAssistantToolCall(messages, ChatToolCall{
				ID: item.CallID, Type: "function", Function: ChatToolCallFunction{Name: item.Name, Arguments: string(arguments)},
			})
			if err != nil {
				return nil, err
			}
		case "function_call_output", "custom_tool_call_output":
			if item.CallID == "" {
				return nil, fmt.Errorf("%s requires call_id", item.Type)
			}
			text, media, err := responsesContent(item.Output)
			if err != nil {
				return nil, err
			}
			if len(media) > 0 {
				return nil, fmt.Errorf("image content is not supported in tool output")
			}
			messages = append(messages, types.NewToolResultMessage(item.CallID, text))
		case "reasoning":
			// Provider reasoning artifacts are not portable to Chat Completions.
		default:
			return nil, fmt.Errorf("unsupported response input item %q", item.Type)
		}
	}
	return messages, nil
}

func appendAssistantToolCall(messages []types.Message, call ChatToolCall) ([]types.Message, error) {
	toolCalls, err := toWormholeToolCalls([]ChatToolCall{call})
	if err != nil {
		return nil, err
	}
	toolCall := toolCalls[0]
	if len(messages) > 0 {
		if assistant, ok := messages[len(messages)-1].(*types.AssistantMessage); ok {
			assistant.ToolCalls = append(assistant.ToolCalls, toolCall)
			return messages, nil
		}
	}
	assistant := types.NewAssistantMessage("")
	assistant.ToolCalls = []types.ToolCall{toolCall}
	return append(messages, assistant), nil
}

func responsesContent(raw json.RawMessage) (string, []types.Media, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil, nil
	}
	var parts []responsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", nil, fmt.Errorf("response content must be a string or array of content parts")
	}
	var out strings.Builder
	var media []types.Media
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			out.WriteString(part.Text)
		case "input_image":
			image, err := parseImageURLPart(part.ImageURL)
			if err != nil {
				return "", nil, err
			}
			media = append(media, image)
		default:
			return "", nil, fmt.Errorf("unsupported response content part %q", part.Type)
		}
	}
	return out.String(), media, nil
}

func parseResponsesToolChoice(raw json.RawMessage) (responsesToolChoiceSelection, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return responsesToolChoiceSelection{}, nil
	}
	if choice, err := parseToolChoice(raw); err == nil && choice != nil {
		return responsesToolChoiceSelection{choice: choice}, nil
	}
	var item struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Mode  string `json:"mode"`
		Tools []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return responsesToolChoiceSelection{}, fmt.Errorf("invalid tool_choice: %w", err)
	}
	switch item.Type {
	case "function", "custom":
		if item.Name == "" {
			return responsesToolChoiceSelection{}, fmt.Errorf("tool_choice %q requires name", item.Type)
		}
		return responsesToolChoiceSelection{choice: &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: item.Name}}, nil
	case "allowed_tools":
		if len(item.Tools) == 0 {
			return responsesToolChoiceSelection{}, fmt.Errorf("allowed_tools requires at least one tool")
		}
		allowed := make(map[string]bool, len(item.Tools))
		for _, tool := range item.Tools {
			if (tool.Type != "function" && tool.Type != "custom") || tool.Name == "" {
				return responsesToolChoiceSelection{}, fmt.Errorf("allowed tool type %q with name %q has no Chat Completions equivalent", tool.Type, tool.Name)
			}
			allowed[tool.Name] = true
		}
		var choice types.ToolChoice
		switch item.Mode {
		case "auto":
			choice.Type = types.ToolChoiceTypeAuto
		case "required":
			choice.Type = types.ToolChoiceTypeAny
		default:
			return responsesToolChoiceSelection{}, fmt.Errorf("unsupported allowed_tools mode %q", item.Mode)
		}
		return responsesToolChoiceSelection{choice: &choice, allowedTools: allowed}, nil
	default:
		return responsesToolChoiceSelection{}, fmt.Errorf("unsupported tool_choice type %q", item.Type)
	}
}
