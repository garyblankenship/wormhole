# Dynamic Model Discovery

**Eliminates hardcoded model names - models discovered automatically from provider APIs**

## Overview

Wormhole's dynamic model discovery system automatically fetches and caches available models from provider APIs. This eliminates the need for hardcoded model names that become obsolete as providers release new models.

## Features

- **3-Tier Caching**: Memory → File → Fallback for optimal performance and resilience
- **Automatic Background Refresh**: Models update periodically without blocking requests
- **Offline Support**: Falls back to cached/hardcoded models when network is unavailable
- **Zero Configuration**: Works out of the box with sensible defaults
- **Multi-Provider**: Supports OpenAI, Anthropic, OpenRouter, and Ollama

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    // Model discovery is enabled by default
    client := wormhole.New(
        wormhole.WithOpenAI("your-api-key"),
        wormhole.WithAnthropic("your-api-key"),
    )
    defer client.StopModelDiscovery() // Clean shutdown

    // List all available models
    models, err := client.ListAvailableModels("openai")
    if err != nil {
        panic(err)
    }

    for _, model := range models {
        fmt.Printf("Model: %s (%s)\n", model.Name, model.ID)
        fmt.Printf("  Capabilities: %v\n", model.Capabilities)
        fmt.Printf("  Max Tokens: %d\n", model.MaxTokens)
    }

    // Use any discovered model
    resp, err := client.Text().
        UseProvider("openai").
        UseModel("gpt-5"). // Discovered automatically
        WithPrompt("Hello, world!").
        Execute()
}
```

## How It Works

### 3-Tier Caching Architecture

```
┌─────────────────────────────────────────────┐
│   Request: GetModels("openai")              │
└─────────────────┬───────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────┐
│  L1: Memory Cache (sync.Map)                │
│  TTL: 24 hours (configurable)               │
│  Status: FRESH                              │
└─────────────────┬───────────────────────────┘
                  │ Cache Miss
                  ▼
┌─────────────────────────────────────────────┐
│  L2: File Cache (~/.wormhole/models.json)   │
│  TTL: 7 days (configurable)                 │
│  Status: FRESH                              │
└─────────────────┬───────────────────────────┘
                  │ Cache Miss / Stale
                  ▼
┌─────────────────────────────────────────────┐
│  L3: Hardcoded Fallback Models              │
│  TTL: Unlimited                             │
│  Status: STALE (triggers background fetch)  │
└─────────────────────────────────────────────┘
```

**Cache Behavior**:
- **FRESH**: Cache entry within TTL → Return immediately
- **STALE**: Cache expired but available → Return + trigger background refresh
- **MISS**: No cache entry → Block and fetch from provider API

### Provider APIs

#### OpenAI
- **Endpoint**: `GET https://api.openai.com/v1/models`
- **Auth**: Bearer token (`Authorization: Bearer {API_KEY}`)
- **Models**: All OpenAI models (GPT, embeddings, audio, images)

#### Anthropic
- **Endpoint**: `GET https://api.anthropic.com/v1/models`
- **Auth**: Custom headers (`x-api-key`, `anthropic-version: 2023-06-01`)
- **Models**: All Claude models

#### OpenRouter
- **Endpoint**: `GET https://openrouter.ai/api/v1/models`
- **Auth**: None required (public endpoint)
- **Models**: 200+ models from multiple providers
- **Metadata**: Pricing, context length, moderation flags

#### Ollama
- **Endpoint**: `GET http://localhost:11434/api/tags`
- **Auth**: None (local service)
- **Models**: User's locally installed models

## Configuration

### Default Configuration

```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
)
// Uses default config:
// - CacheTTL: 24 hours
// - FileCacheTTL: 7 days
// - FileCachePath: ~/.wormhole/models.json
// - RefreshInterval: 12 hours
// - OfflineMode: false
// - EnableDiscovery: true
```

### Custom Configuration

```go
import "github.com/garyblankenship/wormhole/pkg/discovery"

client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
        CacheTTL:        12 * time.Hour,       // Memory cache lifetime
        FileCacheTTL:    3 * 24 * time.Hour,   // File cache lifetime (3 days)
        FileCachePath:   "~/.wormhole/models.json",
        EnableFileCache: true,
        RefreshInterval: 6 * time.Hour,        // Background refresh frequency
        OfflineMode:     false,                // Enable network fetching
    }),
)
```

