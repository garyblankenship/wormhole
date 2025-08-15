# OpenRouter + Claude JSON Schema Guide

This guide shows how to use structured JSON output with Claude models through OpenRouter using wormhole.

## 🚨 Important: OpenRouter Native vs Wormhole Approach

**OpenRouter Native Structured Outputs** (using `response_format`):
- ✅ **Supported**: OpenAI models (GPT-4o+), Fireworks models
- ❌ **NOT Supported**: Anthropic/Claude models, most other providers

**Wormhole Structured Outputs** (using tool calling):
- ✅ **Supported**: ALL Claude models via tool-based approach
- ✅ **Robust**: Works consistently across all Anthropic models
- ✅ **Reliable**: Built-in JSON parsing fixes for Claude responses

**Recommendation**: Use wormhole's structured output approach for Claude models, as OpenRouter's native `response_format` doesn't support Anthropic models.

## When to Use Each Approach

### Use Wormhole Structured (Tool-Based) for:
- ✅ **Anthropic/Claude models** (`anthropic/claude-opus-4.1`, `anthropic/claude-3.5-sonnet`, etc.)
- ✅ **Any model** where you want consistent behavior across providers
- ✅ **Complex reasoning** tasks where Claude excels

### Use OpenRouter Native for:
- ✅ **OpenAI models** (`openai/gpt-4o`, `openai/gpt-4o-mini`, etc.)
- ✅ **Fireworks models** for fastest validation
- ✅ **Simple structured extraction** where native validation is sufficient

## Quick Setup

```go
import (
    "context"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Setup OpenRouter with Claude
w := wormhole.New(
    wormhole.WithDefaultProvider("openrouter"),
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey: "your-openrouter-api-key",
    }),
    wormhole.WithTimeout(2*time.Minute),
)
```

## JSON Schema with Claude Models

### Basic Structured Output

```go
// Define your JSON schema
schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "analysis": map[string]interface{}{
            "type":        "string",
            "description": "Your analysis of the topic",
        },
        "confidence": map[string]interface{}{
            "type":        "number",
            "minimum":     0,
            "maximum":     1,
            "description": "Confidence level (0-1)",
        },
        "keywords": map[string]interface{}{
            "type": "array",
            "items": map[string]interface{}{
                "type": "string",
            },
            "description": "Key concepts identified",
        },
    },
    "required": []string{"analysis", "confidence", "keywords"},
}

// Generate structured response with Claude Opus
response, err := w.Structured().
    Model("anthropic/claude-opus-4.1").  // Latest Claude Opus via OpenRouter
    Prompt("Analyze the impact of quantum computing on cryptography").
    Schema(schema).
    SchemaName("analysis_result").  // Optional: custom name for the tool
    MaxTokens(500).
    Generate(context.Background())

if err != nil {
    log.Fatal(err)
}

// Access the structured data
fmt.Printf("Raw JSON: %s\n", response.Raw)
fmt.Printf("Parsed Data: %+v\n", response.Data)
```

### Available Claude Models on OpenRouter

```go
claudeModels := []string{
    "anthropic/claude-opus-4.1",      // Latest Claude Opus (most capable)
    "anthropic/claude-3.5-sonnet",    // Fast and capable
    "anthropic/claude-3-opus",        // Previous generation Opus
    "anthropic/claude-3-sonnet",      // Previous generation Sonnet
    "anthropic/claude-3-haiku",       // Fast and efficient
}
```

### Complex Schema Example

