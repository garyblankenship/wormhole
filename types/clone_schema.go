package types

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
