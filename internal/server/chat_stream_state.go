package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// chatStreamState accumulates one proxied chat stream's identity, extension
// snapshots, and emission state across chunks.
type chatStreamState struct {
	id             string
	model          string
	extension      *WormholeRequest
	toolState      *streamToolState
	committed      bool
	roleSent       bool
	terminalSent   bool
	terminalReason string
	metadata       *types.ResponseMetadata
	usage          *types.Usage
	sources        []types.Source
	citations      []types.Citation
	warnings       []types.ResponseWarning
}

func newChatStreamState(model string, extension *WormholeRequest) *chatStreamState {
	return &chatStreamState{
		id:             fmt.Sprintf("wh-%d", time.Now().UnixNano()),
		model:          model,
		extension:      extension,
		toolState:      newStreamToolState(),
		terminalReason: "stop",
	}
}

// absorb merges one upstream chunk into the extension snapshots. Terminal
// frames replace non-nil collections; incremental frames append.
func (s *chatStreamState) absorb(chunk types.TextChunk) {
	if chunk.ResponseMetadata != nil {
		s.metadata = chunk.ResponseMetadata
	}
	if chunk.Usage != nil {
		s.usage = chunk.Usage
	}
	if chunk.FinishReason != nil {
		if chunk.Sources != nil {
			s.sources = chunk.Sources
		}
		if chunk.Citations != nil {
			s.citations = chunk.Citations
		}
		if chunk.Warnings != nil {
			s.warnings = chunk.Warnings
		}
		return
	}
	s.sources = append(s.sources, chunk.Sources...)
	s.citations = append(s.citations, chunk.Citations...)
	s.warnings = append(s.warnings, chunk.Warnings...)
}

// commit emits the SSE response header exactly once per stream.
func (s *chatStreamState) commit(w http.ResponseWriter, flusher http.Flusher) {
	forwardResponseMetadata(w, s.metadata)
	s.writeStreamHeaders(w)
	flusher.Flush()
	s.committed = true
}

func (s *chatStreamState) writeStreamHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

// frame renders and writes one chat chunk, honoring extension suppression of
// the inline finish reason. It reports whether the frame reached the client.
func (s *chatStreamState) frame(p *proxy, w http.ResponseWriter, flusher http.Flusher, chunk types.TextChunk) bool {
	resp := s.chunkResponse(chunk)
	if chunk.FinishReason != nil {
		fr := string(normalizedFinishReason(*chunk.FinishReason))
		s.terminalReason = fr
		if s.extension == nil || len(s.extension.Include) == 0 {
			delta := resp.Choices[0].Delta
			if delta.Role != "" || delta.Content != "" || delta.Refusal != "" || len(delta.Annotations) > 0 || len(delta.ToolCalls) > 0 {
				if !s.writeResponse(p, w, flusher, resp) {
					return false
				}
				resp.Choices[0].Delta = &ChatDelta{}
			}
			resp.Choices[0].FinishReason = &fr
			s.terminalSent = true
		}
	}
	if chunk.Usage != nil {
		resp.Usage = toChatUsage(chunk.Usage)
	}
	return s.writeResponse(p, w, flusher, resp)
}

func (s *chatStreamState) writeResponse(p *proxy, w http.ResponseWriter, flusher http.Flusher, resp ChatCompletionResponse) bool {
	data, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		p.logger.Error("failed to marshal chunk", "error", types.SafeErrorValue(marshalErr))
		return false
	}
	if err := writeChatFrame(w, fmt.Sprintf("data: %s\n\n", data)); err != nil {
		p.logger.Error("failed to write stream chunk", "error", types.SafeErrorValue(err))
		return false
	}
	flusher.Flush()
	return true
}

func (s *chatStreamState) chunkResponse(chunk types.TextChunk) ChatCompletionResponse {
	delta := &ChatDelta{Content: chunk.Content(), Refusal: chunk.Refusal, Annotations: nativeChatAnnotations(chunk.Citations)}
	if !s.roleSent {
		delta.Role = "assistant"
		s.roleSent = true
	}
	if tcs := s.toolState.delta(chunk); len(tcs) > 0 {
		delta.ToolCalls = tcs
	}
	return ChatCompletionResponse{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   s.model,
		Choices: []ChatChoice{{Index: 0, Delta: delta}},
	}
}

// finish writes the deferred extension snapshot chunk when one is pending,
// then the SSE terminator. A stream that finished without a mid-loop commit
// still emits headers here, without metadata forwarding, as before.
func (s *chatStreamState) finish(p *proxy, w http.ResponseWriter, flusher http.Flusher) {
	if !s.committed {
		s.writeStreamHeaders(w)
		flusher.Flush()
	}
	if !s.terminalSent && s.extension != nil && len(s.extension.Include) > 0 {
		fr := s.terminalReason
		terminal := ChatCompletionResponse{
			ID:       s.id,
			Object:   "chat.completion.chunk",
			Created:  time.Now().Unix(),
			Model:    s.model,
			Choices:  []ChatChoice{{Index: 0, Delta: &ChatDelta{}, FinishReason: &fr}},
			Wormhole: extensionSnapshot(s.extension, s.metadata, s.usage, s.sources, s.citations, s.warnings),
		}
		data, err := json.Marshal(terminal)
		if err != nil {
			return
		}
		if err := writeChatFrame(w, fmt.Sprintf("data: %s\n\n", data)); err != nil {
			return
		}
	}
	if err := writeChatFrame(w, "data: [DONE]\n\n"); err != nil {
		p.logger.Error("failed to write stream terminator", "error", types.SafeErrorValue(err))
		return
	}
	flusher.Flush()
}
