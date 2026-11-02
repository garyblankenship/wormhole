package wormhole

import (
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestZAIProfileReasoningDefaultsAndOverrides(t *testing.T) {
	t.Parallel()
	profile, ok := ProviderProfileByName("zai")
	if !ok {
		t.Fatal("missing ZAI profile")
	}
	thinking, ok := profile.DefaultProviderOptions["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("ZAI profile thinking = %#v", profile.DefaultProviderOptions["thinking"])
	}
	client := New(WithProfiledOpenAICompatible("zai", types.NewProviderConfig("fixture").
		WithDefaultProviderOptions(map[string]any{"thinking": map[string]any{"type": "enabled"}}).
		WithProviderOptionsForModel("glm", map[string]any{"thinking": map[string]any{"type": "disabled"}})), WithDiscovery(false))
	cfg := client.config.Providers["zai"]
	if got := cfg.DefaultProviderOptions["thinking"].(map[string]any)["type"]; got != "enabled" {
		t.Fatalf("caller default = %#v, want enabled", got)
	}
	if got := cfg.MergedProviderOptions("glm", map[string]any{"thinking": map[string]any{"type": "enabled"}})["thinking"].(map[string]any)["type"]; got != "enabled" {
		t.Fatalf("request override = %#v, want enabled", got)
	}
	if got := cfg.MergedProviderOptions("glm", nil)["thinking"].(map[string]any)["type"]; got != "disabled" {
		t.Fatalf("model override = %#v, want disabled", got)
	}
	thinking["type"] = "mutated"
	again, _ := ProviderProfileByName("zai")
	if got := again.DefaultProviderOptions["thinking"].(map[string]any)["type"]; got != "disabled" {
		t.Fatalf("profile state mutated: %#v", got)
	}
}
