package providers_test

import (
	"os"
	"strings"
	"testing"
)

// TestGeminiDocumentation validates the acceptance criteria for gemini.md
//
// Acceptance Criteria:
// 1. Given gemini.md exists, when reading, then it shows how to create a Gemini client
// 2. Given gemini.md exists, when scanning, then all supported Gemini models are listed
// 3. Given gemini.md exists, when reviewing, then Gemini-specific features are documented

const geminiDocPath = "providers/gemini.md"

func TestGeminiDocumentation(t *testing.T) {
	// Read the documentation file
	content, err := os.ReadFile(geminiDocPath)
	if err != nil {
		t.Fatalf("Failed to read gemini.md: %v", err)
	}

	doc := string(content)

	// Run all test suites
	t.Run("Criterion1_ClientCreation", func(t *testing.T) {
		testClientCreation(t, doc)
	})

	t.Run("Criterion2_SupportedModels", func(t *testing.T) {
		testSupportedModels(t, doc)
	})

	t.Run("Criterion3_SpecificFeatures", func(t *testing.T) {
		testSpecificFeatures(t, doc)
	})
}

// Criterion 1: Given gemini.md exists, when reading, then it shows how to create a Gemini client
func testClientCreation(t *testing.T, doc string) {
	tests := []struct {
		name          string
		required      []string
		description   string
		exampleNeeded bool
	}{
		{
			name: "Quick Start section exists",
			required: []string{
				"## Quick Start",
			},
			description:   "Documentation should have a Quick Start section",
			exampleNeeded: false,
		},
		{
			name: "Client creation with wormhole.New",
			required: []string{
				"wormhole.New",
				"WithDefaultProvider",
				`WithProviderConfig("gemini"`,
				"APIKey",
			},
			description:   "Should show creating client via wormhole.New with Gemini provider",
			exampleNeeded: true,
		},
		{
			name: "Client creation example has proper imports",
			required: []string{
				`"github.com/garyblankenship/wormhole/pkg/wormhole"`,
				`"github.com/garyblankenship/wormhole/pkg/types"`,
			},
			description:   "Example should include required imports",
			exampleNeeded: false,
		},
		{
			name: "Direct provider initialization section",
			required: []string{
				"### Direct Provider Initialization",
				"pkg/providers/gemini",
				"gemini.New",
			},
			description:   "Should document direct provider initialization",
			exampleNeeded: true,
		},
		{
			name: "Environment variable usage",
			required: []string{
				`os.Getenv("GEMINI_API_KEY")`,
			},
			description:   "Should demonstrate API key from environment variable",
			exampleNeeded: false,
		},
		{
			name: "Complete working example",
			required: []string{
				"package main",
				"func main()",
				"context.Background()",
				"client.Text()",
				".Model(",
				".Prompt(",
				".Generate(ctx)",
			},
			description:   "Should include complete runnable example",
			exampleNeeded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, req := range tt.required {
				if !strings.Contains(doc, req) {
					t.Errorf("%s: missing required content: %q", tt.description, req)
				}
			}
		})
	}
}

// Criterion 2: Given gemini.md exists, when scanning, then all supported Gemini models are listed
func testSupportedModels(t *testing.T, doc string) {
	tests := []struct {
		name        string
		modelSeries string
		models      []string
	}{
		{
			name:        "Gemini 3 Series",
			modelSeries: "### Gemini 3 Series",
			models: []string{
				"`gemini-3-pro-preview`",
				"`gemini-3-pro-image-preview`",
			},
		},
		{
			name:        "Gemini 2.5 Series",
			modelSeries: "### Gemini 2.5 Series",
			models: []string{
				"`gemini-2.5-flash`",
				"`gemini-2.5-flash-preview-09-2025`",
				"`gemini-2.5-flash-image`",
				"`gemini-2.5-flash-native-audio-preview-12-2025`",
				"`gemini-2.5-flash-native-audio-preview-09-2025`",
				"`gemini-2.5-flash-preview-tts`",
				"`gemini-2.5-flash-lite`",
				"`gemini-2.5-flash-lite-preview-09-2025`",
				"`gemini-2.5-pro`",
				"`gemini-2.5-pro-preview-tts`",
			},
		},
		{
			name:        "Gemini 2.0 Series",
			modelSeries: "### Gemini 2.0 Series",
			models: []string{
				"`gemini-2.0-flash`",
				"`gemini-2.0-flash-001`",
				"`gemini-2.0-flash-exp`",
				"`gemini-2.0-flash-preview-image-generation`",
				"`gemini-2.0-flash-lite`",
				"`gemini-2.0-flash-lite-001`",
			},
		},
		{
			name:        "Production recommendations",
			modelSeries: "Note",
			models: []string{
				"Prefer stable versions",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check that model series section exists
			if !strings.Contains(doc, tt.modelSeries) {
				t.Errorf("Missing model series section: %s", tt.modelSeries)
			}

			// Check that each model is documented
			for _, model := range tt.models {
				if !strings.Contains(doc, model) {
					t.Errorf("Model not documented: %s", model)
				}
			}
		})
	}

	// Verify supported models section structure
	t.Run("Supported Models section structure", func(t *testing.T) {
		requiredSections := []string{
			"## Supported Models",
			"| Model ID | Description |",
			"### Gemini 3 Series (Latest)",
			"### Gemini 2.5 Series",
			"### Gemini 2.0 Series",
		}

		for _, section := range requiredSections {
			if !strings.Contains(doc, section) {
				t.Errorf("Missing section: %s", section)
			}
		}
	})
}

