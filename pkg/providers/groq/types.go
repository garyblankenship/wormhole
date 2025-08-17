package groq

import "github.com/garyblankenship/wormhole/pkg/types"

// Groq API response types (OpenAI-compatible)
type groqTextResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []groqChoice `json:"choices"`
	Usage   *groqUsage   `json:"usage,omitempty"`
	Error   *groqError   `json:"error,omitempty"`
}

type groqChoice struct {
	Index        int         `json:"index"`
	Message      groqMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type groqMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []groqToolCall `json:"tool_calls,omitempty"`
}

type groqToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function groqFunctionCall `json:"function"`
}

type groqFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type groqUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type groqError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Streaming response types
type groqStreamResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []groqStreamChoice `json:"choices"`
	Error   *groqError         `json:"error,omitempty"`
}

type groqStreamChoice struct {
	Index        int             `json:"index"`
	Delta        groqStreamDelta `json:"delta"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

type groqStreamDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []groqToolCall `json:"tool_calls,omitempty"`
}

// Finish reason mappings
var finishReasonMap = map[string]types.FinishReason{
	"stop":           types.FinishReasonStop,
	"length":         types.FinishReasonLength,
	"content_filter": types.FinishReasonContentFilter,
	"tool_calls":     types.FinishReasonToolCalls,
}
