package middleware

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// cloneValue returns an independent copy of src so that callers cannot
// mutate the cached value. map[string]any gets a recursive copy to preserve
// numeric types (JSON round-trip would convert int→float64). All other types
// round-trip through JSON, which is correct for typed structs.
func cloneValue(src any) (any, error) {
	if src == nil {
		return nil, nil
	}
	if m, ok := src.(map[string]any); ok {
		return deepCopyMap(m), nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("cache clone marshal: %w", err)
	}
	srcType := reflect.TypeOf(src)
	var dst any
	if srcType.Kind() == reflect.Pointer {
		dst = reflect.New(srcType.Elem()).Interface()
	} else {
		dst = reflect.New(srcType).Interface()
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return nil, fmt.Errorf("cache clone unmarshal: %w", err)
	}
	if srcType.Kind() != reflect.Pointer {
		dst = reflect.ValueOf(dst).Elem().Interface()
	}
	return dst, nil
}

func deepCopyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case map[string]string:
		cpy := make(map[string]string, len(val))
		for k, item := range val {
			cpy[k] = item
		}
		return cpy
	case []any:
		cpy := make([]any, len(val))
		for i, item := range val {
			cpy[i] = deepCopyValue(item)
		}
		return cpy
	case []string:
		return append([]string(nil), val...)
	case json.RawMessage:
		return append(json.RawMessage(nil), val...)
	case []byte:
		return append([]byte(nil), val...)
	default:
		// Basic types (int, string, bool, float64, etc.) are already safe to share.
		return v
	}
}