```go
// Schema for a research paper analysis
paperAnalysisSchema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "title": map[string]interface{}{
            "type":        "string",
            "description": "Paper title",
        },
        "abstract_summary": map[string]interface{}{
            "type":        "string",
            "maxLength":   500,
            "description": "Concise summary of the abstract",
        },
        "methodology": map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "approach": map[string]interface{}{
                    "type": "string",
                    "enum": []string{"experimental", "theoretical", "computational", "survey"},
                },
                "datasets": map[string]interface{}{
                    "type": "array",
                    "items": map[string]interface{}{
                        "type": "string",
                    },
                },
                "metrics": map[string]interface{}{
                    "type": "array",
                    "items": map[string]interface{}{
                        "type": "string",
                    },
                },
            },
            "required": []string{"approach"},
        },
        "key_findings": map[string]interface{}{
            "type": "array",
            "items": map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "finding": map[string]interface{}{
                        "type": "string",
                    },
                    "significance": map[string]interface{}{
                        "type": "string",
                        "enum": []string{"low", "medium", "high", "breakthrough"},
                    },
                },
                "required": []string{"finding", "significance"},
            },
            "minItems": 1,
            "maxItems": 10,
        },
        "limitations": map[string]interface{}{
            "type": "array",
            "items": map[string]interface{}{
                "type": "string",
            },
        },
        "overall_score": map[string]interface{}{
            "type":        "number",
            "minimum":     1,
            "maximum":     10,
            "description": "Overall quality score",
        },
    },
    "required": []string{"title", "abstract_summary", "methodology", "key_findings", "overall_score"},
}

response, err := w.Structured().
    Model("anthropic/claude-opus-4.1").
    Prompt("Analyze this research paper: [PAPER_TEXT_HERE]").
    Schema(paperAnalysisSchema).
    SchemaName("paper_analysis").
    MaxTokens(1500).
    Temperature(0.1). // Low temperature for consistent structured output
    Generate(context.Background())
```

### Error Handling

```go
response, err := w.Structured().
    Model("anthropic/claude-3.5-sonnet").
    Prompt("Extract data from this text: [TEXT]").
    Schema(yourSchema).
    Generate(context.Background())

if err != nil {
    // Check for specific error types
    if strings.Contains(err.Error(), "tool arguments") {
        log.Printf("JSON parsing issue with Claude response: %v", err)
        // The new robust JSON utilities will provide better error context
    } else {
        log.Printf("General error: %v", err)
    }
    return
}

// Validate that required fields are present
data, ok := response.Data.(map[string]interface{})
if !ok {
    log.Fatal("Response data is not a JSON object")
}

// Check for required fields
requiredFields := []string{"analysis", "confidence"}
for _, field := range requiredFields {
    if _, exists := data[field]; !exists {
        log.Printf("Warning: Required field '%s' missing from response", field)
    }
}
```

## How It Works

### Anthropic's Tool-Based Approach

Claude models don't have native JSON mode like OpenAI. Instead, wormhole:

1. **Converts your schema into a tool definition**
2. **Forces Claude to call that tool** with structured arguments
3. **Extracts the tool arguments as your JSON response**

This approach is robust and works reliably with all Claude models.

### Schema Requirements

- **Valid JSON Schema**: Must be valid JSON Schema format
- **Clear descriptions**: Claude responds better to detailed field descriptions
- **Reasonable constraints**: Avoid overly complex nested structures
- **Required fields**: Specify which fields are mandatory

## Best Practices

### 1. Model Selection
```go
// For complex analysis requiring reasoning
Model("anthropic/claude-opus-4.1")

// For fast structured extraction
Model("anthropic/claude-3.5-sonnet")

// For simple data extraction
Model("anthropic/claude-3-haiku")
```

### 2. Temperature Settings
```go
// For consistent structured output
Temperature(0.1)

// For creative but structured content
Temperature(0.3)

// Avoid high temperatures with structured output
// Temperature(0.8)  // ❌ May break schema adherence
```

### 3. Token Limits
```go
// Estimate based on your schema complexity
MaxTokens(500)   // Simple schemas
MaxTokens(1000)  // Medium complexity
MaxTokens(2000)  // Complex nested structures
```

### 4. Prompt Engineering
```go
// ✅ Good: Clear, specific instructions
Prompt("Extract the following information from this product review: [REVIEW_TEXT]")

// ✅ Good: Include format hints
Prompt("Analyze this code and return issues in the specified JSON format: [CODE]")

// ❌ Avoid: Vague instructions
Prompt("Process this data")
```

## Troubleshooting

### Common Issues

1. **"failed to parse structured response"**
   - Check that your schema is valid JSON Schema
   - Ensure the model supports tool calling
   - Verify your prompt is clear about the expected output

