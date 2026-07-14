package openai

import "strings"

func extractJSONFromMarkdown(content string) string {
	if !strings.Contains(content, "```") {
		return content
	}
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + len("```json")
		end := strings.LastIndex(content, "```")
		if start < end {
			return strings.TrimSpace(content[start:end])
		}
	}
	start := strings.Index(content, "```") + len("```")
	end := strings.LastIndex(content, "```")
	if start < end {
		cleaned := strings.TrimSpace(content[start:end])
		if (strings.HasPrefix(cleaned, "{") && strings.HasSuffix(cleaned, "}")) ||
			(strings.HasPrefix(cleaned, "[") && strings.HasSuffix(cleaned, "]")) {
			return cleaned
		}
	}
	return content
}
