package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
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
		{name: "version prints version", args: []string{"version"}, wantCode: 0, wantStdout: "wormhole v1.9.0"},
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
