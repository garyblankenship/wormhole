package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// ProviderLoggingMiddleware provides logging at the provider level
type ProviderLoggingMiddleware struct {
	providerName string
	logger       types.Logger
}

// NewProviderLoggingMiddleware creates middleware for provider logging
func NewProviderLoggingMiddleware(providerName string, logger types.Logger) *ProviderLoggingMiddleware {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProviderLoggingMiddleware{
		providerName: providerName,
		logger:       logger,
	}
}

// withProviderLogging wraps a handler with provider-level logging using generics
func withProviderLogging[Req any, Resp any](
	ctx context.Context,
	logger types.Logger,
	providerName string,
	requestType string,
	requestInfo string,
	getSuccessInfo func(Resp) string,
	handler func(context.Context, Req) (Resp, error),
	request Req,
) (Resp, error) {
	providerName = types.SafeLogString(providerName)
	logger.Info(fmt.Sprintf("[%s] %s request: %s", providerName, requestType, requestInfo))

	resp, err := handler(ctx, request)

	if err != nil {
		logger.Info("Provider request failed", "provider", providerName, "request_type", requestType, "error", types.SafeErrorValue(err))
	} else {
		logger.Info(fmt.Sprintf("[%s] %s request succeeded: %s", providerName, requestType, getSuccessInfo(resp)))
	}

	return resp, err
}

func (m *ProviderLoggingMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Text",
			fmt.Sprintf("model=%s, messages=%d", types.SafeLogString(request.Model), len(request.Messages)),
			func(resp *types.TextResponse) string {
				return fmt.Sprintf("%d tokens", resp.Usage.TotalTokens)
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return func(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
		m.logger.Info(fmt.Sprintf("[%s] Stream request: model=%s, messages=%d",
			types.SafeLogString(m.providerName), types.SafeLogString(request.Model), len(request.Messages)))

		stream, err := next(ctx, request)

		if err != nil {
			m.logger.Info("Provider request failed", "provider", types.SafeLogString(m.providerName), "request_type", "Stream", "error", types.SafeErrorValue(err))
		} else {
			m.logger.Info(fmt.Sprintf("[%s] Stream request succeeded", types.SafeLogString(m.providerName)))
		}

		return stream, err
	}
}

func (m *ProviderLoggingMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return func(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Structured",
			fmt.Sprintf("model=%s, schema=%s", types.SafeLogString(request.Model), types.SafeLogString(request.SchemaName)),
			func(resp *types.StructuredResponse) string {
				return fmt.Sprintf("%d chars", len(resp.Raw))
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return func(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Embeddings",
			fmt.Sprintf("model=%s, inputs=%d", types.SafeLogString(request.Model), len(request.Input)),
			func(resp *types.EmbeddingsResponse) string {
				return fmt.Sprintf("%d embeddings", len(resp.Embeddings))
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return func(ctx context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Rerank",
			fmt.Sprintf("model=%s, documents=%d", types.SafeLogString(request.Model), len(request.Documents)),
			func(resp *types.RerankResponse) string {
				return fmt.Sprintf("%d results", len(resp.Results))
			},
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return func(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Audio",
			fmt.Sprintf("model=%s", types.SafeLogString(request.Model)),
			func(_ *types.AudioResponse) string { return "completed" },
			next, request,
		)
	}
}

func (m *ProviderLoggingMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return func(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
		return withProviderLogging(ctx, m.logger, m.providerName, "Image",
			fmt.Sprintf("model=%s, prompt_chars=%d", types.SafeLogString(request.Model), len(request.Prompt)),
			func(resp *types.ImageResponse) string {
				return fmt.Sprintf("%d images", len(resp.Images))
			},
			next, request,
		)
	}
}

// cleanJSONResponse provides basic JSON cleaning for provider responses
func cleanJSONResponse(jsonStr string) string {
	// This is a placeholder for more sophisticated JSON cleaning
	// In the future, this could implement provider-specific cleaning strategies

	// Remove common escape sequence issues
	cleaned := strings.ReplaceAll(jsonStr, `\\\\`, `\\`)
	cleaned = strings.ReplaceAll(cleaned, `\"`, `"`)

	// Remove leading/trailing whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}
