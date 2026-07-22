package wormhole

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
	whtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

type validationRecordingProvider struct {
	*types.BaseProvider
	mu     sync.Mutex
	models []string
}

func newValidationRecordingProvider(name string) *validationRecordingProvider {
	return &validationRecordingProvider{BaseProvider: types.NewBaseProvider(name)}
}

func (p *validationRecordingProvider) Text(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
	p.record(request.Model)
	return &types.TextResponse{Model: request.Model, Text: "ok", FinishReason: types.FinishReasonStop}, nil
}

func (p *validationRecordingProvider) Stream(_ context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	p.record(request.Model)
	chunks := make(chan types.TextChunk, 1)
	stop := types.FinishReasonStop
	chunks <- types.TextChunk{Text: "ok", FinishReason: &stop}
	close(chunks)
	return chunks, nil
}

func (p *validationRecordingProvider) record(model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.models = append(p.models, model)
}

func (p *validationRecordingProvider) calledModels() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.models...)
}

func (p *validationRecordingProvider) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.models = nil
}

func useModelRegistry(t *testing.T, models ...*types.ModelInfo) {
	t.Helper()
	original := types.DefaultModelRegistry
	types.DefaultModelRegistry = types.NewModelRegistry()
	types.DefaultModelRegistry.LoadModelsFromConfig(models)
	t.Cleanup(func() { types.DefaultModelRegistry = original })
}

func validationTestClient(config types.ProviderConfig, opts ...Option) *Wormhole {
	mock := whtest.NewMockProvider("mock").WithTextResponse(whtest.TextResponseWith("ok"))
	base := []Option{
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", config),
		WithDiscovery(false),
	}
	return New(append(base, opts...)...)
}

