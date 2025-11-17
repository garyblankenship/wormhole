# Dynamic Model Discovery & Caching System

**Problem**: Hardcoded model names become obsolete immediately. Every provider release requires code changes.

**Solution**: Fetch models dynamically from provider APIs, cache with TTL, eliminate hardcoding.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Wormhole Client                          │
│                                                             │
│  User Request → Model Registry (Check cache)               │
│                      ↓                                      │
│                 Cache Hit? ──Yes──> Return Models           │
│                      ↓ No                                   │
│              Fetch from Providers (Background)              │
│                      ↓                                      │
│            Update Cache + Return Fallback                   │
└─────────────────────────────────────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────────────┐
│              Model Discovery Service                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Provider Fetchers (Parallel)                       │   │
│  │  - OpenAI: GET /v1/models                          │   │
│  │  - Anthropic: GET /v1/models                       │   │
│  │  - OpenRouter: GET /api/v1/models                  │   │
│  │  - Ollama: GET /api/tags                           │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                       ↓
┌─────────────────────────────────────────────────────────────┐
│                   Cache Layer (Multi-Tier)                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  L1: In-Memory (sync.Map) - 24h TTL                │   │
│  │  L2: File-based (~/.wormhole/models.json) - 7d TTL │   │
│  │  L3: Hardcoded Fallback (Minimal) - No expiration  │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Provider APIs

### 1. OpenAI

**Endpoint**: `GET https://api.openai.com/v1/models`

**Auth**: `Authorization: Bearer $OPENAI_API_KEY`

**Response Format**:
```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-5",
      "object": "model",
      "created": 1725484800,
      "owned_by": "openai"
    }
  ]
}
```

**Capabilities Detection**: Infer from model name patterns
- `gpt-*`: Text, chat, functions, structured
- `text-embedding-*`: Embeddings
- `whisper-*`: Audio
- `dall-e-*`: Images

### 2. Anthropic

**Endpoint**: `GET https://api.anthropic.com/v1/models`

**Auth**:
- `x-api-key: $ANTHROPIC_API_KEY`
- `anthropic-version: 2023-06-01`

**Response Format**:
```json
{
  "data": [
    {
      "id": "claude-sonnet-4-5-20250929",
      "display_name": "Claude Sonnet 4.5",
      "created_at": "2025-09-29T00:00:00Z",
      "type": "model"
    }
  ],
  "has_more": false
}
```

**Capabilities Detection**: All Claude models support text, chat, functions

### 3. OpenRouter

**Endpoint**: `GET https://openrouter.ai/api/v1/models`

**Auth**: None required for model list (optional: `Authorization: Bearer $OPENROUTER_API_KEY`)

**Response Format**:
```json
{
  "data": [
    {
      "id": "anthropic/claude-sonnet-4-5",
      "name": "Claude Sonnet 4.5",
      "pricing": {
        "prompt": "0.000003",
        "completion": "0.000015"
      },
      "context_length": 200000,
      "architecture": {
        "modality": "text+image->text",
        "tokenizer": "Claude",
        "instruct_type": null
      },
      "top_provider": {
        "max_completion_tokens": 8000,
        "is_moderated": false
      },
      "per_request_limits": null
    }
  ]
}
```

**Advantages**:
- **Most comprehensive metadata** (pricing, context length, capabilities)
- **200+ models** from all providers
- **No auth required** for model list

### 4. Ollama (Local)

**Endpoint**: `GET http://localhost:11434/api/tags`

**Auth**: None (local)

**Response Format**:
```json
{
  "models": [
    {
      "name": "llama2:latest",
      "modified_at": "2023-11-04T14:56:49Z",
      "size": 7365960935,
      "digest": "sha256:...",
      "details": {
        "format": "gguf",
        "family": "llama",
        "parameter_size": "7B"
      }
    }
  ]
}
```

---

## Implementation Plan

### Phase 1: Core Discovery Service

**File**: `pkg/discovery/discovery.go`

