package wormhole

// Agent creates a new AgentBuilder for orchestrating multi-turn tool-calling loops.
//
// The agent provides a higher-level abstraction over tool calling:
//   - Scoped tool registry (agent tools don't leak to global client)
//   - Step hooks for observability
//   - Conversation accumulation across the loop
//   - Step history in the result
//
// Tools registered on the client via RegisterTypedTool are automatically
// available to the agent. Agent-scoped tools (via AddTool/AgentAddTool)
// take precedence over global tools with the same name.
//
// Example:
//
//	result, err := client.Agent().
//	    Model("gpt-5.2").
//	    System("You are a helpful assistant").
//	    MaxSteps(15).
//	    OnStep(func(e wormhole.StepEvent) {
//	        log.Printf("Step %d: done=%v, tools=%d", e.Step, e.Done, len(e.ToolCalls))
//	    }).
//	    Run(ctx, "What's the weather in San Francisco?")
func (p *Wormhole) Agent() *AgentBuilder {
	return &AgentBuilder{
		wormhole: p,
		tools:    NewToolRegistry(),
		maxSteps: 10,
	}
}
