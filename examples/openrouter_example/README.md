# OpenRouter Example

This example demonstrates how to use Wormhole with OpenRouter, the multi-provider AI gateway that gives you access to 200+ models from different providers through a single API.

## What This Shows

- **Multi-model access**: Test the same prompt across different providers
- **Cost optimization**: Choose models based on task complexity
- **Fallback strategies**: Try multiple models until one succeeds
- **Function calling**: Use advanced features with compatible models
- **Streaming**: Real-time responses with performance comparison
- **Embeddings**: Generate vectors using different embedding models
- **Structured output**: JSON mode with schema validation

## Setup

1. Get an API key from [OpenRouter](https://openrouter.ai/)
2. Set your environment variable:
   ```bash
   export OPENROUTER_API_KEY="your-key-here"
   ```
3. Run the example:
   ```bash
   go run main.go
   ```

## Key Benefits of OpenRouter

- **Access to 200+ models** from OpenAI, Anthropic, Google, Meta, Mistral, and more
- **Pay-per-use pricing** with competitive rates
- **Automatic fallbacks** when models are unavailable
- **Usage analytics** and cost tracking
- **No need for multiple API keys** - one key for all providers

## Models Featured

- `openai/gpt-4o-mini` - OpenAI's efficient model
- `anthropic/claude-3.5-sonnet` - Anthropic's flagship
- `meta-llama/llama-3.1-8b-instruct` - Meta's open model
- `google/gemini-pro` - Google's offering
- `mistralai/mixtral-8x7b-instruct` - Mistral's mixture-of-experts

## What You'll See

The example demonstrates practical patterns for production use:

1. **Basic text generation** across multiple models
2. **Streaming performance comparison** between providers
3. **Function calling** with models that support it
4. **Structured JSON output** with schema validation
5. **Embeddings generation** for semantic search
6. **Cost tracking** and usage optimization
7. **Model comparison** for different tasks

Perfect for understanding how to build robust AI applications that aren't locked into a single provider!