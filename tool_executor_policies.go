package wormhole

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v3/internal/schemavalidation"
	"github.com/garyblankenship/wormhole/v3/types"
)

func callToolHandler(
	ctx context.Context,
	definition *types.ToolDefinition,
	args map[string]any,
	state *atomic.Uint32,
	everStarted *atomic.Bool,
) (res any, rerr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for state.Load() == toolHandlerPending {
		if state.CompareAndSwap(toolHandlerPending, toolHandlerStarted) {
			break
		}
	}
	if state.Load() == toolHandlerCanceled {
		return nil, context.Canceled
	}
	everStarted.Store(true)
	defer state.CompareAndSwap(toolHandlerStarted, toolHandlerPending)
	defer func() {
		if recovered := recover(); recovered != nil {
			rerr = fmt.Errorf("tool handler panicked: %v", recovered)
		}
	}()
	return definition.Handler(ctx, args)
}

func (e *ToolExecutor) rejectMalformedArguments(toolCall types.ToolCall) (types.ToolResult, bool) {
	if !toolCall.ArgsInvalid {
		return types.ToolResult{}, false
	}
	parseError := toolCall.ArgsParseError
	if parseError == "" {
		parseError = "provider could not parse the arguments as JSON"
	}
	return types.ToolResult{
		ToolCallID: toolCall.ID,
		Error:      fmt.Sprintf("tool %q has malformed arguments: %s", toolCall.Name, parseError),
	}, true
}

func (e *ToolExecutor) rejectInvalidArguments(definition *types.ToolDefinition, toolCall types.ToolCall) (types.ToolResult, bool) {
	if !e.safetyConfig.EnableInputValidation || definition.Tool.InputSchema == nil {
		return types.ToolResult{}, false
	}
	if err := schemavalidation.ValidateAgainstSchema(toolCall.Arguments, definition.Tool.InputSchema); err != nil {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("schema validation failed: %v", err),
		}, true
	}
	return types.ToolResult{}, false
}

type toolBreakerStore struct {
	mu       sync.Mutex
	breakers map[string]*SimpleCircuitBreaker
}

func newToolBreakerStore() *toolBreakerStore {
	return &toolBreakerStore{
		breakers: make(map[string]*SimpleCircuitBreaker),
	}
}

func (s *toolBreakerStore) getOrCreate(name string, threshold int, resetTimeout time.Duration) *SimpleCircuitBreaker {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.breakers == nil {
		s.breakers = make(map[string]*SimpleCircuitBreaker)
	}
	breaker := s.breakers[name]
	if breaker == nil {
		breaker = NewSimpleCircuitBreaker(threshold, resetTimeout)
		s.breakers[name] = breaker
	}
	return breaker
}

func (e *ToolExecutor) breakerForTool(name string) *SimpleCircuitBreaker {
	if !e.safetyConfig.EnableCircuitBreaker {
		return nil
	}
	store := e.breakers
	if store == nil {
		e.breakerMu.Lock()
		if e.breakers == nil {
			e.breakers = newToolBreakerStore()
		}
		store = e.breakers
		e.breakerMu.Unlock()
	}
	return store.getOrCreate(name, e.safetyConfig.CircuitBreakerThreshold, e.safetyConfig.CircuitBreakerResetTimeout)
}

func (e *ToolExecutor) acquirePermit(ctx context.Context) (release func(handlerStarted bool), ok bool) {
	return e.admission.acquire(ctx)
}

func (p *Wormhole) newToolExecutor(registry *ToolRegistry) *ToolExecutor {
	config := p.config.ToolSafety
	executor := &ToolExecutor{
		registry:      registry,
		safetyConfig:  config,
		breakers:      p.toolBreakers,
		configErr:     p.toolConfigErr,
		admission:     p.toolBudget,
		ownsAdmission: false,
	}
	if p.toolBudget != nil {
		executor.limiter = p.toolBudget.limiter
		executor.adaptiveLimiter = p.toolBudget.adaptiveLimiter
	}
	if p.toolConfigErr == nil {
		executor.initializeExecutionPolicies()
	}
	return executor
}

func (e *ToolExecutor) initializeExecutionPolicies() {
	if e.safetyConfig.MaxRetriesPerTool > 0 {
		e.retryExecutor = NewRetryExecutor(e.safetyConfig.MaxRetriesPerTool)
	}
}
