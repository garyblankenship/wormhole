package wormhole

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

// StepEvent provides information about each step in the agent loop.
type StepEvent struct {
	// Step is the 1-based step number.
	Step int

	// Response is the LLM response for this step.
	Response *types.TextResponse

	// ToolCalls contains the tool calls the model wants to make (empty on final step).
	ToolCalls []types.ToolCall

	// ToolResults contains the results of tool executions (empty on final step).
	ToolResults []types.ToolResult

	// Done is true when this is the final step (no more tool calls).
	Done bool
}

// AgentResult is the final result of an agent run.
type AgentResult struct {
	// Response is the final text response after all tool executions.
	Response *types.TextResponse

	// Steps contains the event for each step in the agent loop.
	Steps []StepEvent

	// TotalSteps is the number of LLM calls made.
	TotalSteps int
}

// AgentBuilder builds and runs an agentic tool-calling loop.
//
// The agent orchestrates multi-turn conversations with automatic tool execution:
//  1. Sends the prompt to the LLM with available tools
//  2. If the LLM returns tool calls, executes them
//  3. Sends results back to the LLM
//  4. Repeats until the LLM produces a final text response (or max steps)
//
// Example:
//
//	result, err := client.Agent().
//	    Model("gpt-5.2").
//	    System("You are a research assistant").
//	    MaxSteps(20).
//	    OnStep(func(e wormhole.StepEvent) {
//	        fmt.Printf("Step %d: %d tool calls\n", e.Step, len(e.ToolCalls))
//	    }).
//	    Run(ctx, "Find the latest Go release notes")
type AgentBuilder struct {
	wormhole     *Wormhole
	provider     string
	model        string
	systemPrompt string
	tools        *ToolRegistry
	maxSteps     int
	temperature  *float32
	maxTokens    *int
	onStep       func(StepEvent)
}

// Model sets the LLM model to use.
func (b *AgentBuilder) Model(model string) *AgentBuilder {
	b.model = model
	return b
}

// Using sets the provider to use.
func (b *AgentBuilder) Using(provider string) *AgentBuilder {
	b.provider = provider
	return b
}

// System sets the system prompt for the agent.
func (b *AgentBuilder) System(prompt string) *AgentBuilder {
	b.systemPrompt = prompt
	return b
}

// MaxSteps sets the maximum number of LLM call rounds (default: 10).
func (b *AgentBuilder) MaxSteps(n int) *AgentBuilder {
	b.maxSteps = n
	return b
}

// Temperature sets the sampling temperature.
func (b *AgentBuilder) Temperature(t float32) *AgentBuilder {
	b.temperature = &t
	return b
}

// MaxTokens sets the maximum number of tokens to generate per step.
func (b *AgentBuilder) MaxTokens(n int) *AgentBuilder {
	b.maxTokens = &n
	return b
}

// OnStep registers a callback invoked after each LLM response.
// The callback receives a StepEvent with the response, tool calls, and results.
func (b *AgentBuilder) OnStep(fn func(StepEvent)) *AgentBuilder {
	b.onStep = fn
	return b
}

func (b *AgentBuilder) fireStepEvent(e StepEvent) {
	if b.onStep != nil {
		b.onStep(e)
	}
}

// AddTool registers a tool with the agent using a raw handler.
// Tools added here are scoped to this agent — they don't affect the global client registry.
func (b *AgentBuilder) AddTool(name, description string, schema map[string]any, handler types.ToolHandler) *AgentBuilder {
	b.tools.Register(name, &types.ToolDefinition{
		Tool: types.Tool{
			Type:        "function",
			Name:        name,
			Description: description,
			InputSchema: schema,
			Function: &types.ToolFunction{
				Name:        name,
				Description: description,
				Parameters:  schema,
			},
		},
		Handler: handler,
	})
	return b
}
