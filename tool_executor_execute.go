package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/internal/schemavalidation"
	"github.com/garyblankenship/wormhole/v2/types"
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
}

// NewToolExecutor creates a new ToolExecutor with the given registry and default safety config
func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return NewToolExecutorWithConfig(registry, DefaultToolSafetyConfig())
}

// NewToolExecutorWithConfig creates a new ToolExecutor with custom safety configuration
func NewToolExecutorWithConfig(registry *ToolRegistry, config ToolSafetyConfig) *ToolExecutor {
	// Validate and apply defaults
	validationErr := config.Validate()

	executor := &ToolExecutor{
		registry:     registry,
		safetyConfig: config,
		configErr:    validationErr,
	}

	if validationErr != nil {
		return executor
	}

	// Initialize concurrency limiter if configured
	if config.EnableAdaptiveConcurrency && !config.IsUnlimitedConcurrency() {
		// Use adaptive concurrency control
		executor.adaptiveLimiter = NewAdaptiveLimiter(config.ToAdaptiveConfig())
	} else if !config.IsUnlimitedConcurrency() {
		// Use fixed concurrency limit
		executor.limiter = NewConcurrencyLimiter(config.MaxConcurrentTools)
	}

	// Initialize circuit breaker if enabled
	if config.EnableCircuitBreaker {
		executor.circuitBreaker = NewSimpleCircuitBreaker(
			config.CircuitBreakerThreshold,
			config.CircuitBreakerResetTimeout,
		)
	}

	// Initialize retry executor if configured
	if config.MaxRetriesPerTool > 0 {
		executor.retryExecutor = NewRetryExecutor(config.MaxRetriesPerTool)
	}

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
	if e.configErr != nil {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      e.configErr.Error(),
		}
	}

	// Check circuit breaker if enabled
	if e.circuitBreaker != nil && e.circuitBreaker.IsTripped() {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      "circuit breaker tripped - tool execution temporarily disabled",
		}
	}

	// Get tool definition from registry
	definition := e.registry.Get(toolCall.Name)
	if definition == nil {
		// Record failure for circuit breaker
		if e.circuitBreaker != nil {
			e.circuitBreaker.RecordFailure()
		}
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("tool %q not found in registry", toolCall.Name),
		}
	}

	// Arguments are already a map from the provider
	args := toolCall.Arguments

	// Validate arguments against schema if schema is provided
	if e.safetyConfig.EnableInputValidation && definition.Tool.InputSchema != nil {
		if err := schemavalidation.ValidateAgainstSchema(args, definition.Tool.InputSchema); err != nil {
			// Record failure for circuit breaker
			if e.circuitBreaker != nil {
				e.circuitBreaker.RecordFailure()
			}
			return types.ToolResult{
				ToolCallID: toolCall.ID,
				Error:      fmt.Sprintf("schema validation failed: %v", err),
			}
		}
	}

	// Apply timeout if configured
	var cancel context.CancelFunc
	if e.safetyConfig.HasTimeout() {
		ctx, cancel = context.WithTimeout(ctx, e.safetyConfig.ToolTimeout)
		defer cancel()
	}

	// Execute the tool handler with retry logic if configured. callHandler wraps the
	// user handler so a panic (e.g. nil-map deref on unexpected LLM args) becomes an
	// error instead of crashing the per-tool goroutine — nothing recovers above it,
	// so an unrecovered panic here would take down the whole process (and the proxy).
	var result any
	var err error

	callHandler := func(ctx context.Context) (res any, rerr error) {
		defer func() {
			if r := recover(); r != nil {
				rerr = fmt.Errorf("tool handler panicked: %v", r)
			}
		}()
		return definition.Handler(ctx, args)
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
	go func() {
		r, e := execute()
		done <- outcome{result: r, err: e}
	}()
	select {
	case o := <-done:
		result, err = o.result, o.err
	case <-ctx.Done():
		err = fmt.Errorf("tool %q timed out or was canceled: %w", toolCall.Name, ctx.Err())
	}

	if err != nil {
		// Record failure for circuit breaker
		if e.circuitBreaker != nil {
			e.circuitBreaker.RecordFailure()
		}
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      err.Error(),
		}
	}

	// Apply output size limit if configured
	if e.safetyConfig.HasOutputSizeLimit() && result != nil {
		if err := e.validateOutputSize(result); err != nil {
			// Record failure for circuit breaker
			if e.circuitBreaker != nil {
				e.circuitBreaker.RecordFailure()
			}
			return types.ToolResult{
				ToolCallID: toolCall.ID,
				Error:      fmt.Sprintf("output size limit exceeded: %v", err),
			}
		}
	}

	// Record success for circuit breaker
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordSuccess()
	}

	return types.ToolResult{
		ToolCallID: toolCall.ID,
		Result:     result, // Result is any, not string
	}
}
