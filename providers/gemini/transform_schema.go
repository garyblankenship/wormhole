package gemini

import (
	"encoding/json"

	"github.com/garyblankenship/wormhole/v2/types"
)

// normalizeSchemaMap rewrites JSON Schema union types into Gemini-compatible form,
// in place and recursively. Gemini/Vertex reject an array-valued `type`:
//
//	["T","null"]   -> {type:"T", nullable:true}
//	["A","B",...]  -> {anyOf:[{type:"A"},{type:"B"},...] } (+ nullable:true if "null" present)
//	["T"]          -> {type:"T"}
//
// It recurses into properties, items, and anyOf/oneOf/allOf/$defs/definitions.
func normalizeSchemaMap(schema map[string]any) map[string]any {
	normalized := types.CloneMap(schema)
	normalizeSchemaMapInPlace(normalized)
	return normalized
}

func normalizeSchemaMapInPlace(m map[string]any) {
	if m == nil {
		return
	}
	normalizeSchemaType(m)
	normalizeSchemaChildren(m)
}

func normalizeSchemaType(m map[string]any) {
	if raw, ok := m["type"]; ok {
		if list, ok := typeStringList(raw); ok {
			normalizeSchemaTypeList(m, list)
		}
	}
}

func normalizeSchemaTypeList(m map[string]any, list []string) {
	seen := make(map[string]struct{}, len(list))
	nonNull := make([]string, 0, len(list))
	hasNull := false
	for _, schemaType := range list {
		if schemaType == "null" {
			hasNull = true
			continue
		}
		if _, duplicate := seen[schemaType]; duplicate {
			continue
		}
		seen[schemaType] = struct{}{}
		nonNull = append(nonNull, schemaType)
	}
	if hasNull {
		m["nullable"] = true
	}
	switch len(nonNull) {
	case 0:
		m["type"] = "null"
	case 1:
		m["type"] = nonNull[0]
	default:
		branches := make([]any, len(nonNull))
		for i, schemaType := range nonNull {
			branches[i] = map[string]any{"type": schemaType}
		}
		delete(m, "type")
		m["anyOf"] = branches
	}
}

func normalizeSchemaChildren(m map[string]any) {
	if props, ok := m["properties"].(map[string]any); ok {
		for _, v := range props {
			if sub, ok := v.(map[string]any); ok {
				normalizeSchemaMapInPlace(sub)
			}
		}
	}
	if items, ok := m["items"].(map[string]any); ok {
		normalizeSchemaMapInPlace(items)
	}
	if itemsList, ok := m["items"].([]any); ok {
		normalizeSchemaList(itemsList)
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := m[key].([]any); ok {
			normalizeSchemaList(arr)
		}
	}
	for _, key := range []string{"$defs", "definitions"} {
		if defs, ok := m[key].(map[string]any); ok {
			for _, v := range defs {
				if sub, ok := v.(map[string]any); ok {
					normalizeSchemaMapInPlace(sub)
				}
			}
		}
	}
}

func normalizeSchemaList(schemas []any) {
	for _, schema := range schemas {
		if nested, ok := schema.(map[string]any); ok {
			normalizeSchemaMapInPlace(nested)
		}
	}
}

// typeStringList coerces a JSON Schema `type` value that may be []any (post-unmarshal)
// or []string into []string. Returns ok=false for a plain string or other shapes.
func typeStringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// schemaToMap recursively converts schema to map
func (g *Gemini) schemaToMap(schema types.Schema) map[string]any {
	// Handle raw JSON bytes
	if bytes, ok := schema.([]byte); ok {
		var result map[string]any
		if err := json.Unmarshal(bytes, &result); err == nil {
			return normalizeSchemaMap(result)
		}
	}

	// Concrete schema pointer types (*ObjectSchema/*ArraySchema/*EnumSchema/
	// *NumberSchema/*StringSchema) carry full fidelity (properties/required/
	// items/enum) and MUST be converted here FIRST. They also satisfy
	// SchemaInterface, so the lossy interface branch below would otherwise win
	// and flatten them to {type,description} — gutting the schema Gemini sees.
	switch schema.(type) {
	case *types.ObjectSchema, *types.ArraySchema, *types.EnumSchema,
		*types.NumberSchema, *types.StringSchema:
		return g.schemaTypeToMap(schema)
	}

	// Fallback: a SchemaInterface implementation that is NOT one of the concrete
	// types above (only type+description available).
	if schemaIface, ok := schema.(types.SchemaInterface); ok {
		return g.schemaInterfaceToMap(schemaIface)
	}

	// Final fallback for any other concrete type handled by schemaTypeToMap.
	return g.schemaTypeToMap(schema)
}

// schemaInterfaceToMap converts a SchemaInterface to map
func (g *Gemini) schemaInterfaceToMap(schemaIface types.SchemaInterface) map[string]any {
	result := map[string]any{
		"type": schemaIface.GetType(),
	}
	if desc := schemaIface.GetDescription(); desc != "" {
		result["description"] = desc
	}
	return result
}

// schemaTypeToMap handles specific schema types
func (g *Gemini) schemaTypeToMap(schema types.Schema) map[string]any {
	result := map[string]any{}

	switch s := schema.(type) {
	case *types.ObjectSchema:
		g.objectSchemaToMap(s, result)
	case *types.ArraySchema:
		result["type"] = "array"
		result["items"] = g.schemaToMap(s.Items)
	case *types.EnumSchema:
		// Enum element type varies (string/number); prefer the declared type,
		// fall back to "string" when unset so Gemini always sees a type.
		if t := s.GetType(); t != "" {
			result["type"] = t
		} else {
			result["type"] = "string"
		}
		result["enum"] = s.Enum
	case *types.NumberSchema:
		g.numberSchemaToMap(s, result)
	case *types.StringSchema:
		g.stringSchemaToMap(s, result)
	}

	return result
}

// objectSchemaToMap populates result map from ObjectSchema
func (g *Gemini) objectSchemaToMap(s *types.ObjectSchema, result map[string]any) {
	result["type"] = "object"
	properties := make(map[string]any)
	for name, prop := range s.Properties {
		properties[name] = g.schemaToMap(prop)
	}
	result["properties"] = properties
	if len(s.Required) > 0 {
		result["required"] = s.Required
	}
}

// numberSchemaToMap populates result map from NumberSchema
func (g *Gemini) numberSchemaToMap(s *types.NumberSchema, result map[string]any) {
	result["type"] = "number"
	if s.Minimum != nil {
		result["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		result["maximum"] = *s.Maximum
	}
}

// stringSchemaToMap populates result map from StringSchema
func (g *Gemini) stringSchemaToMap(s *types.StringSchema, result map[string]any) {
	result["type"] = "string"
	if s.MinLength != nil {
		result["minLength"] = *s.MinLength
	}
	if s.MaxLength != nil {
		result["maxLength"] = *s.MaxLength
	}
	if s.Pattern != "" {
		result["pattern"] = s.Pattern
	}
}
