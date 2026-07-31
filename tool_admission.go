package wormhole

import (
	"context"
	"sync"
	"time"
)

type toolAdmissionBudget struct {
	limiter         *ConcurrencyLimiter
	adaptiveLimiter *AdaptiveLimiter
	stopOnce        sync.Once
}

func newToolAdmissionBudget(config ToolSafetyConfig) *toolAdmissionBudget {
	budget := &toolAdmissionBudget{}
	if config.EnableAdaptiveConcurrency && !config.IsUnlimitedConcurrency() {
		budget.adaptiveLimiter = NewAdaptiveLimiter(config.ToAdaptiveConfig())
	} else if !config.IsUnlimitedConcurrency() {
		budget.limiter = NewConcurrencyLimiter(config.MaxConcurrentTools)
	}
	return budget
}

func (b *toolAdmissionBudget) acquire(ctx context.Context) (release func(), ok bool) {
	if b == nil {
		return func() {}, true
	}
	if b.adaptiveLimiter != nil {
		release, ok := b.adaptiveLimiter.AcquireToken(ctx)
		if !ok {
			return nil, false
		}
		started := time.Now()
		return func() {
			b.adaptiveLimiter.RecordLatency(time.Since(started))
			release()
		}, true
	}
	if b.limiter != nil {
		if !b.limiter.Acquire(ctx) {
			return nil, false
		}
		return b.limiter.Release, true
	}
	return func() {}, true
}

func (b *toolAdmissionBudget) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() {
		if b.adaptiveLimiter != nil {
			b.adaptiveLimiter.Stop()
		}
	})
}
