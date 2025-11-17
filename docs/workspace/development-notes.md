# Development Notes

**Internal development content extracted from documentation cleanup (2025-11-16)**

These notes were removed from user-facing docs but preserved for maintainer reference.

---

## Active Refactoring Tasks

### Current Tasks
- [x] Analyze existing provider implementations for consolidation opportunities
- [x] Review WithOpenAICompatible() infrastructure and factory patterns
- [ ] Create comprehensive consolidation implementation plan
- [ ] Map all files requiring modifications with exact changes
- [ ] Design testing strategy for zero-regression validation

### Key Consolidation Findings

**Code Duplication Evidence:**
- Groq provider: 364 lines (groq.go + transform.go)
- Mistral provider: 396 lines (mistral.go + transform.go)
- OpenAI provider: 700+ lines (well-structured, generic)
- All use identical `/chat/completions` endpoint structure
- buildChatPayload() vs buildTextPayload() methods are ~95% identical
- Transform functions follow same patterns across all three providers

**Existing Infrastructure:**
- `WithOpenAICompatible()` function already exists (options.go:147)
- `WithLMStudio()`, `WithVLLM()`, `WithOllamaOpenAI()` already use openai provider via custom factories
- CustomFactories pattern proven and working
- BaseProvider abstraction supports all required functionality

**Current Provider Structure:**
```
pkg/providers/
├── openai/          # Generic, well-structured (target for consolidation)
│   ├── openai.go    # 299 lines, full feature support
│   └── transform.go # 412 lines, robust transforms
├── groq/            # OpenAI-compatible (consolidation candidate)
│   ├── groq.go      # 175 lines, limited features
│   └── transform.go # 364 lines, duplicate logic
└── mistral/         # OpenAI-compatible (consolidation candidate)
    ├── mistral.go   # 227 lines, moderate features
    └── transform.go # 322 lines, duplicate logic
```

**Key API Patterns:**
- All three use `/chat/completions` POST endpoint
- Identical payload structure (model, messages, temperature, max_tokens, tools)
- Same response format (choices[0].message.content, usage, tool_calls)
- Minor differences in parameter names (max_tokens vs max_completion_tokens for GPT-5)

### Backward Compatibility Requirements

**Public API Must Remain Identical:**
- `WithGroq(apiKey string, config ...types.ProviderConfig) Option`
- `WithMistral(config types.ProviderConfig) Option`
- All current functionality preserved
- Same provider-specific error messages and behaviors
- No breaking changes to existing user code

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

## Internal Lessons (Technical Implementation Details)

### Provider Consolidation

**Lesson 1**: BaseURL approach eliminates code duplication
- **Why**: Many providers implement OpenAI's API format
- **Result**: Zero code needed for Groq, Mistral, LM Studio, Ollama
- **Evidence**: WithOpenAICompatible() handles all cases

**Lesson 2**: Provider-specific quirks should be documented, not coded
- **Why**: Unique features (like Mistral OCR) are rare
- **Result**: Clear documentation > complex abstraction
- **Example**: Groq limitations documented in knowledge base

### Performance Optimization

**Lesson 1**: Zero-allocation hot path is achievable in Go
- **Why**: 67ns per request proves it's possible
- **Result**: Connection pooling + pre-allocated slices + minimal interfaces
- **Evidence**: BenchmarkTextGeneration shows 0 B/op, 0 allocs/op

**Lesson 2**: Middleware overhead can be kept under 200ns
- **Why**: Production needs logging, rate limiting, circuit breakers
- **Result**: Atomic operations + lazy initialization
- **Evidence**: BenchmarkWithMiddleware shows 171.5ns total

### Thread Safety

**Lesson 1**: Concurrent map access requires explicit locking
- **Why**: Race detector caught crashes in early versions
- **Result**: sync.RWMutex for all provider map operations
- **Evidence**: BenchmarkConcurrent shows 146ns with thread safety

**Lesson 2**: Double-checked locking pattern reduces contention
- **Why**: Read-heavy workload (many requests, few provider changes)
- **Result**: RWMutex allows concurrent reads
- **Evidence**: No lock contention in production

---

**Source**: Extracted from KNOWLEDGE.md during documentation standardization
**Date**: 2025-11-16
**Purpose**: Maintainer reference for ongoing development
