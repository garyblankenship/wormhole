package openai

import (
	"github.com/garyblankenship/wormhole/v3/internal/pool"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (p *Provider) transformResponsesInput(messages []types.Message) ([]map[string]any, error) {
	items := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		switch m := msg.(type) {
		case *types.UserMessage:
			if len(m.Media) > 0 {
				item, err := responsesMessageItem(types.RoleUser, responsesUserMessageContent(m))
				if err != nil {
					return nil, err
				}
				items = append(items, item)
				continue
			}
			item, err := responsesMessageItem(types.RoleUser, m.Content)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		case *types.AssistantMessage:
			if len(m.ToolCalls) > 0 {
				if m.Content != "" {
					item, err := responsesMessageItem(types.RoleAssistant, m.Content)
					if err != nil {
						return nil, err
					}
					items = append(items, item)
				}
				for _, tc := range m.ToolCalls {
					items = append(items, responsesFunctionCallItem(tc))
				}
				continue
			}
			item, err := responsesMessageItem(types.RoleAssistant, m.Content)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		case *types.ToolResultMessage:
			items = append(items, map[string]any{
				"type":    responsesItemFunctionCallOutput,
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		default:
			item, err := responsesMessageItem(msg.GetRole(), msg.GetContent())
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func responsesUserMessageContent(msg *types.UserMessage) []types.MessagePart {
	parts := make([]types.MessagePart, 0, 1+len(msg.Media))
	if msg.Content != "" {
		parts = append(parts, types.TextPart(msg.Content))
	}
	for _, media := range msg.Media {
		if ref, ok := media.(*types.FileReferenceMedia); ok {
			parts = append(parts, types.MessagePart{Type: "file", Data: ref})
			continue
		}
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

func responsesMessageItem(role types.Role, content any) (map[string]any, error) {
	mapped, err := responsesMessageContent(content)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":    responsesItemMessage,
		"role":    string(role),
		"content": mapped,
	}, nil
}

func responsesMessageContent(content any) (any, error) {
	parts, ok := content.([]types.MessagePart)
	if !ok {
		return content, nil
	}

	out := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "file":
			ref, ok := part.Data.(*types.FileReferenceMedia)
			if !ok || ref == nil {
				return nil, types.NewValidationError("messages", "file_reference", part.Data, "file part requires non-nil *types.FileReferenceMedia")
			}
			typ := "input_file"
			if ref.Kind == "image" {
				typ = "input_image"
			}
			out = append(out, map[string]any{"type": typ, "file_id": ref.ID})
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
	return out, nil
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
