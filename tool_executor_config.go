package wormhole

import (
	"fmt"
	"time"
)

// ToolSafetyConfig defines safety constraints for tool execution
type ToolSafetyConfig struct {
	// MaxToolCallsPerRound rejects oversized provider tool-call batches before
	// allocation or execution. Non-positive values use the default of 32.
	MaxToolCallsPerRound int `json:"max_tool_calls_per_round" yaml:"max_tool_calls_per_round"`

	// MaxConcurrentTools limits the number of tools that can execute concurrently
	// Default: 10, 0 means unlimited
	MaxConcurrentTools int `json:"max_concurrent_tools" yaml:"max_concurrent_tools"`

	// EnableAdaptiveConcurrency enables automatic adjustment of concurrency limits
	// based on observed tool execution latencies.
	// Default: false
	EnableAdaptiveConcurrency bool `json:"enable_adaptive_concurrency" yaml:"enable_adaptive_concurrency"`

	// AdaptiveTargetLatency is the desired average latency for tool executions.
	// Used when adaptive concurrency is enabled.
	// Default: 500ms
	AdaptiveTargetLatency time.Duration `json:"adaptive_target_latency" yaml:"adaptive_target_latency"`

	// AdaptiveMinCapacity is the minimum allowed concurrent tools when using adaptive concurrency.
	// Default: 1
	AdaptiveMinCapacity int `json:"adaptive_min_capacity" yaml:"adaptive_min_capacity"`

	// AdaptiveMaxCapacity is the maximum allowed concurrent tools when using adaptive concurrency.
	// Default: 100
	AdaptiveMaxCapacity int `json:"adaptive_max_capacity" yaml:"adaptive_max_capacity"`

	// AdaptiveAdjustmentInterval is how often to evaluate and adjust capacity.
	// Default: 30s
	AdaptiveAdjustmentInterval time.Duration `json:"adaptive_adjustment_interval" yaml:"adaptive_adjustment_interval"`

	// AdaptiveLatencyWindowSize is the number of recent latencies to consider.
	// Default: 100
	AdaptiveLatencyWindowSize int `json:"adaptive_latency_window_size" yaml:"adaptive_latency_window_size"`

	// ToolTimeout sets a maximum execution time for each individual tool
	// Default: 30 seconds, 0 means no timeout
	ToolTimeout time.Duration `json:"tool_timeout" yaml:"tool_timeout"`

	// ToolQueueTimeout bounds only the wait for an execution permit. The
	// handler-only ToolTimeout starts after admission.
	// Default: 30 seconds.
	ToolQueueTimeout time.Duration `json:"tool_queue_timeout" yaml:"tool_queue_timeout"`

	// EnableCircuitBreaker enables a simple circuit breaker to stop tool execution
	// after a certain number of consecutive failures
	// Default: false
	EnableCircuitBreaker bool `json:"enable_circuit_breaker" yaml:"enable_circuit_breaker"`

	// MaxRetriesPerTool sets the maximum number of retries for a failed tool execution
	// Default: 0 (no retries)
	MaxRetriesPerTool int `json:"max_retries_per_tool" yaml:"max_retries_per_tool"`

	// CircuitBreakerThreshold is the number of consecutive failures needed to trip the circuit breaker
	// Default: 5
	CircuitBreakerThreshold int `json:"circuit_breaker_threshold" yaml:"circuit_breaker_threshold"`

	// CircuitBreakerResetTimeout is the time to wait before resetting the circuit breaker
	// Default: 1 minute
	CircuitBreakerResetTimeout time.Duration `json:"circuit_breaker_reset_timeout" yaml:"circuit_breaker_reset_timeout"`

	// MaxMemoryMB is retained for compatibility but is not enforced.
	// Positive values fail Validate and SDK-managed tool execution before a
	// handler starts.
	//
	// Deprecated: Process memory limits are unsupported.
	MaxMemoryMB int `json:"max_memory_mb" yaml:"max_memory_mb"`

	// MaxCPUTime is retained for compatibility but is not enforced.
	// Positive values fail Validate and SDK-managed tool execution before a
	// handler starts.
	//
	// Deprecated: Process CPU limits are unsupported.
	MaxCPUTime time.Duration `json:"max_cpu_time" yaml:"max_cpu_time"`

	// EnableInputValidation enables strict validation of tool arguments against schemas
	// When enabled, all tool arguments are validated against their JSON schemas
	// Default: true (recommended for production)
	EnableInputValidation bool `json:"enable_input_validation" yaml:"enable_input_validation"`

	// EnableResourceIsolation is retained for compatibility but is not
	// implemented. Enabling it fails Validate and SDK-managed tool execution
	// before a handler starts. Process isolation is outside this provider
	// bridge's scope.
	//
	// Deprecated: Resource isolation is unsupported.
	EnableResourceIsolation bool `json:"enable_resource_isolation" yaml:"enable_resource_isolation"`

	// MaxToolOutputSize limits the size of tool output in bytes
	// Prevents memory exhaustion from large tool outputs
	// Default: 10MB (10 * 1024 * 1024)
	MaxToolOutputSize int `json:"max_tool_output_size" yaml:"max_tool_output_size"`
}

