package wormhole

import (
	"log/slog"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/discovery"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestEmbeddingsBuilderConfigurationCloneAndValidate(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	builder := client.Embeddings().
		Using("openai").
		BaseURL("https://example.test/v1").
		Model("text-embedding-3-small").
		Input("one").
		AddInput("two").
		Dimensions(256).
		ProviderOptions(map[string]any{"trace": true})

	if builder.getProvider() != "openai" || builder.getBaseURL() != "https://example.test/v1" {
		t.Fatalf("embeddings routing = (%q, %q)", builder.getProvider(), builder.getBaseURL())
	}
	if builder.request.Model != "text-embedding-3-small" || len(builder.request.Input) != 2 {
		t.Fatalf("embeddings request = %#v", builder.request)
	}
	if *builder.request.Dimensions != 256 || builder.request.ProviderOptions["trace"] != true {
		t.Fatalf("embeddings options = %#v", builder.request)
	}
	if err := builder.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if got := builder.MustValidate(); got != builder {
		t.Fatal("MustValidate did not return receiver")
	}

	clone := builder.Clone()
	clone.request.Input[0] = "changed"
	clone.request.ProviderOptions["trace"] = false
	if builder.request.Input[0] == "changed" {
		t.Fatal("Clone input mutation changed original")
	}
	if builder.request.ProviderOptions["trace"] != true {
		t.Fatal("Clone provider options mutation changed original")
	}

	invalid := client.Embeddings().Dimensions(0)
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid Validate returned nil")
	}
	assertPanics(t, func() { invalid.MustValidate() })
}

func TestFactoryProviderConstructors(t *testing.T) {
	t.Parallel()
	factory := NewSimpleFactory()

	openai := factory.OpenAI("openai-key")
	if openai.config.DefaultProvider != "openai" || openai.config.Providers["openai"].APIKey != "openai-key" {
		t.Fatalf("OpenAI config = %#v", openai.config)
	}
	_ = openai.Close()

	anthropic := factory.Anthropic("anthropic-key")
	if anthropic.config.DefaultProvider != "anthropic" || anthropic.config.Providers["anthropic"].APIKey != "anthropic-key" {
		t.Fatalf("Anthropic config = %#v", anthropic.config)
	}
	_ = anthropic.Close()

	gemini := factory.Gemini("gemini-key")
	if gemini.config.DefaultProvider != "gemini" || gemini.config.Providers["gemini"].APIKey != "gemini-key" {
		t.Fatalf("Gemini config = %#v", gemini.config)
	}
	_ = gemini.Close()

	groq := factory.Groq("groq-key")
	if groq.config.DefaultProvider != "groq" || groq.config.Providers["groq"].BaseURL == "" {
		t.Fatalf("Groq config = %#v", groq.config)
	}
	_ = groq.Close()

	mistral := factory.Mistral("mistral-key")
	if mistral.config.DefaultProvider != "mistral" || mistral.config.Providers["mistral"].APIKey != "mistral-key" {
		t.Fatalf("Mistral config = %#v", mistral.config)
	}
	_ = mistral.Close()

	ollama, err := factory.Ollama("http://localhost:11434")
	if err != nil {
		t.Fatalf("Ollama returned error: %v", err)
	}
	if ollama.config.DefaultProvider != "ollama" || !ollama.config.Providers["ollama"].DynamicModels {
		t.Fatalf("Ollama config = %#v", ollama.config)
	}
	_ = ollama.Close()

	lmstudio, err := factory.LMStudio("http://localhost:1234")
	if err != nil {
		t.Fatalf("LMStudio returned error: %v", err)
	}
	if lmstudio.config.DefaultProvider != "lmstudio" || !lmstudio.config.Providers["lmstudio"].DynamicModels {
		t.Fatalf("LMStudio config = %#v", lmstudio.config)
	}
	_ = lmstudio.Close()

	local, err := factory.LocalOpenAI("http://localhost:8000/v1")
	if err != nil {
		t.Fatalf("LocalOpenAI returned error: %v", err)
	}
	if local.config.DefaultProvider != "local" || !local.config.Providers["local"].NoAuth || !local.config.Providers["local"].DynamicModels {
		t.Fatalf("LocalOpenAI config = %#v", local.config)
	}
	_ = local.Close()

	openrouter, err := factory.OpenRouter("router-key")
	if err != nil {
		t.Fatalf("OpenRouter returned error: %v", err)
	}
	if openrouter.config.DefaultProvider != "openrouter" || !openrouter.config.Providers["openrouter"].DynamicModels {
		t.Fatalf("OpenRouter config = %#v", openrouter.config)
	}
	_ = openrouter.Close()
}

