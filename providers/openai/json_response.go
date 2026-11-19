package openai

import (
	"encoding/json"
	"strings"
)

// extractJSONFromMarkdown strips markdown code fences only when the entire
// trimmed content is a single fenced JSON block (``` or ```json ... ```) and
// the extracted body is valid JSON. Ordinary prose containing markdown JSON
// snippets or multi-block responses is returned unchanged.
func extractJSONFromMarkdown(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") || !strings.HasSuffix(trimmed, "```") || len(trimmed) < 6 {
		return content
	}
	newline := strings.IndexByte(trimmed, '\n')
	if newline == -1 || newline >= len(trimmed)-3 {
		return content
	}
	header := strings.TrimSpace(trimmed[3:newline])
	if header != "" && !strings.EqualFold(header, "json") {
		return content
	}
	body := strings.TrimSpace(trimmed[newline+1 : len(trimmed)-3])
	if body == "" || strings.Contains(body, "```") || !json.Valid([]byte(body)) {
		return content
	}
	return body
}
