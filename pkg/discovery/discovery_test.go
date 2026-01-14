package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// MockFetcher implements ModelFetcher for testing
type MockFetcher struct {
	name       string
	models     []*types.ModelInfo
	shouldFail bool
}

func (m *MockFetcher) Name() string {
	return m.name
}

func (m *MockFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	if m.shouldFail {
		return nil, ctx.Err()
	}
	return m.models, nil
}

func TestDiscoveryService_GetModels(t *testing.T) {
	mockModels := []*types.ModelInfo{
		{
			ID:       "test-model-1",
			Name:     "Test Model 1",
			Provider: "test",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
			},
			MaxTokens: 8192,
		},
		{
			ID:       "test-model-2",
			Name:     "Test Model 2",
			Provider: "test",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
			},
			MaxTokens: 4096,
		},
	}

	mockFetcher := &MockFetcher{
		name:   "test",
		models: mockModels,
	}

	config := DiscoveryConfig{
		CacheTTL:        1 * time.Hour,
		EnableFileCache: false, // Disable file cache for tests
		OfflineMode:     false,
	}

	service := NewDiscoveryService(config, mockFetcher)

	ctx := context.Background()

	// First call should fetch from provider
	models, err := service.GetModels(ctx, "test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	if models[0].ID != "test-model-1" {
		t.Errorf("Expected first model ID to be 'test-model-1', got %s", models[0].ID)
	}

	// Second call should return cached models
	cachedModels, err := service.GetModels(ctx, "test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(cachedModels) != 2 {
		t.Errorf("Expected 2 cached models, got %d", len(cachedModels))
	}
}

func TestDiscoveryService_OfflineMode(t *testing.T) {
	config := DiscoveryConfig{
		CacheTTL:        1 * time.Hour,
		EnableFileCache: false,
		OfflineMode:     true, // Offline mode enabled
	}

	service := NewDiscoveryService(config)

	ctx := context.Background()

	// Should return fallback models for known providers
	models, err := service.GetModels(ctx, "openai")
	if err != nil {
		t.Fatalf("Expected fallback models in offline mode, got error: %v", err)
	}

	if len(models) == 0 {
		t.Error("Expected fallback models, got none")
	}

	// Should error for unknown providers
	_, err = service.GetModels(ctx, "unknown-provider")
	if err == nil {
		t.Error("Expected error for uncached unknown provider in offline mode")
	}
}

func TestDiscoveryService_RefreshModels(t *testing.T) {
	mockModels := []*types.ModelInfo{
		{
			ID:       "test-model",
			Name:     "Test Model",
			Provider: "test",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
			},
			MaxTokens: 8192,
		},
	}

	mockFetcher := &MockFetcher{
		name:   "test",
		models: mockModels,
	}

	config := DiscoveryConfig{
		CacheTTL:        1 * time.Hour,
		EnableFileCache: false,
		OfflineMode:     false,
	}

	service := NewDiscoveryService(config, mockFetcher)

	ctx := context.Background()

	// Refresh all providers
	err := service.RefreshModels(ctx)
	if err != nil {
		t.Fatalf("Expected no error during refresh, got %v", err)
	}

	// Verify models are cached
	models, err := service.GetModels(ctx, "test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(models) != 1 {
		t.Errorf("Expected 1 model after refresh, got %d", len(models))
	}
}