// Criterion 3: Given gemini.md exists, when reviewing, then Gemini-specific features are documented
func testSpecificFeatures(t *testing.T, doc string) {
	tests := []struct {
		name          string
		required      []string
		description   string
		exampleNeeded bool
	}{
		{
			name: "Gemini-specific features section",
			required: []string{
				"## Gemini-Specific Features",
			},
			description:   "Should have dedicated section for Gemini-specific features",
			exampleNeeded: false,
		},
		{
			name: "API Key Authentication",
			required: []string{
				"### API Key Authentication",
				"URL query parameter",
				"?key=",
			},
			description:   "Should document API key authentication mechanism",
			exampleNeeded: true,
		},
		{
			name: "System Instructions",
			required: []string{
				"### System Instructions",
				"systemInstruction",
				".SystemPrompt(",
			},
			description:   "Should document system instructions feature",
			exampleNeeded: true,
		},
		{
			name: "Multimodal Input",
			required: []string{
				"### Multimodal Input",
				"image and audio input",
				"types.ImageMedia",
				".Media(",
			},
			description:   "Should document multimodal capabilities",
			exampleNeeded: true,
		},
		{
			name: "Tool Calling",
			required: []string{
				"### Tool Calling",
				"function calling",
				"types.Tool",
				"ToolFunction",
				".Tools(",
				".ToolChoice(",
				"response.ToolCalls",
			},
			description:   "Should document function/tool calling with example",
			exampleNeeded: true,
		},
		{
			name: "Structured Output",
			required: []string{
				"### Structured Output",
				".Structured()",
				".SchemaName(",
				".GenerateAs(",
			},
			description:   "Should document structured output feature",
			exampleNeeded: true,
		},
		{
			name: "Embeddings",
			required: []string{
				"### Embeddings",
				".Embeddings()",
				"text-embedding-004",
				"SEMANTIC_SIMILARITY",
				"ProviderOptions",
			},
			description:   "Should document embeddings with provider options",
			exampleNeeded: true,
		},
		{
			name: "Streaming",
			required: []string{
				"### Streaming",
				".Stream(",
				"chunk.Delta",
			},
			description:   "Should document streaming responses",
			exampleNeeded: true,
		},
		{
			name: "Generation Config",
			required: []string{
				"## Generation Config",
				"generationConfig",
				"maxOutputTokens",
				"topP",
				"stopSequences",
				"| Standard Parameter | Gemini Parameter |",
			},
			description:   "Should document parameter mapping table",
			exampleNeeded: true,
		},
		{
			name: "Error Handling",
			required: []string{
				"## Error Handling",
				"*types.WormholeError",
				"StatusCode",
				"401",
				"429",
				"400",
				"500",
			},
			description:   "Should document error handling and status codes",
			exampleNeeded: true,
		},
		{
			name: "Configuration Options",
			required: []string{
				"## Configuration Options",
				"### Base URL",
				"### Timeout",
			},
			description:   "Should document configuration options",
			exampleNeeded: false,
		},
		{
			name: "Unsupported Features",
			required: []string{
				"## Unsupported Features",
				"NotImplementedError",
			},
			description:   "Should document unsupported features",
			exampleNeeded: false,
		},
		{
			name: "Capabilities List",
			required: []string{
				"## Capabilities",
				"CapabilityText",
				"CapabilityChat",
				"CapabilityStructured",
				"CapabilityStream",
				"CapabilityFunctions",
				"CapabilityEmbeddings",
			},
			description:   "Should list all supported capabilities",
			exampleNeeded: false,
		},
		{
			name: "Reference Links",
			required: []string{
				"## Reference",
				"https://ai.google.dev/gemini-api/docs",
				"https://ai.google.dev/gemini-api/docs/models",
			},
			description:   "Should provide reference links to official documentation",
			exampleNeeded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, req := range tt.required {
				if !strings.Contains(doc, req) {
					t.Errorf("%s: missing required content: %q", tt.description, req)
				}
			}
		})
	}

	// Verify code examples are well-formed
	t.Run("Code examples quality", func(t *testing.T) {
		// Check that examples use proper Go syntax
		requiredPatterns := []string{
			"```go",        // All examples should be in Go code blocks
			"package main", // Complete examples should have package declaration
			"import (",     // Examples should show imports
		}

		for _, pattern := range requiredPatterns {
			if !strings.Contains(doc, pattern) {
				t.Errorf("Code example quality: missing pattern: %s", pattern)
			}
		}

		// Count code blocks to ensure sufficient examples
		codeBlockCount := strings.Count(doc, "```go")
		if codeBlockCount < 10 {
			t.Errorf("Expected at least 10 code examples, found %d", codeBlockCount)
		}
	})
}
