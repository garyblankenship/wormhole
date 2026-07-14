package anthropic

import (
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// mergeSystemMessages merges any RoleSystem messages from msgs into base.
// Anthropic's transformMessages skips RoleSystem (system must travel in the
// top-level "system" field), so without this merge a caller-provided system
// message in request.Messages would be silently dropped.
func mergeSystemMessages(base string, msgs []types.Message) string {
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
		case []types.MessagePart:
			for _, p := range c {
				if p.Type == contentTypeText && p.Text != "" {
					parts = append(parts, p.Text)
				}
			}
		default:
			parts = append(parts, fmt.Sprintf("%v", c))
		}
	}
	return strings.Join(parts, "\n\n")
}
