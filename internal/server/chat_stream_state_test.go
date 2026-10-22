package server

import (
	"io"
	"net/http/httptest"
	"testing"
)

type terminalFailureWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *terminalFailureWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.writes == 1 {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(data)
}

func TestChatStreamTerminalWriteFailureStopsWithoutDone(t *testing.T) {
	t.Parallel()
	writer := &terminalFailureWriter{ResponseRecorder: httptest.NewRecorder()}
	state := newChatStreamState("fixture", &WormholeRequest{Include: []string{"usage"}})
	state.committed = true
	state.finish(&proxy{}, writer, writer)
	if writer.writes != 1 {
		t.Fatalf("writes = %d, want only the failed terminal frame", writer.writes)
	}
}
