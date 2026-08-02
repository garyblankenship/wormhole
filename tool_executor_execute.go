package wormhole

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/garyblankenship/wormhole/v3/internal/schemavalidation"
	"github.com/garyblankenship/wormhole/v3/types"
)

// ToolExecutor handles the execution of tools and orchestration of multi-turn conversations
type ToolExecutor struct {
	registry        *ToolRegistry
	safetyConfig    ToolSafetyConfig
	limiter         *ConcurrencyLimiter
	adaptiveLimiter *AdaptiveLimiter
	circuitBreaker  *SimpleCircuitBreaker
	retryExecutor   *RetryExecutor
	configErr       error
	admission       *toolAdmissionBudget
	ownsAdmission   bool
}

const (
	toolHandlerPending uint32 = iota
	toolHandlerStarted
	toolHandlerCanceled
)

// NewToolExecutor creates a new ToolExecutor with the given registry and default safety config
func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return NewToolExecutorWithConfig(registry, DefaultToolSafetyConfig())
}

// NewToolExecutorWithConfig creates a new ToolExecutor with custom safety configuration
func NewToolExecutorWithConfig(registry *ToolRegistry, config ToolSafetyConfig) *ToolExecutor {
	// Validate and apply defaults
	validationErr := config.Validate()

	executor := &ToolExecutor{
		registry:      registry,
		safetyConfig:  config,
		configErr:     validationErr,
		ownsAdmission: true,
	}

	if validationErr != nil {
		return executor
	}

	executor.admission = newToolAdmissionBudget(config)
	executor.limiter = executor.admission.limiter
	executor.adaptiveLimiter = executor.admission.adaptiveLimiter
	executor.initializeExecutionPolicies()

	return executor
}

// Execute executes a single tool call and returns the result
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - toolCall: The tool call from the LLM (contains name, ID, arguments)
//
// Returns:
//   - ToolResult with the execution result or error
func (e *ToolExecutor) Execute(ctx context.Context, toolCall types.ToolCall) types.ToolResult {
	result, _ := e.execute(ctx, toolCall)
	return result
}

