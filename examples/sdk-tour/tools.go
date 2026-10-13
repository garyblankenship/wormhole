package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func add(ctx context.Context, args addArgs) (addResult, error) {
	if err := ctx.Err(); err != nil {
		return addResult{}, err
	}
	return addResult{Sum: args.A + args.B}, nil
}
func toolExample(ctx context.Context, j *journey) error {
	client := j.client()
	var calls atomic.Int64
	err := wormhole.RegisterTypedTool(client, j.cfg.Tool.Name, j.cfg.Tool.Description, func(ctx context.Context, args addArgs) (addResult, error) { calls.Add(1); return add(ctx, args) })
	if err != nil {
		return err
	}
	resp, err := j.text(client).WithToolsEnabled().Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	if calls.Load() != 1 || len(j.fixture.all()) != 2 {
		return errors.New("automatic tool execution round count mismatch")
	}
	if err = client.UnregisterTool(j.cfg.Tool.Name); err != nil {
		return err
	}
	return require(!client.HasTool(j.cfg.Tool.Name) && client.ToolCount() == 0, "tool unregister failed")
}
func manualToolExample(ctx context.Context, j *journey) error {
	client := j.client()
	schema, err := wormhole.SchemaFromStruct(addArgs{})
	if err != nil {
		return err
	}
	tool := types.NewTool(j.cfg.Tool.Name, j.cfg.Tool.Description, schema)
	resp, err := j.text(client).Tools(*tool).WithToolsDisabled().Generate(ctx)
	if err != nil {
		return err
	}
	if !resp.HasToolCalls() || len(resp.ToolCalls) != 1 {
		return errors.New("manual tool call absent")
	}
	call := resp.ToolCalls[0]
	if call.Name != j.cfg.Tool.Name || !equalJSON(call.Arguments, j.cfg.Tool.Args) {
		return errors.New("manual tool name/arguments mismatch")
	}
	data, err := json.Marshal(call.Arguments)
	if err != nil {
		return err
	}
	var args addArgs
	if err = json.Unmarshal(data, &args); err != nil {
		return err
	}
	result, err := add(ctx, args)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	resp, err = client.Text().Model(j.cfg.Models["text"]).Messages(
		types.NewUserMessage(j.cfg.Prompt),
		&types.AssistantMessage{ToolCalls: resp.ToolCalls},
		types.NewToolResultMessage(call.ID, string(encoded)),
	).Tools(*tool).WithToolsDisabled().Generate(ctx)
	return j.expectText(resp, err)
}
func agentExample(ctx context.Context, j *journey) error {
	client := j.client()
	// The deliberately different global result proves the scoped override is used.
	if err := wormhole.RegisterTypedTool(client, j.cfg.Tool.Name, j.cfg.Tool.Description, func(ctx context.Context, args addArgs) (addResult, error) {
		r, err := add(ctx, args)
		r.Sum = -r.Sum
		return r, err
	}); err != nil {
		return err
	}
	var events []wormhole.StepEvent
	builder := client.Agent().Model(j.cfg.Models["text"]).System(j.cfg.System).MaxSteps(j.cfg.Settings.AgentSteps).OnStep(func(e wormhole.StepEvent) { events = append(events, e) })
	if err := wormhole.AgentAddTool(builder, j.cfg.Tool.Name, j.cfg.Tool.Description, add); err != nil {
		return err
	}
	result, err := builder.Run(ctx, j.cfg.Prompt)
	if err != nil {
		return err
	}
	if err = j.expectText(result.Response, nil); err != nil {
		return err
	}
	if result.TotalSteps != 2 || len(result.Steps) != 2 || len(events) != 2 || !events[1].Done || len(events[0].ToolResults) != 1 {
		return errors.New("agent step history/callback mismatch")
	}
	if !equalJSON(events[0].ToolResults[0].Result, j.cfg.Tool.Result) {
		return errors.New("agent scoped result mismatch")
	}
	return require(client.ToolCount() == 1, "agent changed global tool registry")
}
func toolSafetyExample(ctx context.Context, j *journey) error {
	policy := wormhole.DefaultToolSafetyConfig()
	policy.EnableInputValidation = true
	client := j.client(wormhole.WithToolSafetyConfig(policy))
	var calls atomic.Int64
	builder := client.Agent().Model(j.cfg.Models["text"]).MaxSteps(j.cfg.Settings.SafetySteps)
	if err := wormhole.AgentAddTool(builder, j.cfg.Tool.Name, j.cfg.Tool.Description, func(ctx context.Context, args addArgs) (addResult, error) { calls.Add(1); return add(ctx, args) }); err != nil {
		return err
	}
	result, err := builder.Run(ctx, j.cfg.Prompt)
	if err == nil || result == nil || len(result.Steps) != 1 || len(result.Steps[0].ToolResults) != 1 {
		return errors.New("agent safety limit did not retain failed step")
	}
	return require(calls.Load() == 0 && result.Steps[0].ToolResults[0].Error != "", "invalid tool arguments reached handler")
}
