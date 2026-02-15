package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// DocumentValidation provides helpers for validating documentation content.
type DocumentValidation struct {
	content string
	path    string
}

// NewDocumentValidation creates a new validator for the given path.
func NewDocumentValidation(path string) (*DocumentValidation, error) {
	content, err := os.ReadFile(path) //nolint:gosec // G304: test helper reads local doc files, path not user-controlled
	if err != nil {
		return nil, err
	}
	return &DocumentValidation{
		content: string(content),
		path:    path,
	}, nil
}

// Contains checks if the document contains the given substring.
func (dv *DocumentValidation) Contains(substr string) bool {
	return strings.Contains(dv.content, substr)
}

// ContainsPattern checks if the document contains a regex pattern.
func (dv *DocumentValidation) ContainsPattern(pattern string) bool {
	re := regexp.MustCompile(pattern)
	return re.MatchString(dv.content)
}

// ContainsAll checks if the document contains all substrings.
func (dv *DocumentValidation) ContainsAll(substrs []string) bool {
	for _, s := range substrs {
		if !strings.Contains(dv.content, s) {
			return false
		}
	}
	return true
}

// ContainsAny checks if the document contains any of the substrings.
func (dv *DocumentValidation) ContainsAny(substrs []string) bool {
	for _, s := range substrs {
		if strings.Contains(dv.content, s) {
			return true
		}
	}
	return false
}

// CountOccurrences counts how many times a substring appears.
func (dv *DocumentValidation) CountOccurrences(substr string) int {
	return strings.Count(dv.content, substr)
}

// ExtractAllCodeBlocks extracts all code blocks from markdown.
func (dv *DocumentValidation) ExtractAllCodeBlocks() []string {
	re := regexp.MustCompile("```(?:go)?\n([^`]+)```")
	matches := re.FindAllStringSubmatch(dv.content, -1)
	blocks := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			blocks = append(blocks, m[1])
		}
	}
	return blocks
}

// All exported option functions from pkg/wormhole/options.go
var expectedOptionFunctions = []string{
	"WithDefaultProvider",
	"WithOpenAI",
	"WithAnthropic",
	"WithGemini",
	"WithGroq",
	"WithMistral",
	"WithOllama",
	"WithLMStudio",
	"WithVLLM",
	"WithOllamaOpenAI",
	"WithOpenAICompatible",
	"WithCustomProvider",
	"WithProviderConfig",
	"WithMiddleware",
	"WithProviderMiddleware",
	"WithTimeout",
	"WithUnlimitedTimeout",
	"WithDebugLogging",
	"WithLogger",
	"WithModelValidation",
	"WithDiscoveryConfig",
	"WithOfflineMode",
	"WithDiscovery",
	"WithProviderFromEnv",
	"WithAllProvidersFromEnv",
	"WithIdempotencyKey",
}

// TestOptionsDocExists verifies the documentation file exists.
func TestOptionsDocExists(t *testing.T) {
	path := "options.md"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("documentation file does not exist: %s", path)
	}
}

// TestOptionsDocExplainsFunctionalOptionsPattern verifies the functional options pattern is explained.
func TestOptionsDocExplainsFunctionalOptionsPattern(t *testing.T) {
	tests := []struct {
		name     string
		required []string
	}{
		{
			name: "pattern name mentioned",
			required: []string{
				"functional options pattern",
			},
		},
		{
			name: "type definition explained",
			required: []string{
				"type Option",
				"func(*Config)",
			},
		},
		{
			name: "example option implementation shown",
			required: []string{
				"func With",
				"return func(c *Config)",
			},
		},
		{
			name: "benefits explained",
			required: []string{
				"compos",
				"extensib",
			},
		},
		{
			name: "basic usage example",
			required: []string{
				"wormhole.New(",
				"wormhole.With",
			},
		},
	}

	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !doc.ContainsAll(tt.required) {
				t.Errorf("documentation missing required content for %q", tt.name)
				t.Logf("required substrings: %v", tt.required)
			}
		})
	}
}

