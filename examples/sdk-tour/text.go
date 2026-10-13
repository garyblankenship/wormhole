package main

import (
	"context"
	"errors"

	"github.com/garyblankenship/wormhole/v3/types"
)

func textExample(ctx context.Context, j *journey) error {
	client := j.client()
	resp, err := j.text(client).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	return require(resp.Provider == "openai" && resp.Model == j.cfg.Models["text"] && resp.Usage != nil && resp.Usage.TotalTokens == 5, "response source/model/usage mismatch")
}
func conversationExample(ctx context.Context, j *journey) error {
	client := j.client()
	conv := types.NewConversation().System(j.cfg.System).User(j.cfg.Prompt).Assistant(j.cfg.Answer).User(j.cfg.Prompt)
	resp, err := client.Text().Model(j.cfg.Models["text"]).Conversation(conv).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	requests := j.fixture.all()
	messages := array(requests[0].Body["messages"])
	if len(messages) != 4 || str(object(messages[0])["content"]) != j.cfg.System {
		return errors.New("conversation/system order mismatch")
	}
	user := types.NewUserMessage(j.cfg.Prompt)
	user.Media = []types.Media{&types.ImageMedia{MimeType: "image/png", Base64Data: j.cfg.Image}}
	resp, err = client.Text().Model(j.cfg.Models["text"]).Messages(user).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	wire := j.fixture.all()[1].Body
	parts := array(object(array(wire["messages"])[0])["content"])
	if len(parts) != 2 {
		return errors.New("vision content parts absent")
	}
	return require(str(object(object(parts[1])["image_url"])["url"]) == "data:image/png;base64,"+j.cfg.Image, "vision bytes lost on wire")
}
func controlsExample(ctx context.Context, j *journey) error {
	client := j.client()
	s := j.cfg.Settings
	base := j.text(client).Temperature(s.Temperature).TopP(s.TopP).Seed(s.Seed).MaxTokens(s.MaxTokens).ParallelToolCalls(false).Reasoning(types.Reasoning{Effort: types.ReasoningEffortLow}).ProviderOptions(map[string]any{"user": j.cfg.Structured.Name})
	resp, err := base.Clone().Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	wire := j.fixture.all()[0].Body
	return require(wire["temperature"] == float64(s.Temperature) && wire["top_p"] == float64(s.TopP) && wire["seed"] == float64(s.Seed) && str(wire["reasoning_effort"]) == "low" && str(wire["user"]) == j.cfg.Structured.Name, "sampling/reasoning/provider options lost on wire")
}
