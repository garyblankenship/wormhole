package fetchers

import "testing"

// C4-13: GeminiFetcher.AccountDiscriminator must include baseURL so different
// endpoints using the same API key do not collide in the model cache.
func TestC413GeminiAccountDiscriminatorIncludesBaseURL(t *testing.T) {
	t.Parallel()

	f1 := NewGeminiFetcher("https://generativelanguage.googleapis.com/v1beta", "same-key")
	f2 := NewGeminiFetcher("https://proxy.example.com/v1beta", "same-key")
	f1Equivalent := NewGeminiFetcher("HTTPS://GENERATIVELANGUAGE.GOOGLEAPIS.COM/v1beta/", "same-key")

	d1 := f1.AccountDiscriminator()
	d2 := f2.AccountDiscriminator()
	d1Eq := f1Equivalent.AccountDiscriminator()

	if d1 == "" || d2 == "" {
		t.Fatalf("expected non-empty discriminators, got %q and %q", d1, d2)
	}
	if d1 == d2 {
		t.Fatalf("different base URLs produced identical discriminator %q", d1)
	}
	if d1 != d1Eq {
		t.Fatalf("equivalent base URLs produced different discriminators: %q vs %q", d1, d1Eq)
	}
}