// TestOptionsDocListsAllOptionFunctions verifies all option functions are documented.
func TestOptionsDocListsAllOptionFunctions(t *testing.T) {
	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	// Check for option headers (### With... or #### With...)
	// These indicate documented option functions
	re := regexp.MustCompile(`#{3,4}\s+With\w+`)
	headerMatches := re.FindAllString(doc.content, -1)

	documentedOptions := make(map[string]bool)
	for _, match := range headerMatches {
		// Extract the function name
		name := strings.TrimSpace(strings.TrimPrefix(match, "####"))
		name = strings.TrimSpace(strings.TrimPrefix(name, "###"))
		documentedOptions[name] = true
	}

	// Also check for code references to options
	// Some options might be documented in code examples without headers
	for _, option := range expectedOptionFunctions {
		if doc.Contains(option) && !documentedOptions[option] {
			// If mentioned in code, count as documented
			re := regexp.MustCompile(`wormhole\.` + regexp.QuoteMeta(option) + `\(`)
			if re.MatchString(doc.content) {
				documentedOptions[option] = true
			}
		}
	}

	missingOptions := make([]string, 0)
	for _, expected := range expectedOptionFunctions {
		if !documentedOptions[expected] {
			missingOptions = append(missingOptions, expected)
		}
	}

	if len(missingOptions) > 0 {
		t.Errorf("documentation is missing %d option functions: %v", len(missingOptions), missingOptions)
	}

	// Report documented count
	t.Logf("documented %d/%d option functions", len(expectedOptionFunctions)-len(missingOptions), len(expectedOptionFunctions))
}

// TestOptionsDocHasCompositionExamples verifies option composition examples are provided.
func TestOptionsDocHasCompositionExamples(t *testing.T) {
	tests := []struct {
		name     string
		required []string
		reason   string
	}{
		{
			name: "basic composition example",
			required: []string{
				"wormhole.New(",
				",\n    ",
			},
			reason: "show multiple options combined with comma and newline",
		},
		{
			name: "conditional composition",
			required: []string{
				"[]",
				"Option",
				"append",
			},
			reason: "show building options dynamically with slices",
		},
		{
			name: "reusable option groups",
			required: []string{
				"[]",
				"Option",
				"var",
			},
			reason: "show predefined option groups for environments",
		},
		{
			name: "provider configuration composition",
			required: []string{
				"WithOpenAI",
				"WithAnthropic",
				"WithDefaultProvider",
			},
			reason: "show combining multiple provider options",
		},
		{
			name: "composition section exists",
			required: []string{
				"## Option Composition",
				"###",
			},
			reason: "dedicated section for composition patterns",
		},
	}

	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !doc.ContainsAll(tt.required) {
				t.Errorf("composition example missing: %s", tt.reason)
				t.Logf("required substrings: %v", tt.required)
			}
		})
	}

	// Verify code blocks contain composition examples
	codeBlocks := doc.ExtractAllCodeBlocks()
	if len(codeBlocks) == 0 {
		t.Error("no code blocks found in documentation")
	}

	compositionBlocksFound := 0
	for _, block := range codeBlocks {
		// Check for multiple WithXxx calls in a single block
		if strings.Count(block, "With") >= 2 {
			compositionBlocksFound++
		}
	}

	if compositionBlocksFound < 3 {
		t.Errorf("expected at least 3 code blocks showing composition, found %d", compositionBlocksFound)
	}
}

// TestOptionsDocHasProviderConfigurationExamples verifies provider options are well documented.
func TestOptionsDocHasProviderConfigurationExamples(t *testing.T) {
	tests := []struct {
		name    string
		options []string
		atLeast int
	}{
		{
			name:    "major providers documented",
			options: []string{"WithOpenAI", "WithAnthropic", "WithGemini"},
			atLeast: 3,
		},
		{
			name:    "OpenAI-compatible providers documented",
			options: []string{"WithGroq", "WithMistral", "WithOpenAICompatible"},
			atLeast: 2,
		},
		{
			name:    "local providers documented",
			options: []string{"WithOllama", "WithLMStudio", "WithVLLM"},
			atLeast: 2,
		},
	}

	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := 0
			for _, opt := range tt.options {
				if doc.Contains(opt) {
					found++
				}
			}
			if found < tt.atLeast {
				t.Errorf("expected at least %d of %v to be documented, found %d", tt.atLeast, tt.options, found)
			}
		})
	}
}

