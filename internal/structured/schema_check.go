package structured

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// These limits bound untrusted schemas and validation without limiting generation.
const maxSchemaDepth = 128
const maxSchemaWork = 100000

type schemaNode struct {
	edges []string // References and applicators that do not consume instance data.
}

func pointerPart(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func resolveReference(root map[string]any, ref string) (map[string]any, error) {
	if ref == "#" {
		return root, nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported schema reference %q", ref)
	}
	var target any = root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch value := target.(type) {
		case map[string]any:
			target = value[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(value) {
				return nil, fmt.Errorf("invalid schema reference %q", ref)
			}
			target = value[index]
		default:
			return nil, fmt.Errorf("invalid schema reference %q", ref)
		}
	}
	mapped, ok := target.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved schema reference %q", ref)
	}
	return mapped, nil
}

// checkDefinition rejects assertions the local validator cannot enforce before IO.
func checkDefinition(root map[string]any) error {
	nodes := map[string]*schemaNode{}
	work := 0
	var collect func(map[string]any, string, int) error
	collect = func(s map[string]any, path string, depth int) error {
		work += 1 + len(s)
		if depth > maxSchemaDepth || work > maxSchemaWork {
			return fmt.Errorf("schema complexity limit exceeded")
		}
		node := &schemaNode{}
		nodes[path] = node
		child := func(raw any, suffix string, sameValue bool) error {
			m, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s%s: schema must be an object", path, suffix)
			}
			target := path + suffix
			if sameValue {
				node.edges = append(node.edges, target)
			}
			return collect(m, target, depth+1)
		}
		for key, raw := range s {
			switch value := raw.(type) {
			case []any:
				work += len(value)
			case map[string]any:
				work += len(value)
			}
			if work > maxSchemaWork {
				return fmt.Errorf("schema complexity limit exceeded")
			}
			bad := func() error { return fmt.Errorf("%s: invalid schema keyword %s", path, key) }
			switch key {
			case "$schema", "$id", "$comment", "title", "description":
				if _, ok := raw.(string); !ok {
					return bad()
				}
			case "default", "examples", "deprecated", "readOnly", "writeOnly": // Annotations only.
			case "$ref":
				ref, ok := raw.(string)
				if !ok {
					return bad()
				}
				if _, err := resolveReference(root, ref); err != nil {
					return err
				}
				node.edges = append(node.edges, ref)
			case "type":
				valid := func(v any) bool {
					t, ok := v.(string)
					return ok && (t == "null" || t == "object" || t == "array" || t == "string" || t == "boolean" || t == "number" || t == "integer")
				}
				if list, ok := raw.([]any); ok {
					if len(list) == 0 {
						return bad()
					}
					for _, item := range list {
						if !valid(item) {
							return bad()
						}
					}
				} else if !valid(raw) {
					return bad()
				}
			case "const":
			case "enum":
				list, ok := raw.([]any)
				if !ok || len(list) == 0 {
					return bad()
				}
			case "$defs", "definitions", "properties":
				m, ok := raw.(map[string]any)
				if !ok {
					return bad()
				}
				for name, value := range m {
					if err := child(value, "/"+key+"/"+pointerPart(name), false); err != nil {
						return err
					}
				}
			case "allOf", "anyOf", "oneOf":
				list, ok := raw.([]any)
				if !ok || len(list) == 0 {
					return bad()
				}
				for i, value := range list {
					if err := child(value, fmt.Sprintf("/%s/%d", key, i), true); err != nil {
						return err
					}
				}
			case "not", "items":
				if err := child(raw, "/"+key, key == "not"); err != nil {
					return err
				}
			case "additionalProperties":
				if _, ok := raw.(bool); !ok {
					if err := child(raw, "/"+key, false); err != nil {
						return err
					}
				}
			case "required":
				list, ok := raw.([]any)
				if !ok {
					return bad()
				}
				for _, item := range list {
					if _, ok := item.(string); !ok {
						return bad()
					}
				}
			case "uniqueItems":
				if _, ok := raw.(bool); !ok {
					return bad()
				}
			case "pattern":
				pattern, ok := raw.(string)
				if !ok {
					return bad()
				}
				if _, err := regexp.Compile(pattern); err != nil {
					return bad()
				}
			case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minLength", "maxLength", "minItems", "maxItems":
				n, ok := number(raw)
				if !ok {
					return bad()
				}
				if key == "multipleOf" && n <= 0 {
					return bad()
				}
				if strings.HasPrefix(key, "minL") || strings.HasPrefix(key, "maxL") || key == "minItems" || key == "maxItems" {
					if n < 0 || float64(int64(n)) != n {
						return bad()
					}
				}
			default:
				return fmt.Errorf("%s: unsupported schema keyword %s", path, key)
			}
		}
		return nil
	}
	if err := collect(root, "#", 0); err != nil {
		return err
	}
	// Only schema locations are valid reference targets. A cycle through a property
	// or array item consumes data and therefore is not an edge in this graph.
	state := map[string]uint8{}
	var visit func(string, int) error
	visit = func(path string, depth int) error {
		work++
		if depth > maxSchemaDepth || work > maxSchemaWork {
			return fmt.Errorf("schema complexity limit exceeded")
		}
		if state[path] == 1 {
			return fmt.Errorf("non-progressing schema reference cycle at %s", path)
		}
		if state[path] == 2 {
			return nil
		}
		node := nodes[path]
		if node == nil {
			return fmt.Errorf("reference target is not a supported schema location: %s", path)
		}
		state[path] = 1
		for _, edge := range node.edges {
			if err := visit(edge, depth+1); err != nil {
				return err
			}
		}
		state[path] = 2
		return nil
	}
	for path := range nodes {
		if err := visit(path, 0); err != nil {
			return err
		}
	}
	return nil
}
