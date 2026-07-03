package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/internal/server"
)

func TestRunTopLevelCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no args prints usage", wantCode: 0, wantStdout: "wormhole - OpenAI-compatible LLM proxy"},
		{name: "help prints usage", args: []string{"help"}, wantCode: 0, wantStdout: "Commands:"},
		{name: "version prints version", args: []string{"version"}, wantCode: 0, wantStdout: "wormhole dev"},
		{name: "unknown command errors", args: []string{"unknown"}, wantCode: 1, wantStderr: "unknown command: unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr, func(string) string { return "" })

			assert.Equal(t, tt.wantCode, code)
			assert.Contains(t, stdout.String(), tt.wantStdout)
			assert.Contains(t, stderr.String(), tt.wantStderr)
		})
	}
}

func TestRunServeFlagParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "help returns zero", args: []string{"serve", "--help"}, wantCode: 0, wantStderr: "Usage of serve:"},
		{name: "bad flag returns nonzero", args: []string{"serve", "--missing"}, wantCode: 1, wantStderr: "flag provided but not defined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr, func(string) string { return "" })

			assert.Equal(t, tt.wantCode, code)
			assert.Contains(t, stderr.String(), tt.wantStderr)
		})
	}
}

func TestProxyGracefulShutdownWaitsForWormholeShutdown(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM process signaling is not portable on Windows")
	}

	addr := reserveLoopbackAddr(t)
	dir := t.TempDir()
	markerPath := filepath.Join(dir, "shutdown-complete")
	releasePath := filepath.Join(dir, "release-shutdown")

	cmd := exec.Command(os.Args[0], "-test.run=TestProxyGracefulShutdownHelperProcess", "--", "serve", "--addr", addr) //nolint:gosec // test helper process
	var stdout, stderr lockedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"WORMHOLE_TEST_HELPER=1",
		"WORMHOLE_SHUTDOWN_MARKER="+markerPath,
		"WORMHOLE_SHUTDOWN_RELEASE="+releasePath,
	)
	require.NoError(t, cmd.Start())
	waitCh := make(chan error, 1)
	t.Cleanup(func() {
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			_ = cmd.Process.Kill()
			select {
			case <-waitCh:
			case <-time.After(5 * time.Second):
			}
		}
	})

	go func() {
		waitCh <- cmd.Wait()
	}()

	waitForHealth(t, addr, waitCh, &stdout, &stderr)
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	waitForFileBeforeExit(t, markerPath, waitCh, &stdout, &stderr)

	select {
	case err := <-waitCh:
		t.Fatalf("proxy exited before wrapped Shutdown returned: err=%v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	default:
	}

	require.NoError(t, os.WriteFile(releasePath, []byte("release"), 0o644))
	select {
	case err := <-waitCh:
		require.NoError(t, err, "stdout=%s\nstderr=%s", stdout.String(), stderr.String())
	case <-time.After(5 * time.Second):
		t.Fatalf("proxy did not exit after releasing shutdown hook\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestProxyGracefulShutdownHelperProcess(t *testing.T) {
	if os.Getenv("WORMHOLE_TEST_HELPER") != "1" {
		return
	}

	markerPath := os.Getenv("WORMHOLE_SHUTDOWN_MARKER")
	releasePath := os.Getenv("WORMHOLE_SHUTDOWN_RELEASE")
	newProxyServer = func(cfg server.Config) proxyServer {
		return shutdownMarkerProxy{
			proxyServer: server.New(cfg),
			markerPath:  markerPath,
			releasePath: releasePath,
		}
	}

	args := os.Args
	if i := slicesIndex(args, "--"); i >= 0 {
		args = args[i+1:]
	} else {
		args = nil
	}
	os.Exit(run(args, os.Stdout, os.Stderr, os.Getenv))
}

type shutdownMarkerProxy struct {
	proxyServer
	markerPath  string
	releasePath string
}

func (p shutdownMarkerProxy) Shutdown(ctx context.Context) error {
	err := p.proxyServer.Shutdown(ctx)
	if p.markerPath != "" {
		_ = os.WriteFile(p.markerPath, []byte("complete"), 0o644)
	}
	if p.releasePath != "" {
		waitForRelease(ctx, p.releasePath)
	}
	return err
}

func waitForRelease(ctx context.Context, path string) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reserveLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func waitForHealth(t *testing.T, addr string, waitCh <-chan error, stdout, stderr *lockedBuffer) {
	t.Helper()
	client := http.Client{Timeout: 200 * time.Millisecond}
	url := "http://" + addr + "/health"
	waitForCondition(t, 5*time.Second, waitCh, stdout, stderr, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
}

func waitForFileBeforeExit(t *testing.T, path string, waitCh <-chan error, stdout, stderr *lockedBuffer) {
	t.Helper()
	waitForCondition(t, 5*time.Second, waitCh, stdout, stderr, func() bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

func waitForCondition(t *testing.T, timeout time.Duration, waitCh <-chan error, stdout, stderr *lockedBuffer, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if ready() {
			return
		}
		select {
		case err := <-waitCh:
			t.Fatalf("proxy exited before condition was met: err=%v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
		case <-deadline.C:
			t.Fatalf("timed out waiting for condition\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
		case <-ticker.C:
		}
	}
}

func slicesIndex(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

type lockedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
