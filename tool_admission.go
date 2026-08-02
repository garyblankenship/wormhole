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

func (b *toolAdmissionBudget) acquire(ctx context.Context) (release func(handlerStarted bool), ok bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	if b == nil {
		return func(bool) {}, true
	}
	if b.adaptiveLimiter != nil {
		adaptiveRelease, acquired := b.adaptiveLimiter.AcquireToken(ctx)
		if !acquired {
			return nil, false
		}
		if ctx.Err() != nil {
			adaptiveRelease()
			return nil, false
		}
		started := time.Now()
		release = func(handlerStarted bool) {
			if handlerStarted {
				b.adaptiveLimiter.RecordLatency(time.Since(started))
			}
			adaptiveRelease()
		}
		return release, true
	}
	if b.limiter != nil {
		if !b.limiter.Acquire(ctx) {
			return nil, false
		}
		release = func(bool) {
			b.limiter.Release()
		}
		if ctx.Err() != nil {
			release(false)
			return nil, false
		}
		return release, true
	}
	if ctx.Err() != nil {
		return nil, false
	}
	return func(bool) {}, true
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
