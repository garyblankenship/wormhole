package gemini

import (
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// mergeSystemInstruction merges any RoleSystem messages from msgs into base.
// transformMessages skips RoleSystem (system text must travel in the top-level
// systemInstruction field), so without this merge a caller-provided system
// message in request.Messages would be silently dropped.
func mergeSystemInstruction(base string, msgs []types.Message) string {
	var parts []string
	if base != "" {
		parts = append(parts, base)
	}
	for _, m := range msgs {
		if m.GetRole() != types.RoleSystem {
			continue
		}
		switch c := m.GetContent().(type) {
		case string:
			if c != "" {
				parts = append(parts, c)
			}
		default:
			parts = append(parts, fmt.Sprintf("%v", c))
		}
	}
	return strings.Join(parts, "\n\n")
}
