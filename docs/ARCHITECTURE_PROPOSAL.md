# Provider-Aware Model Validation Architecture

## Problem Statement
Current model registry acts as a universal whitelist, blocking legitimate OpenRouter models and creating maintenance overhead. This contradicts our "200+ OpenRouter models" claim.

## Proposed Solution

### 1. Add Dynamic Model Support to ProviderConfig

```go
type ProviderConfig struct {
    APIKey       string
    BaseURL      string
    DynamicModels bool   // NEW: Skip local registry validation
    // ... existing fields
}
```

### 2. Provider-Specific Validation Logic

```go
func (b *TextRequestBuilder) validateModel() error {
    // Global override for testing
    if !b.wormhole.config.ModelValidation {
        return nil
    }
    
    // Check if provider supports dynamic models
    providerName := b.getProviderName()
    if config, exists := b.wormhole.config.Providers[providerName]; exists {
        if config.DynamicModels {
            // Let the provider validate - skip local registry
            return nil
        }
    }
    
    // Use registry validation for traditional providers
    return types.ValidateModelForCapability(b.request.Model, types.CapabilityText)
}
```

### 3. Update Provider Configurations

```go
// OpenRouter: Enable dynamic models by default
func (f *SimpleFactory) OpenRouter(apiKey ...string) *Wormhole {
    return New(
        WithDefaultProvider("openrouter"),
        WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
            APIKey: key,
            DynamicModels: true,  // NEW: Skip registry validation
        }),
    )
}

// Ollama: Enable dynamic models (user can load any model)
func (f *SimpleFactory) Ollama(baseURL ...string) *Wormhole {
    return New(
        WithDefaultProvider("ollama"),
        WithOllama(types.ProviderConfig{
            BaseURL: url,
            DynamicModels: true,  // NEW: Skip registry validation
        }),
    )
}

// OpenAI: Keep registry validation (fixed model catalog)
func (f *SimpleFactory) OpenAI(apiKey ...string) *Wormhole {
    return New(
        WithDefaultProvider("openai"),
        WithOpenAI(key),  // DynamicModels: false (default)
    )
}
```

## Benefits

### ✅ **True OpenRouter Support**
- All 200+ OpenRouter models work immediately
- No manual registration required
- New models available instantly

### ✅ **Maintained Type Safety**  
- OpenAI models still validated for capabilities
- Anthropic models still get proper error checking
- Registry remains valuable for fixed catalogs

### ✅ **Future-Proof Architecture**
- New providers can choose validation strategy
- Easy to add model discovery APIs later
- Backward compatible with existing code

### ✅ **Honest Marketing**
- Can legitimately claim "200+ OpenRouter models"
- Registry only used where it adds value
- Performance improvement (no unnecessary validation)

## Implementation Steps

1. **Add DynamicModels field** to ProviderConfig
2. **Update validation logic** in text_builder.go
3. **Enable dynamic models** for OpenRouter, Ollama, LMStudio
4. **Keep registry validation** for OpenAI, Anthropic, etc.
5. **Update documentation** to reflect true capabilities
6. **Remove hardcoded OpenRouter models** from registry (optional cleanup)

## Migration Path

- **Existing code**: Works unchanged
- **OpenRouter users**: Immediately get access to all models
- **OpenAI users**: Same validation behavior
- **Custom providers**: Can choose validation strategy

This architecture respects the reality that different providers have different model discovery patterns while maintaining type safety where it matters.