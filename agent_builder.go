package wormhole

import (
	"context"
	"encoding/json"
	"fmt"

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

// Run executes the agent loop with the given prompt.
// It returns the final result after all tool executions complete, or an error.
func (b *AgentBuilder) Run(ctx context.Context, prompt string) (*AgentResult, error) {
	if b.model == "" {
		return nil, fmt.Errorf("agent: model is required")
	}

	maxSteps := b.maxSteps

	// Merge agent-scoped tools with global registry tools.
	// Agent tools take precedence over global tools with the same name.
	mergedRegistry := b.mergeTools()

	if mergedRegistry.Count() == 0 {
		return nil, fmt.Errorf("agent: no tools registered — use AddTool() or AgentAddTool()")
	}

	// Resolve provider
	providerName := b.provider
	if providerName == "" {
		providerName = b.wormhole.config.DefaultProvider
	}

	provider, release, err := b.wormhole.leaseProvider(providerName)
	if err != nil {
		return nil, fmt.Errorf("agent: %w", err)
	}
	defer release()

	// Build initial request
	request := types.TextRequest{
		Messages: []types.Message{types.NewUserMessage(prompt)},
		Tools:    mergedRegistry.List(),
	}
	request.Model = b.model
	request.SystemPrompt = b.systemPrompt
	if b.temperature != nil {
		request.Temperature = b.temperature
	}
	if b.maxTokens != nil {
		request.MaxTokens = b.maxTokens
	}

	// Prepare messages (inject system prompt)
	request.Messages = prepareExecutionMessages(request.SystemPrompt, request.Messages)

	// Create executor for tool calls
	executor := NewToolExecutor(mergedRegistry)

	var steps []StepEvent
	ctx = contextWithProviderOperation(ctx, provider, "agent")

	for step := 1; step <= maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("agent step %d: %w", step, err)
		}

		// Call the LLM (through middleware if configured)
		var resp *types.TextResponse
		if b.wormhole.providerMiddleware != nil {
			handler := b.wormhole.providerMiddleware.ApplyText(provider.Text)
			resp, err = handler(ctx, request)
		} else {
			resp, err = provider.Text(ctx, request)
		}
		if err != nil {
			return nil, fmt.Errorf("agent step %d: %w", step, err)
		}

		// No tool calls — final response
		if len(resp.ToolCalls) == 0 {
			event := StepEvent{
				Step:     step,
				Response: resp,
				Done:     true,
			}
			steps = append(steps, event)
			b.fireStepEvent(event)
			return &AgentResult{
				Response:   resp,
				Steps:      steps,
				TotalSteps: step,
			}, nil
		}

		// Execute tool calls
		toolResults := executor.ExecuteAll(ctx, resp.ToolCalls)

		event := StepEvent{
			Step:        step,
			Response:    resp,
			ToolCalls:   resp.ToolCalls,
			ToolResults: toolResults,
			Done:        false,
		}
		steps = append(steps, event)
		b.fireStepEvent(event)

		// Build conversation continuation. Thinking carries the signed reasoning
		// block so Anthropic extended-thinking + tool_use replay doesn't hard-400.
		assistantMsg := &types.AssistantMessage{
			Content:   resp.Text,
			ToolCalls: resp.ToolCalls,
			Thinking:  resp.Thinking,
		}

		request.Messages = append(request.Messages, assistantMsg)
		for _, toolResultMsg := range executor.BuildToolResultMessages(toolResults) {
			request.Messages = append(request.Messages, toolResultMsg)
		}
	}

	return nil, fmt.Errorf("agent: max steps (%d) reached without final response", maxSteps)
}

// mergeTools creates a merged registry with global + agent-scoped tools.
// Agent tools override global tools with the same name.
func (b *AgentBuilder) mergeTools() *ToolRegistry {
	merged := NewToolRegistry()

	// Copy global tools first
	globalTools := b.wormhole.toolRegistry
	for _, name := range globalTools.ListNames() {
		def := globalTools.Get(name)
		if def != nil {
			merged.Register(name, def)
		}
	}

	// Override with agent-scoped tools
	for _, name := range b.tools.ListNames() {
		def := b.tools.Get(name)
		if def != nil {
			merged.Register(name, def)
		}
	}

	return merged
}

// AgentAddTool registers a type-safe tool on the AgentBuilder.
// This is the agent-scoped equivalent of RegisterTypedTool.
//
// Example:
//
//	type SearchArgs struct {
//	    Query string `json:"query" tool:"required" desc:"Search query"`
//	}
//
//	builder := client.Agent().Model("gpt-5.2")
//	wormhole.AgentAddTool(builder, "search", "Search the web",
//	    func(ctx context.Context, args SearchArgs) (SearchResult, error) {
//	        return search(args.Query), nil
//	    },
//	)
//	result, _ := builder.Run(ctx, "Find Go 1.23 release notes")
func AgentAddTool[Args any, Result any](
	builder *AgentBuilder,
	name string,
	description string,
	handler func(ctx context.Context, args Args) (Result, error),
) error {
	var args Args
	schema, err := SchemaFromStruct(args)
	if err != nil {
		return fmt.Errorf("failed to generate schema for tool %q: %w", name, err)
	}

	wrappedHandler := func(ctx context.Context, arguments map[string]any) (any, error) {
		jsonBytes, err := json.Marshal(arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal arguments: %w", err)
		}
		var typedArgs Args
		if err := json.Unmarshal(jsonBytes, &typedArgs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal arguments to %T: %w", typedArgs, err)
		}
		return handler(ctx, typedArgs)
	}

	builder.AddTool(name, description, schema, wrappedHandler)
	return nil
}