```go
package discovery

import (
    "context"
    "sync"
    "time"
    "github.com/garyblankenship/wormhole/pkg/types"
)

// DiscoveryService fetches and caches models from providers
type DiscoveryService struct {
    cache      *ModelCache
    fetchers   map[string]ModelFetcher
    config     DiscoveryConfig
    mu         sync.RWMutex
}

type DiscoveryConfig struct {
    CacheTTL         time.Duration // Default: 24h
    FileCachePath    string        // Default: ~/.wormhole/models.json
    EnableFileCache  bool          // Default: true
    RefreshInterval  time.Duration // Default: 12h (background refresh)
    OfflineMode      bool          // Default: false (disable auto-fetch)
}

type ModelFetcher interface {
    Name() string
    FetchModels(ctx context.Context) ([]*types.ModelInfo, error)
}

// RefreshModels fetches latest models from all providers
func (s *DiscoveryService) RefreshModels(ctx context.Context) error {
    // Parallel fetch from all providers
    // Update cache
    // Persist to file cache
}

// GetModels returns cached models (fetch if expired)
func (s *DiscoveryService) GetModels(ctx context.Context, provider string) ([]*types.ModelInfo, error) {
    // Check L1 cache (memory)
    // Check L2 cache (file)
    // Fetch from provider (background if stale)
    // Return fallback if all fail
}

// StartBackgroundRefresh runs periodic model updates
func (s *DiscoveryService) StartBackgroundRefresh(ctx context.Context) {
    // Ticker for RefreshInterval
    // Refresh all providers
    // Log errors but don't block
}
```

### Phase 2: Provider Fetchers

**File**: `pkg/discovery/fetchers/openai.go`

```go
package fetchers

import (
    "context"
    "encoding/json"
    "net/http"
    "github.com/garyblankenship/wormhole/pkg/types"
)

type OpenAIFetcher struct {
    apiKey  string
    baseURL string // Default: https://api.openai.com/v1
}

func (f *OpenAIFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
    // GET /v1/models
    // Parse response
    // Map to types.ModelInfo with inferred capabilities
    // Return models
}

// inferCapabilities determines model capabilities from model ID
func (f *OpenAIFetcher) inferCapabilities(modelID string) []types.ModelCapability {
    switch {
    case strings.HasPrefix(modelID, "gpt-"):
        return []types.ModelCapability{
            types.CapabilityText,
            types.CapabilityChat,
            types.CapabilityFunctions,
            types.CapabilityStructured,
        }
    case strings.HasPrefix(modelID, "text-embedding-"):
        return []types.ModelCapability{types.CapabilityEmbeddings}
    case strings.HasPrefix(modelID, "whisper-"):
        return []types.ModelCapability{types.CapabilityAudio}
    case strings.HasPrefix(modelID, "dall-e-"):
        return []types.ModelCapability{types.CapabilityImages}
    default:
        return []types.ModelCapability{types.CapabilityText}
    }
}
```

**File**: `pkg/discovery/fetchers/anthropic.go`

```go
type AnthropicFetcher struct {
    apiKey     string
    baseURL    string
    apiVersion string // Default: 2023-06-01
}

func (f *AnthropicFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
    // GET /v1/models
    // Headers: x-api-key, anthropic-version
    // Parse response
    // All Claude models have same capabilities
}
```

**File**: `pkg/discovery/fetchers/openrouter.go`

```go
type OpenRouterFetcher struct {
    apiKey  string // Optional (public endpoint works without auth)
    baseURL string // Default: https://openrouter.ai/api/v1
}

func (f *OpenRouterFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
    // GET /api/v1/models
    // No auth required for listing
    // Parse comprehensive metadata (pricing, context, capabilities)
    // Return 200+ models
}
```

**File**: `pkg/discovery/fetchers/ollama.go`

```go
type OllamaFetcher struct {
    baseURL string // Default: http://localhost:11434
}

func (f *OllamaFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
    // GET /api/tags
    // Parse local models
    // All local models have text capability
}
```

### Phase 3: Cache Layer

**File**: `pkg/discovery/cache.go`

