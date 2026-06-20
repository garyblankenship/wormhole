package anthropic

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
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
