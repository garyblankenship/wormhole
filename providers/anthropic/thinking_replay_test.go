package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

// buildContent must prepend a signed thinking block as the FIRST content part
// when the AssistantMessage carries one — Anthropic hard-400s on multi-turn
// extended thinking otherwise.
func TestBuildContentPrependsThinkingBlock(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msg := &types.AssistantMessage{
		Content:  "answer",
		Thinking: &types.Thinking{Content: "let me think", Signature: "sig123"},
	}

	parts := p.buildContent(msg)

	if len(parts) == 0 {
		t.Fatal("expected at least one content part")
	}
	first := parts[0]
	if first["type"] != contentTypeThinking {
		t.Fatalf("first block type = %v, want %q", first["type"], contentTypeThinking)
	}
	if first["signature"] != "sig123" {
		t.Fatalf("first block signature = %v, want sig123", first["signature"])
	}
	if first["thinking"] != "let me think" {
		t.Fatalf("first block thinking = %v, want \"let me think\"", first["thinking"])
	}
}

// An AssistantMessage with nil Thinking must emit NO thinking block (the
// content array is byte-unchanged from before the fix).
func TestBuildContentNoThinkingBlockWhenNil(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msg := &types.AssistantMessage{Content: "answer"}

	parts := p.buildContent(msg)

	for _, part := range parts {
		if part["type"] == contentTypeThinking {
			t.Fatalf("unexpected thinking block emitted: %v", part)
		}
	}
}

// A signed thinking block with no signature must NOT be replayed (unsigned
// thinking is not echoable to Anthropic).
func TestBuildContentSkipsUnsignedThinking(t *testing.T) {
	t.Parallel()
	p := &Provider{}
	msg := &types.AssistantMessage{
		Content:  "answer",
		Thinking: &types.Thinking{Content: "unsigned", Signature: ""},
	}

	parts := p.buildContent(msg)

	for _, part := range parts {
		if part["type"] == contentTypeThinking {
			t.Fatalf("unsigned thinking must not be replayed: %v", part)
		}
	}
}

// OpenAI-signed thinking must be ignored by Anthropic replay, while Anthropic-signed
// thinking must be reintroduced as a thinking block.
func TestBuildContentSkipsForeignSignature(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	foreign := &types.AssistantMessage{
		Content: "answer",
		Thinking: &types.Thinking{
			Content:   "foreign thought",
			Signature: "openai-sig",
			Provider:  "openai",
		},
	}
	for _, part := range p.buildContent(foreign) {
		if part["type"] == contentTypeThinking {
			t.Fatalf("foreign-signed thinking must not be replayed: %v", part)
		}
		if _, ok := part["signature"]; ok {
			t.Fatalf("unexpected signature key in non-thinking part: %v", part)
		}
	}

	anthropic := &types.AssistantMessage{
		Content: "answer",
		Thinking: &types.Thinking{
			Content:   "my thought",
			Signature: "anthropic-sig",
			Provider:  "anthropic",
		},
	}
	parts := p.buildContent(anthropic)
	if len(parts) == 0 {
		t.Fatal("expected content parts from anthropic-thinking message")
	}
	first := parts[0]
	if first["type"] != contentTypeThinking {
		t.Fatalf("first block type = %v, want %q", first["type"], contentTypeThinking)
	}
	if first["signature"] != "anthropic-sig" {
		t.Fatalf("first block signature = %v, want %q", first["signature"], "anthropic-sig")
	}
	if first["thinking"] != "my thought" {
		t.Fatalf("first block thinking = %v, want %q", first["thinking"], "my thought")
	}
}
