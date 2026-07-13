package types

import "reflect"

// CloneValue returns a detached copy of JSON-like data. Provider options,
// schemas, tool arguments, and message content use these shapes throughout the
// SDK, so ownership stays with the domain types instead of individual adapters.
func CloneValue(value any) any {
	if value == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(value), make(map[cloneVisit]reflect.Value)).Interface()
}

type cloneVisit struct {
	typeOf  reflect.Type
	kind    reflect.Kind
	pointer uintptr
	length  int
}

func cloneReflectValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if !src.IsValid() {
		return src
	}

	switch src.Kind() {
	case reflect.Interface:
		return cloneInterfaceValue(src, visited)
	case reflect.Map:
		return cloneMapValue(src, visited)
	case reflect.Slice:
		return cloneSliceValue(src, visited)
	case reflect.Array:
		dst := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(cloneReflectValue(src.Index(i), visited))
		}
		return dst
	case reflect.Pointer:
		return clonePointerValue(src, visited)
	case reflect.Struct:
		return cloneStructValue(src, visited)
	default:
		return src
	}
}

func cloneInterfaceValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	cloned := cloneReflectValue(src.Elem(), visited)
	dst := reflect.New(src.Type()).Elem()
	dst.Set(cloned)
	return dst
}

func cloneMapValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	visit := cloneVisit{typeOf: src.Type(), kind: src.Kind(), pointer: src.Pointer()}
	if dst, found := visited[visit]; found {
		return dst
	}
	dst := reflect.MakeMapWithSize(src.Type(), src.Len())
	visited[visit] = dst
	iterator := src.MapRange()
	for iterator.Next() {
		dst.SetMapIndex(iterator.Key(), cloneReflectValue(iterator.Value(), visited))
	}
	return dst
}

func cloneSliceValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	visit := cloneVisit{typeOf: src.Type(), kind: src.Kind(), pointer: src.Pointer(), length: src.Len()}
	if dst, found := visited[visit]; found {
		return dst
	}
	dst := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
	visited[visit] = dst
	for i := 0; i < src.Len(); i++ {
		dst.Index(i).Set(cloneReflectValue(src.Index(i), visited))
	}
	return dst
}

func clonePointerValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	if src.IsNil() {
		return reflect.Zero(src.Type())
	}
	visit := cloneVisit{typeOf: src.Type(), kind: src.Kind(), pointer: src.Pointer()}
	if dst, found := visited[visit]; found {
		return dst
	}
	dst := reflect.New(src.Type().Elem())
	visited[visit] = dst
	dst.Elem().Set(cloneReflectValue(src.Elem(), visited))
	return dst
}

func cloneStructValue(src reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.Set(src)
	for i := 0; i < src.NumField(); i++ {
		if src.Type().Field(i).IsExported() {
			dst.Field(i).Set(cloneReflectValue(src.Field(i), visited))
		}
	}
	return dst
}

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

// CloneSchema returns a detached copy of SDK schema types and raw JSON-like
// schema values.
func CloneSchema(src Schema) Schema {
	switch schema := src.(type) {
	case *ObjectSchema:
		if schema == nil {
			return (*ObjectSchema)(nil)
		}
		dst := *schema
		dst.Required = append([]string(nil), schema.Required...)
		if schema.Properties != nil {
			dst.Properties = make(map[string]SchemaInterface, len(schema.Properties))
			for name, property := range schema.Properties {
				cloned, _ := CloneSchema(property).(SchemaInterface)
				dst.Properties[name] = cloned
			}
		}
		return &dst
	case *ArraySchema:
		if schema == nil {
			return (*ArraySchema)(nil)
		}
		dst := *schema
		dst.Items, _ = CloneSchema(schema.Items).(SchemaInterface)
		return &dst
	case *StringSchema:
		if schema == nil {
			return (*StringSchema)(nil)
		}
		dst := *schema
		dst.MinLength = cloneIntPointer(schema.MinLength)
		dst.MaxLength = cloneIntPointer(schema.MaxLength)
		return &dst
	case *NumberSchema:
		if schema == nil {
			return (*NumberSchema)(nil)
		}
		dst := *schema
		dst.Minimum = cloneFloat64Pointer(schema.Minimum)
		dst.Maximum = cloneFloat64Pointer(schema.Maximum)
		return &dst
	case *BooleanSchema:
		if schema == nil {
			return (*BooleanSchema)(nil)
		}
		dst := *schema
		return &dst
	case *EnumSchema:
		if schema == nil {
			return (*EnumSchema)(nil)
		}
		dst := *schema
		if schema.Enum != nil {
			dst.Enum = make([]any, len(schema.Enum))
			for i := range schema.Enum {
				dst.Enum[i] = CloneValue(schema.Enum[i])
			}
		}
		return &dst
	default:
		return CloneValue(src)
	}
}

func cloneIntPointer(src *int) *int {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func cloneFloat64Pointer(src *float64) *float64 {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}
