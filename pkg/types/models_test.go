package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelInfo_Structure(t *testing.T) {
	cost := &ModelCost{
		InputTokens:  0.001,
		OutputTokens: 0.002,
		Currency:     "USD",
	}

	constraints := map[string]interface{}{
		"temperature": 1.0,
		"max_tokens":  4096,
	}

	model := &ModelInfo{
		ID:            "gpt-5",
		Name:          "GPT-5",
		Provider:      "openai",
		Description:   "Next generation language model",
		ContextLength: 128000,
		MaxTokens:     4096,
		Cost:          cost,
		Capabilities: []ModelCapability{
			CapabilityText,
			CapabilityChat,
			CapabilityFunctions,
			CapabilityStream,
		},
		Constraints: constraints,
		Deprecated:  false,
	}

	assert.Equal(t, "gpt-5", model.ID)
	assert.Equal(t, "GPT-5", model.Name)
	assert.Equal(t, "openai", model.Provider)
	assert.Equal(t, "Next generation language model", model.Description)
	assert.Equal(t, 128000, model.ContextLength)
	assert.Equal(t, 4096, model.MaxTokens)
	assert.Equal(t, cost, model.Cost)
	assert.Len(t, model.Capabilities, 4)
	assert.Equal(t, constraints, model.Constraints)
	assert.False(t, model.Deprecated)
}

func TestModelCost_Structure(t *testing.T) {
	cost := &ModelCost{
		InputTokens:  0.0015,
		OutputTokens: 0.006,
		Currency:     "USD",
	}

	assert.Equal(t, 0.0015, cost.InputTokens)
	assert.Equal(t, 0.006, cost.OutputTokens)
	assert.Equal(t, "USD", cost.Currency)
}

func TestModelCapability_Constants(t *testing.T) {
	capabilities := []ModelCapability{
		CapabilityText,
		CapabilityChat,
		CapabilityStructured,
		CapabilityEmbeddings,
		CapabilityImages,
		CapabilityAudio,
		CapabilityVision,
		CapabilityFunctions,
		CapabilityStream,
	}

	expected := []string{
		"text",
		"chat",
		"structured",
		"embeddings",
		"images",
		"audio",
		"vision",
		"functions",
		"stream",
	}

	for i, cap := range capabilities {
		assert.Equal(t, expected[i], string(cap))
	}
}

func TestModelRegistry_Creation(t *testing.T) {
	registry := NewModelRegistry()

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.models)
	assert.NotNil(t, registry.byProvider)
	assert.Len(t, registry.List(), 0)
}

func TestModelRegistry_Register(t *testing.T) {
	registry := NewModelRegistry()

	model1 := &ModelInfo{
		ID:           "gpt-5",
		Provider:     "openai",
		Capabilities: []ModelCapability{CapabilityText, CapabilityChat},
	}

	model2 := &ModelInfo{
		ID:           "claude-3-opus",
		Provider:     "anthropic",
		Capabilities: []ModelCapability{CapabilityText, CapabilityVision},
	}

	registry.Register(model1)
	registry.Register(model2)

	assert.Len(t, registry.List(), 2)

	// Test direct access
	retrieved, exists := registry.Get("gpt-5")
	assert.True(t, exists)
	assert.Equal(t, model1, retrieved)

	retrieved, exists = registry.Get("claude-3-opus")
	assert.True(t, exists)
	assert.Equal(t, model2, retrieved)
}

func TestModelRegistry_Get(t *testing.T) {
	registry := NewModelRegistry()

	model := &ModelInfo{
		ID:       "test-model",
		Provider: "test-provider",
	}

	registry.Register(model)

	t.Run("existing model", func(t *testing.T) {
		retrieved, exists := registry.Get("test-model")
		assert.True(t, exists)
		assert.Equal(t, model, retrieved)
	})

	t.Run("non-existing model", func(t *testing.T) {
		retrieved, exists := registry.Get("non-existent")
		assert.False(t, exists)
		assert.Nil(t, retrieved)
	})
}

func TestModelRegistry_GetByProvider(t *testing.T) {
	registry := NewModelRegistry()

	openaiModel1 := &ModelInfo{ID: "gpt-5", Provider: "openai"}
	openaiModel2 := &ModelInfo{ID: "gpt-4o", Provider: "openai"}
	anthropicModel := &ModelInfo{ID: "claude-3-opus", Provider: "anthropic"}

	registry.Register(openaiModel1)
	registry.Register(openaiModel2)
	registry.Register(anthropicModel)

	t.Run("provider with models", func(t *testing.T) {
		openaiModels := registry.GetByProvider("openai")
		assert.Len(t, openaiModels, 2)
		assert.Contains(t, openaiModels, openaiModel1)
		assert.Contains(t, openaiModels, openaiModel2)
	})

	t.Run("provider with one model", func(t *testing.T) {
		anthropicModels := registry.GetByProvider("anthropic")
		assert.Len(t, anthropicModels, 1)
		assert.Contains(t, anthropicModels, anthropicModel)
	})

	t.Run("provider with no models", func(t *testing.T) {
		groqModels := registry.GetByProvider("groq")
		assert.Len(t, groqModels, 0)
	})
}

