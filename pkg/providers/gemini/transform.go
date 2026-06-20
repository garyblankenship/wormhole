package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
	providerTransform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
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
		CompletionTokens: meta.CandidatesTokenCount,
		TotalTokens:      meta.TotalTokenCount,
		CacheReadTokens:  meta.CachedContentTokenCount,
	}
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

	return contents, nil
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
		parts = append(parts, map[string]any{
			"functionResponse": map[string]any{
				"name": m.ToolCallID,
				"response": map[string]any{
					"result": m.Content,
				},
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

// transformTools converts tools to Gemini format
func (g *Gemini) transformTools(tools []types.Tool) []map[string]any {
	// Use shared RequestBuilder for common tool transformation
	standardTools := g.requestBuilder.TransformTools(tools)

	// Adapt to Gemini-specific format: extract function declarations
	functions := make([]map[string]any, 0, len(standardTools))
	for _, stdTool := range standardTools {
		if toolFunc, ok := stdTool["function"].(map[string]any); ok {
			// Extract name, description, parameters from standard tool format
			function := map[string]any{
				"name":        toolFunc["name"],
				"description": toolFunc["description"],
			}

			// Handle parameters - ensure type: "object" if missing
			if params, ok := toolFunc["parameters"].(map[string]any); ok {
				params = g.transformToolSchema(params)
				function["parameters"] = params
			}

			functions = append(functions, function)
		}
	}

	// Wrap functions in Gemini format
	var geminiTools []map[string]any
	if len(functions) > 0 {
		geminiTools = append(geminiTools, map[string]any{
			"functionDeclarations": functions,
		})
	}

	return geminiTools
}

// transformToolSchema converts tool schema to Gemini format
func (g *Gemini) transformToolSchema(schema map[string]any) map[string]any {
	// Gemini expects JSON Schema format
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	normalizeSchemaMap(schema)
	return schema
}

// transformToolChoice converts tool choice to Gemini format
func (g *Gemini) transformToolChoice(choice *types.ToolChoice) map[string]any {
	if choice == nil {
		return nil
	}

	switch choice.Type {
	case types.ToolChoiceTypeAuto:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "AUTO",
			},
		}
	case types.ToolChoiceTypeNone:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "NONE",
			},
		}
	case types.ToolChoiceTypeAny:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "ANY",
			},
		}
	case types.ToolChoiceTypeSpecific:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode":                 "ANY",
				"allowedFunctionNames": []string{choice.ToolName},
			},
		}
	default:
		return nil
	}
}

// transformSchema converts a types.Schema to Gemini schema format
func (g *Gemini) transformSchema(schema types.Schema) map[string]any {
	return g.schemaToMap(schema)
}

// normalizeSchemaMap rewrites JSON Schema union types into Gemini-compatible form,
// in place and recursively. Gemini/Vertex reject an array-valued `type`:
//
//	["T","null"]   -> {type:"T", nullable:true}
//	["A","B",...]  -> {anyOf:[{type:"A"},{type:"B"},...] } (+ nullable:true if "null" present)
//	["T"]          -> {type:"T"}
//
// It recurses into properties, items, and anyOf/oneOf/allOf/$defs/definitions.
func normalizeSchemaMap(m map[string]any) {
	if m == nil {
		return
	}
	if raw, ok := m["type"]; ok {
		if list, ok := typeStringList(raw); ok {
			seen := map[string]bool{}
			nonNull := make([]string, 0, len(list))
			hasNull := false
			for _, t := range list {
				if t == "null" {
					hasNull = true
					continue
				}
				if !seen[t] {
					seen[t] = true
					nonNull = append(nonNull, t)
				}
			}
			switch {
			case len(nonNull) == 1:
				m["type"] = nonNull[0]
				if hasNull {
					m["nullable"] = true
				}
			case len(nonNull) > 1:
				branches := make([]any, 0, len(nonNull))
				for _, t := range nonNull {
					branches = append(branches, map[string]any{"type": t})
				}
				delete(m, "type")
				m["anyOf"] = branches
				if hasNull {
					m["nullable"] = true
				}
			case hasNull:
				m["type"] = "null"
			}
		}
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for _, v := range props {
			if sub, ok := v.(map[string]any); ok {
				normalizeSchemaMap(sub)
			}
		}
	}
	if items, ok := m["items"].(map[string]any); ok {
		normalizeSchemaMap(items)
	}
	if itemsList, ok := m["items"].([]any); ok {
		for _, v := range itemsList {
			if sub, ok := v.(map[string]any); ok {
				normalizeSchemaMap(sub)
			}
		}
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := m[key].([]any); ok {
			for _, v := range arr {
				if sub, ok := v.(map[string]any); ok {
					normalizeSchemaMap(sub)
				}
			}
		}
	}
	for _, key := range []string{"$defs", "definitions"} {
		if defs, ok := m[key].(map[string]any); ok {
			for _, v := range defs {
				if sub, ok := v.(map[string]any); ok {
					normalizeSchemaMap(sub)
				}
			}
		}
	}
}