// DefaultToolSafetyConfig returns a safe default configuration
func DefaultToolSafetyConfig() ToolSafetyConfig {
	return ToolSafetyConfig{
		MaxToolCallsPerRound:       32,
		MaxConcurrentTools:         10,
		EnableAdaptiveConcurrency:  false,
		AdaptiveTargetLatency:      500 * time.Millisecond,
		AdaptiveMinCapacity:        1,
		AdaptiveMaxCapacity:        100,
		AdaptiveAdjustmentInterval: 30 * time.Second,
		AdaptiveLatencyWindowSize:  100,
		ToolTimeout:                30 * time.Second,
		ToolQueueTimeout:           30 * time.Second,
		EnableCircuitBreaker:       false,
		MaxRetriesPerTool:          0,
		CircuitBreakerThreshold:    5,
		CircuitBreakerResetTimeout: time.Minute,
		MaxMemoryMB:                0,                // Unlimited by default
		MaxCPUTime:                 0,                // Unlimited by default
		EnableInputValidation:      true,             // Enabled by default for safety
		EnableResourceIsolation:    false,            // Disabled by default (performance)
		MaxToolOutputSize:          10 * 1024 * 1024, // 10MB default
	}
}

// Validate validates the safety configuration
func (c *ToolSafetyConfig) Validate() error {
	var unsupported []string

	if c.MaxToolCallsPerRound <= 0 {
		c.MaxToolCallsPerRound = 32
	}
	if c.MaxConcurrentTools < 0 {
		c.MaxConcurrentTools = 0
	}
	if c.ToolTimeout < 0 {
		c.ToolTimeout = 0
	}
	if c.ToolQueueTimeout <= 0 {
		c.ToolQueueTimeout = 30 * time.Second
	}
	if c.MaxRetriesPerTool < 0 {
		c.MaxRetriesPerTool = 0
	}
	if c.CircuitBreakerThreshold < 1 {
		c.CircuitBreakerThreshold = 1
	}
	if c.CircuitBreakerResetTimeout < 0 {
		c.CircuitBreakerResetTimeout = 0
	}
	// Validate adaptive concurrency fields
	if c.AdaptiveMinCapacity < 1 {
		c.AdaptiveMinCapacity = 1
	}
	if c.AdaptiveMaxCapacity < c.AdaptiveMinCapacity {
		c.AdaptiveMaxCapacity = c.AdaptiveMinCapacity
	}
	if c.AdaptiveTargetLatency <= 0 {
		c.AdaptiveTargetLatency = 500 * time.Millisecond
	}
	if c.AdaptiveAdjustmentInterval <= 0 {
		c.AdaptiveAdjustmentInterval = 30 * time.Second
	}
	if c.AdaptiveLatencyWindowSize < 1 {
		c.AdaptiveLatencyWindowSize = 100
	}
	// Validate new security fields
	if c.MaxMemoryMB < 0 {
		c.MaxMemoryMB = 0
	}
	if c.MaxCPUTime < 0 {
		c.MaxCPUTime = 0
	}
	if c.MaxMemoryMB > 0 {
		unsupported = append(unsupported, "MaxMemoryMB")
	}
	if c.MaxCPUTime > 0 {
		unsupported = append(unsupported, "MaxCPUTime")
	}
	if c.EnableResourceIsolation {
		unsupported = append(unsupported, "EnableResourceIsolation")
	}
	if c.MaxToolOutputSize < 0 {
		c.MaxToolOutputSize = 0
	} else if c.MaxToolOutputSize == 0 {
		c.MaxToolOutputSize = 10 * 1024 * 1024 // Default to 10MB if 0
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported tool safety settings: %v", unsupported)
	}
	return nil
}

// ToAdaptiveConfig converts the safety configuration to an adaptive configuration
func (c *ToolSafetyConfig) ToAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		TargetLatency:      c.AdaptiveTargetLatency,
		MinCapacity:        c.AdaptiveMinCapacity,
		MaxCapacity:        c.AdaptiveMaxCapacity,
		InitialCapacity:    c.MaxConcurrentTools,
		AdjustmentInterval: c.AdaptiveAdjustmentInterval,
		LatencyWindowSize:  c.AdaptiveLatencyWindowSize,
	}
}

// IsUnlimitedConcurrency returns true if no concurrency limit is set
func (c *ToolSafetyConfig) IsUnlimitedConcurrency() bool {
	return c.MaxConcurrentTools == 0
}

// HasTimeout returns true if a timeout is configured
func (c *ToolSafetyConfig) HasTimeout() bool {
	return c.ToolTimeout > 0
}

// HasMemoryLimit reports whether the unsupported MaxMemoryMB compatibility
// field is positive.
//
// Deprecated: Process memory limits are unsupported. Positive MaxMemoryMB
// values fail Validate and SDK-managed tool execution before a handler starts.
func (c *ToolSafetyConfig) HasMemoryLimit() bool {
	return c.MaxMemoryMB > 0
}

// HasCPULimit reports whether the unsupported MaxCPUTime compatibility field is
// positive.
//
// Deprecated: Process CPU limits are unsupported. Positive MaxCPUTime values
// fail Validate and SDK-managed tool execution before a handler starts.
func (c *ToolSafetyConfig) HasCPULimit() bool {
	return c.MaxCPUTime > 0
}

// HasOutputSizeLimit returns true if an output size limit is configured
func (c *ToolSafetyConfig) HasOutputSizeLimit() bool {
	return c.MaxToolOutputSize > 0
}
