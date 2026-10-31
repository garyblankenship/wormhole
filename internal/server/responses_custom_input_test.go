package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomInputDecoderFragmentedEscapes(t *testing.T) {
	t.Parallel()
	want := "first\n\"slash\\ rocket 🚀 end"
	encoded, err := json.Marshal(map[string]string{"input": want})
	if err != nil {
		t.Fatal(err)
	}
	// Providers may escape supplementary characters instead of emitting UTF-8.
	raw := strings.Replace(string(encoded), "🚀", `\uD83D\uDE80`, 1)
	var decoder customInputDecoder
	var deltas strings.Builder
	for i := range len(raw) {
		fragment := raw[i : i+1]
		delta := decoder.append(fragment)
		deltas.WriteString(delta)
		if !strings.HasPrefix(want, deltas.String()) {
			t.Fatalf("byte %d emitted non-prefix %q", i, deltas.String())
		}
		if strings.HasSuffix(raw[:i+1], `\uD83D`) && strings.Contains(deltas.String(), "🚀") {
			t.Fatal("emitted incomplete surrogate pair")
		}
		if strings.HasSuffix(raw[:i+1], `\n`) && !strings.Contains(deltas.String(), "\n") {
			t.Fatal("withheld complete newline escape")
		}
	}
	if got := deltas.String(); got != want {
		t.Fatalf("concatenated deltas = %q, want %q", got, want)
	}
	if got := decoder.final(raw); got != want {
		t.Fatalf("final input = %q, want %q", got, want)
	}
	var utf8Decoder customInputDecoder
	var utf8Deltas strings.Builder
	for i := range encoded {
		utf8Deltas.WriteString(utf8Decoder.append(string(encoded[i : i+1])))
		if !strings.HasPrefix(want, utf8Deltas.String()) {
			t.Fatalf("UTF-8 byte %d emitted non-prefix %q", i, utf8Deltas.String())
		}
	}
	if got := utf8Deltas.String(); got != want {
		t.Fatalf("UTF-8 deltas = %q, want %q", got, want)
	}
}

func TestCustomInputDecoderIncompleteAndFinalFallback(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(map[string]string{"input": "abc\nxyz"})
	if err != nil {
		t.Fatal(err)
	}
	var decoder customInputDecoder
	for _, fragment := range []string{string(encoded[:len(encoded)-4]), string(encoded[len(encoded)-4:])} {
		decoder.append(fragment)
	}
	if got := decoder.final(string(encoded)); got != "abc\nxyz" {
		t.Fatalf("complete input = %q", got)
	}

	// For a truncated wrapper, keep the decoded prefix and withhold the escape.
	truncated := string(encoded[:strings.Index(string(encoded), `\n`)])
	var partial customInputDecoder
	partial.append(truncated + `\`)
	if got := partial.final(truncated + `\`); got != "abc" {
		t.Fatalf("truncated input = %q", got)
	}

	// A valid wrapper with another first property is still decoded at completion.
	otherFirst, err := json.Marshal(struct {
		Other string `json:"other"`
		Input string `json:"input"`
	}{Other: "value", Input: "later"})
	if err != nil {
		t.Fatal(err)
	}
	var fallback customInputDecoder
	if delta := fallback.append(string(otherFirst)); delta != "" {
		t.Fatalf("unexpected live delta %q", delta)
	}
	if got := fallback.final(string(otherFirst)); got != "later" {
		t.Fatalf("fallback input = %q", got)
	}
}
