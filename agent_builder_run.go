package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