func (e *ToolExecutor) execute(ctx context.Context, toolCall types.ToolCall) (types.ToolResult, bool) {
	if e.configErr != nil {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      e.configErr.Error(),
		}, false
	}
	if ctx.Err() != nil {
		return e.admissionCanceledResult(toolCall), false
	}

	// Check circuit breaker if enabled
	if e.circuitBreaker != nil && e.circuitBreaker.IsTripped() {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      "circuit breaker tripped - tool execution temporarily disabled",
		}, false
	}

	if result, rejected := e.rejectMalformedArguments(toolCall); rejected {
		return result, false
	}

	// Get tool definition from registry
	definition := e.registry.getStored(toolCall.Name)
	if definition == nil {
		// Record failure for circuit breaker
		e.recordCircuitFailure()
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("tool %q not found in registry", toolCall.Name),
		}, false
	}

	// Arguments are already a map from the provider
	args := toolCall.Arguments

	// Validate arguments against schema if schema is provided
	if result, rejected := e.rejectInvalidArguments(definition, toolCall); rejected {
		return result, false
	}

	// Acquire capacity immediately before starting user code. Pre-start rejection
	// releases synchronously; once a handler starts, its goroutine retains the
	// permit because it may ignore cancellation and outlive Execute.
	queueCtx, cancelQueue := context.WithTimeout(ctx, e.safetyConfig.ToolQueueTimeout)
	releasePermit, ok := e.acquirePermit(queueCtx)
	cancelQueue()
	if !ok {
		return e.admissionCanceledResult(toolCall), false
	}
	var releaseOnce sync.Once
	release := func(handlerStarted bool) {
		releaseOnce.Do(func() {
			releasePermit(handlerStarted)
		})
	}

	// ToolTimeout bounds handler execution, not time spent waiting for capacity.
	// Derive it only after a permit is held, while preserving caller cancellation.
	if e.safetyConfig.HasTimeout() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.safetyConfig.ToolTimeout)
		defer cancel()
	}
	if ctx.Err() != nil {
		release(false)
		return e.admissionCanceledResult(toolCall), false
	}

	// Execute the tool handler with retry logic if configured. callHandler wraps the
	// user handler so a panic (e.g. nil-map deref on unexpected LLM args) becomes an
	// error instead of crashing the per-tool goroutine — nothing recovers above it,
	// so an unrecovered panic here would take down the whole process (and the proxy).
	var result any
	var err error
	var handlerState atomic.Uint32

	callHandler := func(ctx context.Context) (res any, rerr error) {
		return callToolHandler(ctx, definition, args, &handlerState)
	}

	execute := func() (any, error) {
		if e.retryExecutor != nil {
			var r any
			rerr := e.retryExecutor.ExecuteWithRetry(ctx, func(ctx context.Context) error {
				res, herr := callHandler(ctx)
				if herr != nil {
					return herr
				}
				r = res
				return nil
			})
			return r, rerr
		}
		return callHandler(ctx)
	}

	// Race the handler against ctx.Done so a handler that ignores
	// cancellation can't hang Execute (and ExecuteAll/the proxy handler)
	// forever. If ctx fires first, the handler goroutine is left running
	// and its result is discarded via the buffered channel -- a leaked
	// goroutine is the lesser evil compared to an unkillable hang.
	type outcome struct {
		result any
		err    error
	}
	done := make(chan outcome, 1)
	if ctx.Err() != nil {
		release(false)
		return e.admissionCanceledResult(toolCall), false
	}
	go func() {
		defer func() {
			release(handlerState.Load() == toolHandlerStarted)
		}()
		r, e := execute()
		done <- outcome{result: r, err: e}
	}()
	select {
	case o := <-done:
		result, err = o.result, o.err
	case <-ctx.Done():
		if handlerState.CompareAndSwap(toolHandlerPending, toolHandlerCanceled) {
			release(false)
			return e.admissionCanceledResult(toolCall), false
		}
		if handlerState.Load() != toolHandlerStarted {
			release(false)
			return e.admissionCanceledResult(toolCall), false
		}
		err = fmt.Errorf("tool %q timed out or was canceled: %w", toolCall.Name, ctx.Err())
		e.recordCircuitFailure()
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      err.Error(),
		}, true
	}

	if err != nil {
		if ctx.Err() != nil && handlerState.CompareAndSwap(toolHandlerPending, toolHandlerCanceled) {
			release(false)
			return e.admissionCanceledResult(toolCall), false
		}
		// Record failure for circuit breaker
		e.recordCircuitFailure()
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      err.Error(),
		}, false
	}

	// Apply output size limit if configured
	if rejected, oversized := e.rejectOversizedOutput(toolCall, result); oversized {
		return rejected, false
	}

	// Record success for circuit breaker
	e.recordCircuitSuccess()

	return types.ToolResult{
		ToolCallID: toolCall.ID,
		Result:     result, // Result is any, not string
	}, false
}

func (e *ToolExecutor) admissionCanceledResult(toolCall types.ToolCall) types.ToolResult {
	return types.ToolResult{
		ToolCallID: toolCall.ID,
		Error:      "concurrency limit exceeded or context canceled while waiting for tool execution permit",
	}
}

func callToolHandler(
	ctx context.Context,
	definition *types.ToolDefinition,
	args map[string]any,
	state *atomic.Uint32,
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
	e.recordCircuitFailure()
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
		e.recordCircuitFailure()
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("schema validation failed: %v", err),
		}, true
	}
	return types.ToolResult{}, false
}

func (e *ToolExecutor) rejectOversizedOutput(toolCall types.ToolCall, result any) (types.ToolResult, bool) {
	if !e.safetyConfig.HasOutputSizeLimit() || result == nil {
		return types.ToolResult{}, false
	}
	if err := e.validateOutputSize(result); err != nil {
		e.recordCircuitFailure()
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("output size limit exceeded: %v", err),
		}, true
	}
	return types.ToolResult{}, false
}

func (e *ToolExecutor) recordCircuitFailure() {
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordFailure()
	}
}

func (e *ToolExecutor) recordCircuitSuccess() {
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordSuccess()
	}
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
	if e.safetyConfig.EnableCircuitBreaker {
		e.circuitBreaker = NewSimpleCircuitBreaker(
			e.safetyConfig.CircuitBreakerThreshold,
			e.safetyConfig.CircuitBreakerResetTimeout,
		)
	}
	if e.safetyConfig.MaxRetriesPerTool > 0 {
		e.retryExecutor = NewRetryExecutor(e.safetyConfig.MaxRetriesPerTool)
	}
}
