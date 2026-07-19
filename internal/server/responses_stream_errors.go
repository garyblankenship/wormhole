package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garyblankenship/wormhole/v2/types"
)

type responsesSSEWriter struct {
	w        http.ResponseWriter
	sequence int
}

func (s *responsesSSEWriter) write(event responsesEvent) {
	event.SequenceNumber = s.sequence
	s.sequence++
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event.Type, data)
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
		return string(whErr.Code)
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
