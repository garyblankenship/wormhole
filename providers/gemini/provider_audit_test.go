package gemini

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

type auditBlockedBody struct {
	reading   chan struct{}
	closed    chan struct{}
	readOnce  sync.Once
	closeOnce sync.Once
	closes    atomic.Int32
}

func (b *auditBlockedBody) Read([]byte) (int, error) {
	b.readOnce.Do(func() { close(b.reading) })
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *auditBlockedBody) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

type auditEOFBody struct {
	io.Reader
	closes atomic.Int32
}

func (b *auditEOFBody) Close() error { b.closes.Add(1); return nil }

func TestAuditGeminiReaderCancellationAndEOF(t *testing.T) {
	t.Parallel()
	g := &Gemini{}
	body := &auditBlockedBody{reading: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	out := g.handleStream(ctx, body)
	<-body.reading
	cancel()
	for chunk := range out {
		t.Fatalf("cancellation emitted a frame: %#v", chunk)
	}
	if n := body.closes.Load(); n != 1 {
		t.Fatalf("cancellation closed body %d times", n)
	}
	eof := &auditEOFBody{Reader: strings.NewReader("")}
	for range g.handleStream(context.Background(), eof) {
	}
	if n := eof.closes.Load(); n != 1 {
		t.Fatalf("EOF closed body %d times", n)
	}
}

func TestAuditGeminiSchemaMapFidelity(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object", "required": []any{"items"},
		"properties": map[string]any{
			"items": map[string]any{"type": "array", "items": map[string]any{"type": []any{"string", "null"}, "enum": []any{"a", "b"}}},
		},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	g := &Gemini{}
	fromMap, fromBytes := g.schemaToMap(schema), g.schemaToMap(raw)
	if !reflect.DeepEqual(fromMap, fromBytes) {
		t.Fatalf("map=%#v bytes=%#v", fromMap, fromBytes)
	}
	items := fromMap["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	if items["type"] != "string" || items["nullable"] != true || !reflect.DeepEqual(items["enum"], []any{"a", "b"}) {
		t.Fatal(items)
	}
	items["enum"].([]any)[0] = "changed"
	items["type"] = "number"
	before, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(raw) {
		t.Fatalf("caller schema changed: %s", before)
	}
}

func TestAuditGeminiNestedToolDefinition(t *testing.T) {
	t.Parallel()
	g := &Gemini{requestBuilder: providers.NewRequestBuilder()}
	for _, flat := range []bool{false, true} {
		tool := types.Tool{Function: &types.ToolFunction{Name: "nested", Description: "nested description", Parameters: map[string]any{"type": "object"}}}
		wantName, wantDescription := "nested", "nested description"
		if flat {
			tool.Name, tool.Description, tool.InputSchema = "flat", "flat description", map[string]any{"type": "object", "description": "flat schema"}
			wantName, wantDescription = tool.Name, tool.Description
		}
		got := g.transformTools([]types.Tool{tool})[0]["functionDeclarations"].([]map[string]any)[0]
		if got["name"] != wantName || got["description"] != wantDescription {
			t.Fatal(got)
		}
		params := got["parameters"].(map[string]any)
		if params["type"] != "object" || (flat && params["description"] != "flat schema") {
			t.Fatal(params)
		}
	}
}