func TestModelRegistry_GetByCapability(t *testing.T) {
	registry := NewModelRegistry()

	textModel := &ModelInfo{
		ID:           "text-only",
		Capabilities: []ModelCapability{CapabilityText},
	}

	chatModel := &ModelInfo{
		ID:           "chat-model",
		Capabilities: []ModelCapability{CapabilityText, CapabilityChat},
	}

	visionModel := &ModelInfo{
		ID:           "vision-model",
		Capabilities: []ModelCapability{CapabilityText, CapabilityVision},
	}

	registry.Register(textModel)
	registry.Register(chatModel)
	registry.Register(visionModel)

	t.Run("common capability", func(t *testing.T) {
		textModels := registry.GetByCapability(CapabilityText)
		assert.Len(t, textModels, 3)
		assert.Contains(t, textModels, textModel)
		assert.Contains(t, textModels, chatModel)
		assert.Contains(t, textModels, visionModel)
	})

	t.Run("specific capability", func(t *testing.T) {
		visionModels := registry.GetByCapability(CapabilityVision)
		assert.Len(t, visionModels, 1)
		assert.Contains(t, visionModels, visionModel)
	})

	t.Run("no models with capability", func(t *testing.T) {
		audioModels := registry.GetByCapability(CapabilityAudio)
		assert.Len(t, audioModels, 0)
	})
}

func TestModelRegistry_List(t *testing.T) {
	registry := NewModelRegistry()

	model1 := &ModelInfo{ID: "model1"}
	model2 := &ModelInfo{ID: "model2"}
	model3 := &ModelInfo{ID: "model3"}

	registry.Register(model1)
	registry.Register(model2)
	registry.Register(model3)

	models := registry.List()
	assert.Len(t, models, 3)
	assert.Contains(t, models, model1)
	assert.Contains(t, models, model2)
	assert.Contains(t, models, model3)
}

func TestModelRegistry_Search(t *testing.T) {
	registry := NewModelRegistry()

	models := []*ModelInfo{
		{ID: "gpt-5", Name: "GPT-5", Description: "Advanced language model"},
		{ID: "gpt-4o", Name: "GPT-4O", Description: "Multimodal model"},
		{ID: "claude-3-opus", Name: "Claude 3 Opus", Description: "Anthropic's flagship model"},
		{ID: "claude-3-haiku", Name: "Claude 3 Haiku", Description: "Fast and efficient"},
		{ID: "llama-3", Name: "Llama 3", Description: "Open source model"},
	}

	for _, model := range models {
		registry.Register(model)
	}

	t.Run("search by ID", func(t *testing.T) {
		results := registry.Search("gpt")
		assert.Len(t, results, 2)
		assert.Contains(t, results, models[0])
		assert.Contains(t, results, models[1])
	})

	t.Run("search by name", func(t *testing.T) {
		results := registry.Search("claude")
		assert.Len(t, results, 2)
		assert.Contains(t, results, models[2])
		assert.Contains(t, results, models[3])
	})

	t.Run("search by description", func(t *testing.T) {
		results := registry.Search("multimodal")
		assert.Len(t, results, 1)
		assert.Contains(t, results, models[1])
	})

	t.Run("case insensitive search", func(t *testing.T) {
		results := registry.Search("OPUS")
		assert.Len(t, results, 1)
		assert.Contains(t, results, models[2])
	})

	t.Run("no matches", func(t *testing.T) {
		results := registry.Search("nonexistent")
		assert.Len(t, results, 0)
	})

	t.Run("partial match", func(t *testing.T) {
		results := registry.Search("3")
		assert.Len(t, results, 3) // claude-3-opus, claude-3-haiku, llama-3
	})
}

