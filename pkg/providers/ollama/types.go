package ollama

import "time"

// Ollama-specific API request/response types based on Ollama REST API

// chatRequest represents an Ollama chat request
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
	Format   string    `json:"format,omitempty"` // "json" for structured output
	Options  *options  `json:"options,omitempty"`
}

// message represents an Ollama message
type message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`          // string or []contentPart for multimodal
	Images  []string    `json:"images,omitempty"` // base64 encoded images
}

// contentPart represents part of multimodal content
type contentPart struct {
	Type string `json:"type"` // "text" or "image"
	Text string `json:"text,omitempty"`
	// Images are handled separately in the images field
}

// options represents Ollama model options
type options struct {
	Temperature      *float32 `json:"temperature,omitempty"`
	TopP             *float32 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	NumPredict       *int     `json:"num_predict,omitempty"` // equivalent to max_tokens
	Stop             []string `json:"stop,omitempty"`
	RepeatPenalty    *float32 `json:"repeat_penalty,omitempty"`
	PresencePenalty  *float32 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float32 `json:"frequency_penalty,omitempty"`
	Seed             *int     `json:"seed,omitempty"`
}

// chatResponse represents an Ollama chat response
type chatResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Message            message   `json:"message"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration,omitempty"`
	LoadDuration       int64     `json:"load_duration,omitempty"`
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"`
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"`
}

// streamResponse represents an Ollama streaming chat response
type streamResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Message            message   `json:"message"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration,omitempty"`
	LoadDuration       int64     `json:"load_duration,omitempty"`
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"`
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"`
}

// embeddingsRequest represents an Ollama embeddings request
type embeddingsRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"` // Single string for embeddings
}

// embeddingsResponse represents an Ollama embeddings response
type embeddingsResponse struct {
	Embedding []float64 `json:"embedding"`
}

// generateRequest represents an Ollama generate request (legacy API)
type generateRequest struct {
	Model   string   `json:"model"`
	Prompt  string   `json:"prompt"`
	Stream  bool     `json:"stream,omitempty"`
	Format  string   `json:"format,omitempty"`
	Options *options `json:"options,omitempty"`
}

// generateResponse represents an Ollama generate response
type generateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration,omitempty"`
	LoadDuration       int64     `json:"load_duration,omitempty"`
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"`
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"`
}

// modelsResponse represents the Ollama models list response
type modelsResponse struct {
	Models []modelInfo `json:"models"`
}

// modelInfo represents information about an Ollama model
type modelInfo struct {
	Name       string            `json:"name"`
	Model      string            `json:"model"`
	ModifiedAt time.Time         `json:"modified_at"`
	Size       int64             `json:"size"`
	Digest     string            `json:"digest"`
	Details    map[string]string `json:"details"`
}
