package openai

import (
	"github.com/garyblankenship/wormhole/v2/internal/pool"
	"github.com/garyblankenship/wormhole/v2/types"
)

func (p *Provider) transformResponsesInput(messages []types.Message) []map[string]any {
	items := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch m := msg.(type) {
		case *types.UserMessage:
			if len(m.Media) > 0 {
				items = append(items, responsesMessageItem(types.RoleUser, responsesUserMessageContent(m)))
				continue
			}
			items = append(items, responsesMessageItem(types.RoleUser, m.Content))
		case *types.AssistantMessage:
			if len(m.ToolCalls) > 0 {
				if m.Content != "" {
					items = append(items, responsesMessageItem(types.RoleAssistant, m.Content))
				}
				for _, tc := range m.ToolCalls {
					items = append(items, responsesFunctionCallItem(tc))
				}
				continue
			}
			items = append(items, responsesMessageItem(types.RoleAssistant, m.Content))
		case *types.ToolResultMessage:
			items = append(items, map[string]any{
				"type":    responsesItemFunctionCallOutput,
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		default:
			items = append(items, responsesMessageItem(msg.GetRole(), msg.GetContent()))
		}
	}
	return items
}

func responsesUserMessageContent(msg *types.UserMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, 1+len(msg.Media))
	if msg.Content != "" {
		parts = append(parts, types.TextPart(msg.Content))
	}
	for _, media := range msg.Media {
		if image, ok := media.(*types.ImageMedia); ok {
			url, ok := imageMediaURL(image)
			if !ok {
				continue
			}
			parts = append(parts, types.ImagePart(url))
		}
	}
	return parts
}

func responsesMessageItem(role types.Role, content any) map[string]any {
	return map[string]any{
		"type":    responsesItemMessage,
		"role":    string(role),
		"content": responsesMessageContent(content),
	}
}

func responsesMessageContent(content any) any {
	parts, ok := content.([]types.MessagePart)
	if !ok {
		return content
	}

	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			out = append(out, map[string]any{
				"type": responsesContentInputText,
				"text": part.Text,
			})
		case "image":
			item := map[string]any{
				"type": responsesContentInputImage,
			}
			switch data := part.Data.(type) {
			case string:
				item["image_url"] = data
			case map[string]any:
				for k, v := range data {
					item[k] = v
				}
			default:
				item["image_url"] = data
			}
			out = append(out, item)
		}
	}
	return out
}

func responsesFunctionCallItem(tc types.ToolCall) map[string]any {
	callID := tc.ID
	args := tc.Arguments
	if tc.Function != nil {
		callID = tc.ID
	}

	arguments := ""
	if tc.Function != nil && tc.Function.Arguments != "" {
		arguments = tc.Function.Arguments
	} else if len(args) > 0 {
		if b, err := pool.Marshal(args); err == nil {
			arguments = string(b)
			pool.Return(b)
		}
	}

	name := tc.Name
	if name == "" && tc.Function != nil {
		name = tc.Function.Name
	}

	return map[string]any{
		"type":      responsesItemFunctionCall,
		"id":        callID,
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
}

func (p *Provider) transformResponsesTools(tools []types.Tool) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		name := tool.Name
		description := tool.Description
		parameters := tool.InputSchema
		strict := false
		if tool.Function != nil {
			if tool.Function.Name != "" {
				name = tool.Function.Name
			}
			if tool.Function.Description != "" {
				description = tool.Function.Description
			}
			if tool.Function.Parameters != nil {
				parameters = tool.Function.Parameters
			}
		}

		out := map[string]any{
			"type":        "function",
			"name":        name,
			"description": description,
			"parameters":  parameters,
			"strict":      strict,
		}
		result = append(result, out)
	}
	return result
}

func (p *Provider) transformResponsesToolChoice(choice *types.ToolChoice) any {
	if choice == nil {
		return toolChoiceAuto
	}

	switch choice.Type {
	case types.ToolChoiceTypeNone:
		return "none"
	case types.ToolChoiceTypeAuto:
		return "auto"
	case types.ToolChoiceTypeAny:
		return "required"
	case types.ToolChoiceTypeSpecific:
		if choice.ToolName != "" {
			return map[string]any{
				"type": "function",
				"name": choice.ToolName,
			}
		}
	}
	return toolChoiceAuto
}
