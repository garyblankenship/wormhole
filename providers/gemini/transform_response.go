package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	providerTransform "github.com/garyblankenship/wormhole/v3/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v3/types"
)

// transformTextResponse converts Gemini response to types.TextResponse
func (g *Gemini) transformTextResponse(response *geminiTextResponse) (*types.TextResponse, error) {
	if response.Error != nil {
		return nil, g.ProviderError(response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return nil, g.noCandidatesError(response)
	}

	candidate := response.Candidates[0]

	// Extract text and tool calls
	var text string
	var thinking string
	var toolCalls []types.ToolCall

	for idx, part := range candidate.Content.Parts {
		if part.Text != "" {
			if part.Thought {
				thinking += part.Text
			} else {
				text += part.Text
			}
		}
		if part.FunctionCall != nil {
			// Gemini provides no tool-call IDs and the function name alone
			// collides when the same function is called twice in one turn.
			// Synthesize a unique-per-part ID so tool results map correctly.
			toolCalls = append(toolCalls, types.ToolCall{
				ID:               fmt.Sprintf("gemini-call-%d-%s", idx, part.FunctionCall.Name),
				Name:             part.FunctionCall.Name,
				Arguments:        part.FunctionCall.Args,
				ThoughtSignature: part.ThoughtSignature,
			})
		}
	}

	finishReason := providerTransform.MapFinishReason(candidate.FinishReason)

	result := &types.TextResponse{
		Text:         text,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}

	if thinking != "" {
		result.Thinking = &types.Thinking{Content: thinking}
	}

	result.Usage = convertUsage(response.UsageMetadata)
	refs := &citationSet{}
	refs.consume(candidate, text)
	result.Sources, result.Citations, result.Warnings = refs.sources, refs.citations, refs.warnings

	// Add metadata
	result.Metadata = map[string]any{
		"provider":       "gemini",
		"provider_usage": providerUsage(response.UsageMetadata),
	}

	if candidate.CitationMetadata != nil {
		result.Metadata["citationMetadata"] = candidate.CitationMetadata
	}
	if candidate.GroundingMetadata != nil {
		result.Metadata["groundingMetadata"] = candidate.GroundingMetadata
	}

	return result, nil
}

// transformStructuredResponse converts Gemini response to types.StructuredResponse
func (g *Gemini) transformStructuredResponse(response *geminiTextResponse, schema types.Schema) (*types.StructuredResponse, error) {
	if response.Error != nil {
		return nil, g.ProviderError(response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return nil, g.noCandidatesError(response)
	}

	candidate := response.Candidates[0]

	base, err := g.transformTextResponse(response)
	if err != nil {
		return nil, err
	}
	if len(base.ToolCalls) > 0 {
		return &types.StructuredResponse{Provider: g.Name(), ToolCalls: base.ToolCalls, FinishReason: base.FinishReason, Usage: base.Usage, Metadata: base.Metadata, Sources: base.Sources, Citations: base.Citations, Warnings: base.Warnings}, nil
	}
	text := base.Text

	// Parse JSON
	var data any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return nil, g.RequestError("failed to parse structured response", err)
	}

	// Validate against schema if it implements SchemaInterface
	if schemaIface, ok := schema.(types.SchemaInterface); ok {
		if err := schemaIface.Validate(data); err != nil {
			return nil, g.RequestError("response validation failed", err)
		}
	}

	result := &types.StructuredResponse{
		Data:         data,
		Raw:          text,
		Provider:     g.Name(),
		FinishReason: base.FinishReason,
	}

	result.Usage = convertUsage(response.UsageMetadata)
	refs := &citationSet{}
	refs.consume(candidate, text)
	result.Sources, result.Citations, result.Warnings = refs.sources, refs.citations, refs.warnings

	// Add metadata
	result.Metadata = map[string]any{
		"provider":       "gemini",
		"provider_usage": providerUsage(response.UsageMetadata),
	}

	return result, nil
}

// transformEmbeddingsResponse converts Gemini response to types.EmbeddingsResponse
func (g *Gemini) transformEmbeddingsResponse(response *geminiEmbeddingsResponse, requestModel string) *types.EmbeddingsResponse {
	embeddings := make([]types.Embedding, 0, len(response.Embeddings))

	for i, emb := range response.Embeddings {
		embeddings = append(embeddings, types.Embedding{
			Index:     i,
			Embedding: emb.Values,
		})
	}

	return &types.EmbeddingsResponse{
		Model:      requestModel,
		Embeddings: embeddings,
		Metadata: map[string]any{
			"provider": "gemini",
		},
	}
}

func (g *Gemini) transformImagesResponse(response *geminiTextResponse, model string) (*types.ImagesResponse, error) {
	if response.Error != nil {
		return nil, g.ProviderError(response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return nil, g.noCandidatesError(response)
	}

	var text strings.Builder
	var images []types.GeneratedImage
	var mimeTypes []string
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
			}
			if part.InlineData != nil && part.InlineData.Data != "" {
				images = append(images, types.GeneratedImage{
					B64JSON: part.InlineData.Data,
				})
				mimeTypes = append(mimeTypes, part.InlineData.MimeType)
			}
		}
	}
	if len(images) == 0 {
		return nil, g.ProviderError("no images in response")
	}

	metadata := map[string]any{
		"provider":   "gemini",
		"mime_types": mimeTypes,
	}
	if text.Len() > 0 {
		metadata["text"] = text.String()
	}

	return &types.ImagesResponse{
		Model:    model,
		Images:   images,
		Created:  time.Now(),
		Metadata: metadata,
	}, nil
}
