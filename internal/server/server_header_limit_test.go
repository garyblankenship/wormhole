package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const http1HeaderFramingSlack = 4 << 10

func TestProxyRequestHeaderLimit(t *testing.T) {
	t.Parallel()

	p := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	require.Equal(t, 1<<20, maxProxyRequestHeaderBytes)
	require.Equal(t, maxProxyRequestHeaderBytes, p.server.MaxHeaderBytes)

	var handlerCalls atomic.Int32
	originalHandler := p.server.Handler
	p.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalls.Add(1)
		originalHandler.ServeHTTP(w, r)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- p.server.Serve(listener)
	}()
	t.Cleanup(func() {
		require.NoError(t, p.Shutdown(context.Background()))
		require.ErrorIs(t, <-serveErr, http.ErrServerClosed)
	})

	ordinary := proxyHeaderLimitRequest(t, listener.Addr().String(), "GET /health HTTP/1.1\r\nHost: proxy\r\n\r\n")
	require.Equal(t, http.StatusOK, ordinary.StatusCode)
	require.NoError(t, ordinary.Body.Close())
	require.Equal(t, int32(1), handlerCalls.Load())

	oversized := proxyHeaderLimitRequest(t, listener.Addr().String(), fmt.Sprintf(
		"GET /health HTTP/1.1\r\nHost: proxy\r\nX-Oversized: %s\r\n\r\n",
		strings.Repeat("x", maxProxyRequestHeaderBytes+http1HeaderFramingSlack+1),
	))
	require.Equal(t, http.StatusRequestHeaderFieldsTooLarge, oversized.StatusCode)
	require.NoError(t, oversized.Body.Close())
	require.Equal(t, int32(1), handlerCalls.Load())
}

func proxyHeaderLimitRequest(t *testing.T, addr, request string) *http.Response {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	_, err = io.WriteString(conn, request)
	require.NoError(t, err)

	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	return response
}
