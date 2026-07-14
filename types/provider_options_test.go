package types

import "testing"

func TestProviderConfigMergedProviderOptionsPrecedenceAndIsolation(t *testing.T) {
	t.Parallel()
	defaults := map[string]any{"effort": "low", "trace": false, "nested": map[string]any{"source": "default"}}
	modelOptions := map[string]any{"effort": "medium", "service_tier": "batch", "routing": map[string]any{"region": "west"}}
	requestOptions := map[string]any{"effort": "high", "metadata": map[string]any{"owner": "caller"}}

	cfg := NewProviderConfig("key").
		WithDefaultProviderOptions(defaults).
		WithProviderOptionsForModel("model-a", modelOptions)

	defaults["trace"] = true
	modelOptions["service_tier"] = "mutated"
	defaults["nested"].(map[string]any)["source"] = "mutated"
	modelOptions["routing"].(map[string]any)["region"] = "mutated"

	merged := cfg.MergedProviderOptions("model-a", requestOptions)
	if got := merged["effort"]; got != "high" {
		t.Fatalf("effort = %v, want high", got)
	}
	if got := merged["trace"]; got != false {
		t.Fatalf("trace = %v, want false", got)
	}
	if got := merged["service_tier"]; got != "batch" {
		t.Fatalf("service_tier = %v, want batch", got)
	}
	if got := merged["nested"].(map[string]any)["source"]; got != "default" {
		t.Fatalf("nested source = %v, want default", got)
	}
	if got := merged["routing"].(map[string]any)["region"]; got != "west" {
		t.Fatalf("routing region = %v, want west", got)
	}

	merged["trace"] = "changed"
	merged["nested"].(map[string]any)["source"] = "changed"
	merged["metadata"].(map[string]any)["owner"] = "changed"
	if cfg.DefaultProviderOptions["trace"] != false {
		t.Fatal("merged options mutation changed config defaults")
	}
	if cfg.DefaultProviderOptions["nested"].(map[string]any)["source"] != "default" {
		t.Fatal("merged nested options mutation changed config defaults")
	}
	if requestOptions["metadata"].(map[string]any)["owner"] != "caller" {
		t.Fatal("merged nested options mutation changed request options")
	}

	next := cfg.WithProviderOptionsForModel("model-b", map[string]any{"reasoning": "off"})
	next.ProviderOptionsByModel["model-a"]["service_tier"] = "changed"
	if cfg.ProviderOptionsByModel["model-a"]["service_tier"] != "batch" {
		t.Fatal("builder method shared per-model option maps")
	}
}