2. **Missing required fields**
   - Add detailed descriptions to schema fields
   - Use lower temperature (0.1-0.3)
   - Make prompts more specific

3. **Timeout errors**
   - Increase timeout for complex schemas
   - Use faster models for simple extraction
   - Consider breaking complex schemas into smaller parts

### Debug Mode

```go
w := wormhole.New(
    wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
        APIKey: apiKey,
    }),
    wormhole.WithDebugLogging(), // Enable debug output
)
```

## OpenRouter-Specific Features

### Cost Tracking
OpenRouter provides detailed usage analytics in their dashboard. Monitor:
- Per-model costs
- Token usage
- Request volume
- Response times

### Model Availability
Some Claude models may have availability limits:
- Check OpenRouter dashboard for model status
- Consider fallback models for production
- Monitor rate limits and quotas

## Complete Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/garyblankenship/wormhole/pkg/types"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    w := wormhole.New(
        wormhole.WithDefaultProvider("openrouter"),
        wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
            APIKey: os.Getenv("OPENROUTER_API_KEY"),
        }),
        wormhole.WithTimeout(2*time.Minute),
    )

    schema := map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "sentiment": map[string]interface{}{
                "type": "string",
                "enum": []string{"positive", "negative", "neutral"},
            },
            "confidence": map[string]interface{}{
                "type":    "number",
                "minimum": 0,
                "maximum": 1,
            },
            "key_phrases": map[string]interface{}{
                "type": "array",
                "items": map[string]interface{}{
                    "type": "string",
                },
                "maxItems": 5,
            },
        },
        "required": []string{"sentiment", "confidence"},
    }

    response, err := w.Structured().
        Model("anthropic/claude-opus-4.1").
        Prompt("Analyze the sentiment of this review: 'This product exceeded my expectations! Great quality and fast shipping.'").
        Schema(schema).
        SchemaName("sentiment_analysis").
        MaxTokens(300).
        Temperature(0.1).
        Generate(context.Background())

    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Raw JSON: %s\n", response.Raw)
    
    // Pretty print the structured data
    prettyJSON, _ := json.MarshalIndent(response.Data, "", "  ")
    fmt.Printf("Structured Data:\n%s\n", string(prettyJSON))
}
```

This approach gives you robust, reliable structured output from Claude models through OpenRouter with comprehensive error handling and optimization for the latest Claude Opus 4.1.

## Bonus: OpenRouter Native Structured Outputs

For **OpenAI and Fireworks models**, you can also use OpenRouter's native structured outputs:

### Using Native Response Format (OpenAI Models Only)

```go
// This approach works ONLY with OpenAI models on OpenRouter
response, err := w.Text().
    Model("openai/gpt-4o-mini").
    Messages(types.NewUserMessage("What's the weather like in London?")).
    ProviderOptions(map[string]interface{}{
        "response_format": map[string]interface{}{
            "type": "json_schema",
            "json_schema": map[string]interface{}{
                "name":   "weather",
                "strict": true,
                "schema": map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "location": map[string]interface{}{
                            "type": "string",
                        },
                        "temperature": map[string]interface{}{
                            "type": "number",
                        },
                        "conditions": map[string]interface{}{
                            "type": "string",
                        },
                    },
                    "required":             []string{"location", "temperature", "conditions"},
                    "additionalProperties": false,
                },
            },
        },
    }).
    Generate(context.Background())

// Note: This returns a regular TextResponse, not StructuredResponse
// You'll need to parse response.Text as JSON manually
```

### Comparison: Native vs Wormhole

| Feature | OpenRouter Native | Wormhole Structured |
|---------|------------------|-------------------|
| **Claude Support** | ❌ Not supported | ✅ Full support |
| **OpenAI Support** | ✅ Native validation | ✅ Tool-based |
| **Consistency** | ❌ Model-dependent | ✅ Works everywhere |
| **Error Handling** | ❌ Basic JSON errors | ✅ Enhanced context |
| **Response Type** | `TextResponse` | `StructuredResponse` |
| **Parsing** | Manual JSON parse | Automatic parsing |

**Recommendation**: Stick with wormhole's `.Structured()` approach for consistency across all models and providers, especially Claude models.