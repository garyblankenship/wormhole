package testing

import "testing"

func TestRunProviderConformanceWithMockProvider(t *testing.T) {
	t.Parallel()
	RunProviderConformance(t, ProviderConformanceConfig{
		Provider: NewMockProvider("mock").WithTextResponse(TextResponseWith("hello")),
	})
}
