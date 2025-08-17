package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/internal/utils"
)

// TestJSONRobustness tests that the Anthropic provider can handle
// tool call arguments that contain regex patterns and escaped strings
func TestJSONRobustness(t *testing.T) {
	tests := []struct {
		name          string
		toolArguments string
		expectError   bool
	}{
		{
			name: "Claude-style regex patterns",
			toolArguments: `{
				"enhanced_prompt": "regex: \\\\s+ ... \\\\b(API|SQL|JSON|XML)\\\\b",
				"pattern": "\\\\d{4}-\\\\d{2}-\\\\d{2}"
			}`,
			expectError: false,
		},
		{
			name: "Complex escaped strings",
			toolArguments: `{
				"code_example": "if (condition) {\\n  return \\\"success\\\";\\n}",
				"regex_pattern": "\\\\w+@\\\\w+\\\\.\\\\w+"
			}`,
			expectError: false,
		},
		{
			name: "Real Claude response patterns from issue",
			toolArguments: `{
				"enhanced_prompt": "You are a template generation system. Execute these steps sequentially:\\n\\n1. WORD COUNT: Tokenize user input on whitespace (regex: \\\\s+). Count non-empty tokens. Store as integer N.\\n\\n2. CLASSIFICATION: Assign tier based on N:\\n   - SIMPLE: N ∈ [0,10]\\n   - MEDIUM: N ∈ [11,30]\\n   - COMPLEX: N ≥ 31\\nOverride to COMPLEX if: code blocks > 50 chars OR technical specs detected (regex: \\\\b(API|SQL|JSON|XML)\\\\b)"
			}`,
			expectError: false,
		},
		{
			name: "Unicode symbols",
			toolArguments: `{
				"message": "Temperature should be ≤ 30°C",
				"pattern": "\\\\d+°[CF]"
			}`,
			expectError: false,
		},
		{
			name: "Code with backslashes and quotes",
			toolArguments: `{
				"pseudo_code": "if (input.matches(\\\"tell me a joke\\\")) {\\n  return jokeTemplate();\\n}",
				"regex": "\\\\d{4}-\\\\d{2}-\\\\d{2}\\\\s+\\\\d{2}:\\\\d{2}:\\\\d{2}"
			}`,
			expectError: false,
		},
		{
			name:          "Malformed JSON",
			toolArguments: `{"incomplete": }`,
			expectError:   true,
		},
		{
			name:          "Empty arguments",
			toolArguments: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]interface{}
			err := utils.UnmarshalAnthropicToolArgs(tt.toolArguments, &data)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for case %q, but got none", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error parsing case %q: %v", tt.name, err)
					t.Logf("Raw arguments: %s", tt.toolArguments)
				} else {
					// Verify the data was parsed correctly
					if data == nil {
						t.Error("Parsed data should not be nil for valid JSON")
					}

					// Log success for visibility
					t.Logf("Successfully parsed %d fields from %q", len(data), tt.name)
				}
			}
		})
	}
}

// TestJSONRobustness_Benchmarks ensures our JSON parsing doesn't have performance issues
func TestJSONRobustness_Benchmarks(t *testing.T) {
	// Test with a complex real-world example
	complexArgs := `{
		"enhanced_prompt": "You are a template generation system. Execute these steps sequentially:\\n\\n1. WORD COUNT: Tokenize user input on whitespace (regex: \\\\s+). Count non-empty tokens. Store as integer N.\\n\\n2. CLASSIFICATION: Assign tier based on N:\\n   - SIMPLE: N ∈ [0,10]\\n   - MEDIUM: N ∈ [11,30]\\n   - COMPLEX: N ≥ 31\\nOverride to COMPLEX if: code blocks > 50 chars OR technical specs detected (regex: \\\\b(API|SQL|JSON|XML)\\\\b)\\n\\n3. SPECIAL CASES:\\n   - Empty input → Return \\\"Please provide input text\\\" + set tier=SIMPLE\\n   - Match \\\"tell me a joke\\\" (case-insensitive) → Return exactly: \\\"Role: Stand-up comedian\\\\nObjective: Deliver humor\\\\nFormat: [Setup] ... [Punchline]\\\"",
		"key_improvements": ["Added regex patterns for precise tokenization", "Specified exact word limits", "Defined mathematical notation"],
		"complexity_score": 85
	}`

	// Run this multiple times to check for performance issues
	for i := 0; i < 100; i++ {
		var data map[string]interface{}
		err := utils.UnmarshalAnthropicToolArgs(complexArgs, &data)
		if err != nil {
			t.Fatalf("Iteration %d failed: %v", i, err)
		}

		// Verify key fields exist
		if _, ok := data["enhanced_prompt"]; !ok {
			t.Fatal("Missing enhanced_prompt field")
		}
		if _, ok := data["key_improvements"]; !ok {
			t.Fatal("Missing key_improvements field")
		}
	}

	t.Log("Successfully parsed complex Claude response 100 times")
}
