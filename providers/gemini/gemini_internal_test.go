package gemini

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestTransformToolChoiceActual(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	assert.Nil(t, provider.transformToolChoice(nil))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{"mode": "AUTO"},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAuto}))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{"mode": "NONE"},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeNone}))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{"mode": "ANY"},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAny}))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{
			"mode":                 "ANY",
			"allowedFunctionNames": []string{"lookup"},
		},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"}))
	assert.Nil(t, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceType("unknown")}))
}

func TestSchemaToMapActualSchemaTypes(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})
	minLength := 2
	maxLength := 12
	minimum := 1.5
	maximum := 99.5

	schema := &types.ObjectSchema{
		BaseSchema: types.BaseSchema{Type: "object"},
		Required:   []string{"name"},
		Properties: map[string]types.SchemaInterface{
			"name": &types.StringSchema{
				BaseSchema: types.BaseSchema{Type: "string", Description: "display name"},
				MinLength:  &minLength,
				MaxLength:  &maxLength,
				Pattern:    "^[a-z]+$",
			},
			"score": &types.NumberSchema{
				BaseSchema: types.BaseSchema{Type: "number"},
				Minimum:    &minimum,
				Maximum:    &maximum,
			},
			"tags": &types.ArraySchema{
				BaseSchema: types.BaseSchema{Type: "array"},
				Items: &types.EnumSchema{
					BaseSchema: types.BaseSchema{Type: "string"},
					Enum:       []any{"new", "known"},
				},
			},
		},
	}

	assert.Equal(t, map[string]any{
		"type":        "string",
		"description": "display name",
	}, provider.schemaInterfaceToMap(schema.Properties["name"]))

	result := map[string]any{}
	provider.objectSchemaToMap(schema, result)
	assert.Equal(t, []string{"name"}, result["required"])
	properties := result["properties"].(map[string]any)

	nameResult := map[string]any{}
	provider.stringSchemaToMap(schema.Properties["name"].(*types.StringSchema), nameResult)
	assert.Equal(t, map[string]any{
		"type":      "string",
		"minLength": minLength,
		"maxLength": maxLength,
		"pattern":   "^[a-z]+$",
	}, nameResult)

	scoreResult := map[string]any{}
	provider.numberSchemaToMap(schema.Properties["score"].(*types.NumberSchema), scoreResult)
	assert.Equal(t, map[string]any{
		"type":    "number",
		"minimum": minimum,
		"maximum": maximum,
	}, scoreResult)

	assert.Equal(t, map[string]any{
		"type":      "string",
		"minLength": minLength,
		"maxLength": maxLength,
		"pattern":   "^[a-z]+$",
	}, properties["name"])
	assert.Equal(t, map[string]any{
		"type":    "number",
		"minimum": minimum,
		"maximum": maximum,
	}, properties["score"])
	assert.Equal(t, map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
			"enum": []any{"new", "known"},
		},
	}, properties["tags"])

	assert.Equal(t, map[string]any{
		"type": "string",
		"enum": []any{"new", "known"},
	}, provider.schemaTypeToMap(&types.EnumSchema{Enum: []any{"new", "known"}}))
	assert.Equal(t, map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
			"enum": []any{"new", "known"},
		},
	}, provider.schemaTypeToMap(&types.ArraySchema{
		Items: &types.EnumSchema{
			BaseSchema: types.BaseSchema{Type: "string"},
			Enum:       []any{"new", "known"},
		},
	}))
}

