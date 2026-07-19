package types

// CloneMap returns a recursively detached copy of a JSON-like map.
func CloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = CloneValue(value)
	}
	return dst
}

// CloneToolCall returns a detached copy of a tool call.
func CloneToolCall(src ToolCall) ToolCall {
	dst := src
	dst.Arguments = CloneMap(src.Arguments)
	if src.Function != nil {
		function := *src.Function
		dst.Function = &function
	}
	return dst
}

// CloneToolCalls returns detached copies of tool calls.
func CloneToolCalls(src []ToolCall) []ToolCall {
	if src == nil {
		return nil
	}
	dst := make([]ToolCall, len(src))
	for i := range src {
		dst[i] = CloneToolCall(src[i])
	}
	return dst
}

// CloneTool returns a detached copy of a tool definition.
func CloneTool(src Tool) Tool {
	dst := src
	dst.InputSchema = CloneMap(src.InputSchema)
	if src.Function != nil {
		function := *src.Function
		function.Parameters = CloneMap(src.Function.Parameters)
		dst.Function = &function
	}
	return dst
}

// CloneTools returns detached copies of tool definitions.
func CloneTools(src []Tool) []Tool {
	if src == nil {
		return nil
	}
	dst := make([]Tool, len(src))
	for i := range src {
		dst[i] = CloneTool(src[i])
	}
	return dst
}

// CloneModelInfo returns a detached copy of model metadata.
func CloneModelInfo(src *ModelInfo) *ModelInfo {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Cost != nil {
		cost := *src.Cost
		dst.Cost = &cost
	}
	dst.Capabilities = append([]ModelCapability(nil), src.Capabilities...)
	dst.Constraints = CloneMap(src.Constraints)
	return &dst
}

// CloneMedia returns a detached copy of supported media values.
func CloneMedia(src Media) Media {
	switch media := src.(type) {
	case *ImageMedia:
		if media == nil {
			return (*ImageMedia)(nil)
		}
		dst := *media
		dst.Data = append([]byte(nil), media.Data...)
		return &dst
	case *DocumentMedia:
		if media == nil {
			return (*DocumentMedia)(nil)
		}
		dst := *media
		dst.Data = append([]byte(nil), media.Data...)
		return &dst
	default:
		return src
	}
}

// CloneMessage returns a detached copy of the SDK's concrete message types.
func CloneMessage(src Message) Message {
	switch message := src.(type) {
	case *SystemMessage:
		if message == nil {
			return (*SystemMessage)(nil)
		}
		dst := *message
		return &dst
	case *UserMessage:
		if message == nil {
			return (*UserMessage)(nil)
		}
		dst := *message
		if message.Media != nil {
			dst.Media = make([]Media, len(message.Media))
			for i := range message.Media {
				dst.Media[i] = CloneMedia(message.Media[i])
			}
		}
		return &dst
	case *AssistantMessage:
		if message == nil {
			return (*AssistantMessage)(nil)
		}
		dst := *message
		dst.ToolCalls = CloneToolCalls(message.ToolCalls)
		if message.Thinking != nil {
			thinking := *message.Thinking
			dst.Thinking = &thinking
		}
		return &dst
	case *ToolResultMessage:
		if message == nil {
			return (*ToolResultMessage)(nil)
		}
		dst := *message
		return &dst
	case BaseMessage:
		dst := message
		dst.Content = CloneValue(message.Content)
		return dst
	case *BaseMessage:
		if message == nil {
			return (*BaseMessage)(nil)
		}
		dst := *message
		dst.Content = CloneValue(message.Content)
		return &dst
	default:
		return src
	}
}

// CloneMessages returns detached copies of concrete messages.
func CloneMessages(src []Message) []Message {
	if src == nil {
		return nil
	}
	dst := make([]Message, len(src))
	for i := range src {
		dst[i] = CloneMessage(src[i])
	}
	return dst
}
