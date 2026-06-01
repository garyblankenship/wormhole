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

// --- Stream Lifecycle Trace ---

// StreamEventType identifies the kind of stream lifecycle event.
type StreamEventType string

const (
	// StreamStarted is emitted when a streaming request is initiated.
	StreamStarted StreamEventType = "started"
	// StreamChunk is emitted for each chunk received (optional, controlled by config).
	StreamChunk StreamEventType = "chunk"
	// StreamEnded is the terminal event emitted exactly once when a stream completes normally.
	StreamEnded StreamEventType = "ended"
	// StreamError is the terminal event emitted exactly once when a stream fails.
	StreamError StreamEventType = "error"
)

// StreamEvent describes one stream lifecycle event.
type StreamEvent struct {
	Type     StreamEventType
	Provider string
	Model    string
	Attempt  int
	Error    error
	Time     time.Time
}

// StreamTraceFunc receives stream lifecycle events.
// Terminal events (StreamEnded, StreamError) are emitted exactly once.
type StreamTraceFunc func(context.Context, StreamEvent)

func (p *Wormhole) emitStreamEvent(ctx context.Context, event StreamEvent) {
	if p == nil || p.config.StreamTrace == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	p.config.StreamTrace(ctx, event)
}