func TestTransformTextResponse_SyntheticToolCallIDs(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	// Failure mode under test: the same function is called twice in one
	// turn. With name-as-ID the two IDs collided; synthetic IDs must differ.
	resp := &geminiTextResponse{
		Candidates: []candidate{
			{
				Content: content{
					Parts: []part{
						{FunctionCall: &functionCall{Name: "get_weather", Args: map[string]any{"city": "NYC"}}},
						{FunctionCall: &functionCall{Name: "get_weather", Args: map[string]any{"city": "LA"}}},
					},
				},
			},
		},
	}

	out, err := provider.transformTextResponse(resp)
	assert.NoError(t, err)
	assert.Len(t, out.ToolCalls, 2)

	assert.Equal(t, "get_weather", out.ToolCalls[0].Name)
	assert.Equal(t, "get_weather", out.ToolCalls[1].Name)
	assert.Equal(t, "gemini-call-0-get_weather", out.ToolCalls[0].ID)
	assert.Equal(t, "gemini-call-1-get_weather", out.ToolCalls[1].ID)
	assert.NotEqual(t, out.ToolCalls[0].ID, out.ToolCalls[1].ID)
}

func TestTransformResponses_SurfacePromptBlockReason(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})
	response := &geminiTextResponse{
		PromptFeedback: &promptFeedback{BlockReason: "SAFETY"},
	}

	tests := []struct {
		name      string
		transform func() error
	}{
		{
			name: "text",
			transform: func() error {
				_, err := provider.transformTextResponse(response)
				return err
			},
		},
		{
			name: "structured",
			transform: func() error {
				_, err := provider.transformStructuredResponse(response, nil)
				return err
			},
		},
		{
			name: "images",
			transform: func() error {
				_, err := provider.transformImagesResponse(response, "gemini-test")
				return err
			},
		},
		{
			name: "stream",
			transform: func() error {
				_, _, err := provider.parseStreamEvent(`{"promptFeedback":{"blockReason":"SAFETY"}}`)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.transform()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "SAFETY")
			assert.NotContains(t, err.Error(), "no candidates")
			providerErr, ok := types.AsWormholeError(err)
			require.True(t, ok)
			assert.Equal(t, types.ErrorCodeProvider, providerErr.Code)
			assert.Equal(t, "gemini", providerErr.Provider)
		})
	}
}

func TestProcessStreamCandidate_SyntheticToolCallIDs(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	cand := candidate{
		Content: content{
			Parts: []part{
				{FunctionCall: &functionCall{Name: "lookup", Args: map[string]any{"q": "a"}}},
				{FunctionCall: &functionCall{Name: "lookup", Args: map[string]any{"q": "b"}}},
			},
		},
	}

	chunks := provider.processStreamCandidate(cand)

	var ids []string
	for _, c := range chunks {
		if c.ToolCall != nil {
			ids = append(ids, c.ToolCall.ID)
			assert.Equal(t, "lookup", c.ToolCall.Name)
		}
	}
	assert.Equal(t, []string{"gemini-call-0-lookup", "gemini-call-1-lookup"}, ids)
}

func TestTransformTextResponse_ThoughtPartsRouteToThinking(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})
	resp := &geminiTextResponse{
		Candidates: []candidate{
			{
				Content: content{
					Parts: []part{
						{Text: "reasoning", Thought: true},
						{Text: "answer"},
					},
				},
			},
		},
	}

	result, err := provider.transformTextResponse(resp)
	assert.NoError(t, err)
	assert.Equal(t, "answer", result.Text)
	if assert.NotNil(t, result.Thinking) {
		assert.Equal(t, "reasoning", result.Thinking.Content)
	}
}

func TestTransformTextResponse_PreservesReasoningOnlyUsage(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})
	resp := &geminiTextResponse{
		Candidates: []candidate{
			{
				Content:      content{Parts: []part{}},
				FinishReason: "MAX_TOKENS",
			},
		},
		UsageMetadata: &usageMetadata{
			PromptTokenCount:     187,
			ThoughtsTokenCount:   2045,
			CandidatesTokenCount: 0,
			TotalTokenCount:      2232,
		},
	}

	result, err := provider.transformTextResponse(resp)
	assert.NoError(t, err)
	assert.Empty(t, result.Text)
	assert.Equal(t, types.FinishReasonLength, result.FinishReason)
	if assert.NotNil(t, result.Usage) {
		assert.Equal(t, 187, result.Usage.PromptTokens)
		assert.Equal(t, 2045, result.Usage.CompletionTokens)
		assert.Equal(t, 2045, result.Usage.ReasoningTokens)
		assert.Equal(t, 2232, result.Usage.TotalTokens)
	}
}

