package wormhole

import (
	"context"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestTextRequestBuilderConfiguration(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	tool := *types.NewTool("lookup", "Lookup data", map[string]any{"type": "object"})
	builder := client.Text().
		Using("openai").
		BaseURL("https://example.test/v1").
		Model("gpt-5").
		Messages(types.NewUserMessage("hello")).
		AddMessage(types.NewAssistantMessage("hi")).
		SystemPrompt("system").
		Temperature(0.5).
		TopP(0.9).
		MaxTokens(128).
		Stop("END").
		Tools(tool).
		ToolChoice("auto").
		ResponseFormat(map[string]string{"type": "json_object"}).
		ProviderOptions(map[string]any{"trace": true}).
		WithToolsEnabled().
		WithMaxToolIterations(3).
		WithFallback("gpt-5-mini")

	if builder.getProvider() != "openai" || builder.getBaseURL() != "https://example.test/v1" {
		t.Fatalf("builder routing = (%q, %q)", builder.getProvider(), builder.getBaseURL())
	}
	if builder.request.Model != "gpt-5" || len(builder.request.Messages) != 2 {
		t.Fatalf("request = %#v", builder.request)
	}
	if builder.request.SystemPrompt != "system" || *builder.request.Temperature != 0.5 || *builder.request.TopP != 0.9 {
		t.Fatalf("sampling config = %#v", builder.request)
	}
	if *builder.request.MaxTokens != 128 || builder.request.Stop[0] != "END" {
		t.Fatalf("limit config = %#v", builder.request)
	}
	if len(builder.request.Tools) != 1 || builder.request.ToolChoice.Type != types.ToolChoiceTypeAuto {
		t.Fatalf("tool config = %#v", builder.request)
	}
	if builder.toolExecutionOverride == nil || !*builder.toolExecutionOverride || builder.maxToolIterations != 3 || builder.fallbackModels[0] != "gpt-5-mini" {
		t.Fatalf("execution config = override:%v max:%d fallback:%#v", builder.toolExecutionOverride, builder.maxToolIterations, builder.fallbackModels)
	}
	if !builder.shouldAutoExecuteTools(client) {
		t.Fatal("shouldAutoExecuteTools = false, want true for explicit enable")
	}
	builder.WithToolsDisabled()
	if builder.shouldAutoExecuteTools(client) {
		t.Fatal("shouldAutoExecuteTools = true, want false after explicit disable")
	}
}

func TestTextRequestBuilderCloneDetachesNestedState(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	message := &types.UserMessage{Media: []types.Media{&types.ImageMedia{Data: []byte("image")}}}
	tool := types.Tool{Name: "lookup", InputSchema: map[string]any{
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
	}}
	options := map[string]any{"nested": map[string]any{"value": "original"}}
	format := map[string]any{"schema": map[string]any{"type": "object"}}

	builder := client.Text().Messages(message).Tools(tool).ProviderOptions(options).ResponseFormat(format)
	clone := builder.Clone()
	clone.request.Messages[0].(*types.UserMessage).Media[0].(*types.ImageMedia).Data[0] = 'X'
	clone.request.Tools[0].InputSchema["properties"].(map[string]any)["query"].(map[string]any)["type"] = "number"
	clone.request.ProviderOptions["nested"].(map[string]any)["value"] = "changed"
	clone.request.ResponseFormat.(map[string]any)["schema"].(map[string]any)["type"] = "array"

	if got := builder.request.Messages[0].(*types.UserMessage).Media[0].(*types.ImageMedia).Data; string(got) != "image" {
		t.Fatalf("original media = %q", got)
	}
	if got := builder.request.Tools[0].InputSchema["properties"].(map[string]any)["query"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("original tool schema type = %v", got)
	}
	if got := builder.request.ProviderOptions["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("original provider option = %v", got)
	}
	if got := builder.request.ResponseFormat.(map[string]any)["schema"].(map[string]any)["type"]; got != "object" {
		t.Fatalf("original response format = %v", got)
	}
}

// TestWithToolsDisabledIsNotNoOp reproduces a bug where WithToolsDisabled()
// alone (without WithMaxToolIterations) was indistinguishable from the
// zero-value "unset" state, so tools registered on the client would still
// auto-execute despite the explicit opt-out.
func TestWithToolsDisabledIsNotNoOp(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	client.toolRegistry.Register("lookup", &types.ToolDefinition{
		Tool: types.Tool{
			Type:        "function",
			Name:        "lookup",
			Description: "Lookup data",
			InputSchema: map[string]any{"type": "object"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return "ok", nil
		},
	})

	builder := client.Text().Model("gpt-5").Messages(types.NewUserMessage("hello")).WithToolsDisabled()

	if builder.shouldAutoExecuteTools(client) {
		t.Fatal("shouldAutoExecuteTools = true, want false: WithToolsDisabled() alone must not be a no-op")
	}
}

func TestTextRequestBuilderConversationCloneValidateAndJSON(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	conv := types.NewConversation().
		System("system").
		User("hello").
		Assistant("hi")

	builder := client.Text().
		Model("gpt-5").
		Conversation(conv).
		Temperature(0.5).
		MaxTokens(64)

	if builder.request.SystemPrompt != "system" {
		t.Fatalf("SystemPrompt = %q, want system", builder.request.SystemPrompt)
	}
	if len(builder.request.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2 without system", len(builder.request.Messages))
	}
	if err := builder.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if got := builder.MustValidate(); got != builder {
		t.Fatal("MustValidate did not return receiver")
	}

	clone := builder.Clone().Prompt("changed").WithFallback("fallback")
	if builder.request.Messages[0].GetContent() == "changed" {
		t.Fatal("Clone mutation changed original builder")
	}
	if len(builder.fallbackModels) != 0 || clone.fallbackModels[0] != "fallback" {
		t.Fatalf("fallback clone state original=%#v clone=%#v", builder.fallbackModels, clone.fallbackModels)
	}

	jsonText, err := builder.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON returned error: %v", err)
	}
	if !strings.Contains(jsonText, `"model": "gpt-5"`) {
		t.Fatalf("ToJSON output missing model: %s", jsonText)
	}

	invalid := client.Text().Temperature(3).TopP(2).MaxTokens(0)
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid Validate returned nil")
	}
	assertPanics(t, func() { invalid.MustValidate() })
}

