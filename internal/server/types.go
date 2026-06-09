package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ChatCompletionRequest is the OpenAI-compatible chat completion request.
type ChatCompletionRequest struct {
	Model       string                         `json:"model"`
	Messages    []ChatCompletionRequestMessage `json:"messages"`
	Temperature *float64                       `json:"temperature,omitempty"`
	MaxTokens   *int                           `json:"max_tokens,omitempty"`
	TopP        *float64                       `json:"top_p,omitempty"`
	Stop        []string                       `json:"stop,omitempty"`
	Stream      bool                           `json:"stream,omitempty"`
}

// ChatCompletionRequestMessage is a request-only chat message. OpenAI clients
// may send content as either a plain string or a multimodal parts array.
type ChatCompletionRequestMessage struct {
	Role    string             `json:"role"`
	Content ChatMessageContent `json:"content"`
}

type ChatMessageContent struct {
	Text  string
	Media []types.Media
}

func (c *ChatMessageContent) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = text
		c.Media = nil
		return nil
	}

	var parts []chatContentPart
	if err := json.Unmarshal(data, &parts); err != nil {
		return fmt.Errorf("content must be a string or array of content parts")
	}

	var textParts []string
	var media []types.Media
	for _, part := range parts {
		switch part.Type {
		case "text":
			textParts = append(textParts, part.Text)
		case "image_url":
			image, err := parseImageURLPart(part.ImageURL.URL)
			if err != nil {
				return err
			}
			media = append(media, image)
		default:
			return fmt.Errorf("unsupported content part type %q", part.Type)
		}
	}

	c.Text = strings.Join(textParts, "")
	c.Media = media
	return nil
}

type chatContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

func parseImageURLPart(rawURL string) (*types.ImageMedia, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("image_url.url is required")
	}
	if !strings.HasPrefix(rawURL, "data:") {
		return &types.ImageMedia{URL: rawURL}, nil
	}

	header, data, ok := strings.Cut(strings.TrimPrefix(rawURL, "data:"), ",")
	if !ok {
		return nil, fmt.Errorf("malformed image data URL")
	}
	mimeType, encoding, ok := strings.Cut(header, ";")
	if !ok || encoding != "base64" || !strings.HasPrefix(mimeType, "image/") {
		return nil, fmt.Errorf("malformed image data URL")
	}
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return nil, fmt.Errorf("malformed image data URL: %w", err)
	}

	return &types.ImageMedia{
		MimeType:   mimeType,
		Base64Data: data,
	}, nil
}

// ChatMessage is a message in the OpenAI chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse is the OpenAI-compatible chat completion response.
type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *ChatUsage   `json:"usage,omitempty"`
}

// ChatChoice is a single choice in a chat completion response.
type ChatChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason *string      `json:"finish_reason,omitempty"`
}

// ChatUsage is token usage in OpenAI format.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// EmbeddingRequest is the OpenAI-compatible embeddings request.
type EmbeddingRequest struct {
	Model string         `json:"model"`
	Input EmbeddingInput `json:"input"`
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
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// EmbeddingUsage is token usage for embeddings.
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
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
