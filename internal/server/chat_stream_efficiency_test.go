package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestChatStreamSelectedReferenceCollections(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		include  []string
		selected [3]bool
	}{
		{name: "ordinary"},
		{name: "unrelated", include: []string{"usage_details", "response_metadata"}},
		{name: "sources", include: []string{"sources"}, selected: [3]bool{true, false, false}},
		{name: "citations", include: []string{"citations"}, selected: [3]bool{false, true, false}},
		{name: "warnings", include: []string{"warnings"}, selected: [3]bool{false, false, true}},
		{name: "all", include: []string{"sources", "citations", "warnings"}, selected: [3]bool{true, true, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var extension *WormholeRequest
			if tc.include != nil {
				extension = &WormholeRequest{Include: tc.include}
			}
			state := newChatStreamState("fixture", extension)
			chunk := types.TextChunk{
				Sources:   []types.Source{{ID: "incremental"}},
				Citations: []types.Citation{{Raw: map[string]any{"type": "url_citation", "url_citation": map[string]any{"url": "https://example.com"}}}},
				Warnings:  []types.ResponseWarning{{}},
			}
			check := func(wantSelected int) {
				t.Helper()
				for i, got := range []int{len(state.sources), len(state.citations), len(state.warnings)} {
					want := 0
					if tc.selected[i] {
						want = wantSelected
					}
					if got != want {
						t.Fatalf("collection %d length = %d, want %d", i, got, want)
					}
				}
			}
			state.absorb(chunk)
			state.absorb(chunk)
			check(2)
			if got := state.chunkResponse(chunk).Choices[0].Delta.Annotations; !reflect.DeepEqual(got, []map[string]any{chunk.Citations[0].Raw}) {
				t.Fatalf("inline annotations changed: %#v", got)
			}
			finish := types.FinishReasonStop
			state.absorb(types.TextChunk{FinishReason: &finish})
			check(2) // Nil terminal collections preserve prior snapshots.
			chunk.FinishReason = &finish
			state.absorb(chunk)
			check(1) // Non-nil terminal collections replace rather than append.
			state.absorb(types.TextChunk{FinishReason: &finish, Sources: []types.Source{}, Citations: []types.Citation{}, Warnings: []types.ResponseWarning{}})
			check(0)
			for i, isNil := range []bool{state.sources == nil, state.citations == nil, state.warnings == nil} {
				if isNil == tc.selected[i] {
					t.Fatalf("collection %d nil = %v, selected = %v", i, isNil, tc.selected[i])
				}
			}
		})
	}
}

type chatFrameRecorder struct {
	*httptest.ResponseRecorder
	frames  [][]byte
	flushes int
	short   bool
}

func (w *chatFrameRecorder) Write(data []byte) (int, error) {
	w.frames = append(w.frames, bytes.Clone(data))
	if w.short {
		return len(data) - 1, nil
	}
	return w.ResponseRecorder.Write(data)
}

func (w *chatFrameRecorder) Flush() { w.flushes++; w.ResponseRecorder.Flush() }

func TestChatStreamFrameWireAndFlush(t *testing.T) {
	t.Parallel()
	p := &proxy{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	response := ChatCompletionResponse{ID: "fixture", Object: "chat.completion.chunk", Created: 123, Model: "fixture", Choices: []ChatChoice{{Index: 0, Delta: &ChatDelta{Content: "line\n雪"}}}}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	wantResponseFrame := append(append([]byte("data: "), encoded...), '\n', '\n')
	for _, short := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "short"}[short], func(t *testing.T) {
			t.Parallel()
			writer := &chatFrameRecorder{ResponseRecorder: httptest.NewRecorder(), short: short}
			state := newChatStreamState("fixture", nil)
			if got := state.writeResponse(p, writer, writer, response); got == short {
				t.Fatalf("writeResponse = %v, short = %v", got, short)
			}
			if len(writer.frames) != 1 || !bytes.Equal(writer.frames[0], wantResponseFrame) {
				t.Fatalf("frames = %q, want one %q", writer.frames, wantResponseFrame)
			}
			wantFlushes := 1
			if short {
				wantFlushes = 0
			}
			if writer.flushes != wantFlushes {
				t.Fatalf("flushes = %d, want %d", writer.flushes, wantFlushes)
			}
		})
	}
	writer := &chatFrameRecorder{ResponseRecorder: httptest.NewRecorder()}
	state := newChatStreamState("fixture", nil)
	state.committed = true
	state.finish(p, writer, writer)
	if len(writer.frames) != 1 || string(writer.frames[0]) != "data: [DONE]\n\n" || writer.flushes != 1 {
		t.Fatalf("DONE frames = %q, flushes = %d", writer.frames, writer.flushes)
	}
	writer = &chatFrameRecorder{ResponseRecorder: httptest.NewRecorder()}
	writeStreamError(writer, writer, errors.New("private upstream detail"))
	_, kind, message := upstreamErrorStatus(errors.New("private upstream detail"))
	encodedError, err := json.Marshal(ErrorResponse{Error: ErrorDetail{Message: message, Type: kind, Code: "upstream_error"}})
	if err != nil {
		t.Fatal(err)
	}
	wantErrorFrame := append(append([]byte("data: "), encodedError...), '\n', '\n')
	if len(writer.frames) != 1 || !bytes.Equal(writer.frames[0], wantErrorFrame) || writer.flushes != 1 {
		t.Fatalf("error frames = %q, flushes = %d", writer.frames, writer.flushes)
	}
}

func TestChatStreamTerminalSnapshotWireAndFlush(t *testing.T) {
	t.Parallel()
	writer := &chatFrameRecorder{ResponseRecorder: httptest.NewRecorder()}
	state := newChatStreamState("fixture", &WormholeRequest{Include: []string{"sources"}})
	state.id = "fixture-id"
	state.committed = true
	state.absorb(types.TextChunk{Sources: []types.Source{{ID: "source"}}})
	state.finish(&proxy{}, writer, writer)
	if len(writer.frames) != 2 || writer.flushes != 1 {
		t.Fatalf("terminal writes = %d, flushes = %d; want 2, 1", len(writer.frames), writer.flushes)
	}
	if string(writer.frames[1]) != "data: [DONE]\n\n" {
		t.Fatalf("terminator = %q", writer.frames[1])
	}
	var terminal ChatCompletionResponse
	if err := json.Unmarshal(bytes.TrimSuffix(bytes.TrimPrefix(writer.frames[0], []byte("data: ")), []byte("\n\n")), &terminal); err != nil {
		t.Fatal(err)
	}
	finish := "stop"
	want := ChatCompletionResponse{ID: "fixture-id", Object: "chat.completion.chunk", Created: terminal.Created, Model: "fixture", Choices: []ChatChoice{{Index: 0, Delta: &ChatDelta{}, FinishReason: &finish}}, Wormhole: map[string]any{"sources": []types.Source{{ID: "source"}}}}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	frame := append(append([]byte("data: "), encoded...), '\n', '\n')
	if terminal.Created == 0 || !bytes.Equal(writer.frames[0], frame) {
		t.Fatalf("terminal = %q, want %q", writer.frames[0], frame)
	}
}
