package middleware

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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

func TestHealthCheckerUsesOneCallbackPerSweep(t *testing.T) {
	checker := NewHealthChecker(time.Hour)
	providers := make([]string, 256)
	for i := range providers {
		providers[i] = fmt.Sprintf("provider-%d", i)
	}

	var originalCalls atomic.Int32
	var replacementCalls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	checker.SetCheckFunction(func(context.Context, string) error {
		originalCalls.Add(1)
		enteredOnce.Do(func() { close(entered) })
		<-release
		return nil
	})

	done := make(chan struct{})
	go func() {
		checker.checkAll(providers)
		close(done)
	}()
	<-entered
	checker.SetCheckFunction(func(context.Context, string) error {
		replacementCalls.Add(1)
		return nil
	})
	close(release)
	<-done

	if got := originalCalls.Load(); got != int32(len(providers)) {
		t.Fatalf("original callback calls = %d, want %d", got, len(providers))
	}
	if got := replacementCalls.Load(); got != 0 {
		t.Fatalf("replacement callback ran %d times during in-flight sweep", got)
	}

	checker.SetCheckFunction(nil)
	checker.checkAll(providers)
}
