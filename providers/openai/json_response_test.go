package openai

import "testing"

func TestExtractJSONFromMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain JSON", input: `{"ok":true}`, want: `{"ok":true}`},
		{name: "json code block", input: "```json\n{\"ok\":true}\n```", want: `{"ok":true}`},
		{name: "prose with json snippet unchanged", input: "before\n```json\n{\"ok\":true}\n```\nafter", want: "before\n```json\n{\"ok\":true}\n```\nafter"},
		{name: "multi-block unchanged", input: "```json\n{\"a\":1}\n```\n\n```json\n{\"b\":2}\n```", want: "```json\n{\"a\":1}\n```\n\n```json\n{\"b\":2}\n```"},
		{name: "generic object", input: "```\n{\"ok\":true}\n```", want: `{"ok":true}`},
		{name: "generic array", input: "```\n[1,2,3]\n```", want: `[1,2,3]`},
		{name: "non JSON unchanged", input: "```\nnot json\n```", want: "```\nnot json\n```"},
		{name: "invalid JSON in json fence unchanged", input: "```json\n{not valid}\n```", want: "```json\n{not valid}\n```"},
		{name: "unterminated unchanged", input: "```json\n{\"ok\":true}", want: "```json\n{\"ok\":true}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := extractJSONFromMarkdown(test.input); got != test.want {
				t.Fatalf("extractJSONFromMarkdown() = %q, want %q", got, test.want)
			}
		})
	}
}
