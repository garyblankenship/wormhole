package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

const maxEventBytes = 1 << 20
const maxStreamBytes = 32 << 20

type response struct {
	ID                string `json:"id"`
	Model             string `json:"model"`
	Status            string `json:"status"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		Input        int `json:"input_tokens"`
		Output       int `json:"output_tokens"`
		Total        int `json:"total_tokens"`
		InputDetails struct {
			Cached int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputDetails struct {
			Reasoning int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

func (r response) text() string {
	var text strings.Builder
	for _, item := range r.Output {
		if item.Type != "message" && item.Type != "" {
			continue
		}
		for _, part := range item.Content {
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String()
}

func (r response) usage() *types.Usage {
	return &types.Usage{PromptTokens: r.Usage.Input, CompletionTokens: r.Usage.Output, TotalTokens: r.Usage.Total, CacheReadTokens: r.Usage.InputDetails.Cached, ReasoningTokens: r.Usage.OutputDetails.Reasoning}
}

// consume accepts only framed SSE events. A partial event, terminal failure,
// incomplete response, or EOF without completion is never treated as success.
func consume(ctx context.Context, body io.Reader, delta func(string) bool) (response, error) {
	scanner := bufio.NewScanner(io.LimitReader(body, maxStreamBytes+1))
	scanner.Buffer(make([]byte, 0, 4096), maxEventBytes)
	var eventType string
	var data bytes.Buffer
	var completed response
	seen := false
	bytesRead := 0
	process := func() error {
		if data.Len() == 0 {
			eventType = ""
			return nil
		}
		var event struct {
			Type     string   `json:"type"`
			Delta    string   `json:"delta"`
			Response response `json:"response"`
		}
		if err := json.Unmarshal(data.Bytes(), &event); err != nil {
			return errors.New("codex malformed SSE JSON")
		}
		if event.Type == "" || eventType != "" && eventType != event.Type {
			return errors.New("codex mismatched SSE event type")
		}
		data = bytes.Buffer{}
		eventType = ""
		switch event.Type {
		case "response.output_text.delta":
			if delta != nil && !delta(event.Delta) {
				return ctx.Err()
			}
		case "response.completed":
			if err := validateCompletedResponse(event.Response); err != nil {
				return err
			}
			completed = event.Response
			seen = true
		case "response.failed", "response.incomplete":
			return fmt.Errorf("codex %s", event.Type)
		}
		return nil
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return response{}, err
		}
		line := scanner.Text()
		bytesRead += len(line) + 1
		if bytesRead > maxStreamBytes {
			return response{}, errors.New("codex stream exceeds size limit")
		}
		if line == "" {
			if err := process(); err != nil {
				return response{}, err
			}
			if seen {
				return completed, nil
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if part == "[DONE]" {
				return response{}, errors.New("codex DONE before completion")
			}
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(part)
		}
	}
	if err := ctx.Err(); err != nil {
		return response{}, err
	}
	if err := scanner.Err(); err != nil {
		return response{}, fmt.Errorf("read codex SSE: %w", err)
	}
	return response{}, errors.New("codex stream ended without framed response.completed")
}

func validateCompletedResponse(completed response) error {
	if completed.Status != "completed" || completed.ID == "" || completed.Model == "" {
		return errors.New("codex invalid completed response")
	}
	for _, item := range completed.Output {
		if item.Type == "reasoning" {
			continue // Reasoning metadata is not user-visible text.
		}
		if item.Type != "message" && item.Type != "" {
			return errors.New("codex unsupported output item")
		}
		for _, part := range item.Content {
			if part.Type != "output_text" {
				return errors.New("codex unsupported output content")
			}
		}
	}
	return nil
}