// TestOptionsDocHasBehaviorOptions verifies client behavior options are documented.
func TestOptionsDocHasBehaviorOptions(t *testing.T) {
	tests := []struct {
		name    string
		options []string
	}{
		{
			name:    "timeout options",
			options: []string{"WithTimeout", "WithUnlimitedTimeout"},
		},
		{
			name:    "logging options",
			options: []string{"WithDebugLogging", "WithLogger"},
		},
		{
			name:    "middleware options",
			options: []string{"WithMiddleware", "WithProviderMiddleware"},
		},
		{
			name:    "discovery options",
			options: []string{"WithDiscovery", "WithDiscoveryConfig", "WithOfflineMode"},
		},
		{
			name:    "validation options",
			options: []string{"WithModelValidation"},
		},
	}

	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing := []string{}
			for _, opt := range tt.options {
				if !doc.Contains(opt) {
					missing = append(missing, opt)
				}
			}
			if len(missing) > 0 {
				t.Errorf("%s: missing documentation for: %v", tt.name, missing)
			}
		})
	}
}

// TestOptionsDocHasEnvironmentVariableExamples verifies env var configuration is documented.
func TestOptionsDocHasEnvironmentVariableExamples(t *testing.T) {
	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	required := []string{
		"WithProviderFromEnv",
		"WithAllProvidersFromEnv",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"GEMINI_API_KEY",
	}

	for _, req := range required {
		if !doc.Contains(req) {
			t.Errorf("missing environment variable documentation for: %s", req)
		}
	}
}

// TestOptionsDocHasBestPractices verifies best practices section exists.
func TestOptionsDocHasBestPractices(t *testing.T) {
	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	// Look for best practices section
	if !doc.Contains("## Best Practices") && !doc.Contains("### Best Practices") {
		t.Error("missing Best Practices section")
	}

	// Check for DO/DON'T sections
	if !doc.Contains("### DO") && !doc.Contains("**DO**") {
		t.Error("missing DO recommendations in best practices")
	}
	if !doc.Contains("### DON'T") && !doc.Contains("**DON'T**") {
		t.Error("missing DON'T recommendations in best practices")
	}
}

// TestOptionsDocHasProductionAndTestExamples verifies production and test configuration examples.
func TestOptionsDocHasProductionAndTestExamples(t *testing.T) {
	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	// Look for production configuration examples
	productionIndicators := []string{
		"Production",
		"production",
		"WithAllProvidersFromEnv",
		"CircuitBreaker",
		"RateLimiter",
	}

	if !doc.ContainsAny(productionIndicators) {
		t.Error("missing production configuration example")
	}

	// Look for test configuration examples
	testIndicators := []string{
		"Test",
		"test",
		"WithDebugLogging",
		"WithDiscovery(false)",
	}

	if !doc.ContainsAny(testIndicators) {
		t.Error("missing test configuration example")
	}
}

// TestOptionsDocStructure verifies the document has proper structure.
func TestOptionsDocStructure(t *testing.T) {
	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	requiredSections := []struct {
		pattern string
		name    string
	}{
		{"# Options and Configuration", "title"},
		{"## Overview", "overview"},
		{"## Available Options", "available options section"},
		{"## Option Composition", "composition section"},
		{"## Best Practices", "best practices section"},
	}

	for _, section := range requiredSections {
		if !doc.Contains(section.pattern) {
			t.Errorf("missing section: %s (expected pattern: %s)", section.name, section.pattern)
		}
	}

	// Check that document starts with a level 1 heading
	if !strings.HasPrefix(doc.content, "# ") {
		t.Error("document should start with a level 1 heading (#)")
	}
}

// TestOptionsDocCodeBlocksAreGo verifies code examples are in Go.
func TestOptionsDocCodeBlocksAreGo(t *testing.T) {
	path := filepath.Join("docs", "concepts", "options.md")
	doc, err := NewDocumentValidation(path)
	if err != nil {
		t.Fatalf("failed to read documentation: %v", err)
	}

	// Extract code blocks
	re := regexp.MustCompile("```(\\w+)?")
	matches := re.FindAllString(doc.content, -1)

	goBlocks := 0
	for _, match := range matches {
		if match == "```go" || match == "```" {
			goBlocks++
		}
	}

	if goBlocks < 3 {
		t.Errorf("expected at least 3 code blocks, found %d", goBlocks)
	}
}
