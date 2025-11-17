# Wormhole SDK Feature Roadmap

*This is a local-only roadmap for tracking potential feature additions to the Wormhole SDK based on competitive analysis with frameworks like LangChain and Vercel's AI SDK.*

## Current State Analysis

Wormhole is exceptionally strong for its core purpose: providing a fast, reliable, and unified interface for text and structured data generation from multiple LLMs. The SDK excels at:

- Multi-provider support with unified API
- Sub-microsecond latency (67ns core overhead)
- Enterprise-grade middleware (circuit breakers, rate limiting, retry logic)
- Thread-safe concurrent operations
- Clean architecture with capability-based interfaces

## Missing Feature Categories

### 1. Multi-Modal Capabilities
Current API focuses on `Text()` and `Structured()` generation. Missing:
- Image generation (text-to-image)
- Embeddings for RAG/semantic search
- Vision capabilities (image-to-text)
- Audio processing (speech-to-text/text-to-speech)

### 2. Advanced Orchestration
Provides foundational blocks but lacks higher-level abstractions:
- Native function calling/tool use
- Chain orchestration
- RAG components and helpers

### 3. Developer Experience
- Token counting utilities
- Prompt templating system
- Context window management

---

## Tier 1: Core AI Capabilities (Highest Priority)

### 1. Embeddings API
**Implementation**: `client.Embeddings().Model(...).Input(...).Create(ctx)`
- **Priority**: Critical - #1 missing piece for RAG applications
- **Providers**: OpenAI, Anthropic, Google, Cohere
- **Use Cases**: Semantic search, recommendations, RAG retrieval

### 2. Native Tool Use / Function Calling
**Implementation**: `client.ToolCall()` or similar API
- **Priority**: High - Standard for agent applications
- **Features**: Go function/struct registration, dynamic model selection
- **Advantage**: More powerful than fixed structured output

### 3. Vision API (Image Input)
**Implementation**: Enhance `client.Text()` and `client.Structured()` for image inputs
- **Priority**: High - Multi-modal is standard for flagship models
- **Input Types**: URLs, byte arrays, base64
- **Providers**: OpenAI GPT-4V, Claude 3.5 Sonnet, Gemini

### 4. Image Generation API
**Implementation**: `client.Images().Model(...).Prompt(...).Generate(ctx)`
- **Priority**: Medium-High - Opens new application categories
- **Providers**: OpenAI DALL-E, Stability AI, Midjourney
- **Features**: Size control, style parameters, batch generation

## ✅ Already Supported via OpenAI-Compatible API

### Groq and Mistral Support
**Status**: ✅ **Already Available** - No separate implementation needed
- **Implementation**: Use existing OpenAI provider with `BaseURL()` method
- **Groq Example**: `client.Embeddings().BaseURL("https://api.groq.com/openai/v1").Model("mixtral-8x7b-32768")`
- **Mistral Example**: `client.Embeddings().BaseURL("https://api.mistral.ai/v1").Model("mistral-large-latest")`
- **Why This Approach**: Follows Wormhole's philosophy of eliminating complexity - no separate provider packages needed for APIs that speak OpenAI protocol

---

## Tier 2: Application Layer Abstractions

### 5. RAG Helpers
**Implementation**: `rag` sub-package with core utilities
- **Priority**: High - Most common LLM application pattern
- **Features**: Context management, document stuffing, result re-ranking
- **Scope**: Utilities, not full framework

### 6. Token-Aware Utilities
**Implementation**: `client.CountTokens(model, text)` and text splitters
- **Priority**: High - Fundamental context window management
- **Features**: Model-specific counting, document chunking, smart splitting
- **Use Cases**: Context limit avoidance, cost optimization

### 7. Conversation History Management
**Implementation**: Helper object or middleware for chat history
- **Priority**: Medium - Common chatbot requirement
- **Features**: Automatic history management, summarization strategies, token windowing
- **Integration**: Works with existing middleware system

---

## Tier 3: Enterprise & Ecosystem Enhancements

### 8. Deeper Observability (OpenTelemetry)
**Implementation**: `OpenTelemetryMiddleware` with standardized traces/metrics
- **Priority**: Medium - Enterprise integration requirement
- **Features**: Distributed tracing, performance metrics, error tracking
- **Integration**: Existing enterprise monitoring platforms

### 9. Content Moderation & Security
**Implementation**: `ModerationMiddleware` and `PIIRedactionMiddleware`
- **Priority**: Medium - Production safety requirement
- **Features**: Prompt/response checking, PII detection, content filtering
- **Providers**: OpenAI Moderation API, custom filters

### 10. Extensible Agent Framework
**Implementation**: `AgentExecutor` for orchestrating tool calls and reasoning
- **Priority**: Low-Medium - Advanced use case
- **Features**: Agentic loops, multi-step reasoning, autonomous task completion
- **Positioning**: Compete with LangChain agent capabilities

---

## Implementation Considerations

### Architecture Principles
- Maintain 67ns core latency performance
- Preserve capability-based provider system
- Extend existing middleware architecture
- Follow Go best practices and clean architecture

### Backward Compatibility
- All additions should be opt-in
- Existing APIs must remain unchanged
- New features should integrate with current middleware

### Provider Support
- Prioritize features supported by multiple providers
- Graceful degradation for provider-specific limitations
- Clear documentation of provider capabilities

### Performance Impact
- New features must not impact core text/structured generation performance
- Separate execution paths for advanced features
- Memory-efficient implementations with zero-allocation hot paths where possible

---

## Success Metrics

### Technical
- Maintain sub-microsecond core latency
- Zero breaking changes to existing API
- Comprehensive test coverage for new features
- Documentation completeness

### Adoption
- Developer feedback on missing features
- Community contributions to new capabilities
- Enterprise adoption of advanced features
- Competitive positioning vs. LangChain/Vercel AI SDK

---

*Last Updated: August 2025*
*Status: Planning Phase - No implementation commitments*