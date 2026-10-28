package server

import (
	"encoding/json"
	"fmt"
)

// ChatMessage is a message in the OpenAI chat format.
type ChatMessage struct {
	Role        string           `json:"role"`
	Content     string           `json:"content"`
	Refusal     string           `json:"refusal,omitempty"`
	Annotations []map[string]any `json:"annotations,omitempty"`
	ToolCalls   []ChatToolCall   `json:"tool_calls,omitempty"`
}

// ChatDelta contains only fields present in a streaming message fragment.
// Full messages retain their required role and content fields in ChatMessage.
type ChatDelta struct {
	Role        string           `json:"role,omitempty"`
	Content     string           `json:"content,omitempty"`
	Refusal     string           `json:"refusal,omitempty"`
	Annotations []map[string]any `json:"annotations,omitempty"`
	ToolCalls   []ChatToolCall   `json:"tool_calls,omitempty"`
}

// ChatTool is an OpenAI-format tool definition on a request.
type ChatTool struct {
	Type     string           `json:"type"`
	Function ChatToolFunction `json:"function"`
}

// ChatToolFunction is the function schema inside a ChatTool.
type ChatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ChatToolCall is an OpenAI-format tool call on a response/delta or an inbound
// assistant message. Index is set only on streaming deltas.
type ChatToolCall struct {
	Index    *int                 `json:"index,omitempty"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function ChatToolCallFunction `json:"function"`
}

// ChatToolCallFunction holds the called function name and raw JSON arguments.
type ChatToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ChatCompletionResponse is the OpenAI-compatible chat completion response.
type ChatCompletionResponse struct {
	ID       string         `json:"id"`
	Object   string         `json:"object"`
	Created  int64          `json:"created"`
	Model    string         `json:"model"`
	Choices  []ChatChoice   `json:"choices"`
	Usage    *ChatUsage     `json:"usage,omitempty"`
	Wormhole map[string]any `json:"wormhole,omitempty"`
}

// ChatChoice is a single choice in a chat completion response.
type ChatChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatDelta   `json:"delta,omitempty"`
	FinishReason *string      `json:"finish_reason,omitempty"`
}

// ChatUsage is token usage in OpenAI format.
type ChatUsage struct {
	PromptTokens            int                         `json:"prompt_tokens"`
	CompletionTokens        int                         `json:"completion_tokens"`
	TotalTokens             int                         `json:"total_tokens"`
	PromptTokensDetails     *ChatPromptTokenDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *ChatCompletionTokenDetails `json:"completion_tokens_details,omitempty"`
}

type ChatPromptTokenDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

type ChatCompletionTokenDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// EmbeddingRequest is the OpenAI-compatible embeddings request.
type EmbeddingRequest struct {
	Model          string         `json:"model"`
	Input          EmbeddingInput `json:"input"`
	Dimensions     *int           `json:"dimensions,omitempty"`
	EncodingFormat string         `json:"encoding_format,omitempty"`
}

// EmbeddingInput accepts the OpenAI-compatible string-or-array input shape.
type EmbeddingInput []string

func (i *EmbeddingInput) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*i = []string{single}
		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err == nil {
		*i = many
		return nil
	}

	return fmt.Errorf("input must be a string or array of strings")
}

// EmbeddingResponse is the OpenAI-compatible embeddings response.
type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  *EmbeddingUsage `json:"usage,omitempty"`
}

// EmbeddingData is a single embedding vector.
type EmbeddingData struct {
	Object    string `json:"object"`
	Index     int    `json:"index"`
	Embedding any    `json:"embedding"`
}

// EmbeddingUsage is token usage for embeddings.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// RerankRequest is the OpenAI-compatible rerank request.
type RerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      *int     `json:"top_n,omitempty"`
}

// RerankResponse is the OpenAI-compatible rerank response.
type RerankResponse struct {
	ID      string         `json:"id,omitempty"`
	Model   string         `json:"model"`
	Results []RerankResult `json:"results"`
	Usage   *RerankUsage   `json:"usage,omitempty"`
}

// RerankResult is a single document and its relevance score.
type RerankResult struct {
	Index          int            `json:"index"`
	RelevanceScore float64        `json:"relevance_score"`
	Document       RerankDocument `json:"document"`
}

// RerankDocument is the OpenAI-compatible document result shape.
type RerankDocument struct {
	Text string `json:"text"`
}

// RerankUsage reports token usage when the provider supplies it.
type RerankUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// ModelEntry is a single model in the list response.
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelListResponse is the /v1/models response.
type ModelListResponse struct {
	Object string       `json:"object"`
	Data   []ModelEntry `json:"data"`
}

// ErrorResponse is an OpenAI-compatible error.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail holds error info.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// HealthResponse is the health check response.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}
