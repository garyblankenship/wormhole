package wormhole_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStreamAndCollectManyDeltasWithPartialError(t *testing.T) {
	t.Parallel()
	failure := errors.New("stream interrupted")
	const count = 128
	chunks := make([]types.TextChunk, 0, count+1)
	for range count {
		chunks = append(chunks, types.TextChunk{
			Text:     "text",
			Refusal:  "no",
			Thinking: &types.Thinking{Content: "thought", Signature: "signed"},
		})
	}
	chunks = append(chunks, types.TextChunk{Error: failure})
	client := collectFixture(t, chunks)
	stream, result, err := client.Text().Model("fixture").Prompt("test").StreamAndCollect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	got, err := result()
	if !errors.Is(err, failure) {
		t.Fatalf("result error = %v; want %v", err, failure)
	}
	if got.Text != strings.Repeat("text", count) || got.Refusal != strings.Repeat("no", count) {
		t.Fatalf("collected text/refusal lengths = %d/%d", len(got.Text), len(got.Refusal))
	}
	if got.Thinking == nil || got.Thinking.Content != strings.Repeat("thought", count) || got.Thinking.Signature != "signed" {
		t.Fatalf("collected thinking = %+v", got.Thinking)
	}
	got.Thinking.Content = "mutated"
	again, err := result()
	if !errors.Is(err, failure) || again.Thinking.Content != strings.Repeat("thought", count) {
		t.Fatalf("repeated result = %+v, %v", again.Thinking, err)
	}
}