func setEnvForFactoryTest(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func TestFactoryEnvironmentAndMiddlewareOptions(t *testing.T) {
	setEnvForFactoryTest(t, "OLLAMA_BASE_URL", "")
	setEnvForFactoryTest(t, "LMSTUDIO_BASE_URL", "")
	setEnvForFactoryTest(t, "OPENROUTER_API_KEY", "")
	setEnvForFactoryTest(t, "OPENAI_API_KEY", "env-openai")

	factory := NewSimpleFactory()
	if got := factory.getAPIKey(nil, "OPENAI_API_KEY"); got != "env-openai" {
		t.Fatalf("getAPIKey env = %q, want env-openai", got)
	}
	if got := factory.getAPIKey([]string{"direct"}, "OPENAI_API_KEY"); got != "direct" {
		t.Fatalf("getAPIKey direct = %q, want direct", got)
	}
	if _, err := factory.Ollama(); err == nil {
		t.Fatal("Ollama without base URL returned nil error")
	}
	if _, err := factory.LMStudio(); err == nil {
		t.Fatal("LMStudio without base URL returned nil error")
	}
	if _, err := factory.LocalOpenAI(""); err == nil {
		t.Fatal("LocalOpenAI without base URL returned nil error")
	}
	if _, err := factory.OpenRouter(); err == nil {
		t.Fatal("OpenRouter without API key returned nil error")
	}

	logger := slog.Default()
	var cfg Config
	options := []Option{
		factory.WithRateLimit(10),
		factory.WithCircuitBreaker(2, time.Second),
		factory.WithCache(time.Minute),
		factory.WithTimeout(time.Second),
		factory.WithLogging(logger),
		factory.WithDetailedLogging(logger),
		factory.WithDebugLogging(logger),
	}
	metricsOpt, metrics := factory.WithMetrics()
	options = append(options, metricsOpt)
	for _, opt := range options {
		opt(&cfg)
	}
	if metrics == nil {
		t.Fatal("WithMetrics returned nil metrics")
	}
	if len(cfg.Middleware) != 5 {
		t.Fatalf("legacy middleware count = %d, want 5", len(cfg.Middleware))
	}
	if len(cfg.ProviderMiddlewares) != 8 {
		t.Fatalf("provider middleware count = %d, want 8", len(cfg.ProviderMiddlewares))
	}
}

func TestOptionHelpersAndConfigWarnings(t *testing.T) {
	setEnvForFactoryTest(t, "OPENAI_API_KEY", "openai-env")
	setEnvForFactoryTest(t, "ANTHROPIC_API_KEY", "anthropic-env")
	setEnvForFactoryTest(t, "GEMINI_API_KEY", "gemini-env")
	setEnvForFactoryTest(t, "GROQ_API_KEY", "groq-env")
	setEnvForFactoryTest(t, "MISTRAL_API_KEY", "mistral-env")
	setEnvForFactoryTest(t, "OPENROUTER_API_KEY", "router-env")

	logger := slog.Default()
	var cfg Config
	options := []Option{
		WithDefaultProvider("missing"),
		WithProviderMiddleware(nil),
		WithDebugLogging(logger),
		WithLogger(logger),
		WithDiscoveryConfig(discovery.DiscoveryConfig{OfflineMode: true}),
		WithOfflineMode(true),
		WithOpenAICompatible("vllm", "http://localhost:8000/v1", types.ProviderConfig{}),
		WithLocalOpenAI("http://localhost:8000/v1"),
		WithVLLM(types.ProviderConfig{BaseURL: "http://localhost:8000/v1"}),
		WithOllamaOpenAI(types.ProviderConfig{BaseURL: "http://localhost:11434/v1"}),
		WithProviderFromEnv("openai"),
		WithProviderFromEnv("unknown"),
		WithAllProvidersFromEnv(),
	}
	for _, opt := range options {
		opt(&cfg)
	}

	if cfg.Logger != logger || !cfg.DebugLogging {
		t.Fatal("logger/debug options not applied")
	}
	if !cfg.DiscoveryConfig.OfflineMode {
		t.Fatal("offline mode not applied")
	}
	if cfg.DiscoveryConfig.CacheTTL == 0 || cfg.DiscoveryConfig.FileCachePath == "" || !cfg.DiscoveryConfig.EnableFileCache {
		t.Fatalf("discovery defaults not preserved for partial config: %#v", cfg.DiscoveryConfig)
	}
	for _, provider := range []string{"openai", "anthropic", "gemini", "groq", "mistral", "openrouter", "local", "vllm", "ollama-openai"} {
		if _, ok := cfg.Providers[provider]; !ok {
			t.Fatalf("provider %q not configured by options", provider)
		}
	}

	warnings := validateConfig(&cfg)
	if len(warnings) < 1 {
		t.Fatalf("validateConfig warnings = %#v, want at least one", warnings)
	}
	if got := formatList([]string{"b", "a"}); got != "b, a" {
		t.Fatalf("formatList = %q, want b, a", got)
	}
	if got := formatList([]string{"one"}); got != "one" {
		t.Fatalf("formatList single = %q, want one", got)
	}
	if got := formatList(nil); got != "" {
		t.Fatalf("formatList empty = %q, want empty", got)
	}
	if got := capitalize("openai"); got != "Openai" {
		t.Fatalf("capitalize = %q, want Openai", got)
	}
	if got := capitalize(""); got != "" {
		t.Fatalf("capitalize empty = %q, want empty", got)
	}
}

func TestDiscoveryConfigExplicitDisableOptions(t *testing.T) {
	t.Parallel()
	var cfg Config
	WithDiscoveryConfig(discovery.DiscoveryConfig{
		DisableFileCache:         true,
		DisableBackgroundRefresh: true,
	})(&cfg)

	if cfg.DiscoveryConfig.EnableFileCache {
		t.Fatalf("EnableFileCache = true, want false")
	}
	if cfg.DiscoveryConfig.RefreshInterval != 0 {
		t.Fatalf("RefreshInterval = %s, want disabled", cfg.DiscoveryConfig.RefreshInterval)
	}
	if cfg.DiscoveryConfig.CacheTTL == 0 || cfg.DiscoveryConfig.FileCachePath == "" {
		t.Fatalf("discovery defaults not preserved: %#v", cfg.DiscoveryConfig)
	}
}

func TestWithModels_PopulatesRegistry(t *testing.T) {
	original := types.DefaultModelRegistry
	types.DefaultModelRegistry = types.NewModelRegistry()
	t.Cleanup(func() { types.DefaultModelRegistry = original })

	model := &types.ModelInfo{
		ID:           "test-model-x",
		Provider:     "openai",
		Capabilities: []types.ModelCapability{types.CapabilityChat},
	}

	if _, ok := types.DefaultModelRegistry.Get("test-model-x"); ok {
		t.Fatal("registry should be empty before WithModels")
	}

	_ = New(
		WithDefaultProvider("openai"),
		WithOpenAI("test-key"),
		WithModels(model),
		WithDiscovery(false),
	)

	got, ok := types.DefaultModelRegistry.Get("test-model-x")
	if !ok {
		t.Fatal("WithModels did not populate DefaultModelRegistry")
	}
	if got.Provider != "openai" {
		t.Errorf("provider = %q, want %q", got.Provider, "openai")
	}
}
