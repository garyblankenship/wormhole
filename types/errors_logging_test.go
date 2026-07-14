package types

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestWormholeErrorSlogValueExcludesDetailsAndCause(t *testing.T) {
	t.Parallel()

	const secret = "secret-upstream-body"
	err := NewWormholeError(ErrorCodeProvider, strings.Repeat(secret, 100), true).
		WithProvider("openai").
		WithModel("gpt-test").
		WithStatusCode(503).
		WithDetails(secret).
		WithCause(errors.New(secret))

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Error("failed", "error", err)

	output := buf.String()
	if strings.Contains(output, secret) || strings.Contains(output, "details") || strings.Contains(output, "cause") {
		t.Fatalf("slog output exposed error internals: %s", output)
	}
	for _, expected := range []string{"code=PROVIDER_ERROR", "provider=openai", "model=gpt-test", "status_code=503", "retryable=true"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("slog output missing %q: %s", expected, output)
		}
	}
	if len(output) > 1200 {
		t.Fatalf("slog output was not bounded: %d bytes", len(output))
	}
}

func TestSafeErrorValueDoesNotFormatArbitraryError(t *testing.T) {
	t.Parallel()

	const secret = "raw-provider-secret"
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Error("failed", "error", SafeErrorValue(errors.New(secret)))
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("safe error value formatted raw error: %s", buf.String())
	}
}