// typeStringList coerces a JSON Schema `type` value that may be []any (post-unmarshal)
// or []string into []string. Returns ok=false for a plain string or other shapes.
func typeStringList(v any) ([]string, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// schemaToMap recursively converts schema to map
func (g *Gemini) schemaToMap(schema types.Schema) map[string]any {
	// Handle raw JSON bytes
	if bytes, ok := schema.([]byte); ok {
		var result map[string]any
		if err := json.Unmarshal(bytes, &result); err == nil {
			normalizeSchemaMap(result)
			return result
		}
	}

	// Concrete schema pointer types (*ObjectSchema/*ArraySchema/*EnumSchema/
	// *NumberSchema/*StringSchema) carry full fidelity (properties/required/
	// items/enum) and MUST be converted here FIRST. They also satisfy
	// SchemaInterface, so the lossy interface branch below would otherwise win
	// and flatten them to {type,description} — gutting the schema Gemini sees.
	switch schema.(type) {
	case *types.ObjectSchema, *types.ArraySchema, *types.EnumSchema,
		*types.NumberSchema, *types.StringSchema:
		return g.schemaTypeToMap(schema)
	}

	// Fallback: a SchemaInterface implementation that is NOT one of the concrete
	// types above (only type+description available).
	if schemaIface, ok := schema.(types.SchemaInterface); ok {
		return g.schemaInterfaceToMap(schemaIface)
	}

	// Final fallback for any other concrete type handled by schemaTypeToMap.
	return g.schemaTypeToMap(schema)
}

// schemaInterfaceToMap converts a SchemaInterface to map
func (g *Gemini) schemaInterfaceToMap(schemaIface types.SchemaInterface) map[string]any {
	result := map[string]any{
		"type": schemaIface.GetType(),
	}
	if desc := schemaIface.GetDescription(); desc != "" {
		result["description"] = desc
	}
	return result
}

// schemaTypeToMap handles specific schema types
func (g *Gemini) schemaTypeToMap(schema types.Schema) map[string]any {
	result := map[string]any{}

	switch s := schema.(type) {
	case *types.ObjectSchema:
		g.objectSchemaToMap(s, result)
	case *types.ArraySchema:
		result["type"] = "array"
		result["items"] = g.schemaToMap(s.Items)
	case *types.EnumSchema:
		// Enum element type varies (string/number); prefer the declared type,
		// fall back to "string" when unset so Gemini always sees a type.
		if t := s.GetType(); t != "" {
			result["type"] = t
		} else {
			result["type"] = "string"
		}
		result["enum"] = s.Enum
	case *types.NumberSchema:
		g.numberSchemaToMap(s, result)
	case *types.StringSchema:
		g.stringSchemaToMap(s, result)
	}

	return result
}

// objectSchemaToMap populates result map from ObjectSchema
func (g *Gemini) objectSchemaToMap(s *types.ObjectSchema, result map[string]any) {
	result["type"] = "object"
	properties := make(map[string]any)
	for name, prop := range s.Properties {
		properties[name] = g.schemaToMap(prop)
	}
	result["properties"] = properties
	if len(s.Required) > 0 {
		result["required"] = s.Required
	}
}

// numberSchemaToMap populates result map from NumberSchema
func (g *Gemini) numberSchemaToMap(s *types.NumberSchema, result map[string]any) {
	result["type"] = "number"
	if s.Minimum != nil {
		result["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		result["maximum"] = *s.Maximum
	}
}

// stringSchemaToMap populates result map from StringSchema
func (g *Gemini) stringSchemaToMap(s *types.StringSchema, result map[string]any) {
	result["type"] = "string"
	if s.MinLength != nil {
		result["minLength"] = *s.MinLength
	}
	if s.MaxLength != nil {
		result["maxLength"] = *s.MaxLength
	}
	if s.Pattern != "" {
		result["pattern"] = s.Pattern
	}
}

// transformTextResponse converts Gemini response to types.TextResponse
func (g *Gemini) transformTextResponse(response *geminiTextResponse) (*types.TextResponse, error) {
	if response.Error != nil {
		return nil, g.ProviderError(response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return nil, g.ProviderError("no candidates in response")
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

	// Add metadata
	result.Metadata = map[string]any{
		"provider": "gemini",
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
		return nil, g.ProviderError("no candidates in response")
	}

	candidate := response.Candidates[0]

	// Extract text (should be JSON)
	var text string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
	}

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
		Data: data,
		Raw:  text,
	}

	result.Usage = convertUsage(response.UsageMetadata)

	// Add metadata
	result.Metadata = map[string]any{
		"provider": "gemini",
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
		return nil, g.ProviderError("no candidates in response")
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

		scanner := utils.NewSSEScanner(stream)
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
		}
	}()

	return ch
}
