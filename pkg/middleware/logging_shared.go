package middleware

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// deref returns the underlying value if v is a pointer, otherwise returns v unchanged.
// A nil pointer returns v as-is so downstream type switches fall through to default.
func deref(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		return rv.Elem().Interface()
	}
	return v
}

var defaultLoggingRedactKeys = []string{"api_key", "apikey", "token", "authorization"}

func newDebugLoggingConfig(logger types.Logger) LoggingConfig {
	return LoggingConfig{
		Logger:       logger,
		LogRequests:  true,
		LogResponses: true,
		LogTiming:    true,
		LogErrors:    true,
		RedactKeys:   append([]string(nil), defaultLoggingRedactKeys...),
	}
}

func logRequestDetails(config LoggingConfig, req any) {
	switch r := deref(req).(type) {
	case types.TextRequest:
		logTextRequestDetails(config.Logger, r)
	case types.StructuredRequest:
		logStructuredRequestDetails(config.Logger, r)
	case types.EmbeddingsRequest:
		logEmbeddingsRequestDetails(config.Logger, r)
	case types.AudioRequest:
		logAudioRequestDetails(config.Logger, r)
	case types.ImageRequest:
		logImageRequestDetails(config.Logger, r)
	default:
		sanitized := redactSensitiveData(req, config.RedactKeys)
		if jsonData, err := json.MarshalIndent(sanitized, "", "  "); err == nil {
			config.Logger.Debug("Request", "data", string(jsonData))
		}
	}
}

func logResponseDetails(config LoggingConfig, resp any, duration time.Duration) {
	switch r := deref(resp).(type) {
	case types.TextResponse:
		logTextResponseDetails(config.Logger, r, duration)
	case types.StructuredResponse:
		logStructuredResponseDetails(config.Logger, r, duration)
	case types.EmbeddingsResponse:
		logEmbeddingsResponseDetails(config.Logger, r, duration)
	case types.AudioResponse:
		logAudioResponseDetails(config.Logger, r, duration)
	case types.ImageResponse:
		logImageResponseDetails(config.Logger, r, duration)
	default:
		config.Logger.Debug("Response received", "duration", duration)
	}
}

func logTextRequestDetails(logger types.Logger, request types.TextRequest) {
	logger.Debug("Text request", "model", request.Model)
	if len(request.Messages) > 0 {
		logger.Debug("Messages", "count", len(request.Messages))
		for i, msg := range request.Messages {
			logger.Debug("Message",
				"index", i,
				"role", msg.GetRole(),
				"content", truncateString(getMessageContent(msg), 100))
		}
	}
	if request.Temperature != nil {
		logger.Debug("Temperature", "value", *request.Temperature)
	}
	if request.MaxTokens != nil {
		logger.Debug("Max tokens", "value", *request.MaxTokens)
	}
	if len(request.Tools) > 0 {
		logger.Debug("Tools available", "count", len(request.Tools))
	}
}

func logTextResponseDetails(logger types.Logger, response types.TextResponse, duration time.Duration) {
	logger.Debug("Text response received", "duration", duration, "model", response.Model)
	logger.Debug("Text details", "length", len(response.Text), "finish_reason", response.FinishReason)

	if response.Usage != nil {
		logger.Debug("Token usage",
			"prompt_tokens", response.Usage.PromptTokens,
			"completion_tokens", response.Usage.CompletionTokens,
			"total_tokens", response.Usage.TotalTokens)

		if cost, err := types.EstimateModelCost(response.Model, response.Usage.PromptTokens, response.Usage.CompletionTokens); err == nil && cost > 0 {
			logger.Debug("Estimated cost", "cost", cost)
		}
	}

	if len(response.ToolCalls) > 0 {
		logger.Debug("Tool calls", "count", len(response.ToolCalls))
		for i, call := range response.ToolCalls {
			logger.Debug("Tool call", "index", i, "name", call.Name)
		}
	}

	logger.Debug("Preview", "text", truncateString(response.Text, 200))
}

func logStructuredRequestDetails(logger types.Logger, request types.StructuredRequest) {
	logger.Debug("Structured request", "model", request.Model)
	if request.Schema != nil {
		logger.Debug("Schema provided for structured output")
	}
}

func logStructuredResponseDetails(logger types.Logger, response types.StructuredResponse, duration time.Duration) {
	logger.Debug("Structured response received", "duration", duration, "model", response.Model)
	logger.Debug("Structured data received")

	if response.Usage != nil {
		logger.Debug("Token usage",
			"prompt_tokens", response.Usage.PromptTokens,
			"completion_tokens", response.Usage.CompletionTokens,
			"total_tokens", response.Usage.TotalTokens)
	}
}

func logEmbeddingsRequestDetails(logger types.Logger, request types.EmbeddingsRequest) {
	logger.Debug("Embeddings request", "model", request.Model, "input_count", len(request.Input))
}

func logEmbeddingsResponseDetails(logger types.Logger, response types.EmbeddingsResponse, duration time.Duration) {
	logger.Debug("Embeddings response received", "duration", duration, "model", response.Model)
	logger.Debug("Embeddings details", "count", len(response.Embeddings))
	if len(response.Embeddings) > 0 {
		logger.Debug("Dimensions", "value", len(response.Embeddings[0].Embedding))
	}

	if response.Usage != nil {
		logger.Debug("Token usage", "total_tokens", response.Usage.TotalTokens)
	}
}

func logAudioRequestDetails(logger types.Logger, request types.AudioRequest) {
	logger.Debug("Audio request", "model", request.Model)
	switch request.Type {
	case types.AudioRequestTypeSTT:
		logger.Debug("Type", "value", "Speech to Text")
	case types.AudioRequestTypeTTS:
		logger.Debug("Type", "value", "Text to Speech")
		if request.Voice != "" {
			logger.Debug("Voice", "value", request.Voice)
		}
	}
}

func logAudioResponseDetails(logger types.Logger, response types.AudioResponse, duration time.Duration) {
	logger.Debug("Audio response received", "duration", duration, "model", response.Model)

	if response.Text != "" {
		logger.Debug("Text", "value", truncateString(response.Text, 100))
	}
	if len(response.Audio) > 0 {
		logger.Debug("Audio data", "bytes", len(response.Audio))
	}
	if !response.Created.IsZero() {
		logger.Debug("Created", "value", response.Created)
	}
}

func logImageRequestDetails(logger types.Logger, request types.ImageRequest) {
	logger.Debug("Image request", "model", request.Model)
	logger.Debug("Prompt", "value", truncateString(request.Prompt, 100))
	if request.Size != "" {
		logger.Debug("Size", "value", request.Size)
	}
	if request.Quality != "" {
		logger.Debug("Quality", "value", request.Quality)
	}
	if request.N > 0 {
		logger.Debug("Count", "value", request.N)
	}
}

func logImageResponseDetails(logger types.Logger, response types.ImageResponse, duration time.Duration) {
	logger.Debug("Image response received", "duration", duration, "model", response.Model)
	logger.Debug("Images generated", "count", len(response.Images))

	for i, img := range response.Images {
		if img.URL != "" {
			logger.Debug("Image", "index", i, "type", "URL provided")
		}
		if len(img.B64JSON) > 0 {
			logger.Debug("Image", "index", i, "type", "Base64 data", "chars", len(img.B64JSON))
		}
	}
}
