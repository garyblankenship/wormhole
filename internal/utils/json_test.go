package utils

import (
	"encoding/json"
	"testing"
)

// Test constants
const testJSONWithPatterns = `{"pattern": "\\\\d+", "text": "some text with \\\\s+ patterns"}`

func TestLenientUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]any
		wantErr bool
	}{
		{
			name:  "valid JSON",
			input: `{"pattern": "\\d+", "text": "normal text"}`,
			want:  map[string]any{"pattern": "\\d+", "text": "normal text"},
		},
		{
			name:  "Claude regex patterns (properly escaped)",
			input: `{"enhanced_prompt": "regex: \\\\s+ ... \\\\b(API|SQL|JSON|XML)\\\\b"}`,
			want:  map[string]any{"enhanced_prompt": "regex: \\\\s+ ... \\\\b(API|SQL|JSON|XML)\\\\b"},
		},
		{
			name:  "complex regex with date patterns",
			input: `{"pattern": "\\\\d{4}-\\\\d{2}-\\\\d{2}\\\\s+\\\\d{2}:\\\\d{2}:\\\\d{2}"}`,
			want:  map[string]any{"pattern": "\\\\d{4}-\\\\d{2}-\\\\d{2}\\\\s+\\\\d{2}:\\\\d{2}:\\\\d{2}"},
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json`,
			wantErr: true,
		},
		{
			name:    "malformed escape sequence",
			input:   `{"pattern": "\u"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]any
			err := LenientUnmarshal([]byte(tt.input), &result)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LenientUnmarshal() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("LenientUnmarshal() unexpected error: %v", err)
				return
			}

			if tt.want != nil {
				// Compare the parsed results
				for key, expectedValue := range tt.want {
					if actualValue, exists := result[key]; !exists {
						t.Errorf("LenientUnmarshal() missing key %q", key)
					} else if actualValue != expectedValue {
						t.Errorf("LenientUnmarshal() key %q: got %q, want %q", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

func TestUnmarshalAnthropicToolArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{
			name: "valid tool arguments",
			args: `{"function_name": "search", "query": "test"}`,
		},
		{
			name: "tool arguments with properly escaped regex",
			args: `{"pattern": "\\\\d+", "replacement": "\\\\$1"}`,
		},
		{
			name:    "empty arguments",
			args:    "",
			wantErr: true,
		},
		{
			name: "arguments with escaped quotes",
			args: `{"message": "He said \"Hello\""}`,
		},
		{
			name:    "malformed JSON arguments",
			args:    `{"incomplete": }`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]any
			err := UnmarshalAnthropicToolArgs(tt.args, &result)

			if tt.wantErr {
				if err == nil {
					t.Errorf("UnmarshalAnthropicToolArgs() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("UnmarshalAnthropicToolArgs() unexpected error: %v", err)
			}
		})
	}
}

// Test with the actual Claude response that was problematic
func TestUnmarshalAnthropicToolArgs_RealWorld(t *testing.T) {
	// This is the type of content that might come from Claude in tool arguments
	realWorldArgs := `{
		"enhanced_prompt": "You are a template generation system. Execute these steps sequentially:\\n\\n1. WORD COUNT: Tokenize user input on whitespace (regex: \\\\s+). Count non-empty tokens. Store as integer N.\\n\\n2. CLASSIFICATION: Assign tier based on N:\\n   - SIMPLE: N ∈ [0,10]\\n   - MEDIUM: N ∈ [11,30]\\n   - COMPLEX: N ≥ 31\\nOverride to COMPLEX if: code blocks > 50 chars OR technical specs detected (regex: \\\\b(API|SQL|JSON|XML)\\\\b)"
	}`

	var result map[string]any
	err := UnmarshalAnthropicToolArgs(realWorldArgs, &result)

	if err != nil {
		t.Errorf("UnmarshalAnthropicToolArgs() failed on real-world data: %v", err)
		return
	}

	// Verify we can access the enhanced_prompt field
	if prompt, ok := result["enhanced_prompt"].(string); !ok {
		t.Error("UnmarshalAnthropicToolArgs() failed to parse enhanced_prompt field")
	} else if len(prompt) == 0 {
		t.Error("UnmarshalAnthropicToolArgs() parsed empty enhanced_prompt")
	}
}

// Benchmark to ensure our functions don't significantly impact performance
func BenchmarkLenientUnmarshal(b *testing.B) {
	var result map[string]any

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LenientUnmarshal([]byte(testJSONWithPatterns), &result)
	}
}

func BenchmarkStandardUnmarshal(b *testing.B) {
	var result map[string]any

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = json.Unmarshal([]byte(testJSONWithPatterns), &result)
	}
}

func BenchmarkUnmarshalAnthropicToolArgs(b *testing.B) {
	toolArgs := `{"function_name": "search", "query": "\\\\d+ pattern", "context": "regex matching"}`
	var result map[string]any

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = UnmarshalAnthropicToolArgs(toolArgs, &result)
	}
}
