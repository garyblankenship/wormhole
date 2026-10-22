package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	providerstream "github.com/garyblankenship/wormhole/v3/providers/internal/stream"
	providerTransform "github.com/garyblankenship/wormhole/v3/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v3/types"
)

// processStreamCandidate extracts chunks from a candidate response
func (g *Gemini) processStreamCandidate(candidate candidate) []types.TextChunk {
	callIndex := 0
	return g.processStreamCandidateWithIndex(candidate, &callIndex)
}

// processStreamCandidateWithIndex uses a request-scoped call counter. Gemini
// does not provide function-call IDs, and each SSE frame resets its part
// indexes, so the counter must live for the whole stream rather than one
// candidate frame.
func (g *Gemini) processStreamCandidateWithIndex(candidate candidate, callIndex *int) []types.TextChunk {
	chunks := make([]types.TextChunk, 0, len(candidate.Content.Parts)+1)

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			if part.Thought {
				chunks = append(chunks, types.TextChunk{
					Thinking: &types.Thinking{Content: part.Text},
					Model:    "gemini",
				})
			} else {
				chunks = append(chunks, types.TextChunk{
					Text:  part.Text,
					Model: "gemini",
				})
			}
		}
		if part.FunctionCall != nil {
			// Synthetic unique-per-part ID (Gemini provides none); see
			// transformTextResponse for rationale.
			chunks = append(chunks, types.TextChunk{
				ToolCall: &types.ToolCall{
					ID:               fmt.Sprintf("gemini-call-%d-%s", *callIndex, part.FunctionCall.Name),
					Name:             part.FunctionCall.Name,
					Arguments:        part.FunctionCall.Args,
					ThoughtSignature: part.ThoughtSignature,
				},
				Model: "gemini",
			})
			*callIndex++
		}
	}

	if candidate.FinishReason != "" {
		finishReason := providerTransform.MapFinishReason(candidate.FinishReason)
		chunks = append(chunks, types.TextChunk{
			FinishReason: &finishReason,
			Model:        "gemini",
		})
	}

	return chunks
}

// parseStreamEvent parses an SSE event and returns chunks or an error
func (g *Gemini) parseStreamEvent(data string) ([]types.TextChunk, bool, error) {
	callIndex := 0
	return g.parseStreamEventWithIndex(data, &callIndex)
}

// handleStream processes streaming responses. Every send is guarded by
// ctx.Done() so the goroutine exits and the body closes if the consumer
// stops reading.
func (g *Gemini) handleStream(ctx context.Context, stream io.ReadCloser) <-chan types.TextChunk {
	ch := make(chan types.TextChunk)

	go func() {
		defer close(ch)
		defer func() {
			_ = stream.Close()
		}()

		scanner := providerstream.NewSSEScanner(stream)
		terminal := false
		callIndex := 0
		refs := &citationSet{}
		var text strings.Builder
		var annotations []any
		for scanner.Scan() {
			data := scanner.Event().Data
			chunks, done, err := g.parseStreamEventWithIndex(data, &callIndex)
			var frame geminiTextResponse
			if json.Unmarshal([]byte(data), &frame) == nil && len(frame.Candidates) > 0 {
				for _, part := range frame.Candidates[0].Content.Parts {
					if !part.Thought {
						text.WriteString(part.Text)
					}
				}
				oldSources, oldCitations, oldWarnings := len(refs.sources), len(refs.citations), len(refs.warnings)
				refs.consume(frame.Candidates[0], text.String())
				if frame.Candidates[0].GroundingMetadata != nil || frame.Candidates[0].CitationMetadata != nil {
					annotations = append(annotations, map[string]any{"groundingMetadata": frame.Candidates[0].GroundingMetadata, "citationMetadata": frame.Candidates[0].CitationMetadata})
				}
				if len(chunks) > 0 {
					chunks[0].Sources = types.CloneSources(refs.sources[oldSources:])
					chunks[0].Citations = types.CloneCitations(refs.citations[oldCitations:])
					chunks[0].Warnings = refs.warnings[oldWarnings:]
				}
			}
			if err != nil {
				select {
				case ch <- types.TextChunk{Error: err}:
				case <-ctx.Done():
				}
				return
			}
			if done {
				return
			}
			for _, chunk := range chunks {
				if chunk.FinishReason != nil {
					terminal = true
					chunk.Sources = types.CloneSources(refs.sources)
					chunk.Citations = types.CloneCitations(refs.citations)
					if len(annotations) > 0 {
						chunk.Metadata = map[string]any{"provider_annotations": types.CloneValue(annotations)}
					}
					chunk.Warnings = append([]types.ResponseWarning(nil), refs.warnings...)
				}
				select {
				case ch <- chunk:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case ch <- types.TextChunk{Error: err}:
			case <-ctx.Done():
			}
			return
		}
		if !terminal {
			select {
			case ch <- types.TextChunk{Error: fmt.Errorf("Gemini stream ended before terminal event")}:
			case <-ctx.Done():
			}
		}
	}()

	return ch
}

func (g *Gemini) parseStreamEventWithIndex(data string, callIndex *int) ([]types.TextChunk, bool, error) {
	if data == "" {
		return nil, false, nil
	}
	if strings.TrimSpace(data) == streamDoneMarker {
		return nil, true, nil
	}
	var response geminiTextResponse
	if err := json.Unmarshal([]byte(data), &response); err != nil {
		return nil, false, err
	}
	if response.Error != nil {
		return nil, false, g.ProviderError(response.Error.Message)
	}
	if len(response.Candidates) == 0 && promptBlockReason(&response) != "" {
		return nil, false, g.noCandidatesError(&response)
	}
	// Usage can appear without a candidate in a trailing frame.
	var chunks []types.TextChunk
	if len(response.Candidates) > 0 {
		chunks = g.processStreamCandidateWithIndex(response.Candidates[0], callIndex)
	}

	if usage := convertUsage(response.UsageMetadata); usage != nil {
		chunks = append(chunks, types.TextChunk{Usage: usage, Model: "gemini", Metadata: map[string]any{"provider_usage": providerUsage(response.UsageMetadata)}})
	}
	return chunks, false, nil
}

// geminiCallName recovers the function name from a synthetic
// "gemini-call-<idx>-<name>" tool-call ID (the format minted when adapting a
// Gemini functionCall part above). Returns id unchanged when it is not in that
// format. Fallback for a ToolResultMessage that carries no explicit FunctionName
// (e.g. a manually-constructed multi-turn result echoing the synthesized ID).
func geminiCallName(id string) string {
	rest, ok := strings.CutPrefix(id, "gemini-call-")
	if !ok {
		return id
	}
	if _, name, found := strings.Cut(rest, "-"); found && name != "" {
		return name
	}
	return id
}
