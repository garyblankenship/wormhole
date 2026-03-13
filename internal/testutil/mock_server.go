package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockOpenAIServer creates a mock OpenAI API server for testing
func MockOpenAIServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return server
}
