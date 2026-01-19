package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestClientConcepts verifies the client.md documentation meets acceptance criteria
func TestClientConcepts(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "concepts", "client.md"))
	if err != nil {
		t.Fatalf("Failed to read client.md: %v", err)
	}

	doc := string(content)

	t.Run("AC1_UnifiedClientInterface", func(t *testing.T) {
		// Given client.md exists, when reading, then the unified Client interface is explained
		requiredSections := []string{
			"Unified Client Interface",
			"builder pattern",
			"functional options",
			"Request Builders",
		}

		for _, section := range requiredSections {
			if !strings.Contains(doc, section) {
				t.Errorf("Documentation missing required section: %s", section)
			}
		}

		// Verify builder pattern example is present
		if !strings.Contains(doc, "client.Text().") {
			t.Error("Documentation missing builder pattern example")
		}

		// Verify request builders table is present
		if !strings.Contains(doc, "| **`Text()`**") {
			t.Error("Documentation missing request builders table")
		}
	})

	t.Run("AC2_ClientLifecycle", func(t *testing.T) {
		// Given client.md exists, when scanning, then client lifecycle is documented
		requiredPhases := []string{
			"Creation",
			"Usage",
			"Cleanup",
		}

		for _, phase := range requiredPhases {
			if !strings.Contains(doc, phase) {
				t.Errorf("Documentation missing lifecycle phase: %s", phase)
			}
		}

		// Verify With* configuration options are documented
		configOptions := []string{
			"WithOpenAI",
			"WithAnthropic",
			"WithDefaultProvider",
			"WithDebugLogging",
		}

		for _, option := range configOptions {
			if !strings.Contains(doc, option) {
				t.Errorf("Documentation missing configuration option: %s", option)
			}
		}

		// Verify shutdown documentation
		if !strings.Contains(doc, "Graceful shutdown") {
			t.Error("Documentation missing graceful shutdown information")
		}

		if !strings.Contains(doc, "defer client.Close()") {
			t.Error("Documentation missing cleanup example")
		}
	})

	t.Run("AC3_ThreadSafety", func(t *testing.T) {
		// Given client.md exists, when reviewing, then thread-safety characteristics are documented
		requiredSections := []string{
			"Thread Safety",
			"sync.RWMutex",
			"atomic",
			"sync.Map",
		}

		for _, section := range requiredSections {
			if !strings.Contains(doc, section) {
				t.Errorf("Documentation missing thread-safety information: %s", section)
			}
		}

		// Verify thread-safe components table
		if !strings.Contains(doc, "Thread-Safe Components") {
			t.Error("Documentation missing thread-safe components table")
		}

		// Verify concurrent request pattern example
		if !strings.Contains(doc, "Concurrent Request Pattern") {
			t.Error("Documentation missing concurrent request pattern")
		}

		// Verify provider caching explanation
		if !strings.Contains(doc, "double-checked locking") {
			t.Error("Documentation missing double-checked locking explanation")
		}

		if !strings.Contains(doc, "atomic reference counting") {
			t.Error("Documentation missing atomic reference counting explanation")
		}
	})

	t.Run("AdditionalDocumentation", func(t *testing.T) {
		// Verify additional important sections are present
		importantSections := []string{
			"Provider Handles",
			"Model Discovery",
			"Tool Registration",
			"Error Handling",
			"Best Practices",
		}

		for _, section := range importantSections {
			if !strings.Contains(doc, section) {
				t.Errorf("Documentation missing important section: %s", section)
			}
		}
	})

	t.Run("CodeExamples", func(t *testing.T) {
		// Verify code examples are present and use correct syntax
		if !strings.Contains(doc, "```go") {
			t.Error("Documentation missing Go code examples")
		}

		// Count code blocks - should have multiple examples
		codeBlockCount := strings.Count(doc, "```go")
		if codeBlockCount < 5 {
			t.Errorf("Documentation should have at least 5 code examples, found %d", codeBlockCount)
		}
	})

	t.Run("ArchitectureUnderstanding", func(t *testing.T) {
		// Verify the doc explains the client architecture from wormhole.go
		keyConcepts := []string{
			"Wormhole client",
			"Provider delegation",
			"middleware chain",
			"Request builders",
			"provider cache",
			"graceful shutdown",
		}

		for _, concept := range keyConcepts {
			if !strings.Contains(strings.ToLower(doc), strings.ToLower(concept)) {
				t.Errorf("Documentation missing key concept: %s", concept)
			}
		}
	})
}