func TestTextRequestBuilderGenerateAndStreamValidation(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	ctx := context.Background()

	if _, err := client.Text().Model("gpt-5").Generate(ctx); err == nil {
		t.Fatal("Generate without messages returned nil error")
	}
	if _, err := client.Text().Prompt("hello").Generate(ctx); err == nil {
		t.Fatal("Generate without model returned nil error")
	}
	if _, err := client.Text().Model("gpt-5").Stream(ctx); err == nil {
		t.Fatal("Stream without messages returned nil error")
	}
	if _, _, err := client.Text().Model("gpt-5").StreamAndAccumulate(ctx); err == nil {
		t.Fatal("StreamAndAccumulate without messages returned nil error")
	}
}

func TestStructuredRequestBuilderConfigurationAndValidation(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	builder := client.Structured().
		Using("openai").
		BaseURL("https://example.test/v1").
		Model("gpt-5").
		Messages(types.NewUserMessage("hello")).
		AddMessage(types.NewAssistantMessage("hi")).
		SystemPrompt("system").
		Schema(schema).
		SchemaName("person").
		Mode(types.StructuredModeStrict).
		Temperature(0.2).
		MaxTokens(64)

	if builder.getProvider() != "openai" || builder.getBaseURL() != "https://example.test/v1" {
		t.Fatalf("builder routing = (%q, %q)", builder.getProvider(), builder.getBaseURL())
	}
	if builder.request.Model != "gpt-5" || len(builder.request.Messages) != 2 || builder.request.Schema == nil {
		t.Fatalf("structured request = %#v", builder.request)
	}
	if builder.request.SchemaName != "person" || builder.request.Mode != types.StructuredModeStrict {
		t.Fatalf("structured schema config = %#v", builder.request)
	}
	if err := builder.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if got := builder.MustValidate(); got != builder {
		t.Fatal("MustValidate did not return receiver")
	}

	cloned := cloneStructuredRequest(builder.request)
	cloned.Messages[0] = types.NewUserMessage("changed")
	if builder.request.Messages[0].GetContent() == "changed" {
		t.Fatal("cloneStructuredRequest mutation changed original messages")
	}

	prepared := cloneStructuredRequest(builder.request)
	prepareStructuredExecutionRequest(prepared)
	if prepared.Messages[0].GetRole() != types.RoleSystem {
		t.Fatalf("prepared first role = %s, want system", prepared.Messages[0].GetRole())
	}

	invalid := client.Structured().Temperature(3).MaxTokens(0)
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid structured Validate returned nil")
	}
	assertPanics(t, func() { invalid.MustValidate() })
}

func TestStructuredRequestBuilderGenerateValidation(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	ctx := context.Background()
	schema := map[string]any{"type": "object"}

	if _, err := client.Structured().Model("gpt-5").Schema(schema).Generate(ctx); err == nil {
		t.Fatal("Generate without messages returned nil error")
	}
	if _, err := client.Structured().Prompt("hello").Schema(schema).Generate(ctx); err == nil {
		t.Fatal("Generate without model returned nil error")
	}
	if _, err := client.Structured().Model("gpt-5").Prompt("hello").Generate(ctx); err == nil {
		t.Fatal("Generate without schema returned nil error")
	}
	var result struct{}
	if err := client.Structured().Model("gpt-5").Schema(schema).GenerateAs(ctx, &result); err == nil {
		t.Fatal("GenerateAs without messages returned nil error")
	}
}

// Regression: an unmarshalable Schema() argument must surface the actual marshal
// error from Generate, not be misattributed as "no schema provided" (which is
// reserved for the case where Schema was never called at all).
func TestStructuredRequestBuilderGenerateReturnsSchemaMarshalError(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	ctx := context.Background()

	_, err := client.Structured().Model("gpt-5").Prompt("hello").Schema(make(chan int)).Generate(ctx)
	if err == nil {
		t.Fatal("Generate with unmarshalable schema returned nil error")
	}
	if strings.Contains(err.Error(), "no schema provided") {
		t.Fatalf("Generate misattributed a schema marshal error as a missing schema: %v", err)
	}
}

func TestStructuredRequestBuilderSchemaSuccessClearsPriorSchemaError(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	builder := client.Structured()

	builder.Schema(make(chan int))
	if builder.schemaErr == nil {
		t.Fatal("expected invalid schema to set schemaErr")
	}

	builder.Schema(map[string]any{"type": "object"})
	if builder.schemaErr != nil {
		t.Fatalf("expected valid schema to clear schemaErr, got %v", builder.schemaErr)
	}
	if builder.request.Schema == nil {
		t.Fatal("expected valid schema bytes to be stored")
	}
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("function did not panic")
		}
	}()
	fn()
}
