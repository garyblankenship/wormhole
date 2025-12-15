package types

// BaseRequest contains common request fields
type BaseRequest struct {
	Model            string         `json:"model"`
	Temperature      *float32       `json:"temperature,omitempty"`
	TopP             *float32       `json:"top_p,omitempty"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	Stop             []string       `json:"stop,omitempty"`
	PresencePenalty  *float32       `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float32       `json:"frequency_penalty,omitempty"`
	Seed             *int           `json:"seed,omitempty"`
	ProviderOptions  map[string]any `json:"-"`
}

// TextRequest represents a text generation request
type TextRequest struct {
	BaseRequest
	Messages       []Message   `json:"messages"`
	SystemPrompt   string      `json:"-"`
	Tools          []Tool      `json:"tools,omitempty"`
	ToolChoice     *ToolChoice `json:"tool_choice,omitempty"`
	ResponseFormat any         `json:"response_format,omitempty"`
}

// StructuredRequest represents a structured output request
type StructuredRequest struct {
	BaseRequest
	Messages     []Message      `json:"messages"`
	SystemPrompt string         `json:"-"`
	Schema       Schema         `json:"schema"`
	SchemaName   string         `json:"schema_name,omitempty"`
	Mode         StructuredMode `json:"mode,omitempty"`
}

// StructuredMode defines how structured output is generated
type StructuredMode string

const (
	StructuredModeJSON   StructuredMode = "json"
	StructuredModeTools  StructuredMode = "tools"
	StructuredModeStrict StructuredMode = "strict"
)

// EmbeddingsRequest represents an embeddings request
type EmbeddingsRequest struct {
	Model           string         `json:"model"`
	Input           []string       `json:"input"`
	Dimensions      *int           `json:"dimensions,omitempty"`
	ProviderOptions map[string]any `json:"-"`
}

// ImagesRequest represents an image generation request
type ImagesRequest struct {
	Model           string         `json:"model"`
	Prompt          string         `json:"prompt"`
	Size            string         `json:"size,omitempty"`
	Quality         string         `json:"quality,omitempty"`
	Style           string         `json:"style,omitempty"`
	N               int            `json:"n,omitempty"`
	ResponseFormat  string         `json:"response_format,omitempty"`
	ProviderOptions map[string]any `json:"-"`
}

// SpeechToTextRequest represents a speech-to-text request
type SpeechToTextRequest struct {
	Model       string   `json:"model"`
	Audio       []byte   `json:"-"`
	AudioFormat string   `json:"audio_format"`
	Language    string   `json:"language,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Temperature *float32 `json:"temperature,omitempty"`
}

// AudioRequestType represents the type of audio request
type AudioRequestType string

const (
	AudioRequestTypeTTS AudioRequestType = "tts"
	AudioRequestTypeSTT AudioRequestType = "stt"
)

// TextToSpeechRequest represents a text-to-speech request
type TextToSpeechRequest struct {
	Model           string         `json:"model"`
	Input           string         `json:"input"`
	Voice           string         `json:"voice,omitempty"`
	Speed           float32        `json:"speed,omitempty"`
	ResponseFormat  string         `json:"response_format,omitempty"`
	ProviderOptions map[string]any `json:"-"`
}

// ImageRequest represents an image generation request (alias for ImagesRequest)
type ImageRequest = ImagesRequest

// AudioRequest represents a unified audio request
type AudioRequest struct {
	Type            AudioRequestType `json:"type"`
	Model           string           `json:"model"`
	Input           any              `json:"input,omitempty"`    // string for TTS, []byte for STT
	Voice           string           `json:"voice,omitempty"`    // TTS only
	Speed           float32          `json:"speed,omitempty"`    // TTS only
	Language        string           `json:"language,omitempty"` // STT only
	Prompt          string           `json:"prompt,omitempty"`   // STT only
	Temperature     *float32         `json:"temperature,omitempty"`
	ResponseFormat  string           `json:"response_format,omitempty"`
	ProviderOptions map[string]any   `json:"-"`
}
