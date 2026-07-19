package gemini

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// Stream sentinel value
const streamDoneMarker = "[DONE]"

// Gemini role mappings
const geminiRoleModel = "model"

// geminiThoughtSignatureSentinel is Gemini's documented dummy thoughtSignature that
// skips thought-signature validation. Emitted only for Gemini-3 targets on functionCall
// parts that carry no real signature (cross-provider or synthetic-repair calls), which
// would otherwise hard-400. Gemini 2.5 does not validate,
// so emit nothing there to keep currently-working wire bytes unchanged.
const geminiThoughtSignatureSentinel = "skip_thought_signature_validator"

// convertUsage converts Gemini usage metadata to types.Usage.
// Returns nil when metadata is absent.
func convertUsage(meta *usageMetadata) *types.Usage {
	if meta == nil {
		return nil
	}
	return &types.Usage{
		PromptTokens:     meta.PromptTokenCount,
		CompletionTokens: meta.CandidatesTokenCount + meta.ThoughtsTokenCount,
		TotalTokens:      meta.TotalTokenCount,
		CacheReadTokens:  meta.CachedContentTokenCount,
		ReasoningTokens:  meta.ThoughtsTokenCount,
	}
}

func (g *Gemini) noCandidatesError(response *geminiTextResponse) error {
	if reason := promptBlockReason(response); reason != "" {
		return g.ProviderErrorf("prompt blocked: %s", reason)
	}
	return g.ProviderError("no candidates in response")
}

func promptBlockReason(response *geminiTextResponse) string {
	if response.PromptFeedback == nil {
		return ""
	}
	return strings.TrimSpace(response.PromptFeedback.BlockReason)
}

// transformMessages converts types.Message to Gemini format. The model name is
// threaded through so the replay path can apply Gemini-3-specific thoughtSignature
// handling (see transformMessageToParts).
func (g *Gemini) transformMessages(messages []types.Message, model string) ([]map[string]any, error) {
	contents := make([]map[string]any, 0, len(messages))

	for _, msg := range messages {
		// Skip system messages — Gemini carries system text in the top-level
		// systemInstruction field (see buildTextPayload), not in contents.
		if msg.GetRole() == types.RoleSystem {
			continue
		}

		content := map[string]any{
			"role": g.mapRole(string(msg.GetRole())),
		}

		parts, err := g.transformMessageToParts(msg, model)
		if err != nil {
			return nil, err
		}

		content["parts"] = parts
		contents = append(contents, content)
	}

	return coalesceSameRole(contents), nil
}

// coalesceSameRole merges consecutive contents entries that share the same role
// into a single {role, parts} entry whose parts slice is the concatenation of
// the merged entries' parts. Gemini requires alternating roles, so a function
// turn (tool result) followed by a real user turn -- both mapping to a non-model
// role -- would otherwise emit two adjacent same-role entries and 400. Order is
// preserved; nothing is dropped.
func coalesceSameRole(contents []map[string]any) []map[string]any {
	if len(contents) <= 1 {
		return contents
	}
	merged := make([]map[string]any, 0, len(contents))
	for _, c := range contents {
		if n := len(merged); n > 0 && merged[n-1]["role"] == c["role"] {
			prevParts, _ := merged[n-1]["parts"].([]map[string]any)
			curParts, _ := c["parts"].([]map[string]any)
			merged[n-1]["parts"] = append(prevParts, curParts...)
			continue
		}
		merged = append(merged, c)
	}
	return merged
}

// mapRole maps message roles to Gemini roles
func (g *Gemini) mapRole(role string) string {
	switch role {
	case "system":
		return geminiRoleModel
	case "assistant":
		return geminiRoleModel
	case "tool":
		return "function"
	default:
		return "user"
	}
}

// transformMessageToParts converts a message to Gemini parts. model is the target
// Gemini model name, used to decide Gemini-3 sentinel thoughtSignature handling.
func (g *Gemini) transformMessageToParts(msg types.Message, model string) ([]map[string]any, error) {
	var parts []map[string]any

	switch m := msg.(type) {
	case *types.UserMessage:
		if m.Content != "" {
			parts = append(parts, map[string]any{"text": m.Content})
		}

		// Handle media
		for _, media := range m.Media {
			part, err := g.transformMedia(media)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}

	case *types.AssistantMessage:
		if m.Content != "" {
			parts = append(parts, map[string]any{"text": m.Content})
		}

		// Handle tool calls
		for _, toolCall := range m.ToolCalls {
			p := map[string]any{
				"functionCall": map[string]any{
					"name": toolCall.Name,
					"args": toolCall.Arguments,
				},
			}
			switch {
			case toolCall.ThoughtSignature != "":
				p["thoughtSignature"] = toolCall.ThoughtSignature
			case strings.HasPrefix(model, "gemini-3"):
				// Gemini 3 hard-400s on a functionCall part with no thoughtSignature.
				// Cross-provider (OpenAI/Anthropic) or synthetic-repair calls have none;
				// the documented sentinel skips validation. Gemini 2.5 does not validate,
				// so emit nothing there to keep currently-working wire bytes unchanged.
				p["thoughtSignature"] = geminiThoughtSignatureSentinel
			}
			parts = append(parts, p)
		}

	case *types.ToolResultMessage:
		fnName := m.FunctionName
		if fnName == "" {
			fnName = geminiCallName(m.ToolCallID)
		}
		response := map[string]any{}
		if m.Error != "" {
			// Gemini surfaces tool failures via response.error; without it the
			// error text would be parsed as a successful response.result.
			response["error"] = map[string]any{"message": m.Error}
		} else {
			// Content is often already a JSON string; parse it so Gemini receives a
			// structured object under response.result instead of a re-escaped string.
			// Non-JSON content (plain text, empty) passes through verbatim as before.
			var result any
			if err := json.Unmarshal([]byte(m.Content), &result); err != nil {
				result = m.Content
			}
			response["result"] = result
		}
		parts = append(parts, map[string]any{
			"functionResponse": map[string]any{
				"name":     fnName,
				"response": response,
			},
		})
	case *types.SystemMessage:
		parts = append(parts, map[string]any{"text": m.Content})

	default:
		return nil, g.ProviderErrorf("unsupported message type: %T", msg)
	}

	return parts, nil
}

// transformMedia converts media to Gemini format
func (g *Gemini) transformMedia(media types.Media) (map[string]any, error) {
	switch m := media.(type) {
	case *types.ImageMedia:
		data := m.Base64Data
		if data == "" && len(m.Data) > 0 {
			data = base64.StdEncoding.EncodeToString(m.Data)
		}
		if data == "" {
			if m.URL != "" {
				return nil, g.ValidationError("Gemini requires inline image data", "URL-only images are not supported")
			}
			return nil, g.ValidationError("Gemini requires inline image data")
		}

		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": m.MimeType,
				"data":     data,
			},
		}, nil

	case *types.DocumentMedia:
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": m.MimeType,
				"data":     base64.StdEncoding.EncodeToString(m.Data),
			},
		}, nil

	default:
		return nil, g.ProviderErrorf("unsupported media type: %T", media)
	}
}
