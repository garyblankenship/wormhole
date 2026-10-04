package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

type responsesSSEWriter struct {
	w        http.ResponseWriter
	sequence int
	err      error
	cancel   context.CancelFunc
	ctx      context.Context
}

func (s *responsesSSEWriter) write(event responsesEvent) {
	if s.err != nil {
		return
	}
	if s.ctx != nil && s.ctx.Err() != nil {
		s.fail(s.ctx.Err())
		return
	}
	event.SequenceNumber = s.sequence
	s.sequence++
	data, err := json.Marshal(event)
	if err != nil {
		s.fail(err)
		return
	}
	frame := fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, data)
	n, err := s.w.Write([]byte(frame))
	if err == nil && n != len(frame) {
		err = io.ErrShortWrite
	}
	if err != nil {
		s.fail(err)
	}
}

func (s *responsesSSEWriter) fail(err error) {
	s.err = err
	if s.cancel != nil {
		s.cancel()
	}
}

func writeResponsesFailure(sse *responsesSSEWriter, responseID, model string, createdAt int64, err error) {
	_, errType, clientMsg := upstreamErrorStatus(err)
	code := responsesErrorCode(err)
	event := responsesEvent{
		Type: "response.failed",
		Response: &responsesEnvelope{
			ID: responseID, Object: "response", CreatedAt: createdAt, Status: "failed", Model: model,
			Output: []responsesOutputItem{}, Error: map[string]any{"code": code, "message": clientMsg, "type": errType},
		},
	}
	sse.write(event)
}

func responsesErrorCode(err error) string {
	whErr, ok := types.AsWormholeError(err)
	if !ok {
		return "upstream_error"
	}
	if whErr.Code == types.ErrorCodeProvider && validResponsesErrorCode(whErr.Details) {
		return whErr.Details
	}
	if whErr.Code != "" {
		return strings.ToLower(string(whErr.Code))
	}
	return "upstream_error"
}

func validResponsesErrorCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