func TestModelRegistry_ValidateModel(t *testing.T) {
	registry := NewModelRegistry()

	validModel := &ModelInfo{
		ID:           "valid-model",
		Capabilities: []ModelCapability{CapabilityText, CapabilityChat, CapabilityFunctions},
		Deprecated:   false,
	}

	deprecatedModel := &ModelInfo{
		ID:           "deprecated-model",
		Capabilities: []ModelCapability{CapabilityText},
		Deprecated:   true,
	}

	registry.Register(validModel)
	registry.Register(deprecatedModel)

	t.Run("valid model with supported capabilities", func(t *testing.T) {
		err := registry.ValidateModel("valid-model", []ModelCapability{CapabilityText, CapabilityChat})
		assert.NoError(t, err)
	})

	t.Run("valid model with single capability", func(t *testing.T) {
		err := registry.ValidateModel("valid-model", []ModelCapability{CapabilityFunctions})
		assert.NoError(t, err)
	})

	t.Run("valid model with no required capabilities", func(t *testing.T) {
		err := registry.ValidateModel("valid-model", []ModelCapability{})
		assert.NoError(t, err)
	})

	t.Run("model not found", func(t *testing.T) {
		err := registry.ValidateModel("non-existent", []ModelCapability{CapabilityText})
		assert.Error(t, err)

		wormholeErr, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.Equal(t, ErrorCodeModel, wormholeErr.Code)
		assert.Equal(t, "non-existent", wormholeErr.Model)
	})

	t.Run("deprecated model", func(t *testing.T) {
		err := registry.ValidateModel("deprecated-model", []ModelCapability{CapabilityText})
		assert.Error(t, err)

		wormholeErr, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.Equal(t, ErrorCodeModel, wormholeErr.Code)
		assert.Contains(t, wormholeErr.Message, "deprecated")
		assert.Equal(t, "deprecated-model", wormholeErr.Model)
	})

	t.Run("unsupported capability", func(t *testing.T) {
		err := registry.ValidateModel("valid-model", []ModelCapability{CapabilityImages})
		assert.Error(t, err)

		wormholeErr, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.Equal(t, ErrorCodeModel, wormholeErr.Code)
		assert.Contains(t, wormholeErr.Details, "missing capability: images")
		assert.Equal(t, "valid-model", wormholeErr.Model)
	})

	t.Run("multiple unsupported capabilities", func(t *testing.T) {
		err := registry.ValidateModel("valid-model", []ModelCapability{CapabilityImages, CapabilityAudio})
		assert.Error(t, err)

		// Should fail on first missing capability
		wormholeErr, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.Contains(t, wormholeErr.Details, "missing capability: images")
	})
}

