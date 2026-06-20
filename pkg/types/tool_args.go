package types

import "encoding/json"

// ParseToolArgs parses a tool-call arguments JSON string into a map. On malformed
// JSON it returns (nil, <error message>); the caller passes that message to
// (*ToolCall).MarkArgsError and keeps the raw string in Function.Arguments.
// emptyVal is returned verbatim for empty input so each caller preserves its own
// no-args representation (some sites use nil, others an empty map).
func ParseToolArgs(raw string, emptyVal map[string]any) (map[string]any, string) {
	if raw == "" {
		return emptyVal, ""
	}
	m := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err.Error()
	}
	return m, ""
}

// MarkArgsError records a tool-call argument parse failure on the receiver,
// enforcing the ArgsInvalid contract in one place: Arguments is cleared to nil
// whenever ArgsInvalid is set. No-op when errMsg is empty, so callers can invoke
// it unconditionally after constructing the ToolCall.
func (tc *ToolCall) MarkArgsError(errMsg string) {
	if errMsg == "" {
		return
	}
	tc.Arguments = nil
	tc.ArgsInvalid = true
	tc.ArgsParseError = errMsg
}