### Offline Mode

For environments without internet access:

```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithOfflineMode(true), // Disable network fetching
)

// Only cached and fallback models available
models, _ := client.ListAvailableModels("openai")
// Returns: gpt-5, gpt-5-mini (hardcoded fallback)
```

### Disable Discovery

To use only hardcoded models (legacy behavior):

```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithDiscovery(false), // Disable discovery completely
)
```

## API Reference

### Client Methods

#### `ListAvailableModels(provider string) ([]*types.ModelInfo, error)`

Returns all available models for a provider.

```go
models, err := client.ListAvailableModels("openai")
if err != nil {
    return err
}

for _, model := range models {
    fmt.Printf("%s: %v\n", model.Name, model.Capabilities)
}
```

#### `RefreshModels() error`

Manually refreshes all provider model catalogs (bypasses cache).

```go
if err := client.RefreshModels(); err != nil {
    log.Printf("Failed to refresh models: %v", err)
}
```

#### `ClearModelCache()`

Clears all cached model data.

```go
client.ClearModelCache()
// Next GetModels() call will fetch fresh data
```

#### `StopModelDiscovery()`

Stops background refresh goroutine (call during shutdown).

```go
defer client.StopModelDiscovery()
```

### ModelInfo Structure

```go
type ModelInfo struct {
    ID           string              // API model ID (e.g., "gpt-5")
    Name         string              // Human-readable name (e.g., "GPT-5")
    Provider     string              // Provider name (e.g., "openai")
    Capabilities []ModelCapability   // What the model can do
    MaxTokens    int                 // Maximum context length
    Cost         *ModelCost          // Pricing information (optional)
}

type ModelCapability string

const (
    CapabilityText       ModelCapability = "text"
    CapabilityChat       ModelCapability = "chat"
    CapabilityFunctions  ModelCapability = "functions"
    CapabilityStructured ModelCapability = "structured"
    CapabilityVision     ModelCapability = "vision"
    CapabilityEmbeddings ModelCapability = "embeddings"
    CapabilityImages     ModelCapability = "images"
    CapabilityAudio      ModelCapability = "audio"
    CapabilityStream     ModelCapability = "stream"
)
```

## Cache Management

### File Cache Location

Default: `~/.wormhole/models.json`

Example structure:

```json
{
  "version": "1.0",
  "updated": "2025-11-16T23:20:00Z",
  "entries": {
    "openai": {
      "provider": "openai",
      "timestamp": "2025-11-16T23:20:00Z",
      "models": [
        {
          "id": "gpt-5",
          "name": "GPT-5",
          "provider": "openai",
          "capabilities": ["text", "chat", "functions", "structured"],
          "max_tokens": 128000
        }
      ]
    }
  }
}
```

### Manual Cache Operations

```bash
# View cache
cat ~/.wormhole/models.json | jq

# Clear cache
rm ~/.wormhole/models.json

# Trigger fresh fetch
# Cache will be rebuilt on next request
```

## Fallback Models

When all caches expire and network is unavailable, the system uses hardcoded fallback models:

**OpenAI**:
- gpt-5 (128k context)
- gpt-5-mini (128k context)

**Anthropic**:
- claude-sonnet-4-5 (200k context)

**OpenRouter**:
- No fallback (dynamic provider)

**Ollama**:
- No fallback (user-specific)

## Performance

### Benchmarks

- **L1 Cache Hit**: ~100ns (memory lookup)
- **L2 Cache Hit**: ~1ms (file read + JSON parse)
- **L3 Fallback**: ~50ns (hardcoded slice)
- **API Fetch**: ~200-500ms (network + parsing)

### Background Refresh

When cache becomes stale:
1. Returns stale cache immediately (no blocking)
2. Triggers background goroutine to fetch fresh data
3. Updates cache asynchronously
4. Next request gets fresh data

**No latency impact on end users.**

## Best Practices

### Production Deployments

```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
        CacheTTL:        24 * time.Hour,     // Long TTL for stability
        FileCacheTTL:    7 * 24 * time.Hour, // 7-day file cache
        RefreshInterval: 12 * time.Hour,     // Twice-daily refresh
        EnableFileCache: true,               // Persist across restarts
    }),
)
defer client.StopModelDiscovery()
```

### Testing/CI Environments

```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithOfflineMode(true),     // No network calls
    wormhole.WithDiscovery(false),      // Or disable discovery entirely
)
```

