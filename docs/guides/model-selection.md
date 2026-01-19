# Model Selection Guide

Choosing the right model for your use case is critical for balancing cost, latency, and quality. This guide helps you make informed decisions.

## Table of Contents

- [Model Categories](#model-categories)
- [Cost Considerations](#cost-considerations)
- [Use-Case Recommendations](#use-case-recommendations)
- [Capability Matrix](#capability-matrix)
- [Selection Decision Tree](#selection-decision-tree)

---

## Model Categories

### Fast Models

**Low latency, low cost, good for simple tasks.**

| Model | Provider | Best For |
|-------|----------|----------|
| `claude-haiku-4-5` | Anthropic | High-volume Q&A, summarization |
| `gemini-2.5-flash-lite` | Google | Real-time interactions |
| `gpt-4.1-mini` | OpenAI | Simple classification |
| `gemini-2.5-flash-lite-preview-09-2025` | Google | Cost-sensitive batch processing |

**Characteristics:**
- Latency: 100-500ms
- Cost: $0.01-$0.10 per million tokens
- Throughput: 10-100x higher than powerful models
- Quality: Adequate for straightforward tasks

**When to use:**
- High-volume APIs (>1000 requests/minute)
- Simple classification or extraction
- Real-time chat interfaces
- Prototyping and development
- Batch document processing

**When to avoid:**
- Complex reasoning tasks
- Multi-step logic chains
- Code generation requiring correctness
- Creative writing with nuance

---

### Balanced Models

**Middle ground for most applications.**

| Model | Provider | Best For |
|-------|----------|----------|
| `claude-sonnet-4-5` | Anthropic | General-purpose, code analysis |
| `gemini-2.5-flash` | Google | Balanced performance/cost |
| `gpt-4.1` | OpenAI | Chat, content creation |
| `gpt-5-mini` | OpenAI | Mid-complexity reasoning |
| `gemini-2.5-pro` | Google | Multimodal applications |

**Characteristics:**
- Latency: 500ms-2s
- Cost: $0.50-$5.00 per million tokens
- Throughput: Moderate
- Quality: Good for most business use cases

**When to use:**
- Production applications
- Code review and analysis
- Content generation (articles, summaries)
- Customer support chatbots
- Data extraction and transformation

**When to avoid:**
- Extreme low-latency requirements (<100ms)
- Deep reasoning requirements
- Research-grade accuracy needed

---

### Powerful Models

**Highest quality, higher latency and cost.**

| Model | Provider | Best For |
|-------|----------|----------|
| `claude-opus-4-5` | Anthropic | Complex reasoning, research |
| `o3` | OpenAI | Mathematical proofs, deep reasoning |
| `gpt-5.2` | OpenAI | Complex tasks requiring latest knowledge |
| `gemini-3-pro-preview` | Google | Cutting-edge capabilities |
| `o3-pro` | OpenAI | Research, advanced problem-solving |

**Characteristics:**
- Latency: 2-10s
- Cost: $10-$60 per million tokens
- Throughput: Lower due to inference complexity
- Quality: State-of-the-art

**When to use:**
- Complex multi-step reasoning
- Research and analysis
- Critical decision support
- Architectural design
- Advanced code generation
- Scientific/Mathematical problem-solving

**When to avoid:**
- High-volume real-time applications
- Simple tasks where cost matters
- User-facing real-time chat

---

## Cost Considerations

### Token Pricing Tiers (Approximate, January 2026)

| Tier | Cost per 1M tokens | Annual cost for 1B tokens |
|------|-------------------|---------------------------|
| Fast | $0.01 - $0.10 | $10 - $100 |
| Balanced | $0.50 - $5.00 | $500 - $5,000 |
| Powerful | $10 - $60 | $10,000 - $60,000 |

> [!TIP]
> Prices change frequently. Always check provider pricing pages for current rates:
> - [OpenAI Pricing](https://openai.com/pricing)
> - [Anthropic Pricing](https://www.anthropic.com/pricing)
> - [Google AI Pricing](https://ai.google.dev/pricing)

### Cost Optimization Strategies

**1. Tiered Model Usage**

```go
// Use fast model for initial classification
classification := classifyWithFastModel(userInput)

// Route to appropriate model based on complexity
switch classification.Complexity {
case "low":
    return respondWithFastModel(userInput)
case "medium":
    return respondWithBalancedModel(userInput)
case "high":
    return respondWithPowerfulModel(userInput)
}
```

**2. Token Reduction**

- Use system prompts efficiently (cached by providers)
- Truncate input to essential information
- Use structured output to minimize response tokens
- Implement context window management

**3. Caching**

```go
import (
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Cache responses for identical queries
func generateWithCache(client *wormhole.Client, prompt string) (string, error) {
    // Check cache first
    if cached, found := cache.Get(prompt); found {
        return cached, nil
    }

    // Generate response
    response, err := client.Text().
        Model("claude-haiku-4-5").
        Prompt(prompt).
        Generate(ctx)

    if err != nil {
        return "", err
    }

    // Cache for future use
    cache.Set(prompt, response.Text, 1*time.Hour)
    return response.Text, nil
}
```

**4. Provider Switching**

Compare costs across providers for similar capabilities:
- Gemini Flash vs Haiku for fast tasks
- Sonnet vs GPT-4.1 for balanced tasks
- Opus vs GPT-5.2 for powerful tasks

---

## Use-Case Recommendations

### Chatbots and Conversational AI

**Recommended:** `claude-sonnet-4-5`, `gemini-2.5-flash`, `gpt-4.1`

```go
response, err := client.Text().
    Model("claude-sonnet-4-5").
    Messages(messages).
    MaxTokens(1000).
    Generate(ctx)
```

**Why:** Balanced models provide good response quality with acceptable latency for real-time chat. Use Haiku or Flash-lite for high-volume, simple Q&A bots.

**Alternative for low-cost:** `gemini-2.5-flash-lite` for FAQ bots
**Alternative for quality:** `claude-opus-4-5` for complex advisory chatbots

---

### Code Generation and Analysis

**Recommended:** `claude-sonnet-4-5`, `gpt-5.1-codex`, `gemini-2.5-flash`

```go
codeResponse, err := client.Text().
    Model("gpt-5.1-codex").
    Prompt("Write a Go function to parse JSON with error handling").
    Temperature(0.2). // Lower temperature for more deterministic code
    Generate(ctx)
```

**Why:** Code generation benefits from models trained on code. Codex-max excels at complex code; Sonnet is better for code review and explanation.

**For simple code:** `gemini-2.5-flash-lite`
**For complex architectures:** `o3` for reasoning-heavy design tasks

---

### Content Creation

**Recommended:** `claude-opus-4-5`, `gpt-5.2`, `gemini-2.5-pro`

```go
article, err := client.Text().
    Model("claude-opus-4-5").
    Prompt("Write a 1000-word article about microservices patterns").
    Temperature(0.8). // Higher temperature for creativity
    Generate(ctx)
```

**Why:** Creative tasks benefit from powerful models' nuanced language understanding and generation capabilities.

**For social media:** `claude-sonnet-4-5` (shorter content)
**For technical writing:** `gemini-2.5-pro` (technical accuracy)

---

### Data Extraction and Transformation

**Recommended:** `claude-sonnet-4-5`, `gemini-2.5-flash`, `gpt-4.1-mini`

```go
type ExtractedData struct {
    Name     string `json:"name"`
    Email    string `json:"email"`
    Phone    string `json:"phone"`
}

var result ExtractedData
err := client.Structured().
    Model("claude-sonnet-4-5").
    Prompt("Extract contact info from: ...").
    SchemaName("contact").
    GenerateAs(ctx, &result)
```

**Why:** Structured extraction is well-suited to balanced models. Use fast models (Haiku, Flash-lite) for high-volume batch processing.

**For batch processing:** `claude-haiku-4-5` or `gemini-2.5-flash-lite`
**For complex documents:** `claude-opus-4-5` for nuanced extraction

---

### Summarization

**Recommended:** `claude-haiku-4-5`, `gemini-2.5-flash-lite`, `gpt-4.1-mini`

```go
summary, err := client.Text().
    Model("claude-haiku-4-5").
    Prompt(fmt.Sprintf("Summarize in 3 bullet points:\n\n%s", longText)).
    MaxTokens(200).
    Generate(ctx)
```

**Why:** Summarization is computationally simpler; fast models handle it well with significant cost savings at scale.

**For executive summaries:** `claude-sonnet-4-5` for better nuance
**For research synthesis:** `claude-opus-4-5` for complex material

---

### Classification and Categorization

**Recommended:** `claude-haiku-4-5`, `gemini-2.5-flash-lite`, `gpt-4.1-mini`

```go
category, err := client.Text().
    Model("gemini-2.5-flash-lite").
    Prompt(fmt.Sprintf("Classify this email into: spam, promo, primary:\n\n%s", emailText)).
    Temperature(0.1). // Low temperature for consistent classification
    MaxTokens(10).
    Generate(ctx)
```

**Why:** Classification tasks are ideal for fast models—they're simple, high-volume, and cost-sensitive.

**For sentiment analysis:** `claude-haiku-4-5`
**For complex taxonomy:** `claude-sonnet-4-5` for nuanced categorization

---

### Research and Analysis

**Recommended:** `claude-opus-4-5`, `o3`, `gpt-5.2`, `gemini-3-pro-preview`

```go
analysis, err := client.Text().
    Model("claude-opus-4-5").
    Prompt("Analyze the competitive landscape of AI APIs...").
    Temperature(0.5).
    MaxTokens(4000).
    Generate(ctx)
```

**Why:** Research requires deep reasoning, synthesis of multiple concepts, and high-quality output—perfect for powerful models.

**For quick research:** `claude-sonnet-4-5`
**For mathematical analysis:** `o3` or `o3-pro`

---

### Real-Time Applications

**Recommended:** `claude-haiku-4-5`, `gemini-2.5-flash-lite`, `gpt-4.1-mini`

```go
// Streaming for real-time feel
chunks, err := client.Text().
    Model("gemini-2.5-flash-lite").
    Prompt(userMessage).
    Stream(ctx)

for chunk := range chunks {
    if chunk.Delta != nil {
        sendToUser(chunk.Delta.Content) // Real-time delivery
    }
}
```

**Why:** Real-time applications require low latency. Fast models provide sub-500ms responses.

**For voice assistants:** `gemini-2.5-flash-native-audio-preview-12-2025`
**For live transcription:** `gpt-realtime-mini`

---

### Batch Processing

**Recommended:** `claude-haiku-4-5`, `gemini-2.5-flash-lite`

```go
// Process 10,000 documents overnight
for _, doc := range documents {
    summary, err := client.Text().
        Model("claude-haiku-4-5").
        Prompt(fmt.Sprintf("Summarize: %s", doc)).
        MaxTokens(100).
        Generate(ctx)

    // Save summary
    saveSummary(doc.ID, summary.Text)
}
```

**Why:** Batch processing prioritizes cost over latency. Fast models provide 10-100x cost savings.

**For complex batch tasks:** `claude-sonnet-4-5` if quality is critical

---

## Capability Matrix

### Text Generation

| Capability | Fast | Balanced | Powerful |
|------------|------|----------|----------|
| Simple Q&A | Excellent | Excellent | Excellent |
| Summarization | Good | Excellent | Excellent |
| Creative Writing | Fair | Good | Excellent |
| Technical Writing | Fair | Good | Excellent |

### Code

| Capability | Fast | Balanced | Powerful |
|------------|------|----------|----------|
| Code Generation | Fair | Good | Excellent |
| Code Explanation | Fair | Good | Excellent |
| Code Review | Poor | Good | Excellent |
| Debugging | Poor | Good | Excellent |

### Reasoning

| Capability | Fast | Balanced | Powerful |
|------------|------|----------|----------|
| Logical Reasoning | Poor | Good | Excellent |
| Mathematical | Poor | Fair | Excellent |
| Multi-step Planning | Poor | Good | Excellent |
| Analysis | Fair | Good | Excellent |

### Multimodal

| Capability | Fast | Balanced | Powerful |
|------------|------|----------|----------|
| Image Understanding | N/A | Good | Excellent |
| Audio Processing | N/A | Good | Excellent |
| Video Analysis | N/A | Fair | Good |

---

## Selection Decision Tree

```
Start
 │
 ├─ Is latency critical (<500ms)?
 │  └─ YES → Use Fast Model
 │          │
 │          ├─ Is cost also critical?
 │          │  └─ YES → gemini-2.5-flash-lite or gpt-4.1-mini
 │          └─ NO → claude-haiku-4-5 or gemini-2.5-flash
 │
 └─ NO (Latency acceptable)
    │
    ├─ Is task complex reasoning?
    │  └─ YES → Use Powerful Model
    │          │
    │          ├─ Mathematical/Scientific?
    │          │  └─ YES → o3 or o3-pro
    │          └─ NO → claude-opus-4-5 or gpt-5.2
    │
    └─ NO (Standard complexity)
       └─ Use Balanced Model
               │
               ├─ Code-focused?
               │  └─ YES → gpt-5.1-codex or claude-sonnet-4-5
               ├─ Chat/Conversational?
               │  └─ YES → claude-sonnet-4-5 or gemini-2.5-flash
               └─ General purpose?
                  └─ YES → gpt-4.1 or gemini-2.5-pro
```

---

## Quick Reference

| Use Case | Fast | Balanced | Powerful |
|----------|------|----------|----------|
| Chatbot (FAQ) | Haiku, Flash-lite | | |
| Chatbot (General) | | Sonnet, Flash, GPT-4.1 | |
| Code Generation | | Codex, Sonnet | |
| Code Review | | Sonnet | Opus, O3 |
| Content Creation | | | Opus, GPT-5.2 |
| Data Extraction | Flash, Haiku | Sonnet | |
| Summarization | Haiku, Flash-lite | Sonnet | |
| Classification | Haiku, Flash-lite | Sonnet | |
| Research | | | Opus, O3, GPT-5.2 |
| Real-time | Haiku, Flash-lite | | |
| Batch Processing | Haiku, Flash-lite | | |

---

## Best Practices

1. **Start with balanced models** for development, optimize for production
2. **Measure actual costs** during development—token counts add up quickly
3. **Implement fallback** to cheaper models for degraded performance scenarios
4. **Cache aggressively** for repeated queries
5. **Use structured output** to minimize response tokens
6. **Monitor latency** and adjust model choice based on user experience metrics
7. **A/B test** model choices for your specific use case
8. **Stay updated**—model capabilities and pricing change rapidly

---

## References

- [OpenAI Models](https://platform.openai.com/docs/models)
- [Anthropic Models](https://docs.anthropic.com/en/docs/about-claude/models)
- [Google Gemini Models](https://ai.google.dev/gemini-api/docs/models)
- [OpenRouter Models](https://openrouter.ai/models)
