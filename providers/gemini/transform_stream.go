package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	providerstream "github.com/garyblankenship/wormhole/v2/providers/internal/stream"
	providerTransform "github.com/garyblankenship/wormhole/v2/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v2/types"
)

// processStreamCandidate extracts chunks from a candidate response
func (g *Gemini) processStreamCandidate(candidate candidate) []types.TextChunk {
	chunks := make([]types.TextChunk, 0, len(candidate.Content.Parts)+1)

	for idx, part := range candidate.Content.Parts {
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
					ID:               fmt.Sprintf("gemini-call-%d-%s", idx, part.FunctionCall.Name),
					Name:             part.FunctionCall.Name,
					Arguments:        part.FunctionCall.Args,
					ThoughtSignature: part.ThoughtSignature,
				},
				Model: "gemini",
			})
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
	if data == "" {
		return nil, false, nil
	}
	if strings.TrimSpace(data) == streamDoneMarker {
		return nil, true, nil // done
	}

	var response geminiTextResponse
	if err := json.Unmarshal([]byte(data), &response); err != nil {
		return nil, false, err
	}
	if response.Error != nil {
		return nil, false, g.ProviderError(response.Error.Message)
	}
	if len(response.Candidates) == 0 {
		if promptBlockReason(&response) != "" {
			return nil, false, g.noCandidatesError(&response)
		}
		return nil, false, nil
	}

	chunks := g.processStreamCandidate(response.Candidates[0])

	// usageMetadata is top-level on the Gemini response (not on the candidate)
	// and the non-streaming path reads it via convertUsage; the stream path
	// otherwise drops it. Append a usage-bearing chunk when present so streamed
	// consumers see token counts.
	if usage := convertUsage(response.UsageMetadata); usage != nil {
		chunks = append(chunks, types.TextChunk{
			Usage: usage,
			Model: "gemini",
		})
	}

	return chunks, false, nil
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
		sawEvent := false
		for scanner.Scan() {
			chunks, done, err := g.parseStreamEvent(scanner.Event().Data)
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
				sawEvent = true
				if chunk.FinishReason != nil {
					terminal = true
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
		if sawEvent && !terminal {
			select {
			case ch <- types.TextChunk{Error: fmt.Errorf("Gemini stream ended before terminal event")}:
			case <-ctx.Done():
			}
		}
	}()

	return ch
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
