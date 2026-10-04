package wormhole

import (
	"context"
	"fmt"
	"sync/atomic"

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
	e.recordCircuitFailure(toolCall.Name)
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
		e.recordCircuitFailure(toolCall.Name)
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("schema validation failed: %v", err),
		}, true
	}
	return types.ToolResult{}, false
}

func (e *ToolExecutor) rejectOversizedOutput(toolCall types.ToolCall, result any) (types.ToolResult, bool) {

	if err := e.validateOutputSize(result); err != nil {
		e.recordCircuitFailure(toolCall.Name)
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("tool output rejected: %v", err),
		}, true
	}
	return types.ToolResult{}, false
}

func (e *ToolExecutor) recordCircuitFailure(name string) {
	if breaker := e.breakerForTool(name); breaker != nil {
		breaker.RecordFailure()
	}
}

func (e *ToolExecutor) recordCircuitSuccess(name string) {
	if breaker := e.breakerForTool(name); breaker != nil {
		breaker.RecordSuccess()
	}
}

func (e *ToolExecutor) breakerForTool(name string) *SimpleCircuitBreaker {
	if !e.safetyConfig.EnableCircuitBreaker {
		return nil
	}
	e.breakerMu.Lock()
	defer e.breakerMu.Unlock()
	if e.circuitBreakers == nil {
		e.circuitBreakers = make(map[string]*SimpleCircuitBreaker)
	}
	breaker := e.circuitBreakers[name]
	if breaker == nil {
		breaker = NewSimpleCircuitBreaker(e.safetyConfig.CircuitBreakerThreshold, e.safetyConfig.CircuitBreakerResetTimeout)
		e.circuitBreakers[name] = breaker
	}
	return breaker
}

func (e *ToolExecutor) acquirePermit(ctx context.Context) (release func(handlerStarted bool), ok bool) {
	return e.admission.acquire(ctx)
}

func (p *Wormhole) newToolExecutor(registry *ToolRegistry) *ToolExecutor {
	config := p.config.ToolSafety
	executor := &ToolExecutor{
		registry:      registry,
		safetyConfig:  config,
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