func TestModelRegistry_EstimateCost(t *testing.T) {
	registry := NewModelRegistry()

	costModel := &ModelInfo{
		ID: "cost-model",
		Cost: &ModelCost{
			InputTokens:  0.001, // $0.001 per 1K input tokens
			OutputTokens: 0.002, // $0.002 per 1K output tokens
			Currency:     "USD",
		},
	}

	noCostModel := &ModelInfo{
		ID:   "no-cost-model",
		Cost: nil,
	}

	registry.Register(costModel)
	registry.Register(noCostModel)

	t.Run("model with cost information", func(t *testing.T) {
		// 1000 input tokens = $0.001, 500 output tokens = $0.001
		cost, err := registry.EstimateCost("cost-model", 1000, 500)
		assert.NoError(t, err)
		assert.Equal(t, 0.002, cost)
	})

	t.Run("different token counts", func(t *testing.T) {
		// 2000 input tokens = $0.002, 1000 output tokens = $0.002
		cost, err := registry.EstimateCost("cost-model", 2000, 1000)
		assert.NoError(t, err)
		assert.Equal(t, 0.004, cost)
	})

	t.Run("zero tokens", func(t *testing.T) {
		cost, err := registry.EstimateCost("cost-model", 0, 0)
		assert.NoError(t, err)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("input tokens only", func(t *testing.T) {
		cost, err := registry.EstimateCost("cost-model", 1000, 0)
		assert.NoError(t, err)
		assert.Equal(t, 0.001, cost)
	})

	t.Run("output tokens only", func(t *testing.T) {
		cost, err := registry.EstimateCost("cost-model", 0, 1000)
		assert.NoError(t, err)
		assert.Equal(t, 0.002, cost)
	})

	t.Run("model without cost information", func(t *testing.T) {
		cost, err := registry.EstimateCost("no-cost-model", 1000, 500)
		assert.NoError(t, err)
		assert.Equal(t, 0.0, cost) // Should return 0 when no cost info
	})

	t.Run("model not found", func(t *testing.T) {
		cost, err := registry.EstimateCost("non-existent", 1000, 500)
		assert.Error(t, err)
		assert.Equal(t, 0.0, cost)

		wormholeErr, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.Equal(t, ErrorCodeModel, wormholeErr.Code)
		assert.Equal(t, "non-existent", wormholeErr.Model)
	})

	t.Run("fractional token calculations", func(t *testing.T) {
		// 500 input tokens = $0.0005, 250 output tokens = $0.0005
		cost, err := registry.EstimateCost("cost-model", 500, 250)
		assert.NoError(t, err)
		assert.InDelta(t, 0.001, cost, 0.0001) // Use delta for floating point comparison
	})
}

func TestModelRegistry_GetConstraints(t *testing.T) {
	registry := NewModelRegistry()

	constraints := map[string]interface{}{
		"temperature": 1.0,
		"max_tokens":  4096,
		"top_p":       0.9,
	}

	constrainedModel := &ModelInfo{
		ID:          "constrained-model",
		Constraints: constraints,
	}

	unconstrainedModel := &ModelInfo{
		ID:          "unconstrained-model",
		Constraints: nil,
	}

	registry.Register(constrainedModel)
	registry.Register(unconstrainedModel)

	t.Run("model with constraints", func(t *testing.T) {
		result, err := registry.GetConstraints("constrained-model")
		assert.NoError(t, err)
		assert.Equal(t, constraints, result)
	})

	t.Run("model without constraints", func(t *testing.T) {
		result, err := registry.GetConstraints("unconstrained-model")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("model not found", func(t *testing.T) {
		result, err := registry.GetConstraints("non-existent")
		assert.Error(t, err)
		assert.Nil(t, result)

		wormholeErr, ok := AsWormholeError(err)
		assert.True(t, ok)
		assert.Equal(t, ErrorCodeModel, wormholeErr.Code)
		assert.Equal(t, "non-existent", wormholeErr.Model)
	})
}

func TestModelRegistry_LoadModelsFromConfig(t *testing.T) {
	registry := NewModelRegistry()

	models := []*ModelInfo{
		{ID: "model1", Provider: "provider1"},
		{ID: "model2", Provider: "provider1"},
		{ID: "model3", Provider: "provider2"},
	}

	registry.LoadModelsFromConfig(models)

	// Verify all models were registered
	assert.Len(t, registry.List(), 3)

	// Verify each model is accessible
	for _, model := range models {
		retrieved, exists := registry.Get(model.ID)
		assert.True(t, exists)
		assert.Equal(t, model, retrieved)
	}

	// Verify provider grouping
	provider1Models := registry.GetByProvider("provider1")
	assert.Len(t, provider1Models, 2)

	provider2Models := registry.GetByProvider("provider2")
	assert.Len(t, provider2Models, 1)
}

func TestGlobalHelperFunctions(t *testing.T) {
	// Note: These test the global functions that use DefaultModelRegistry
	// We need to be careful not to interfere with other tests

	// Save current state
	originalRegistry := DefaultModelRegistry

	// Create temporary registry for testing
	DefaultModelRegistry = NewModelRegistry()

	// Cleanup after test
	defer func() {
		DefaultModelRegistry = originalRegistry
	}()

	// Setup test data
	testModel := &ModelInfo{
		ID:           "test-global-model",
		Provider:     "test-provider",
		Capabilities: []ModelCapability{CapabilityText, CapabilityChat},
		Cost: &ModelCost{
			InputTokens:  0.001,
			OutputTokens: 0.002,
			Currency:     "USD",
		},
		Constraints: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	DefaultModelRegistry.Register(testModel)

	t.Run("ListAvailableModels", func(t *testing.T) {
		models := ListAvailableModels("test-provider")
		assert.Len(t, models, 1)
		assert.Contains(t, models, testModel)

		// Non-existent provider
		models = ListAvailableModels("non-existent")
		assert.Len(t, models, 0)
	})

	t.Run("ValidateModelForCapability", func(t *testing.T) {
		err := ValidateModelForCapability("test-global-model", CapabilityText)
		assert.NoError(t, err)

		err = ValidateModelForCapability("test-global-model", CapabilityImages)
		assert.Error(t, err)

		err = ValidateModelForCapability("non-existent", CapabilityText)
		assert.Error(t, err)
	})

	t.Run("GetModelConstraints", func(t *testing.T) {
		constraints, err := GetModelConstraints("test-global-model")
		assert.NoError(t, err)
		assert.Equal(t, testModel.Constraints, constraints)

		_, err = GetModelConstraints("non-existent")
		assert.Error(t, err)
	})

	t.Run("EstimateModelCost", func(t *testing.T) {
		cost, err := EstimateModelCost("test-global-model", 1000, 500)
		assert.NoError(t, err)
		assert.Equal(t, 0.002, cost)

		_, err = EstimateModelCost("non-existent", 1000, 500)
		assert.Error(t, err)
	})
}

func TestModelRegistry_ConcurrentAccess(t *testing.T) {
	// Test basic thread safety - the current implementation is not thread-safe
	// but this test documents the expected behavior
	registry := NewModelRegistry()

	model := &ModelInfo{
		ID:       "concurrent-test",
		Provider: "test",
	}

	registry.Register(model)

	// Multiple concurrent reads should work
	t.Run("concurrent reads", func(t *testing.T) {
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				defer func() { done <- true }()

				retrieved, exists := registry.Get("concurrent-test")
				assert.True(t, exists)
				assert.Equal(t, model, retrieved)
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}
