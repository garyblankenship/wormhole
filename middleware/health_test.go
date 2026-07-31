package middleware

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthCheckerStatusAndProviders(t *testing.T) {
	t.Parallel()
	checker := NewHealthChecker(time.Hour)
	checker.SetCheckFunction(func(_ context.Context, provider string) error {
		if provider == "bad" {
			return errors.New("bad provider")
		}
		return nil
	})
	checker.Start([]string{"good", "bad"})
	t.Cleanup(checker.Stop)

	if !checker.IsHealthy("good") || !checker.IsHealthy("unknown") {
		t.Fatal("new and unknown providers should begin healthy")
	}
	checker.checkAll([]string{"good", "bad"})
	checker.checkAll([]string{"bad"})
	checker.checkAll([]string{"bad"})

	good := checker.GetStatus("good")
	if !good.Healthy || good.LastError != nil || good.LastCheck.IsZero() {
		t.Fatalf("good status = %#v", good)
	}
	bad := checker.GetStatus("bad")
	if bad.Healthy || bad.ConsecutiveFails < 3 || bad.LastError == nil {
		t.Fatalf("bad status = %#v", bad)
	}
	providers := checker.GetHealthyProviders([]string{"good", "bad"})
	if len(providers) != 1 || providers[0] != "good" {
		t.Fatalf("healthy providers = %v, want [good]", providers)
	}
}
