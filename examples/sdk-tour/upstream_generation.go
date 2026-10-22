package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (u *upstream) output() string {
	if strings.Contains(u.mode, "structured") || u.mode == "schema-helpers" {
		data, _ := json.Marshal(u.cfg.Structured)
		return string(data)
	}
	return u.cfg.Answer
}
func (u *upstream) toolRequest(body map[string]any) (string, map[string]any) {
	tools := array(body["tools"])
	if len(tools) == 0 {
		return "", nil
	}
	tool := object(tools[0])
	fn := object(tool["function"])
	if fn != nil {
		return str(fn["name"]), object(fn["parameters"])
	}
	return str(tool["name"]), object(tool["input_schema"])
}
func (u *upstream) chat(w http.ResponseWriter, r *http.Request, body map[string]any) {
	if stream, _ := body["stream"].(bool); stream {
		chunk := map[string]any{"id": "fixture-stream", "model": body["model"], "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": u.cfg.Answer}}}}
		u.sse(w, "", chunk)
		if u.mode == "stream-error" {
			return
		}
		if u.mode == "stream-idle" {
			<-r.Context().Done()
			return
		}
		u.sse(w, "", map[string]any{"id": "fixture-stream", "model": body["model"], "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}})
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		return
	}
	message := map[string]any{"role": "assistant", "content": u.output()}
	finish := "stop"
	name, schema := u.toolRequest(body)
	if name != "" {
		if str(schema["type"]) != "object" || object(schema["properties"]) == nil {
			u.reject(w, errors.New("tool schema/name missing on wire"))
			return
		}
		var toolResult string
		for _, msg := range array(body["messages"]) {
			if str(object(msg)["role"]) == "tool" {
				toolResult = str(object(msg)["content"])
			}
		}
		if toolResult != "" {
			var got any
			if err := json.Unmarshal([]byte(toolResult), &got); err != nil || !equalJSON(got, u.cfg.Tool.Result) {
				u.reject(w, errors.New("tool result attribution mismatch"))
				return
			}
		} else {
			args := any(u.cfg.Tool.Args)
			if u.mode == "tool-safety" {
				args = map[string]any{"a": u.cfg.Prompt, "b": u.cfg.Tool.Args.B}
			}
			if strings.Contains(u.mode, "structured") || u.mode == "schema-helpers" {
				args = u.cfg.Structured
			}
			data, _ := json.Marshal(args)
			message["content"] = nil
			message["tool_calls"] = []any{map[string]any{"id": "fixture-call", "type": "function", "function": map[string]any{"name": name, "arguments": string(data)}}}
			finish = "tool_calls"
		}
	}
	if u.mode == "incomplete-output" {
		message["content"] = ""
		finish = "length"
	}
	u.json(w, map[string]any{"id": "fixture-chat", "object": "chat.completion", "created": 1, "model": body["model"], "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}})
}
func (u *upstream) responses(w http.ResponseWriter, body map[string]any) {
	response := map[string]any{"id": "fixture-responses", "object": "response", "status": "completed", "model": body["model"], "created_at": 1, "output": []any{map[string]any{"id": "fixture-message", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": u.output()}}}}, "usage": map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5}}
	if u.mode == "responses-structured" {
		tools := array(body["tools"])
		if len(tools) != 1 || str(object(tools[0])["name"]) == "" || object(object(tools[0])["parameters"]) == nil {
			u.reject(w, errors.New("responses structured function/schema absent"))
			return
		}
		args, _ := json.Marshal(u.cfg.Structured)
		response["output"] = []any{map[string]any{"type": "function_call", "id": "fixture-function", "call_id": "fixture-call", "name": object(tools[0])["name"], "arguments": string(args), "status": "completed"}}
	}
	if stream, _ := body["stream"].(bool); stream {
		u.sse(w, "response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": "fixture-responses", "model": body["model"]}})
		u.sse(w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": u.cfg.Answer})
		u.sse(w, "response.completed", map[string]any{"type": "response.completed", "response": response})
		return
	}
	u.json(w, response)
}
func (u *upstream) anthropic(w http.ResponseWriter, body map[string]any) {
	response := map[string]any{"id": "fixture-anthropic", "type": "message", "role": "assistant", "model": body["model"], "content": []any{map[string]any{"type": "text", "text": u.output()}}, "stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 3, "output_tokens": 2}}
	if strings.Contains(u.mode, "structured") {
		name, schema := u.toolRequest(body)
		if name == "" || schema == nil {
			u.reject(w, errors.New("anthropic structured tool absent"))
			return
		}
		response["content"] = []any{map[string]any{"type": "tool_use", "id": "fixture-call", "name": name, "input": u.cfg.Structured}}
		response["stop_reason"] = "tool_use"
	}
	if stream, _ := body["stream"].(bool); stream {
		u.sse(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "fixture-anthropic", "model": body["model"], "usage": map[string]any{"input_tokens": 3}}})
		u.sse(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": u.cfg.Answer}})
		u.sse(w, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 2}})
		u.sse(w, "message_stop", map[string]any{"type": "message_stop"})
		return
	}
	u.json(w, response)
}
func (u *upstream) gemini(w http.ResponseWriter, r *http.Request, body map[string]any) {
	parts := []any{map[string]any{"text": u.output()}}
	if strings.Contains(u.mode, "image") {
		parts = []any{map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": u.cfg.Image}}}
	}
	response := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": parts}, "finishReason": "STOP"}}, "usageMetadata": map[string]any{"promptTokenCount": 3, "candidatesTokenCount": 2, "totalTokenCount": 5}}
	if strings.Contains(r.URL.Path, ":streamGenerateContent") {
		if r.URL.Query().Get("alt") != "sse" {
			u.reject(w, errors.New("gemini streaming must request SSE"))
			return
		}
		u.sse(w, "", response)
		return
	}
	if strings.Contains(u.mode, "structured") && object(body["generationConfig"])["responseSchema"] == nil {
		tools := array(body["tools"])
		if len(tools) != 1 {
			u.reject(w, errors.New("gemini structured result tool absent"))
			return
		}
		declarations := array(object(tools[0])["functionDeclarations"])
		if len(declarations) != 1 {
			u.reject(w, errors.New("gemini structured function declaration absent"))
			return
		}
		function := object(declarations[0])
		name := str(function["name"])
		schema := object(function["parameters"])
		properties := object(schema["properties"])
		choice := object(object(body["toolConfig"])["functionCallingConfig"])
		if name != "__wormhole_structured_output" || !strings.EqualFold(str(schema["type"]), "object") ||
			properties["name"] == nil || properties["age"] == nil ||
			!equalJSON(choice["allowedFunctionNames"], []string{name}) || str(choice["mode"]) != "ANY" {
			u.reject(w, errors.New("gemini structured schema or forced choice absent"))
			return
		}
		parts = []any{map[string]any{"functionCall": map[string]any{"name": name, "args": u.cfg.Structured}}}
		object(object(array(response["candidates"])[0])["content"])["parts"] = parts
	}
	u.json(w, response)
}
func (u *upstream) ollama(w http.ResponseWriter, body map[string]any) {
	response := map[string]any{"model": body["model"], "message": map[string]any{"role": "assistant", "content": u.output()}, "done": true, "done_reason": "stop", "prompt_eval_count": 3, "eval_count": 2}
	if stream, _ := body["stream"].(bool); stream {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	if strings.Contains(u.mode, "structured") {
		if body["format"] == nil {
			u.reject(w, errors.New("ollama structured format absent"))
			return
		}
		messages := array(body["messages"])
		if len(messages) == 0 || !strings.Contains(str(object(messages[len(messages)-1])["content"]), `"properties"`) {
			u.reject(w, errors.New("ollama schema instruction lost JSON object"))
			return
		}
	}
	u.json(w, response)
}
