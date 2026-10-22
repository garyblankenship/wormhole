package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiStructuredFixture(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"none", "missing-schema", "missing-choice"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			var cfg fixtureConfig
			if err := json.Unmarshal(fixtureData, &cfg); err != nil {
				t.Fatal(err)
			}
			name := "__wormhole_structured_output"
			function := map[string]any{
				"name": name,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
						"age":  map[string]any{"type": "integer"},
					},
				},
			}
			body := map[string]any{
				"tools": []any{map[string]any{"functionDeclarations": []any{function}}},
				"toolConfig": map[string]any{"functionCallingConfig": map[string]any{
					"mode": "ANY", "allowedFunctionNames": []string{name},
				}},
			}
			if fault == "missing-schema" {
				delete(function, "parameters")
			}
			if fault == "missing-choice" {
				delete(body, "toolConfig")
			}
			fixture := &upstream{cfg: cfg, mode: "native-gemini-structured"}
			out := httptest.NewRecorder()
			fixture.gemini(out, httptest.NewRequest(http.MethodPost, "/gemini/models/fixture:generateContent", nil), body)
			if fault != "none" {
				if out.Code != http.StatusBadRequest || fixture.errors() == nil {
					t.Fatal("invalid structured request accepted")
				}
				return
			}
			if out.Code != http.StatusOK || fixture.errors() != nil {
				t.Fatalf("valid request rejected: %s", out.Body.String())
			}
			var response map[string]any
			if err := json.Unmarshal(out.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			candidates := array(response["candidates"])
			if len(candidates) != 1 {
				t.Fatal("missing candidate")
			}
			parts := array(object(object(candidates[0])["content"])["parts"])
			if len(parts) != 1 {
				t.Fatal("missing result part")
			}
			call := object(object(parts[0])["functionCall"])
			if str(call["name"]) != name || !equalJSON(call["args"], map[string]any{"name": cfg.Structured.Name, "age": cfg.Structured.Age}) {
				t.Fatalf("result source attribution lost: %#v", call)
			}
		})
	}
}
