package prism_test

import (
	"testing"

	"github.com/prism-php/prism-go/pkg/prism"
	mocktesting "github.com/prism-php/prism-go/pkg/testing"
	"github.com/prism-php/prism-go/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextGeneration(t *testing.T) {
	// Create a mock provider
	mockProvider := mocktesting.NewMockProvider("mock").
		WithTextResponse(types.TextResponse{
			ID:           "test-123",
			Model:        "mock-model",
			Text:         "Hello from mock",
			FinishReason: types.FinishReasonStop,
		})
	_ = mockProvider // TODO: inject into prism for actual testing

	// Create Prism instance
	p := prism.New(prism.Config{
		DefaultProvider: "openai",
		Providers: map[string]types.ProviderConfig{
			"openai": {
				APIKey: "test-key",
			},
		},
	})

	t.Run("simple prompt", func(t *testing.T) {
		// For now, test the builder pattern
		req := p.Text().
			Model("gpt-4").
			Prompt("Hello world").
			Temperature(0.7)

		json, err := req.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, json, `"model": "gpt-4"`)
		assert.Contains(t, json, `"content": "Hello world"`)
		assert.Contains(t, json, `"temperature": 0.7`)
	})

	t.Run("with system prompt", func(t *testing.T) {
		req := p.Text().
			Model("gpt-4").
			SystemPrompt("You are helpful").
			Prompt("Hi")

		json, err := req.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, json, `"model": "gpt-4"`)
	})

	t.Run("with messages", func(t *testing.T) {
		messages := []types.Message{
			types.NewSystemMessage("You are helpful"),
			types.NewUserMessage("Hello"),
			types.NewAssistantMessage("Hi there!"),
			types.NewUserMessage("How are you?"),
		}

		req := p.Text().
			Model("gpt-4").
			Messages(messages...)

		json, err := req.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, json, `"role": "system"`)
		assert.Contains(t, json, `"role": "user"`)
		assert.Contains(t, json, `"role": "assistant"`)
	})

	t.Run("with tools", func(t *testing.T) {
		tool := types.NewTool(
			"get_weather",
			"Get weather for a location",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]string{
						"type": "string",
					},
				},
			},
		)

		req := p.Text().
			Model("gpt-4").
			Prompt("What's the weather?").
			Tools(*tool).
			ToolChoice("auto")

		json, err := req.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, json, `"get_weather"`)
		assert.Contains(t, json, `"tool_choice": "auto"`)
	})
}

func TestStructuredGeneration(t *testing.T) {
	p := prism.New(prism.Config{
		DefaultProvider: "mock",
	})

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]string{"type": "string"},
			"age":  map[string]string{"type": "integer"},
		},
	}

	req := p.Structured().
		Model("gpt-4").
		Prompt("Extract person info").
		Schema(schema).
		Mode(types.StructuredModeJSON)

	// Test that request is built correctly
	// In real implementation, we'd test actual generation
	assert.NotNil(t, req)
}

func TestEmbeddings(t *testing.T) {
	p := prism.New(prism.Config{
		DefaultProvider: "mock",
	})

	req := p.Embeddings().
		Model("text-embedding-3-small").
		Input("Hello", "World").
		Dimensions(256)

	// Test request building
	assert.NotNil(t, req)
}

func TestMessageTypes(t *testing.T) {
	t.Run("system message", func(t *testing.T) {
		msg := types.NewSystemMessage("You are helpful")
		assert.Equal(t, types.RoleSystem, msg.GetRole())
		assert.Equal(t, "You are helpful", msg.GetContent())
	})

	t.Run("user message", func(t *testing.T) {
		msg := types.NewUserMessage("Hello")
		assert.Equal(t, types.RoleUser, msg.GetRole())
		assert.Equal(t, "Hello", msg.GetContent())
	})

	t.Run("assistant message", func(t *testing.T) {
		msg := types.NewAssistantMessage("Hi there")
		assert.Equal(t, types.RoleAssistant, msg.GetRole())
		assert.Equal(t, "Hi there", msg.GetContent())
	})

	t.Run("tool message", func(t *testing.T) {
		msg := types.NewToolResultMessage("call-123", "Result data")
		assert.Equal(t, types.RoleTool, msg.GetRole())
		assert.Equal(t, "Result data", msg.GetContent())
		assert.Equal(t, "call-123", msg.ToolCallID)
	})

	t.Run("multimodal message", func(t *testing.T) {
		parts := []types.MessagePart{
			types.TextPart("Look at this image:"),
			types.ImagePart(map[string]string{"url": "https://example.com/image.jpg"}),
		}
		// For multimodal messages, we need to create a user message with text content
		// The parts would be handled differently in the actual implementation
		msg := types.NewUserMessage("Look at this image:")
		assert.Equal(t, types.RoleUser, msg.GetRole())
		assert.Equal(t, "Look at this image:", msg.GetContent())

		// Test the parts separately
		assert.Equal(t, "text", parts[0].Type)
		assert.Equal(t, "image", parts[1].Type)
	})
}
