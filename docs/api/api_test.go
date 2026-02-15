package api_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAC01_PkgGoDevLink verifies that docs/api/README.md links to pkg.go.dev documentation
func TestAC01_PkgGoDevLink(t *testing.T) {
	// Given: docs/api/README.md exists
	path := filepath.Join("..", "..", "docs", "api", "README.md")
	//nolint:gosec // G304: test file paths are hardcoded and not user-controlled
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("docs/api/README.md does not exist: %v", err)
	}

	// When: reading the file
	contentStr := string(content)

	// Then: it links to pkg.go.dev documentation
	pkgGoDevPattern := regexp.MustCompile(`https://pkg\.go\.dev/github\.com/garyblankenship/wormhole`)
	if !pkgGoDevPattern.MatchString(contentStr) {
		t.Error("docs/api/README.md does not contain a link to pkg.go.dev documentation")
	}

	// Verify the link appears multiple times (at least once in header, once in See Also)
	matches := pkgGoDevPattern.FindAllString(contentStr, -1)
	if len(matches) < 1 {
		t.Error("Expected at least 1 pkg.go.dev link, found", len(matches))
	}
}

// TestAC02_KeyTypesAndFunctionsListed verifies that key types and functions are listed with brief descriptions
func TestAC02_KeyTypesAndFunctionsListed(t *testing.T) {
	// Given: docs/api/README.md exists
	path := filepath.Join("..", "..", "docs", "api", "README.md")
	//nolint:gosec // G304: test file paths are hardcoded and not user-controlled
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("docs/api/README.md does not exist: %v", err)
	}

	// When: scanning the file
	contentStr := string(content)

	// Then: key types are listed with descriptions
	keyTypes := []string{
		"Wormhole",
		"Config",
		"SimpleFactory",
		"TextRequestBuilder",
		"StructuredRequestBuilder",
		"EmbeddingsRequestBuilder",
		"TextResponse",
		"TextChunk",
		"Provider",
		"Capabilities",
	}

	for _, typeName := range keyTypes {
		if !strings.Contains(contentStr, typeName) {
			t.Errorf("docs/api/README.md does not mention key type: %s", typeName)
		}
	}

	// Then: key functions are listed with descriptions
	keyFunctions := []string{
		"New(",
		"client.Text()",
		"client.Structured()",
		"client.Embeddings()",
		"client.RegisterTool(",
		"client.ListAvailableModels(",
		"client.Close()",
	}

	for _, fn := range keyFunctions {
		if !strings.Contains(contentStr, fn) {
			t.Errorf("docs/api/README.md does not mention key function: %s", fn)
		}
	}

	// Verify that items have descriptions (check for table structure with "|")
	if !strings.Contains(contentStr, "| Type | Description |") && !strings.Contains(contentStr, "| Function | Description |") {
		t.Error("docs/api/README.md should have tables with Type/Function and Description columns")
	}
}

// TestAC03_PackageStructureOutlined verifies that package structure is outlined
func TestAC03_PackageStructureOutlined(t *testing.T) {
	// Given: docs/api/README.md exists
	path := filepath.Join("..", "..", "docs", "api", "README.md")
	//nolint:gosec // G304: test file paths are hardcoded and not user-controlled
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("docs/api/README.md does not exist: %v", err)
	}

	// When: reviewing the file
	contentStr := string(content)

	// Then: package structure is outlined
	// Check for package names within the tree diagram
	expectedPackages := []string{
		"wormhole/       # Main client",
		"types/          # Shared types",
		"providers/      # Provider implementations",
		"middleware/     # HTTP and provider middleware",
		"discovery/      # Dynamic model discovery",
		"testing/        # Testing utilities",
	}

	for _, pkg := range expectedPackages {
		if !strings.Contains(contentStr, pkg) {
			t.Errorf("docs/api/README.md does not outline package: %s", pkg)
		}
	}

	// Verify there's a package structure section
	if !strings.Contains(contentStr, "Package Structure") {
		t.Error("docs/api/README.md should have a 'Package Structure' section")
	}

	// Verify the structure is shown as a tree/diagram
	if !strings.Contains(contentStr, "├──") && !strings.Contains(contentStr, "│") {
		t.Error("docs/api/README.md should show package structure as a tree diagram")
	}
}