```go
package discovery

import (
    "encoding/json"
    "os"
    "sync"
    "time"
    "github.com/garyblankenship/wormhole/pkg/types"
)

type ModelCache struct {
    memory     *sync.Map                    // L1: In-memory cache
    filePath   string                       // L2: File cache path
    ttl        time.Duration                // Cache TTL
    fallback   map[string][]*types.ModelInfo // L3: Hardcoded fallback
    mu         sync.RWMutex
}

type CacheEntry struct {
    Models    []*types.ModelInfo
    Timestamp time.Time
}

// Get retrieves models from cache (L1 → L2 → L3)
func (c *ModelCache) Get(provider string) ([]*types.ModelInfo, bool) {
    // L1: Check memory cache
    if entry, ok := c.memory.Load(provider); ok {
        cached := entry.(*CacheEntry)
        if time.Since(cached.Timestamp) < c.ttl {
            return cached.Models, true
        }
    }

    // L2: Check file cache
    if models, ok := c.loadFromFile(provider); ok {
        c.memory.Store(provider, &CacheEntry{
            Models:    models,
            Timestamp: time.Now(),
        })
        return models, true
    }

    // L3: Return fallback
    if models, ok := c.fallback[provider]; ok {
        return models, false // Indicate fallback used
    }

    return nil, false
}

// Set stores models in cache (L1 + L2)
func (c *ModelCache) Set(provider string, models []*types.ModelInfo) {
    entry := &CacheEntry{
        Models:    models,
        Timestamp: time.Now(),
    }

    // L1: Memory cache
    c.memory.Store(provider, entry)

    // L2: File cache
    c.saveToFile(provider, models)
}

// loadFromFile loads models from ~/.wormhole/models.json
func (c *ModelCache) loadFromFile(provider string) ([]*types.ModelInfo, bool) {
    // Read file cache
    // Check timestamp (7 day TTL for file cache)
    // Parse JSON
    // Return models
}

// saveToFile persists models to ~/.wormhole/models.json
func (c *ModelCache) saveToFile(provider string, models []*types.ModelInfo) {
    // Marshal to JSON
    // Write to file (atomic write)
}
```

### Phase 4: Integration with Wormhole

**File**: `pkg/wormhole/wormhole.go` (modifications)

```go
type Wormhole struct {
    // ... existing fields ...
    discovery  *discovery.DiscoveryService
}

// New creates a new Wormhole client with dynamic model discovery
func New(options ...Option) *Wormhole {
    // ... existing code ...

    // Initialize discovery service
    w.discovery = discovery.NewDiscoveryService(discovery.DiscoveryConfig{
        CacheTTL:        24 * time.Hour,
        FileCachePath:   "~/.wormhole/models.json",
        EnableFileCache: true,
        RefreshInterval: 12 * time.Hour,
    })

    // Start background refresh
    go w.discovery.StartBackgroundRefresh(context.Background())

    return w
}

// ListAvailableModels returns all available models (dynamic)
func (w *Wormhole) ListAvailableModels(ctx context.Context, provider string) ([]*types.ModelInfo, error) {
    return w.discovery.GetModels(ctx, provider)
}

// RefreshModels manually triggers model discovery
func (w *Wormhole) RefreshModels(ctx context.Context) error {
    return w.discovery.RefreshModels(ctx)
}
```

**File**: `pkg/wormhole/options.go` (modifications)

```go
// Remove hardcoded model registries
// Keep minimal fallback for offline mode

var fallbackModels = map[string][]*types.ModelInfo{
    "openai": {
        {ID: "gpt-5", Name: "GPT-5", Provider: "openai"},
        {ID: "gpt-5-mini", Name: "GPT-5 Mini", Provider: "openai"},
    },
    "anthropic": {
        {ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Provider: "anthropic"},
    },
    "openrouter": {
        // Empty - OpenRouter is dynamic by design
    },
    "ollama": {
        // Empty - Local models vary per installation
    },
}

// WithDiscoveryConfig configures model discovery
func WithDiscoveryConfig(config discovery.DiscoveryConfig) Option {
    return func(w *Wormhole) error {
        w.discovery = discovery.NewDiscoveryService(config)
        return nil
    }
}

// WithOfflineMode disables model discovery (use fallback only)
func WithOfflineMode() Option {
    return WithDiscoveryConfig(discovery.DiscoveryConfig{
        OfflineMode: true,
    })
}
```

---

## Configuration Options

### Environment Variables

```bash
# Cache TTL (duration)
WORMHOLE_MODEL_CACHE_TTL=24h

# File cache path
WORMHOLE_MODEL_CACHE_PATH=~/.wormhole/models.json

# Background refresh interval
WORMHOLE_MODEL_REFRESH_INTERVAL=12h

# Disable auto-discovery (offline mode)
WORMHOLE_OFFLINE_MODE=false

# Disable file cache (memory only)
WORMHOLE_DISABLE_FILE_CACHE=false
```

