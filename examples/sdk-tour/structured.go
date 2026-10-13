package main

import (
	"context"
	"errors"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func structuredExample(ctx context.Context, j *journey) error {
	client := j.client()
	builder := client.Structured().Model(j.cfg.Models["text"]).Prompt(j.cfg.Prompt).Schema(wormhole.MustSchemaFromStruct(person{})).
		TopP(j.cfg.Settings.TopP).
		Reasoning(types.Reasoning{Effort: types.ReasoningEffortLow}).
		ProviderOptions(map[string]any{"user": j.cfg.Structured.Name})
	switch j.fixture.mode {
	case "structured-json":
		builder.Mode(types.StructuredModeJSON)
	case "structured-tools":
		builder.Mode(types.StructuredModeTools)
	case "structured-strict":
		builder.Mode(types.StructuredModeStrict)
	}
	var got person
	if err := builder.GenerateAs(ctx, &got); err != nil {
		return err
	}
	if err := require(got == j.cfg.Structured, "typed structured values mismatch"); err != nil {
		return err
	}
	requests := j.fixture.all()
	if len(requests) != 1 {
		return errors.New("structured workflow did not make exactly one request")
	}
	wire := requests[0].Body
	return require(wire["top_p"] == float64(j.cfg.Settings.TopP) && str(wire["reasoning_effort"]) == "low" && str(wire["user"]) == j.cfg.Structured.Name, "structured sampling/reasoning/provider options lost on wire")
}
func schemaExample(ctx context.Context, j *journey) error {
	name := wormhole.NewStringSchema().Build()
	age := wormhole.NewNumberSchema().Build()
	schema := wormhole.NewObjectSchema().Property("name", &name).Property("age", &age).Required("name").Required("age").Build()
	if err := schema.Validate(map[string]any{"name": j.cfg.Structured.Name, "age": j.cfg.Structured.Age}); err != nil {
		return err
	}
	if err := schema.Validate(map[string]any{"name": j.cfg.Structured.Name}); err == nil {
		return errors.New("required schema field not enforced")
	}
	client := j.client()
	response, err := client.Structured().Model(j.cfg.Models["text"]).Prompt(j.cfg.Prompt).Schema(&schema).Generate(ctx)
	if err != nil {
		return err
	}
	var got person
	if err = response.ContentAs(&got); err != nil {
		return err
	}
	return require(got == j.cfg.Structured, "schema-builder structured content mismatch")
}
