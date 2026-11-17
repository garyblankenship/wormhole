package gemini

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestCitationRangesAcrossAppendOnlyText(t *testing.T) {
	t.Parallel()
	type step struct {
		text       string
		start, end int
		valid      bool
	}
	cases := []struct {
		name  string
		steps []step
	}{
		{"multibyte boundaries and earlier offsets", []step{
			{"é", 0, 2, true},
			{"é界", 2, 5, true},
			{"é界", 1, 5, false},
			{"é界", 0, 4, false},
			{"é界", 0, 2, true},
			{"é界x", 5, 6, true},
			{"é界x", 2, 2, true},
		}},
		{"invalid suffix preserves earlier valid offsets", []step{
			{"ok\xff", 0, 2, true},
			{"ok\xff", 0, 3, false},
			{"ok\xffmore", 3, 7, false},
			{"ok\xffmore", 0, 2, true},
			{"ok\xffmore", 0, 0, true},
		}},
		{"valid prefix before stray continuation", []step{
			{"a\x80", 0, 1, true},
			{"a\x80", 1, 1, true},
			{"a\x80", 0, 2, false},
			{"a\x80", 1, 2, false},
			{"a\x80", 1, 1, true},
		}},
		{"empty prefix before stray continuation", []step{
			{"\x80", 0, 0, true},
			{"\x80", 0, 1, false},
			{"\x80more", 0, 0, true},
			{"\x80more", 1, 5, false},
		}},
		{"incomplete suffix completed by append", []step{
			{"a\xe7", 0, 2, false},
			{"a\xe7", 0, 1, true},
			{"a界", 1, 4, true},
			{"a界", 1, 2, false},
			{"a界z", 4, 5, true},
		}},
		{"encoded replacement rune is valid", []step{
			{"�", 0, 3, true},
			{"�", 0, 1, false},
			{"�", -1, 3, false},
			{"�", 0, 4, false},
			{"�", 3, 2, false},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			refs := &citationSet{}
			for i, step := range tc.steps {
				before := len(refs.warnings)
				r := refs.segmentOutputRange(map[string]any{"startIndex": step.start, "endIndex": step.end}, step.text)
				if (r != nil) != step.valid {
					t.Fatalf("step %d: range=%#v, valid=%v", i, r, step.valid)
				}
				if r != nil && (r.Start != step.start || r.End != step.end) {
					t.Fatalf("step %d: range=%#v", i, r)
				}
				wantWarnings := before
				if !step.valid {
					wantWarnings++
				}
				if len(refs.warnings) != wantWarnings {
					t.Fatalf("step %d: warnings=%#v", i, refs.warnings)
				}
			}
		})
	}
}

func TestHandleStreamGroundingFramesAndTrailingUsage(t *testing.T) {
	t.Parallel()
	var wire strings.Builder
	for i, text := range []string{"é", "界"} {
		start, end := 0, 2
		if i == 1 {
			start, end = 2, 5
		}
		writeEfficiencyEvent(t, &wire, geminiTextResponse{Candidates: []candidate{{
			Content:           content{Parts: []part{{Text: "private", Thought: true}, {Text: text}}},
			GroundingMetadata: efficiencyGrounding(start, end),
		}}})
	}
	writeEfficiencyEvent(t, &wire, geminiTextResponse{Candidates: []candidate{{FinishReason: "STOP"}}})
	writeEfficiencyEvent(t, &wire, geminiTextResponse{UsageMetadata: &usageMetadata{PromptTokenCount: 7, CandidatesTokenCount: 2, TotalTokenCount: 9}})
	wire.WriteString("data: [DONE]\n\ndata: invalid ignored after done\n\n")
	var text, thinking strings.Builder
	var terminal types.TextChunk
	var usage *types.Usage
	terminalSeen, usageAfterTerminal := false, false
	for chunk := range (&Gemini{}).handleStream(context.Background(), io.NopCloser(strings.NewReader(wire.String()))) {
		if chunk.Error != nil {
			t.Fatal(chunk.Error)
		}
		text.WriteString(chunk.Text)
		if chunk.Thinking != nil {
			thinking.WriteString(chunk.Thinking.Content)
		}
		if chunk.FinishReason != nil {
			terminal, terminalSeen = chunk, true
		}
		if chunk.Usage != nil {
			usage, usageAfterTerminal = chunk.Usage, terminalSeen
		}
	}
	if text.String() != "é界" || thinking.String() != "privateprivate" {
		t.Fatalf("text=%q thinking=%q", text.String(), thinking.String())
	}
	if !terminalSeen || len(terminal.Sources) != 1 || len(terminal.Citations) != 2 || len(terminal.Warnings) != 0 {
		t.Fatalf("terminal=%#v", terminal)
	}
	for i, want := range []types.TextRange{{Start: 0, End: 2}, {Start: 2, End: 5}} {
		if r := terminal.Citations[i].OutputRange; r == nil || *r != want {
			t.Fatalf("citation %d: range=%#v, want=%#v", i, r, want)
		}
	}
	if annotations, ok := terminal.Metadata["provider_annotations"].([]any); !ok || len(annotations) != 2 {
		t.Fatalf("metadata=%#v", terminal.Metadata)
	}
	if !usageAfterTerminal || usage == nil || usage.PromptTokens != 7 || usage.TotalTokens != 9 {
		t.Fatalf("usage=%#v afterTerminal=%v", usage, usageAfterTerminal)
	}
}
