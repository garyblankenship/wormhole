package testing

import "testing"

func TestRunProviderConformanceWithMockProvider(t *testing.T) {
	RunProviderConformance(t, ProviderConformanceConfig{
		Provider: NewMockProvider("mock").WithTextResponse(TextResponseWith("hello")),
	})
}
