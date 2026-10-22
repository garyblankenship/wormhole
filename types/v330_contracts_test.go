package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestV330ResponseCloneDetachesFidelityFields(t *testing.T) {
	t.Parallel()
	start := 2
	original := &TextResponse{
		ResponseMetadata: &ResponseMetadata{Headers: map[string][]string{"Request-Id": {"one"}}},
		Usage:            &Usage{CacheWriteTokens: 10, CacheWriteTokensByTTL: map[string]int{"5m": 10}},
		Sources:          []Source{{ID: "source-1", Raw: map[string]any{"items": []string{"first"}}}},
		Citations:        []Citation{{SourceID: "source-1", OutputRange: &TextRange{Start: 0, End: 3}, SourceLocation: &SourceLocation{StartPage: &start}, Raw: map[string]any{"type": "native"}}},
		Warnings:         []ResponseWarning{{Code: WarningMalformedCitation, Message: "invalid optional annotation"}},
	}
	cloned := CloneTextResponse(original)
	cloned.ResponseMetadata.Headers["Request-Id"][0] = "changed"
	cloned.Usage.CacheWriteTokensByTTL["5m"] = 20
	cloned.Sources[0].Raw["items"].([]string)[0] = "changed"
	cloned.Citations[0].OutputRange.End = 8
	*cloned.Citations[0].SourceLocation.StartPage = 8
	cloned.Citations[0].Raw["type"] = "changed"
	cloned.Warnings[0].Message = "changed"
	if original.ResponseMetadata.Headers["Request-Id"][0] != "one" || original.Usage.CacheWriteTokensByTTL["5m"] != 10 || original.Sources[0].Raw["items"].([]string)[0] != "first" || original.Citations[0].OutputRange.End != 3 || *original.Citations[0].SourceLocation.StartPage != 2 || original.Citations[0].Raw["type"] != "native" || original.Warnings[0].Message == "changed" {
		t.Fatal("clone shares mutable fidelity fields")
	}
}

func TestV330FileReferenceValidationAndCloning(t *testing.T) {
	t.Parallel()
	for _, sample := range []struct {
		name      string
		reference *FileReferenceMedia
		valid     bool
	}{
		{"id", &FileReferenceMedia{Provider: "profile", Kind: "document", ID: "file-1"}, true},
		{"uri", &FileReferenceMedia{Provider: "gemini", Kind: "video", URI: "gs://bucket/object", MimeType: "video/mp4"}, true},
		{"nil", nil, false},
		{"no provider", &FileReferenceMedia{Kind: "image", ID: "file-1"}, false},
		{"neither", &FileReferenceMedia{Provider: "profile", Kind: "image"}, false},
		{"both", &FileReferenceMedia{Provider: "profile", Kind: "image", ID: "file-1", URI: "uri"}, false},
		{"kind", &FileReferenceMedia{Provider: "profile", Kind: "unknown", ID: "file-1"}, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			t.Parallel()
			if err := sample.reference.Validate(); (err == nil) != sample.valid {
				t.Fatalf("Validate() = %v; valid %v", err, sample.valid)
			}
		})
	}
	original := &UserMessage{Media: []Media{&FileReferenceMedia{Provider: "profile", Kind: "image", ID: "file-1"}}}
	cloned := CloneMessage(original).(*UserMessage)
	cloned.Media[0].(*FileReferenceMedia).ID = "file-2"
	if original.Media[0].(*FileReferenceMedia).ID != "file-1" {
		t.Fatal("file reference clone shares state")
	}
}

func TestV330ToolTurnCannotDecodeAndRetainsSnapshot(t *testing.T) {
	t.Parallel()
	original := &StructuredResponse{ToolCalls: []ToolCall{{Name: "lookup", Arguments: map[string]any{"key": "original"}}}}
	var target struct{ Value string }
	err := original.ContentAs(&target)
	var required *ToolCallsRequiredError
	if !errors.As(err, &required) {
		t.Fatalf("ContentAs() error = %v", err)
	}
	required.Response.ToolCalls[0].Arguments["key"] = "changed"
	if original.ToolCalls[0].Arguments["key"] != "original" {
		t.Fatal("typed tool-turn error shares mutable response")
	}
}

func TestV330ErrorMetadataAndAdditiveSerialization(t *testing.T) {
	t.Parallel()
	original := ErrInvalidRequest.WithResponseMetadata(&ResponseMetadata{Headers: map[string][]string{"Request-Id": {"original"}}})
	wrapped := WrapError(ErrorCodeProvider, "wrapped", false, original).WithOperation("test")
	wrapped.ResponseMetadata.Headers["Request-Id"][0] = "changed"
	if original.ResponseMetadata.Headers["Request-Id"][0] != "original" {
		t.Fatal("wrapped metadata shares header state")
	}
	data, err := json.Marshal(TextResponse{})
	if err != nil {
		t.Fatal(err)
	}
	for _, optional := range []string{"response_metadata", "warnings", "sources", "citations"} {
		if strings.Contains(string(data), optional) {
			t.Fatalf("zero response includes optional %s", optional)
		}
	}
}
