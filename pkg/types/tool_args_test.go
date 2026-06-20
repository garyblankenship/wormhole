package types

import "testing"

// ParseToolArgs is the single source of truth for decoding provider tool-call
// argument JSON. It must: parse valid JSON to a map, return (nil, errMsg) on
// malformed input, and return emptyVal verbatim for empty input.
func TestParseToolArgs(t *testing.T) {
	t.Parallel()

	t.Run("valid JSON parses to map", func(t *testing.T) {
		t.Parallel()
		got, errMsg := ParseToolArgs(`{"a":1,"b":"x"}`, nil)
		if errMsg != "" {
			t.Fatalf("unexpected errMsg: %q", errMsg)
		}
		if got["b"] != "x" {
			t.Fatalf("got[b] = %v, want x", got["b"])
		}
		if got["a"] != float64(1) { // JSON numbers decode to float64
			t.Fatalf("got[a] = %v (%T), want 1", got["a"], got["a"])
		}
	})

	t.Run("malformed JSON returns nil and error message", func(t *testing.T) {
		t.Parallel()
		got, errMsg := ParseToolArgs(`{not json`, map[string]any{})
		if got != nil {
			t.Fatalf("got = %v, want nil on malformed input", got)
		}
		if errMsg == "" {
			t.Fatal("want non-empty errMsg on malformed input")
		}
	})

	t.Run("empty input returns emptyVal verbatim", func(t *testing.T) {
		t.Parallel()
		sentinel := map[string]any{}
		got, errMsg := ParseToolArgs("", sentinel)
		if errMsg != "" {
			t.Fatalf("unexpected errMsg: %q", errMsg)
		}
		got["k"] = "v" // same underlying map -> mutation is visible on sentinel
		if sentinel["k"] != "v" {
			t.Fatal("emptyVal not returned verbatim")
		}
	})

	t.Run("empty input with nil emptyVal returns nil", func(t *testing.T) {
		t.Parallel()
		got, errMsg := ParseToolArgs("", nil)
		if errMsg != "" {
			t.Fatalf("unexpected errMsg: %q", errMsg)
		}
		if got != nil {
			t.Fatalf("got = %v, want nil", got)
		}
	})
}

// MarkArgsError enforces the ArgsInvalid contract in one place: it is a no-op on
// empty errMsg, and otherwise sets ArgsInvalid + ArgsParseError while clearing
// Arguments to nil.
func TestToolCallMarkArgsError(t *testing.T) {
	t.Parallel()

	t.Run("empty errMsg is a no-op", func(t *testing.T) {
		t.Parallel()
		tc := ToolCall{Arguments: map[string]any{"a": float64(1)}}
		tc.MarkArgsError("")
		if tc.ArgsInvalid {
			t.Fatal("ArgsInvalid set on empty errMsg")
		}
		if tc.Arguments == nil {
			t.Fatal("Arguments cleared on empty errMsg")
		}
		if tc.ArgsParseError != "" {
			t.Fatalf("ArgsParseError = %q, want empty", tc.ArgsParseError)
		}
	})

	t.Run("non-empty errMsg sets contract and clears Arguments", func(t *testing.T) {
		t.Parallel()
		tc := ToolCall{Arguments: map[string]any{"a": float64(1)}}
		tc.MarkArgsError("boom")
		if !tc.ArgsInvalid {
			t.Fatal("ArgsInvalid not set")
		}
		if tc.Arguments != nil {
			t.Fatalf("Arguments = %v, want nil when ArgsInvalid", tc.Arguments)
		}
		if tc.ArgsParseError != "boom" {
			t.Fatalf("ArgsParseError = %q, want boom", tc.ArgsParseError)
		}
	})
}
