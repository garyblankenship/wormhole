package fetchers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestAuditDiscoveryPagination(t *testing.T) {
	// Serial: useTestHTTPClient replaces the package-global HTTP client.
	for _, provider := range []string{"anthropic", "gemini"} {
		for _, scenario := range []string{"complete", "repeated", "later-error", "missing"} {
			if provider == "gemini" && scenario == "missing" {
				continue
			}
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				server := httptest.NewServer(auditPaginationHandler(t, provider, scenario))
				t.Cleanup(server.Close)
				useTestHTTPClient(t, server.Client())
				var f discovery.ModelFetcher
				if provider == "anthropic" {
					f = NewAnthropicFetcherWithConfig(types.ProviderConfig{APIKey: "key", BaseURL: server.URL, Headers: map[string]string{"x-api-key": "custom-key", "X-Trace": "trace"}})
				} else {
					f = NewGeminiFetcher(server.URL, "key +&")
				}
				models, err := f.FetchModels(context.Background())
				if scenario != "complete" {
					if err == nil || models != nil {
						t.Fatalf("partial result models=%v err=%v", models, err)
					}
					return
				}
				if err != nil || len(models) != 2 || models[0].ID != "first" || models[1].ID != "second" {
					t.Fatalf("models=%v err=%v", models, err)
				}
				if provider == "gemini" && (models[0].ContextLength != 1000 || models[0].MaxTokens != 100) {
					t.Fatalf("limits=%#v", models[0])
				}
			})
		}
	}
}

func auditPaginationHandler(t *testing.T, provider, scenario string) http.HandlerFunc {
	const cursor = "next /+&?"
	pages := 0
	return func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Has("limit") || r.URL.Query().Has("pageSize") {
			t.Error("overrode default page size")
		}
		cursorField := "after_id"
		if provider == "gemini" {
			cursorField = "pageToken"
			if r.URL.Query().Get("key") != "key +&" {
				t.Error("key changed")
			}
		} else {
			if r.Header.Get("x-api-key") != "custom-key" || r.Header.Get("X-Trace") != "trace" || r.Header.Get("anthropic-version") != "2023-06-01" {
				t.Error("headers changed between pages")
			}
		}
		if pages == 1 && r.URL.Query().Has(cursorField) {
			t.Error("first request had cursor")
		}
		if pages == 2 && r.URL.Query().Get(cursorField) != cursor {
			t.Errorf("cursor=%q", r.URL.Query().Get(cursorField))
		}
		if pages > 2 {
			t.Error("pagination failed to stop")
			w.WriteHeader(500)
			return
		}
		if pages == 2 && scenario == "later-error" {
			w.WriteHeader(503)
			return
		}
		more := pages == 1 || scenario == "repeated"
		if provider == "anthropic" {
			lastID := cursor
			if scenario == "missing" {
				lastID = ""
			}
			data := []map[string]any{{"id": "first", "display_name": "First"}}
			if pages == 2 {
				data = append(data, map[string]any{"id": "second"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "has_more": more, "last_id": lastID})
		} else {
			data := []map[string]any{{"name": "models/first", "inputTokenLimit": 1000, "outputTokenLimit": 100}}
			token := ""
			if more {
				token = cursor
			}
			if pages == 2 {
				data = append(data, map[string]any{"name": "models/second"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": data, "nextPageToken": token})
		}
	}
}

func TestAuditDiscoveryEndpointScopes(t *testing.T) {
	t.Parallel()
	scoped := func(endpoint, key string, headers map[string]string) string {
		return NewOpenAICompatibleFetcher("compatible", endpoint, key, headers).(*OpenAICompatibleFetcher).AccountDiscriminator()
	}
	first := scoped("https://one.example/v1/", "secret", map[string]string{"x-account": "one"})
	if first == scoped("https://two.example/v1", "secret", map[string]string{"X-Account": "one"}) {
		t.Fatal("endpoints share identity")
	}
	if first == scoped("https://one.example/v1", "secret", map[string]string{"X-Account": "two"}) {
		t.Fatal("headers share identity")
	}
	if first != scoped("HTTPS://ONE.EXAMPLE/v1", "secret", map[string]string{"X-Account": "one"}) {
		t.Fatal("equivalent endpoints/headers have different identities")
	}
	if first == scoped("https://one.example/v1", "different", map[string]string{"X-Account": "one"}) {
		t.Fatal("credentials share identity")
	}
	override := map[string]string{"authorization": "override"}
	if scoped("https://one.example", "first", override) != scoped("https://one.example", "second", override) {
		t.Fatal("overridden credentials changed effective auth identity")
	}
	if strings.Contains(first, "secret") {
		t.Fatal("scope exposed credential")
	}
	ollama := NewOllamaFetcher("http://one.example:11434/")
	if ollama.AccountDiscriminator() == NewOllamaFetcher("http://two.example:11434").AccountDiscriminator() {
		t.Fatal("ollama endpoints share identity")
	}
	if ollama.AccountDiscriminator() != NewOllamaFetcher("HTTP://ONE.EXAMPLE:11434").AccountDiscriminator() {
		t.Fatal("equivalent ollama endpoint changed identity")
	}
	cache := discovery.NewModelCache(discovery.DiscoveryConfig{CacheTTL: time.Hour})
	t.Cleanup(func() { _ = cache.Close() })
	cache.Set("compatible", []*types.ModelInfo{{ID: "old-unscoped"}})
	cache.Set("compatible__"+accountKeyDiscriminator("secret"), []*types.ModelInfo{{ID: "old-key-only"}})
	if models, fresh := cache.Get("compatible__" + first); fresh || len(models) != 0 {
		t.Fatalf("reused old cache: %v", models)
	}
	cache.Set("ollama", []*types.ModelInfo{{ID: "old-local"}})
	if models, fresh := cache.Get("ollama__" + ollama.AccountDiscriminator()); fresh {
		t.Fatalf("reused unscoped ollama cache: %v", models)
	}
}

func TestAuditOpenRouterProviderLookup(t *testing.T) {
	// Serial: useTestHTTPClient replaces the package-global HTTP client.
	const modelID = "upstream-vendor/full-model-id"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": modelID, "name": "Local catalog model"}},
		})
	}))
	t.Cleanup(server.Close)
	useTestHTTPClient(t, server.Client())
	fetcher := NewOpenRouterFetcherWithConfig(types.ProviderConfig{BaseURL: server.URL, APIKey: "local-key"})
	service := discovery.NewDiscoveryService(discovery.DiscoveryConfig{
		DisableFileCache: true, DisableBackgroundRefresh: true,
	}, fetcher)
	t.Cleanup(func() { _ = service.Stop() })
	// Populate the live local catalog explicitly so fallback data cannot satisfy
	// the subsequent provider lookup.
	if err := service.RefreshModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	models, err := service.GetModels(context.Background(), "openrouter")
	if err != nil || len(models) != 1 || models[0].ID != modelID || models[0].Provider != "openrouter" {
		t.Fatalf("openrouter lookup: models=%#v err=%v", models, err)
	}
	// The discovered provider identity must itself route back to the registered
	// discovery provider, while retaining the complete upstream model ID.
	lookup, err := service.GetModels(context.Background(), models[0].Provider)
	if err != nil || len(lookup) != 1 || lookup[0].ID != modelID {
		t.Fatalf("discovered-provider lookup: models=%#v err=%v", lookup, err)
	}
}
