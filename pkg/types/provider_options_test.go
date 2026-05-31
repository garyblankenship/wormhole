package types

import "testing"

func TestProviderConfigMergedProviderOptionsPrecedenceAndIsolation(t *testing.T) {
	defaults := map[string]any{"effort": "low", "trace": false}
	modelOptions := map[string]any{"effort": "medium", "service_tier": "batch"}
	requestOptions := map[string]any{"effort": "high"}

	cfg := NewProviderConfig("key").
		WithDefaultProviderOptions(defaults).
		WithProviderOptionsForModel("model-a", modelOptions)

	defaults["trace"] = true
	modelOptions["service_tier"] = "mutated"

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

	merged["trace"] = "changed"
	if cfg.DefaultProviderOptions["trace"] != false {
		t.Fatal("merged options mutation changed config defaults")
	}

	next := cfg.WithProviderOptionsForModel("model-b", map[string]any{"reasoning": "off"})
	next.ProviderOptionsByModel["model-a"]["service_tier"] = "changed"
	if cfg.ProviderOptionsByModel["model-a"]["service_tier"] != "batch" {
		t.Fatal("builder method shared per-model option maps")
	}
}