func TestParseStreamEvent_ThoughtPartsRouteToThinkingChunks(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	chunks, done, err := provider.parseStreamEvent(`{"candidates":[{"content":{"parts":[{"text":"reasoning","thought":true},{"text":"answer"}],"role":"model"},"finishReason":"STOP"}]}`)
	assert.NoError(t, err)
	assert.False(t, done)

	var thinkingFound bool
	var textChunkFound bool
	for _, chunk := range chunks {
		if chunk.Thinking != nil {
			thinkingFound = true
			assert.Equal(t, "reasoning", chunk.Thinking.Content)
		}
		if chunk.Text != "" {
			assert.NotContains(t, chunk.Text, "reasoning")
			if chunk.Text == "answer" {
				textChunkFound = true
			}
		}
	}
	assert.True(t, thinkingFound)
	assert.True(t, textChunkFound)
}

func TestNormalizeModelResource(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"google/gemini-3-pro-preview":  "gemini-3-pro-preview",
		"models/gemini-2.5-flash":      "gemini-2.5-flash",
		"gemini-2.5-flash":             "gemini-2.5-flash",
		"google/models/gemini-2.5-pro": "gemini-2.5-pro",
		"models/foo?bar#baz":           url.PathEscape("foo?bar#baz"),
		"google/../../etc/passwd":      url.PathEscape("../../etc/passwd"),
	}
	for in, want := range cases {
		if got := normalizeModelResource(in); got != want {
			t.Errorf("normalizeModelResource(%q) = %q, want %q", in, got, want)
		}
	}
}

// FIX (coalesce): consecutive messages mapping to the same Gemini role must
// merge into ONE {role, parts} entry. Two consecutive user messages (both
// mapping to "user") must NOT produce two adjacent same-role entries (Gemini
// 400s on non-alternating roles).
func TestTransformMessages_CoalescesConsecutiveSameRole(t *testing.T) {
	t.Parallel()
	g := &Gemini{}
	msgs := []types.Message{
		types.NewUserMessage("first user message"),
		types.NewUserMessage("second user message"),
	}

	out, err := g.transformMessages(msgs, "gemini-2.5-flash")
	require.NoError(t, err)

	require.Len(t, out, 1, "two consecutive user messages must merge into one user turn")
	assert.Equal(t, "user", out[0]["role"])

	mergedParts, ok := out[0]["parts"].([]map[string]any)
	require.True(t, ok, "merged parts must be []map[string]any")
	require.Len(t, mergedParts, 2, "both user text parts must be present in the single merged turn")
	assert.Equal(t, "first user message", mergedParts[0]["text"])
	assert.Equal(t, "second user message", mergedParts[1]["text"])
}

func TestTransformStructuredResponse_ThoughtPartsExcludedFromJSON(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	// Simulate a Gemini thinking-model structured-output response:
	// a thought part (prose) followed by a JSON-answer part.
	resp := &geminiTextResponse{
		Candidates: []candidate{
			{
				Content: content{
					Parts: []part{
						{Text: "Let me think about the structure needed...", Thought: true},
						{Text: `{"name":"Alice","age":30}`},
					},
				},
			},
		},
	}

	result, err := provider.transformStructuredResponse(resp, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	// The parsed data must match the JSON part only (thought prose excluded).
	data, ok := result.Data.(map[string]any)
	require.True(t, ok, "parsed data must be a map")
	assert.Equal(t, "Alice", data["name"])
	assert.Equal(t, float64(30), data["age"])

	// Raw must contain only the JSON part (no thought prose).
	assert.Equal(t, `{"name":"Alice","age":30}`, result.Raw)
}
