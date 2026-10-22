package structured

import (
	"fmt"
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

func invalidKeyword(path, key string) error {
	return fmt.Errorf("%s: invalid schema keyword %s", path, key)
}

// definitionCollector walks one schema definition, records reference edges, and
// enforces the shared work and depth budgets across collection and cycle checks.
type definitionCollector struct {
	root  map[string]any
	nodes map[string]*schemaNode
	work  int
}

// checkDefinition rejects assertions the local validator cannot enforce before IO.
func checkDefinition(root map[string]any) error {
	collector := &definitionCollector{root: root, nodes: map[string]*schemaNode{}}
	if err := collector.collect(root, "#", 0); err != nil {
		return err
	}
	return collector.checkReferenceCycles()
}

func (collector *definitionCollector) collect(s map[string]any, path string, depth int) error {
	collector.work += 1 + len(s)
	if depth > maxSchemaDepth || collector.work > maxSchemaWork {
		return fmt.Errorf("schema complexity limit exceeded")
	}
	node := &schemaNode{}
	collector.nodes[path] = node
	for key, raw := range s {
		switch value := raw.(type) {
		case []any:
			collector.work += len(value)
		case map[string]any:
			collector.work += len(value)
		}
		if collector.work > maxSchemaWork {
			return fmt.Errorf("schema complexity limit exceeded")
		}
		if err := collector.checkKeyword(node, path, key, raw, depth); err != nil {
			return err
		}
	}
	return nil
}

// child validates a subschema location and records applicator edges that do not
// consume instance data.
func (collector *definitionCollector) child(node *schemaNode, target string, depth int, raw any, sameValue bool) error {
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: schema must be an object", target)
	}
	if sameValue {
		node.edges = append(node.edges, target)
	}
	return collector.collect(m, target, depth+1)
}

func (collector *definitionCollector) checkKeyword(node *schemaNode, path, key string, raw any, depth int) error {
	handled, err := checkMetaKeyword(path, key, raw)
	if handled || err != nil {
		return err
	}
	handled, err = collector.checkApplicatorKeyword(node, path, key, raw, depth)
	if handled || err != nil {
		return err
	}
	handled, err = checkNumericBoundKeyword(path, key, raw)
	if handled || err != nil {
		return err
	}
	return fmt.Errorf("%s: unsupported schema keyword %s", path, key)
}

// checkReferenceCycles rejects reference graphs that never reach instance data.
// Only schema locations are valid reference targets. A cycle through a property
// or array item consumes data and therefore is not an edge in this graph.
func (collector *definitionCollector) checkReferenceCycles() error {
	state := map[string]uint8{}
	var visit func(string, int) error
	visit = func(path string, depth int) error {
		collector.work++
		if depth > maxSchemaDepth || collector.work > maxSchemaWork {
			return fmt.Errorf("schema complexity limit exceeded")
		}
		if state[path] == 1 {
			return fmt.Errorf("non-progressing schema reference cycle at %s", path)
		}
		if state[path] == 2 {
			return nil
		}
		node := collector.nodes[path]
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
	for path := range collector.nodes {
		if err := visit(path, 0); err != nil {
			return err
		}
	}
	return nil
}
