package types

import (
	"encoding/json"
	"testing"
)

func TestV330IsCachePrewarmRequiresExplicitNumericZero(t *testing.T) {
	t.Parallel()
	for _, sample := range []struct {
		name  string
		value any
		want  bool
	}{
		{"integer", 0, true}, {"int64", int64(0), true}, {"unsigned", uint(0), true},
		{"float", float64(0), true}, {"json number", json.Number("0"), true},
		{"underflow number", json.Number("1e-999"), false},
		{"positive", 1, false}, {"negative", -1, false}, {"string", "0", false},
		{"boolean", false, false}, {"nil", nil, false}, {"malformed number", json.Number("invalid"), false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			t.Parallel()
			request := TextRequest{BaseRequest: BaseRequest{ProviderOptions: map[string]any{"max_output_tokens": sample.value}}}
			if got := IsCachePrewarm(request); got != sample.want {
				t.Fatalf("IsCachePrewarm() = %v, want %v", got, sample.want)
			}
		})
	}
	var nilRequest *TextRequest
	if IsCachePrewarm(nilRequest) {
		t.Fatal("nil request inferred prewarm")
	}
	if IsCachePrewarm(nil) || IsCachePrewarm(TextRequest{}) || IsCachePrewarm(StructuredRequest{}) {
		t.Fatal("missing options inferred prewarm")
	}
	zero := 0
	if IsCachePrewarm(TextRequest{BaseRequest: BaseRequest{MaxTokens: &zero}}) {
		t.Fatal("typed MaxTokens inferred prewarm")
	}
	request := &StructuredRequest{BaseRequest: BaseRequest{ProviderOptions: map[string]any{"max_output_tokens": 0}}}
	if !IsCachePrewarm(request) {
		t.Fatal("structured request pointer lost explicit prewarm")
	}
}
