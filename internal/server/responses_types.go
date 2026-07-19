package server

import (
	"encoding/json"
	"fmt"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
)

type responsesRequest struct {
	Model              string              `json:"model"`
	Instructions       string              `json:"instructions,omitempty"`
	Input              responsesInput      `json:"input"`
	Tools              []responsesTool     `json:"tools,omitempty"`
	ToolChoice         json.RawMessage     `json:"tool_choice,omitempty"`
	Stream             bool                `json:"stream,omitempty"`
	Store              bool                `json:"store,omitempty"`
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	Temperature        *float64            `json:"temperature,omitempty"`
	TopP               *float64            `json:"top_p,omitempty"`
	MaxOutputTokens    *int                `json:"max_output_tokens,omitempty"`
	Reasoning          *responsesReasoning `json:"reasoning,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type responsesInput struct {
	Text  string
	Items []responsesInputItem
}

func (i *responsesInput) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &i.Text); err == nil {
		i.Items = nil
		return nil
	}
	if err := json.Unmarshal(data, &i.Items); err != nil {
		return fmt.Errorf("input must be a string or array of response input items")
	}
	return nil
}

type responsesInputItem struct {
	Type        string          `json:"type"`
	Role        string          `json:"role,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	Name        string          `json:"name,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
	CustomInput string          `json:"input,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
}

type responsesContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responsesExecution struct {
	builder     *wormhole.TextRequestBuilder
	model       string
	customTools map[string]bool
}

type responsesUsage struct {
	InputTokens        int                         `json:"input_tokens"`
	OutputTokens       int                         `json:"output_tokens"`
	TotalTokens        int                         `json:"total_tokens"`
	InputTokenDetails  responsesInputTokenDetails  `json:"input_tokens_details"`
	OutputTokenDetails responsesOutputTokenDetails `json:"output_tokens_details"`
}

type responsesInputTokenDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

type responsesOutputTokenDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type responsesOutputItem struct {
	ID        string                `json:"id"`
	Type      string                `json:"type"`
	Status    string                `json:"status,omitempty"`
	Role      string                `json:"role,omitempty"`
	Content   []responsesOutputText `json:"content"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments string                `json:"arguments,omitempty"`
	Input     string                `json:"input,omitempty"`
}

type responsesOutputText struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Refusal     string `json:"refusal,omitempty"`
	Annotations []any  `json:"annotations,omitempty"`
}

type responsesEnvelope struct {
	ID                string                `json:"id"`
	Object            string                `json:"object"`
	CreatedAt         int64                 `json:"created_at"`
	Status            string                `json:"status"`
	Model             string                `json:"model"`
	Output            []responsesOutputItem `json:"output"`
	Usage             *responsesUsage       `json:"usage,omitempty"`
	Error             any                   `json:"error"`
	IncompleteDetails any                   `json:"incomplete_details"`
}

type responsesEvent struct {
	Type           string               `json:"type"`
	SequenceNumber int                  `json:"sequence_number"`
	Response       *responsesEnvelope   `json:"response,omitempty"`
	OutputIndex    *int                 `json:"output_index,omitempty"`
	ContentIndex   *int                 `json:"content_index,omitempty"`
	ItemID         string               `json:"item_id,omitempty"`
	Delta          string               `json:"delta,omitempty"`
	Arguments      string               `json:"arguments,omitempty"`
	Input          string               `json:"input,omitempty"`
	Text           string               `json:"text,omitempty"`
	Refusal        string               `json:"refusal,omitempty"`
	Part           *responsesOutputText `json:"part,omitempty"`
	Item           *responsesOutputItem `json:"item,omitempty"`
}

type responsesToolChoiceSelection struct {
	choice       *types.ToolChoice
	allowedTools map[string]bool
}
