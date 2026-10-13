package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type wireRequest struct {
	Path          string
	Body          map[string]any
	Authorization string
}
type upstream struct {
	cfg      fixtureConfig
	mode     string
	mu       sync.Mutex
	requests []wireRequest
	issues   []error
}

func (u *upstream) record(r *http.Request, body map[string]any) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, wireRequest{r.URL.Path, body, r.Header.Get("Authorization")})
	return len(u.requests)
}
func (u *upstream) all() []wireRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]wireRequest(nil), u.requests...)
}
func (u *upstream) errors() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	return errors.Join(u.issues...)
}
func (u *upstream) note(err error) {
	if err == nil {
		return
	}
	u.mu.Lock()
	u.issues = append(u.issues, err)
	u.mu.Unlock()
}
func (u *upstream) reject(w http.ResponseWriter, err error) {
	u.note(err)
	http.Error(w, err.Error(), http.StatusBadRequest)
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func str(v any) string            { s, _ := v.(string); return s }

func (u *upstream) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect || r.URL.IsAbs() || !strings.HasPrefix(r.Host, "127.0.0.1:") {
		u.reject(w, errors.New("non-loopback destination blocked by fixture egress proxy"))
		return
	}
	body := map[string]any{}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if r.Method == http.MethodPost && !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			u.reject(w, err)
			return
		}
	}
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil { // #nosec G120 -- Entire body is capped by MaxBytesReader above.
			u.reject(w, err)
			return
		}
		defer func() { u.note(r.MultipartForm.RemoveAll()) }()
		for k, v := range r.MultipartForm.Value {
			if len(v) > 0 {
				body[k] = v[0]
			}
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			u.reject(w, err)
			return
		}
		defer func() { u.note(file.Close()) }()
		data, err := io.ReadAll(io.LimitReader(file, 1<<20))
		if err != nil {
			u.reject(w, err)
			return
		}
		want, err := decodeFixture(u.cfg.Audio)
		if err != nil || !equalJSON(data, want) {
			u.reject(w, errors.New("uploaded audio differs from fixture"))
			return
		}
	}
	count := u.record(r, body)
	if u.mode == "retry" && count == 1 {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
		u.json(w, map[string]any{"error": map[string]any{"message": "fixture transient failure", "type": "server_error"}})
		return
	}
	if u.mode == "key-rotation" && count == 1 {
		w.WriteHeader(http.StatusTooManyRequests)
		u.json(w, map[string]any{"error": map[string]any{"message": "fixture rate limit", "type": "rate_limit"}})
		return
	}
	if str(body["model"]) == u.cfg.Models["fail"] {
		w.WriteHeader(http.StatusServiceUnavailable)
		u.json(w, map[string]any{"error": map[string]any{"message": "fixture unavailable route", "type": "server_error"}})
		return
	}
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/models"):
		u.json(w, map[string]any{"object": "list", "data": []any{map[string]any{"id": u.cfg.Models["text"], "object": "model", "owned_by": "fixture"}, map[string]any{"id": u.cfg.Models["fallback"], "object": "model", "owned_by": "fixture"}}})
	case strings.HasSuffix(path, "/chat/completions"):
		u.chat(w, r, body)
	case strings.HasSuffix(path, "/responses"):
		u.responses(w, body)
	case strings.HasSuffix(path, "/messages"):
		u.anthropic(w, body)
	case strings.Contains(path, ":generateContent") || strings.Contains(path, ":streamGenerateContent"):
		u.gemini(w, r, body)
	case strings.HasSuffix(path, "/api/chat"):
		u.ollama(w, body)
	case strings.Contains(path, ":batchEmbedContents"):
		var embeddings []any
		for _, req := range array(body["requests"]) {
			parts := array(object(object(req)["content"])["parts"])
			if len(parts) == 0 {
				u.reject(w, errors.New("gemini embedding content absent"))
				return
			}
			vector, err := u.vector(str(object(parts[0])["text"]))
			if err != nil {
				u.reject(w, err)
				return
			}
			embeddings = append(embeddings, map[string]any{"values": vector})
		}
		u.json(w, map[string]any{"embeddings": embeddings})
	case strings.HasSuffix(path, "/api/embeddings"):
		vector, err := u.vector(str(body["prompt"]))
		if err != nil {
			u.reject(w, err)
			return
		}
		u.json(w, map[string]any{"embedding": vector})
	case strings.HasSuffix(path, "/embeddings"):
		u.embeddings(w, body)
	case strings.HasSuffix(path, "/rerank"):
		if !equalJSON(body["documents"], u.cfg.Documents) || str(body["query"]) != u.cfg.Prompt {
			u.reject(w, errors.New("rerank query/documents mismatch"))
			return
		}
		u.json(w, map[string]any{"id": "fixture-rerank", "model": body["model"], "results": []any{map[string]any{"index": 1, "relevance_score": u.cfg.Scores[1], "document": map[string]any{"text": u.cfg.Documents[1]}}, map[string]any{"index": 0, "relevance_score": u.cfg.Scores[0], "document": map[string]any{"text": u.cfg.Documents[0]}}}})
	case strings.HasSuffix(path, "/images/generations"):
		if str(body["prompt"]) != u.cfg.Prompt || str(body["response_format"]) != "b64_json" {
			u.reject(w, errors.New("image request mismatch"))
			return
		}
		u.json(w, map[string]any{"created": 1, "data": []any{map[string]any{"b64_json": u.cfg.Image}}})
	case strings.HasSuffix(path, "/audio/transcriptions"):
		u.json(w, map[string]any{"text": u.cfg.Answer})
	case strings.HasSuffix(path, "/audio/speech"):
		if str(body["input"]) != u.cfg.Prompt || str(body["voice"]) != u.cfg.Settings.Voice {
			u.reject(w, errors.New("speech request mismatch"))
			return
		}
		data, err := decodeFixture(u.cfg.Audio)
		if err != nil {
			u.reject(w, err)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(data)
	default:
		u.reject(w, fmt.Errorf("unrecognized fixture endpoint %s", path))
	}
}

func (u *upstream) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Canceled consumers may close the connection; the caller checks completion.
	_ = json.NewEncoder(w).Encode(v)
}
func (u *upstream) sse(w http.ResponseWriter, event string, v any) {
	w.Header().Set("Content-Type", "text/event-stream")
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return
		}
	}
	data, _ := json.Marshal(v)
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
func (u *upstream) vector(input string) ([]float64, error) {
	for i, want := range u.cfg.Inputs {
		if input == want {
			return u.cfg.Vectors[i], nil
		}
	}
	return nil, fmt.Errorf("unknown embedding input %q", input)
}
func (u *upstream) embeddings(w http.ResponseWriter, body map[string]any) {
	var result []any
	for i, input := range array(body["input"]) {
		vector, err := u.vector(str(input))
		if err != nil {
			u.reject(w, err)
			return
		}
		result = append(result, map[string]any{"object": "embedding", "index": i, "embedding": vector})
	}
	u.json(w, map[string]any{"object": "list", "model": body["model"], "data": result, "usage": map[string]any{"prompt_tokens": 3, "total_tokens": 3}})
}
