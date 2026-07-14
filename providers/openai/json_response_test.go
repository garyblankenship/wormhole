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
		{name: "json code block", input: "before\n```json\n{\"ok\":true}\n```\nafter", want: `{"ok":true}`},
		{name: "generic object", input: "```\n{\"ok\":true}\n```", want: `{"ok":true}`},
		{name: "generic array", input: "```\n[1,2,3]\n```", want: `[1,2,3]`},
		{name: "non JSON unchanged", input: "```\nnot json\n```", want: "```\nnot json\n```"},
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