func TestDiscoveryService_ClearCache(t *testing.T) {
	mockModels := []*types.ModelInfo{
		{
			ID:       "test-model",
			Name:     "Test Model",
			Provider: "test",
		},
	}

	mockFetcher := &MockFetcher{
		name:   "test",
		models: mockModels,
	}

	config := DiscoveryConfig{
		CacheTTL:        1 * time.Hour,
		EnableFileCache: false,
		OfflineMode:     false,
	}

	service := NewDiscoveryService(config, mockFetcher)

	ctx := context.Background()

	// Fetch to populate cache
	_, err := service.GetModels(ctx, "test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Clear cache
	service.ClearCache()

	// After clearing, should fetch again (not from cache)
	models, err := service.GetModels(ctx, "test")
	if err != nil {
		t.Fatalf("Expected no error after cache clear, got %v", err)
	}

	if len(models) != 1 {
		t.Errorf("Expected 1 model after cache clear and re-fetch, got %d", len(models))
	}
}

func TestDiscoveryService_UnknownProvider(t *testing.T) {
	config := DiscoveryConfig{
		CacheTTL:        1 * time.Hour,
		EnableFileCache: false,
		OfflineMode:     false,
	}

	service := NewDiscoveryService(config)

	ctx := context.Background()

	// Should error for unknown provider with no fallback
	_, err := service.GetModels(ctx, "unknown-provider")
	if err == nil {
		t.Error("Expected error for unknown provider")
	}
}

func TestModelCache_TTL(t *testing.T) {
	config := DiscoveryConfig{
		CacheTTL:        100 * time.Millisecond, // Short TTL for testing
		EnableFileCache: false,
		OfflineMode:     false,
	}

	cache := NewModelCache(config)

	models := []*types.ModelInfo{
		{
			ID:       "test-model",
			Name:     "Test Model",
			Provider: "test",
		},
	}

	// Set models in cache
	cache.Set("test", models)

	// Should be fresh immediately
	cached, fresh := cache.Get("test")
	if !fresh {
		t.Error("Expected cache to be fresh immediately after set")
	}
	if len(cached) != 1 {
		t.Errorf("Expected 1 cached model, got %d", len(cached))
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should now be stale (fresh=false)
	// For "test" provider, no fallback exists, so should return empty
	_, fresh = cache.Get("test")
	if fresh {
		t.Error("Expected cache to be stale after TTL expiration")
	}

	// Test with a provider that has fallback models
	cache.Set("openai", models)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should return fallback models for openai
	fallback, fresh := cache.Get("openai")
	if fresh {
		t.Error("Expected cache to be stale after TTL expiration")
	}

	if len(fallback) == 0 {
		t.Error("Expected fallback models for openai provider after cache expiration")
	}
}

func BenchmarkModelCache_ShardedFiles(b *testing.B) {
	// Create temporary directory for cache files
	tmpDir, err := os.MkdirTemp("", "wormhole-cache-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := DiscoveryConfig{
		CacheTTL:        24 * time.Hour,
		FileCachePath:   filepath.Join(tmpDir, "models.json"),
		EnableFileCache: true,
		FileCacheTTL:    7 * 24 * time.Hour,
	}

	cache := NewModelCache(config)
	defer cache.Clear()

	// Create mock models for multiple providers
	providers := []string{"openai", "anthropic", "gemini", "openrouter", "ollama"}
	modelsByProvider := make(map[string][]*types.ModelInfo)
	for _, provider := range providers {
		modelsByProvider[provider] = []*types.ModelInfo{
			{
				ID:       provider + "-model-1",
				Name:     provider + " Model 1",
				Provider: provider,
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
				},
				MaxTokens: 8192,
			},
			{
				ID:       provider + "-model-2",
				Name:     provider + " Model 2",
				Provider: provider,
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
				},
				MaxTokens: 4096,
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark sequential Set operations
	for i := 0; i < b.N; i++ {
		for _, provider := range providers {
			cache.Set(provider, modelsByProvider[provider])
		}
	}
}

func BenchmarkModelCache_ConcurrentShardedFiles(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "wormhole-cache-concurrent")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := DiscoveryConfig{
		CacheTTL:        24 * time.Hour,
		FileCachePath:   filepath.Join(tmpDir, "models.json"),
		EnableFileCache: true,
		FileCacheTTL:    7 * 24 * time.Hour,
	}

	cache := NewModelCache(config)
	defer cache.Clear()

	providers := []string{"openai", "anthropic", "gemini", "openrouter", "ollama"}
	modelsByProvider := make(map[string][]*types.ModelInfo)
	for _, provider := range providers {
		modelsByProvider[provider] = []*types.ModelInfo{
			{
				ID:       provider + "-model-1",
				Name:     provider + " Model 1",
				Provider: provider,
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
				},
				MaxTokens: 8192,
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine picks a provider and calls Set
		// Simulate concurrent updates to different providers
		idx := 0
		for pb.Next() {
			provider := providers[idx%len(providers)]
			cache.Set(provider, modelsByProvider[provider])
			idx++
		}
	})
}
