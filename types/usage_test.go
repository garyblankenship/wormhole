package types

import "testing"

func TestUsageIsZeroIncludesReasoningTokens(t *testing.T) {
	t.Parallel()

	if (Usage{ReasoningTokens: 2045}).IsZero() {
		t.Fatal("reasoning-only usage must not be zero")
	}
	if !(Usage{}).IsZero() {
		t.Fatal("empty usage should be zero")
	}
}
