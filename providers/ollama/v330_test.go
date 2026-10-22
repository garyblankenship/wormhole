package ollama

import (
	"context"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330OllamaRejectsFileReferences(t *testing.T) {
	t.Parallel()
	p, err := New(types.ProviderConfig{BaseURL: "http://unused.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	message := types.NewUserMessage("document")
	message.Media = []types.Media{&types.FileReferenceMedia{Provider: "ollama", Kind: "document", ID: "file-1"}}
	request := types.TextRequest{Messages: []types.Message{message}}
	if _, err := p.Text(context.Background(), request); err == nil {
		t.Fatal("Text accepted file reference")
	}
	if _, err := p.Stream(context.Background(), request); err == nil {
		t.Fatal("Stream accepted file reference")
	}
}
