package types

import "testing"

func TestProviderConfigEffectiveAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		config ProviderConfig
		want   string
	}{
		{name: "direct key wins", config: ProviderConfig{APIKey: "direct", APIKeys: []string{"pooled"}}, want: "direct"},
		{name: "first pooled key", config: ProviderConfig{APIKeys: []string{"first", "second"}}, want: "first"},
		{name: "no key", config: ProviderConfig{}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.config.EffectiveAPIKey(); got != test.want {
				t.Fatalf("EffectiveAPIKey() = %q, want %q", got, test.want)
			}
		})
	}
}
