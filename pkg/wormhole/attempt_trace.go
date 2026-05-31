package wormhole

import (
	"context"
	"time"
)

// AttemptPhase describes where a provider/model attempt is in its lifecycle.
type AttemptPhase string

const (
	AttemptStarted AttemptPhase = "started"
	AttemptSuccess AttemptPhase = "success"
	AttemptError   AttemptPhase = "error"
)

// AttemptEvent describes one provider/model attempt made by a request builder.
type AttemptEvent struct {
	Operation string
	Phase     AttemptPhase
	Provider  string
	Model     string
	Attempt   int
	Fallback  bool
	Stream    bool
	Error     error
	Time      time.Time
}

// AttemptTraceFunc receives best-effort attempt events.
type AttemptTraceFunc func(context.Context, AttemptEvent)

func (p *Wormhole) emitAttempt(ctx context.Context, event AttemptEvent) {
	if p == nil || p.config.AttemptTrace == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	p.config.AttemptTrace(ctx, event)
}
