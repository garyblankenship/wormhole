package types

import (
	"context"
	"encoding/json"
)

// TextEvidenceProvider is an optional capability for a single nonstreaming
// generation with original wire evidence. It does not extend Provider.
// A non-nil result may accompany an error when a complete body was received.
type TextEvidenceProvider interface {
	TextWithEvidence(context.Context, TextRequest) (*TextEvidenceResult, error)
}

// TextEvidenceResult separates the convenience response from untrusted evidence.
type TextEvidenceResult struct {
	Response *TextResponse `json:"response,omitempty"`
	Evidence TextEvidence  `json:"evidence"`
}

// TextEvidence owns its body and decoded fields. Complete certifies a complete,
// bounded HTTP body, not valid JSON, successful normalization, or task acceptance.
// RawBody and RawUsage may contain secrets; callers must not log them implicitly.
type TextEvidence struct {
	Provider         string               `json:"provider"`
	APIFamily        string               `json:"api_family"`
	RawBody          []byte               `json:"raw_body,omitempty"`
	Complete         bool                 `json:"complete"`
	Choices          []TextChoiceEvidence `json:"choices,omitempty"`
	RawUsage         json.RawMessage      `json:"raw_usage,omitempty"`
	Usage            TokenUsageEvidence   `json:"usage"`
	ResponseMetadata *ResponseMetadata    `json:"response_metadata,omitempty"`
}

// TextChoiceEvidence retains every choice without normalizing content or calls.
// Raw JSON preserves null and non-string content/refusal representations.
type TextChoiceEvidence struct {
	Index        int                       `json:"index"`
	Role         string                    `json:"role"`
	Content      json.RawMessage           `json:"content,omitempty"`
	Refusal      json.RawMessage           `json:"refusal,omitempty"`
	FinishReason *string                   `json:"finish_reason"`
	ToolCalls    []TextToolCallEvidence    `json:"tool_calls,omitempty"`
	FunctionCall *TextFunctionCallEvidence `json:"function_call,omitempty"`
}

// TextToolCallEvidence preserves the original function argument text.
type TextToolCallEvidence struct {
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function TextFunctionCallEvidence `json:"function"`
}

// TextFunctionCallEvidence preserves a legacy or tool function call.
type TextFunctionCallEvidence struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TokenUsageEvidence only populates nonnegative JSON integers representable by
// int64. Nil means absent, null, or invalid; RawUsage distinguishes those cases.
// No omitted count is inferred or synthesized from other counts.
type TokenUsageEvidence struct {
	PromptTokens     *int64 `json:"prompt_tokens"`
	CompletionTokens *int64 `json:"completion_tokens"`
	TotalTokens      *int64 `json:"total_tokens"`
	CachedTokens     *int64 `json:"cached_tokens"`
	ReasoningTokens  *int64 `json:"reasoning_tokens"`
}