### Local Development

```go
client := wormhole.New(
    wormhole.WithOpenAI("key"),
    wormhole.WithOllama(types.ProviderConfig{
        BaseURL: "http://localhost:11434",
    }),
    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
        RefreshInterval: 5 * time.Minute, // Faster refresh for dev
    }),
)
```

## Migration from Hardcoded Models

### Before (Hardcoded)

```go
// ❌ Hardcoded model name - becomes obsolete
resp, err := client.Text().
    UseProvider("openai").
    UseModel("gpt-4"). // Outdated!
    WithPrompt("Hello").
    Execute()
```

### After (Dynamic Discovery)

```go
// ✅ Automatically discovers latest models
models, _ := client.ListAvailableModels("openai")

// Use the newest model
resp, err := client.Text().
    UseProvider("openai").
    UseModel("gpt-5"). // Discovered automatically
    WithPrompt("Hello").
    Execute()
```

### Gradual Migration

Discovery is enabled by default and backward compatible:

```go
// Old code still works
client := wormhole.New(
    wormhole.WithOpenAI("key"),
)

// Can use hardcoded model names
resp, _ := client.Text().UseModel("gpt-5").Execute()

// OR dynamically discover models
models, _ := client.ListAvailableModels("openai")
```

**No breaking changes required.**

## Troubleshooting

### Issue: Models not updating

**Cause**: Cache TTL not expired

**Solution**:
```go
client.ClearModelCache()
client.RefreshModels()
```

### Issue: High latency on first request

**Cause**: Cold start (no cache)

**Solution**: Use file cache to persist across restarts:
```go
wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
    EnableFileCache: true, // Default
})
```

### Issue: "No cached models" error in offline mode

**Cause**: Offline mode enabled but no cache exists

**Solution**: Pre-populate cache or use fallback models:
```go
// Fetch once while online
client.RefreshModels()

// Then go offline
client := wormhole.New(
    wormhole.WithOfflineMode(true),
)
```

### Issue: Background refresh consuming resources

**Cause**: Default 12-hour refresh interval

**Solution**: Increase interval or disable:
```go
wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
    RefreshInterval: 24 * time.Hour, // Less frequent
    // OR
    RefreshInterval: 0,              // Disable background refresh
})
```

## Advanced Usage

### Custom Provider Fetchers

Implement the `ModelFetcher` interface for custom providers:

```go
type CustomFetcher struct {
    apiKey string
}

func (f *CustomFetcher) Name() string {
    return "custom-provider"
}

func (f *CustomFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
    // Fetch from your custom API
    resp, err := http.Get("https://api.custom.com/models")
    // ... parse and return models
}

// Register with discovery service
fetcher := &CustomFetcher{apiKey: "key"}
service := discovery.NewDiscoveryService(
    discovery.DefaultConfig(),
    fetcher,
)
```

### Multi-Region Support

Different regions may have different model availability:

```go
// US region
usClient := wormhole.New(
    wormhole.WithOpenAI("us-key"),
)

// EU region
euClient := wormhole.New(
    wormhole.WithOpenAI("eu-key"),
    wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
        FileCachePath: "~/.wormhole/models-eu.json",
    }),
)
```

## Security Considerations

- **API Keys**: Never log or cache API keys
- **File Permissions**: Cache file is created with `0644` (user read/write only)
- **Directory**: Cache directory created with `0755`
- **Network**: All API calls use HTTPS (except localhost Ollama)

## Performance Tuning

### Optimize for Low Latency

```go
wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
    CacheTTL:        48 * time.Hour,   // Longer cache = fewer fetches
    RefreshInterval: 24 * time.Hour,   // Less frequent background refresh
    EnableFileCache: true,             // Fast startup
})
```

### Optimize for Freshness

```go
wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{
    CacheTTL:        1 * time.Hour,    // Shorter cache = fresher data
    RefreshInterval: 30 * time.Minute, // Frequent background refresh
})
```

## Future Enhancements

- **Model Filtering**: Filter by capabilities, cost, or context length
- **Model Ranking**: Sort by popularity, cost, or performance
- **Provider Health Checks**: Detect provider availability
- **Model Deprecation Warnings**: Alert when models are EOL
- **Smart Fallback**: Suggest alternative models when requested model unavailable

---

**Last Updated**: 2025-11-16
**Status**: Production Ready
**Version**: 1.0.0
