package types

import "testing"

// C1-09: provider metadata remains distinct and returned records are defensive.
func TestRemediationC109ProviderModelLookup(t *testing.T) {
	t.Parallel()
	registry := NewModelRegistry()
	registry.Register(&ModelInfo{ID: "shared", Provider: "alpha", Capabilities: []ModelCapability{CapabilityText}})
	registry.Register(&ModelInfo{ID: "shared", Provider: "beta", Capabilities: []ModelCapability{CapabilityEmbeddings}})
	registry.Register(&ModelInfo{ID: "unscoped", Capabilities: []ModelCapability{CapabilityChat}})
	if err := registry.ValidateModelForProvider("alpha", "shared", []ModelCapability{CapabilityText}); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateModelForProvider("beta", "shared", []ModelCapability{CapabilityText}); err == nil {
		t.Fatal("beta accepted text capability")
	}
	if err := registry.ValidateModel("shared", []ModelCapability{CapabilityEmbeddings}); err != nil {
		t.Fatalf("global validation = %v", err)
	}
	if _, ok := registry.GetForProvider("third", "shared"); ok {
		t.Fatal("third resolved another provider's model")
	}
	if _, ok := registry.GetForProvider("third", "unscoped"); !ok {
		t.Fatal("unscoped model unavailable")
	}
	model, _ := registry.GetForProvider("alpha", "shared")
	model.Capabilities[0] = CapabilityEmbeddings
	if err := registry.ValidateModelForProvider("alpha", "shared", []ModelCapability{CapabilityText}); err != nil {
		t.Fatalf("returned model mutated registry: %v", err)
	}
	registry.Register(&ModelInfo{ID: "unscoped", Provider: "alpha", Capabilities: []ModelCapability{CapabilityFunctions}})
	if err := registry.ValidateModelForProvider("alpha", "unscoped", []ModelCapability{CapabilityFunctions}); err != nil {
		t.Fatal(err)
	}
}
