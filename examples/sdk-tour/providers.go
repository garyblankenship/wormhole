package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/garyblankenship/wormhole/v3/wormholetest"
)

func nativeExample(ctx context.Context, j *journey) error {
	parts := strings.Split(j.fixture.mode, "-")
	provider, operation := parts[1], parts[2]
	config := j.config("/" + provider)
	if provider != "ollama" {
		config.APIKey = j.cfg.FakeKeys[0]
		config.NoAuth = false
	}
	var option wormhole.Option
	switch provider {
	case "anthropic":
		option = wormhole.WithAnthropic(config.APIKey, config)
	case "gemini":
		option = wormhole.WithGemini(config.APIKey, config)
	case "ollama":
		option = wormhole.WithOllama(config)
	}
	client := j.client(option)
	model := j.cfg.Models[provider]
	switch operation {
	case "text":
		resp, err := client.Text().Using(provider).Model(model).Prompt(j.cfg.Prompt).MaxTokens(j.cfg.Settings.MaxTokens).Generate(ctx)
		if err = j.expectText(resp, err); err != nil {
			return err
		}
		return require(resp.Provider == provider, "native provider attribution lost")
	case "stream":
		got, err := consume(ctx, client.Text().Using(provider).Model(model).Prompt(j.cfg.Prompt).MaxTokens(j.cfg.Settings.MaxTokens))
		if err != nil {
			return err
		}
		return require(got == j.cfg.Answer, "native stream content mismatch")
	case "structured":
		var got person
		err := client.Structured().Using(provider).Model(model).Prompt(j.cfg.Prompt).MaxTokens(j.cfg.Settings.MaxTokens).Schema(wormhole.MustSchemaFromStruct(person{})).GenerateAs(ctx, &got)
		if err != nil {
			return err
		}
		return require(got == j.cfg.Structured, "native typed structured result mismatch")
	case "embeddings":
		resp, err := client.Embeddings().Using(provider).Model(j.cfg.Models["embedding"]).Input(j.cfg.Inputs...).Generate(ctx)
		if err != nil {
			return err
		}
		if len(resp.Embeddings) != len(j.cfg.Inputs) {
			return errors.New("native embedding count mismatch")
		}
		for i, e := range resp.Embeddings {
			if e.Index != i || !slices.Equal(e.Embedding, j.cfg.Vectors[i]) {
				return fmt.Errorf("native embedding %d mismatch", i)
			}
		}
		return nil
	case "image":
		resp, err := client.Image().Using(provider).Model(model).Prompt(j.cfg.Prompt).Generate(ctx)
		if err != nil {
			return err
		}
		return require(len(resp.Images) == 1 && resp.Images[0].B64JSON == j.cfg.Image, "native image bytes mismatch")
	}
	return errors.New("unknown native operation")
}
func responsesExample(ctx context.Context, j *journey) error {
	client := j.client(wormhole.WithOpenAIResponses("", j.config("/v1")))
	if j.fixture.mode == "responses-stream" {
		got, err := consume(ctx, j.text(client))
		if err != nil {
			return err
		}
		return require(got == j.cfg.Answer, "Responses stream content mismatch")
	}
	if j.fixture.mode == "responses-structured" {
		var got person
		if err := client.Structured().Model(j.cfg.Models["text"]).Prompt(j.cfg.Prompt).Schema(wormhole.MustSchemaFromStruct(person{})).GenerateAs(ctx, &got); err != nil {
			return err
		}
		return require(got == j.cfg.Structured, "Responses structured content mismatch")
	}
	resp, err := j.text(client).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	return require(j.fixture.all()[0].Path == "/v1/responses", "Responses adapter used Chat endpoint")
}
func catalogExample(ctx context.Context, j *journey) error {
	client := j.client(wormhole.WithDiscovery(true))
	if err := client.RefreshModelsWithContext(ctx); err != nil {
		return err
	}
	models, err := client.ListAvailableModelsWithContext(ctx, "openai")
	if err != nil {
		return err
	}
	var ids []string
	for _, model := range models {
		if model.Provider != "openai" {
			return errors.New("catalog attribution mismatch")
		}
		ids = append(ids, model.ID)
	}
	slices.Sort(ids)
	want := []string{j.cfg.Models["text"], j.cfg.Models["fallback"]}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		return fmt.Errorf("catalog returned fallback instead of fixture models: %v", ids)
	}
	model, err := client.SelectModel(ctx, wormhole.ModelQuery{Providers: []string{"openai"}, NameContains: j.cfg.Models["fallback"], Capabilities: []types.ModelCapability{types.CapabilityText}})
	if err != nil {
		return err
	}
	if model.ID != j.cfg.Models["fallback"] {
		return errors.New("model query selected wrong source")
	}
	caps, err := client.ModelCapabilities("openai", model.ID)
	if err != nil {
		return err
	}
	return require(caps.SupportsText() && client.ProviderCapabilities("openai").SupportsToolCalling(), "capabilities inconsistent with selected provider")
}
func validationExample(ctx context.Context, j *journey) error {
	config := j.config("/v1")
	config.DynamicModels = false
	client := j.client(wormhole.WithOpenAI("", config), wormhole.WithModelValidation(true), wormhole.WithModels(&types.ModelInfo{ID: j.cfg.Models["text"], Provider: "openai", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityStream}}))
	resp, err := j.text(client).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	_, err = client.Text().Model(j.cfg.Models["fail"]).Prompt(j.cfg.Prompt).Generate(ctx)
	return require(err != nil && len(j.fixture.all()) == 1, "unknown model passed local validation")
}
func profilesExample(ctx context.Context, j *journey) error {
	for _, profile := range wormhole.KnownProviderProfiles() {
		if profile.Kind != "openai-compatible" {
			continue
		}
		config := j.config("/v1")
		client := j.client(wormhole.WithProfiledOpenAICompatible(profile.Name, config))
		resp, err := j.text(client).Using(profile.Name).Generate(ctx)
		if err = j.expectText(resp, err); err != nil {
			return fmt.Errorf("profile %s: %w", profile.Name, err)
		}
		if resp.Provider != profile.Name {
			return fmt.Errorf("profile %s attribution mismatch", profile.Name)
		}
	}
	client := j.client(wormhole.WithLocalOpenAI(j.server.URL + "/v1"))
	resp, err := j.text(client).Using("local").BaseURL(j.server.URL + "/v1").Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	for _, req := range j.fixture.all() {
		if req.Authorization != "" {
			return errors.New("local/profile fixture accidentally authenticated")
		}
	}
	return nil
}
func customExample(ctx context.Context, j *journey) error {
	mock := wormholetest.NewMockProvider("fixture-custom").WithTextResponse(types.TextResponse{Text: j.cfg.Answer, FinishReason: types.FinishReasonStop})
	client := j.client(wormhole.WithCustomProvider(mock.Name(), func(types.ProviderConfig) (types.Provider, error) { return mock, nil }), wormhole.WithProviderConfig(mock.Name(), types.ProviderConfig{}))
	resp, err := j.text(client).Using(mock.Name()).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	return require(client.ProviderCapabilities(mock.Name()).SupportsText() && len(j.fixture.all()) == 0, "custom provider went through HTTP")
}
