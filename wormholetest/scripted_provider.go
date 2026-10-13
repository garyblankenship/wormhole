package wormholetest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/garyblankenship/wormhole/v3/types"
)

// ScriptOperation identifies the next permitted call in a script.
type ScriptOperation string

const (
	ScriptText   ScriptOperation = "text"
	ScriptStream ScriptOperation = "stream"
)

// ErrScriptMismatch means a call does not match the next script; it is not consumed.
var ErrScriptMismatch = errors.New("script operation mismatch")

// ErrScriptExhausted means every supplied script has been consumed.
var ErrScriptExhausted = errors.New("script exhausted")

// Script supplies either a Text response, Stream chunks, or an immediate Error.
// A stream without a finish marker or error ends with io.ErrUnexpectedEOF.
// Error cannot be combined with Response or Chunks; use a chunk's Error for a
// failure after partial output. Scripts contain all data; no defaults are added.
type Script struct {
	Operation ScriptOperation
	Response  *types.TextResponse
	Chunks    []types.TextChunk
	Error     error
}

// ScriptedProvider replays one shared ordered sequence of Text and Stream calls.
// Calls reserve a script atomically; which racing caller acquires it is unspecified.
// A matching call consumes its entry even if later canceled. Pre-canceled and
// mismatched calls consume nothing. Close is a no-op; contexts own pending streams.
// JSON-like fixtures and results are detached; opaque values follow CloneValue.
type ScriptedProvider struct {
	*types.BaseProvider
	mu      sync.Mutex
	scripts []Script
	next    int
}

// NewScriptedProvider validates and takes detached ownership of caller fixtures.
func NewScriptedProvider(name string, scripts ...Script) (*ScriptedProvider, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("script provider name is empty")
	}
	provider := &ScriptedProvider{BaseProvider: types.NewBaseProvider(name), scripts: make([]Script, len(scripts))}
	for i, script := range scripts {
		if err := validateScript(script); err != nil {
			return nil, fmt.Errorf("script %d: %w", i, err)
		}
		provider.scripts[i] = cloneScript(script)
	}
	return provider, nil
}

func validateScript(script Script) error {
	if script.Error != nil && (script.Response != nil || len(script.Chunks) > 0) {
		return fmt.Errorf("immediate error cannot have a payload")
	}
	switch script.Operation {
	case ScriptText:
		if len(script.Chunks) > 0 {
			return fmt.Errorf("text script cannot have chunks")
		}
		if script.Response == nil && script.Error == nil {
			return fmt.Errorf("text script requires a response or error")
		}
	case ScriptStream:
		if script.Response != nil {
			return fmt.Errorf("stream script cannot have a response")
		}
	default:
		return fmt.Errorf("unknown operation %q", script.Operation)
	}
	return nil
}

func cloneScript(script Script) Script {
	script.Response = types.CloneTextResponse(script.Response)
	if script.Chunks != nil {
		chunks := make([]types.TextChunk, len(script.Chunks))
		for i, chunk := range script.Chunks {
			chunks[i] = types.CloneTextChunk(chunk)
		}
		script.Chunks = chunks
	}
	return script
}

// ScriptedProviderFactory injects the same ordered provider into a client.
func ScriptedProviderFactory(provider *ScriptedProvider) types.ProviderFactory {
	return func(types.ProviderConfig) (types.Provider, error) {
		if provider == nil {
			return nil, fmt.Errorf("script provider is nil")
		}
		return provider, nil
	}
}

// SupportedCapabilities advertises only the implemented text/chat/stream operations.
func (p *ScriptedProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityStream}
}

func (p *ScriptedProvider) reserve(ctx context.Context, operation ScriptOperation) (Script, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Script{}, err
	}
	if p.next == len(p.scripts) {
		return Script{}, ErrScriptExhausted
	}
	script := p.scripts[p.next]
	if script.Operation != operation {
		return Script{}, fmt.Errorf("%w: next %q, called %q", ErrScriptMismatch, script.Operation, operation)
	}
	p.next++
	return cloneScript(script), script.Error
}

// Text returns the next scripted text result.
func (p *ScriptedProvider) Text(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
	script, err := p.reserve(ctx, ScriptText)
	if err != nil {
		return nil, err
	}
	return script.Response, nil
}

// Stream sends the next scripted chunks until closure, error, or cancellation.
func (p *ScriptedProvider) Stream(ctx context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	script, err := p.reserve(ctx, ScriptStream)
	if err != nil {
		return nil, err
	}
	out := make(chan types.TextChunk)
	// Exit after bounded fixture replay, a chunk error, or context cancellation.
	go func() {
		defer close(out)
		complete := false
		for _, chunk := range script.Chunks {
			if ctx.Err() != nil {
				return
			}
			// Delivery transfers mutable fields to the receiver. Capture control
			// state before sending so it never reads receiver-owned pointers.
			done, chunkErr := chunk.IsDone(), chunk.Error
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
			if chunkErr != nil {
				return
			}
			complete = complete || done
		}
		if !complete {
			select {
			case out <- types.TextChunk{Error: io.ErrUnexpectedEOF}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}