func TestModelValidationActivationRules(t *testing.T) {
	t.Run("empty registry is permissive", func(t *testing.T) {
		useModelRegistry(t)
		client := validationTestClient(types.ProviderConfig{})
		if err := client.validateModelAttempt("mock", "unknown", textModelCapabilities, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("disabled validation is permissive", func(t *testing.T) {
		useModelRegistry(t, &types.ModelInfo{ID: "known", Capabilities: []types.ModelCapability{types.CapabilityText}})
		client := validationTestClient(types.ProviderConfig{}, WithModelValidation(false))
		if err := client.validateModelAttempt("mock", "unknown", textModelCapabilities, nil); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("dynamic provider is permissive", func(t *testing.T) {
		useModelRegistry(t, &types.ModelInfo{ID: "known", Capabilities: []types.ModelCapability{types.CapabilityText}})
		client := validationTestClient(types.ProviderConfig{DynamicModels: true})
		if err := client.validateModelAttempt("mock", "unknown", textModelCapabilities, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestModelValidationRejectsRegistryViolations(t *testing.T) {
	useModelRegistry(t,
		&types.ModelInfo{ID: "text", Capabilities: []types.ModelCapability{types.CapabilityText}},
		&types.ModelInfo{ID: "chat", Capabilities: []types.ModelCapability{types.CapabilityChat}},
		&types.ModelInfo{ID: "full", Capabilities: []types.ModelCapability{
			types.CapabilityText,
			types.CapabilityStream,
			types.CapabilityFunctions,
			types.CapabilityVision,
		}},
		&types.ModelInfo{ID: "old", Deprecated: true, Capabilities: []types.ModelCapability{types.CapabilityText}},
	)
	client := validationTestClient(types.ProviderConfig{})

	tests := []struct {
		name     string
		model    string
		anyOf    []types.ModelCapability
		required []types.ModelCapability
		want     string
	}{
		{name: "unknown", model: "unknown", anyOf: textModelCapabilities, want: "not available"},
		{name: "deprecated", model: "old", anyOf: textModelCapabilities, want: "deprecated"},
		{name: "base capability", model: "text", anyOf: []types.ModelCapability{types.CapabilityEmbeddings}, want: "missing one of"},
		{name: "stream modifier", model: "text", anyOf: textModelCapabilities, required: []types.ModelCapability{types.CapabilityStream}, want: "stream"},
		{name: "tool modifier", model: "text", anyOf: textModelCapabilities, required: []types.ModelCapability{types.CapabilityFunctions}, want: "functions"},
		{name: "vision modifier", model: "text", anyOf: textModelCapabilities, required: []types.ModelCapability{types.CapabilityVision}, want: "vision"},
		{name: "chat satisfies text base", model: "chat", anyOf: textModelCapabilities},
		{name: "all modifiers", model: "full", anyOf: textModelCapabilities, required: []types.ModelCapability{types.CapabilityStream, types.CapabilityFunctions, types.CapabilityVision}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateModelAttempt("mock", tt.model, tt.anyOf, tt.required)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestModelValidationRequiresRegisteredProvider(t *testing.T) {
	useModelRegistry(t,
		&types.ModelInfo{ID: "alpha-model", Provider: "alpha", Capabilities: []types.ModelCapability{types.CapabilityText}},
		&types.ModelInfo{ID: "shared-model", Capabilities: []types.ModelCapability{types.CapabilityText}},
	)
	client := New(
		WithDefaultProvider("alpha"),
		WithCustomProvider("alpha", whtest.MockProviderFactory(whtest.NewMockProvider("alpha"))),
		WithProviderConfig("alpha", types.ProviderConfig{}),
		WithCustomProvider("beta", whtest.MockProviderFactory(whtest.NewMockProvider("beta"))),
		WithProviderConfig("beta", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	if err := client.validateModelAttempt("", "alpha-model", textModelCapabilities, nil); err != nil {
		t.Fatalf("default provider validation = %v", err)
	}
	if err := client.validateModelAttempt("beta", "alpha-model", textModelCapabilities, nil); err == nil || !strings.Contains(err.Error(), "registered for provider \"alpha\"") {
		t.Fatalf("beta validation error = %v", err)
	}
	if err := client.validateModelAttempt("beta", "shared-model", textModelCapabilities, nil); err != nil {
		t.Fatalf("provider-agnostic model validation = %v", err)
	}
}

func TestModelValidationDuplicateIDUsesCurrentProviderRegistration(t *testing.T) {
	useModelRegistry(t,
		&types.ModelInfo{ID: "shared-id", Provider: "alpha", Capabilities: []types.ModelCapability{types.CapabilityText}},
		&types.ModelInfo{ID: "shared-id", Provider: "beta", Capabilities: []types.ModelCapability{types.CapabilityText}},
	)
	client := New(
		WithDefaultProvider("alpha"),
		WithCustomProvider("alpha", whtest.MockProviderFactory(whtest.NewMockProvider("alpha"))),
		WithProviderConfig("alpha", types.ProviderConfig{}),
		WithCustomProvider("beta", whtest.MockProviderFactory(whtest.NewMockProvider("beta"))),
		WithProviderConfig("beta", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	if err := client.validateModelAttempt("alpha", "shared-id", textModelCapabilities, nil); err == nil || !strings.Contains(err.Error(), "registered for provider \"beta\"") {
		t.Fatalf("alpha validation error = %v", err)
	}
	if err := client.validateModelAttempt("beta", "shared-id", textModelCapabilities, nil); err != nil {
		t.Fatalf("beta validation = %v", err)
	}
}

func TestNonTextAndAgentValidationHappensBeforeProviderLease(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{ID: "text-only", Capabilities: []types.ModelCapability{types.CapabilityText}})

	var factoryCalls atomic.Int32
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			factoryCalls.Add(1)
			return whtest.NewMockProvider("mock"), nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	if _, err := client.Embeddings().Model("text-only").Input("hello").Generate(context.Background()); err == nil || !strings.Contains(err.Error(), "embeddings") {
		t.Fatalf("embeddings error = %v", err)
	}
	if _, err := client.Rerank().Model("text-only").Query("q").Documents("d").Generate(context.Background()); err == nil || !strings.Contains(err.Error(), "rerank") {
		t.Fatalf("rerank error = %v", err)
	}
	agent := client.Agent().Model("text-only").AddTool("noop", "noop", map[string]any{}, func(context.Context, map[string]any) (any, error) {
		return nil, nil
	})
	if _, err := agent.Run(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "functions") {
		t.Fatalf("agent error = %v", err)
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("provider factory called %d times before validation", got)
	}
}

func TestTextRequiredCapabilities(t *testing.T) {
	request := &types.TextRequest{
		Messages: []types.Message{&types.UserMessage{
			Content: "describe",
			Media:   []types.Media{&types.ImageMedia{URL: "https://example.test/image.png"}},
		}},
	}
	got := textRequiredCapabilities(request, true, true)
	want := []types.ModelCapability{types.CapabilityStream, types.CapabilityFunctions, types.CapabilityVision}
	if len(got) != len(want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("capabilities = %v, want %v", got, want)
		}
	}
}

func TestTextModelValidationAdvancesAcrossFallbacks(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{
		ID:           "valid",
		Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStream},
	})
	provider := newValidationRecordingProvider("mock")
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	response, err := client.Text().Model("invalid").WithFallback("valid").Prompt("hello").Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "valid" {
		t.Fatalf("response model = %q", response.Model)
	}
	if got := provider.calledModels(); len(got) != 1 || got[0] != "valid" {
		t.Fatalf("provider calls = %v, want [valid]", got)
	}

	provider.reset()
	stream, err := client.Text().Model("invalid").WithFallback("valid").Prompt("hello").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text, err := whtest.CollectStreamText(context.Background(), stream); err != nil || text != "ok" {
		t.Fatalf("stream text = %q, error = %v", text, err)
	}
	if got := provider.calledModels(); len(got) != 1 || got[0] != "valid" {
		t.Fatalf("stream provider calls = %v, want [valid]", got)
	}
}

func TestTextModelValidationAdvancesToProviderFallback(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{ID: "secondary-model", Capabilities: []types.ModelCapability{types.CapabilityChat}})
	primary := newValidationRecordingProvider("primary")
	secondary := newValidationRecordingProvider("secondary")
	client := New(
		WithDefaultProvider("primary"),
		WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) { return primary, nil }),
		WithProviderConfig("primary", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) { return secondary, nil }),
		WithProviderConfig("secondary", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	response, err := client.Text().Model("invalid").Prompt("hello").
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "secondary-model"}).
		Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "secondary-model" {
		t.Fatalf("response model = %q", response.Model)
	}
	if got := primary.calledModels(); len(got) != 0 {
		t.Fatalf("primary calls = %v", got)
	}
	if got := secondary.calledModels(); len(got) != 1 || got[0] != "secondary-model" {
		t.Fatalf("secondary calls = %v", got)
	}
}

func TestTextModelValidationFeatureModifiersPreventInvocation(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{ID: "text-only", Capabilities: []types.ModelCapability{types.CapabilityText}})
	provider := newValidationRecordingProvider("mock")
	var factoryCalls atomic.Int32
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			factoryCalls.Add(1)
			return provider, nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	tool := types.Tool{Name: "noop", InputSchema: map[string]any{}}
	if _, err := client.Text().Model("text-only").Prompt("hello").Tools(tool).Generate(context.Background()); err == nil || !strings.Contains(err.Error(), "functions") {
		t.Fatalf("tool request error = %v", err)
	}

	mediaMessage := &types.UserMessage{Content: "describe", Media: []types.Media{&types.ImageMedia{URL: "https://example.test/image.png"}}}
	if _, err := client.Text().Model("text-only").Messages(mediaMessage).Generate(context.Background()); err == nil || !strings.Contains(err.Error(), "vision") {
		t.Fatalf("media request error = %v", err)
	}

	if _, err := client.Text().Model("text-only").Prompt("hello").Stream(context.Background()); err == nil || !strings.Contains(err.Error(), "stream") {
		t.Fatalf("stream error = %v", err)
	}
	if got := provider.calledModels(); len(got) != 0 {
		t.Fatalf("provider calls = %v", got)
	}
	if got := factoryCalls.Load(); got != 0 {
		t.Fatalf("provider factory calls = %d, want pre-lease validation", got)
	}
}
