package transform

import (
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

// MapFinishReason maps a provider's finish reason string to the canonical FinishReason.
// It handles all known provider-specific aliases (e.g., "end_turn" for Anthropic,
// "STOP" for Gemini) in addition to the standard values.
func MapFinishReason(reason string) types.FinishReason {
	switch strings.ToLower(reason) {
	case "stop", "end_turn":
		return types.FinishReasonStop
	case "length", "max_tokens":
		return types.FinishReasonLength
	case "tool_calls", "function_call", "tool_use":
		return types.FinishReasonToolCalls
	case "content_filter", "safety", "recitation":
		return types.FinishReasonContentFilter
	case "other", "finish_reason_unspecified", "load", "unload":
		return types.FinishReasonOther
	default:
		return types.FinishReasonOther
	}
}
