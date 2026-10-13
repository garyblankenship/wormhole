package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}
func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

type proxyProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	log  synchronizedBuffer
}

func startProxy(ctx context.Context, j *journey, home string) (*proxyProcess, string, error) {
	binary := os.Getenv("WORMHOLE_PROXY_BIN")
	if binary == "" {
		return nil, "", errors.New("proxy scenario requires WORMHOLE_PROXY_BIN pointing to the built project CLI")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	addr := listener.Addr().String()
	if err = listener.Close(); err != nil {
		return nil, "", err
	}
	p := &proxyProcess{done: make(chan struct{})}
	p.cmd = exec.CommandContext(ctx, binary, "serve", "--addr", addr, "--default-provider", "openai") // #nosec G204 G702 -- Operator-selected trusted build artifact; direct exec without shell expansion.
	p.cmd.Env = []string{"HOME=" + home, "OPENAI_API_KEY=" + j.cfg.FakeKeys[0], "OPENAI_BASE_URL=" + j.server.URL + "/v1", "WORMHOLE_API_KEY=" + j.cfg.FakeKeys[1], "HTTP_PROXY=" + j.server.URL, "HTTPS_PROXY=" + j.server.URL, "NO_PROXY=127.0.0.1,localhost"}
	p.cmd.Stdout = &p.log
	p.cmd.Stderr = &p.log
	if err = p.cmd.Start(); err != nil {
		return nil, "", err
	}
	// Wait returns only when this owned process exits; done is its shutdown receipt.
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	return p, "http://" + addr, nil
}
func (p *proxyProcess) stop() error {
	select {
	case <-p.done:
		return p.err
	default:
	}
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	<-p.done
	return p.err
}
func proxyRequest(ctx context.Context, url, key string, payload any) (int, []byte, error) {
	var input io.Reader
	method := http.MethodGet
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		input = bytes.NewReader(data)
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, url, input)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, err
}
func proxyExample(ctx context.Context, j *journey) (err error) {
	home, err := os.MkdirTemp("", "wormhole-sdk-tour-proxy-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(home)) }()
	p, base, err := startProxy(ctx, j, home)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, p.stop()) }()
	ticker := time.NewTicker(time.Duration(j.cfg.Settings.PollMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		status, _, probeErr := proxyRequest(ctx, base+"/health", "", nil)
		if probeErr == nil && status == http.StatusOK {
			break
		}
		select {
		case <-p.done:
			return fmt.Errorf("proxy exited: %v: %s", p.err, p.log.String())
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	key := j.cfg.FakeKeys[1]
	payload := map[string]any{"model": "openai/" + j.cfg.Models["text"], "messages": []any{map[string]any{"role": "user", "content": j.cfg.Prompt}}}
	status, _, err := proxyRequest(ctx, base+"/v1/chat/completions", "", payload)
	if err != nil {
		return err
	}
	if status != http.StatusUnauthorized {
		return errors.New("proxy auth gate failed")
	}
	status, data, err := proxyRequest(ctx, base+"/v1/chat/completions", key, payload)
	if err != nil {
		return err
	}
	var body map[string]any
	if err = json.Unmarshal(data, &body); err != nil {
		return err
	}
	choices := array(body["choices"])
	if status != http.StatusOK || len(choices) != 1 || str(object(object(choices[0])["message"])["content"]) != j.cfg.Answer {
		return fmt.Errorf("proxy chat content mismatch: %s", data)
	}
	payload["stream"] = true
	status, data, err = proxyRequest(ctx, base+"/v1/chat/completions", key, payload)
	if err != nil {
		return err
	}
	if status != http.StatusOK || !strings.Contains(string(data), j.cfg.Answer) || !strings.Contains(string(data), "[DONE]") {
		return errors.New("proxy stream content/completion mismatch")
	}
	if err = proxyReasoningExample(ctx, j, base, key, payload); err != nil {
		return err
	}
	status, data, err = proxyRequest(ctx, base+"/v1/responses", key, map[string]any{"model": "openai/" + j.cfg.Models["text"], "input": j.cfg.Prompt})
	if err != nil {
		return err
	}
	if status != http.StatusOK || !strings.Contains(string(data), j.cfg.Answer) {
		return fmt.Errorf("proxy Responses content mismatch: %s", data)
	}
	status, data, err = proxyRequest(ctx, base+"/v1/embeddings", key, map[string]any{"model": "openai/" + j.cfg.Models["embedding"], "input": j.cfg.Inputs})
	if err != nil {
		return err
	}
	body = nil
	if err = json.Unmarshal(data, &body); err != nil {
		return err
	}
	vectors := array(body["data"])
	if status != http.StatusOK || len(vectors) != len(j.cfg.Inputs) {
		return errors.New("proxy embeddings count mismatch")
	}
	for i, v := range vectors {
		if !equalJSON(object(v)["embedding"], j.cfg.Vectors[i]) {
			return errors.New("proxy embedding source fidelity mismatch")
		}
	}
	status, data, err = proxyRequest(ctx, base+"/v1/rerank", key, map[string]any{"model": "openai/" + j.cfg.Models["text"], "query": j.cfg.Prompt, "documents": j.cfg.Documents})
	if err != nil {
		return err
	}
	body = nil
	if err = json.Unmarshal(data, &body); err != nil {
		return err
	}
	results := array(body["results"])
	if status != http.StatusOK || len(results) != 2 || object(results[0])["index"] != float64(1) || object(results[0])["relevance_score"] != j.cfg.Scores[1] {
		return fmt.Errorf("proxy rerank source/order mismatch: %s", data)
	}
	for {
		status, data, err = proxyRequest(ctx, base+"/v1/models", key, nil)
		if err != nil {
			return err
		}
		if status == http.StatusOK && strings.Contains(string(data), j.cfg.Models["text"]) && strings.Contains(string(data), j.cfg.Models["fallback"]) {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("proxy catalog never refreshed to fixture: %s", data)
		case <-ticker.C:
		}
	}
	for _, req := range j.fixture.all() {
		if str(req.Body["model"]) != "" && strings.HasPrefix(str(req.Body["model"]), "openai/") {
			return errors.New("proxy failed to strip model prefix")
		}
	}
	return nil
}

// The public HTTP request goes through the built CLI and the real OpenAI adapter.
func proxyReasoningExample(ctx context.Context, j *journey, base, key string, payload map[string]any) error {
	chatRequests := func() []wireRequest {
		var requests []wireRequest
		for _, req := range j.fixture.all() {
			if strings.HasSuffix(req.Path, "/chat/completions") {
				requests = append(requests, req)
			}
		}
		return requests
	}
	payload["reasoning_effort"] = "low"
	for _, stream := range []bool{false, true} {
		payload["stream"] = stream
		before := len(chatRequests())
		status, data, err := proxyRequest(ctx, base+"/v1/chat/completions", key, payload)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("proxy reasoning request failed: %s", data)
		}
		var content string
		if stream {
			done := false
			for _, line := range strings.Split(string(data), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				event := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				if event == "[DONE]" {
					done = true
					continue
				}
				var body map[string]any
				if err := json.Unmarshal([]byte(event), &body); err != nil {
					return err
				}
				for _, choice := range array(body["choices"]) {
					content += str(object(object(choice)["delta"])["content"])
				}
			}
			if !done {
				return errors.New("proxy reasoning stream omitted completion")
			}
		} else {
			var body map[string]any
			if err := json.Unmarshal(data, &body); err != nil {
				return err
			}
			choices := array(body["choices"])
			if len(choices) != 1 {
				return errors.New("proxy reasoning chat choices mismatch")
			}
			content = str(object(object(choices[0])["message"])["content"])
		}
		if content != j.cfg.Answer {
			return fmt.Errorf("proxy reasoning content mismatch: %q", content)
		}
		requests := chatRequests()
		if len(requests) != before+1 {
			return errors.New("proxy reasoning request count mismatch")
		}
		wire := requests[len(requests)-1].Body
		wireStream, _ := wire["stream"].(bool)
		if str(wire["reasoning_effort"]) != "low" || wireStream != stream {
			return errors.New("proxy reasoning/stream controls lost on wire")
		}
	}
	before := len(chatRequests())
	payload["reasoning_effort"] = "unsupported"
	status, data, err := proxyRequest(ctx, base+"/v1/chat/completions", key, payload)
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return err
	}
	errorBody := object(body["error"])
	return require(status == http.StatusBadRequest && str(errorBody["type"]) == "invalid_request_error" && strings.Contains(str(errorBody["message"]), "reasoning_effort") && len(chatRequests()) == before, "proxy invalid reasoning did not fail locally with HTTP 400")
}