### Programmatic Configuration

```go
client := wormhole.New(
    wormhole.WithOpenAI("sk-..."),
    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
        CacheTTL:         6 * time.Hour,     // Refresh more frequently
        RefreshInterval:  3 * time.Hour,     // Background refresh every 3h
        EnableFileCache:  true,              // Persist to disk
        OfflineMode:      false,             // Allow network calls
    }),
)

// Manual refresh when needed
err := client.RefreshModels(context.Background())

// Get available models for provider
models, err := client.ListAvailableModels(ctx, "openai")
for _, model := range models {
    fmt.Printf("%s: %s\n", model.ID, model.Name)
}
```

---

## Benefits

### 1. **Always Up-to-Date**
- No code changes when providers release new models
- Models available within 12-24 hours (configurable)
- Instant access with manual refresh

### 2. **Zero Hardcoding**
- Model names fetched dynamically
- Capabilities inferred or fetched from metadata
- Pricing updated automatically (OpenRouter)

### 3. **Resilient**
- Multi-tier caching (memory → file → fallback)
- Works offline with cached data
- Graceful degradation on API failures

### 4. **Performance**
- In-memory cache = instant lookups
- Background refresh = no blocking
- Parallel provider fetching = fast updates

### 5. **Comprehensive Metadata**
- OpenRouter provides pricing, context length, capabilities
- Can show users estimated costs before requests
- Smart model selection based on capabilities

---

## Migration Strategy

### Phase 1: Implement Discovery (Non-Breaking)
- Add `pkg/discovery/` package
- Keep existing hardcoded models
- Add `ListAvailableModels()` API

### Phase 2: Enable by Default (Gradual)
- Initialize discovery service by default
- Fall back to hardcoded if discovery fails
- Log warnings when using fallback

### Phase 3: Remove Hardcoded Models (Breaking)
- Remove hardcoded model registries
- Keep minimal fallback for offline
- Update documentation

### Phase 4: Advanced Features
- Model capability filtering
- Cost estimation before requests
- Model recommendations based on task

---

## File Structure

```
pkg/
├── discovery/
│   ├── discovery.go          # Core discovery service
│   ├── cache.go              # Multi-tier cache
│   ├── fallback.go           # Minimal hardcoded fallback
│   ├── fetchers/
│   │   ├── openai.go         # OpenAI model fetcher
│   │   ├── anthropic.go      # Anthropic model fetcher
│   │   ├── openrouter.go     # OpenRouter model fetcher
│   │   └── ollama.go         # Ollama model fetcher
│   └── discovery_test.go     # Tests
```

---

## Success Criteria

✅ **Zero hardcoded model names** (except minimal fallback)
✅ **Models auto-update** within configured TTL
✅ **Works offline** with cached/fallback data
✅ **No performance degradation** (cached lookups)
✅ **Backward compatible** with existing code

---

## Timeline

- **Week 1**: Core discovery service + cache layer
- **Week 2**: Provider fetchers (OpenAI, Anthropic, OpenRouter, Ollama)
- **Week 3**: Integration with Wormhole + testing
- **Week 4**: Documentation + release

---

## Open Questions

1. **Should we fetch models on every client initialization?**
   - **Recommendation**: No, load from cache first, refresh in background

2. **How to handle API rate limits?**
   - **Recommendation**: Aggressive caching (24h TTL), exponential backoff on failures

3. **Should file cache be enabled by default?**
   - **Recommendation**: Yes, improves offline experience and reduces API calls

4. **How to detect model capabilities?**
   - **OpenRouter**: Provided in API response (best)
   - **OpenAI/Anthropic**: Infer from model name patterns
   - **Future**: Fetch from provider documentation endpoints

5. **Should we support custom provider fetchers?**
   - **Recommendation**: Yes, interface-based design allows users to add custom providers

---

## Next Steps

1. ✅ Research provider APIs (DONE)
2. 🔄 Implement core discovery service
3. ⏳ Implement provider fetchers
4. ⏳ Create caching layer
5. ⏳ Integrate with Wormhole
6. ⏳ Write tests and documentation
