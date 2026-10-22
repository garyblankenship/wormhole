package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

// ChatCompletionRequest is the OpenAI-compatible chat completion request.
type ChatCompletionRequest struct {
	Wormhole            *WormholeRequest               `json:"wormhole,omitempty"`
	Thinking            map[string]any                 `json:"thinking,omitempty"`
	Model               string                         `json:"model"`
	Messages            []ChatCompletionRequestMessage `json:"messages"`
	Temperature         *float64                       `json:"temperature,omitempty"`
	MaxTokens           *int                           `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                           `json:"max_completion_tokens,omitempty"`
	TopP                *float64                       `json:"top_p,omitempty"`
	ReasoningEffort     *types.ReasoningEffort         `json:"reasoning_effort,omitempty"`
	FrequencyPenalty    *float64                       `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64                       `json:"presence_penalty,omitempty"`
	Seed                *int                           `json:"seed,omitempty"`
	N                   *int                           `json:"n,omitempty"`
	ParallelToolCalls   *bool                          `json:"parallel_tool_calls,omitempty"`
	Stop                []string                       `json:"stop,omitempty"`
	Stream              bool                           `json:"stream,omitempty"`
	Tools               []ChatTool                     `json:"tools,omitempty"`
	ToolChoice          json.RawMessage                `json:"tool_choice,omitempty"`
	ResponseFormat      json.RawMessage                `json:"response_format,omitempty"`
	// ChatTemplateKwargs is the raw top-level chat_template_kwargs extension
	// accepted for OpenAI-chat-compatible upstreams. It stays raw so an
	// explicit null remains distinguishable from an omitted field;
	// parseChatTemplateKwargs validates the shape before any upstream call.
	ChatTemplateKwargs json.RawMessage `json:"chat_template_kwargs,omitempty"`
}

// parseChatTemplateKwargs validates the raw top-level chat_template_kwargs
// field and returns its decoded object. Omission yields (nil, nil). An explicit
// null, any non-object form, or a non-boolean enable_thinking value is rejected
// here so malformed requests never reach an upstream call. Other JSON template
// kwargs pass through as-is; the decoded map is only re-marshaled by the
// provider, never re-read by the proxy.
func parseChatTemplateKwargs(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var kwargs map[string]any
	if err := json.Unmarshal(raw, &kwargs); err != nil || kwargs == nil {
		return nil, fmt.Errorf("chat_template_kwargs must be a JSON object")
	}
	if enabled, present := kwargs["enable_thinking"]; present {
		if _, ok := enabled.(bool); !ok {
			return nil, fmt.Errorf("chat_template_kwargs.enable_thinking must be a boolean")
		}
	}
	return kwargs, nil
}

// ChatCompletionRequestMessage is a request-only chat message. OpenAI clients
// may send content as either a plain string or a multimodal parts array.
type ChatCompletionRequestMessage struct {
	Role       string             `json:"role"`
	Content    ChatMessageContent `json:"content"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolCalls  []ChatToolCall     `json:"tool_calls,omitempty"`
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
		case "file":
			if part.File.FileID == "" {
				return fmt.Errorf("file.file_id is required; inline file data is unsupported")
			}
			media = append(media, &types.FileReferenceMedia{ID: part.File.FileID, Kind: "document"})
		case "wormhole_file":
			ref := &types.FileReferenceMedia{Provider: part.Provider, ID: part.ID, URI: part.URI, Kind: part.Kind, MimeType: part.MimeType}
			if ref.Provider == "" {
				return fmt.Errorf("wormhole_file.provider is required")
			}
			if err := ref.Validate(); err != nil {
				return err
			}
			media = append(media, ref)
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
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind,omitempty"`
	ID       string `json:"id,omitempty"`
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	File     struct {
		FileID string `json:"file_id"`
	} `json:"file,omitempty"`

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
