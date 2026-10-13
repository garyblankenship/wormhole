package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/pprof"
	"strings"

	wormhole "github.com/garyblankenship/wormhole/v3"
)

// Environment examples run in a child so tests and the user's process retain
// their original environment. HOME confines factory-owned discovery cache writes.
func environmentExample(ctx context.Context, j *journey) (err error) {
	binary := os.Getenv("WORMHOLE_SDK_TOUR_BIN")
	if binary == "" {
		var err error
		binary, err = os.Executable()
		if err != nil {
			return err
		}
	}
	home, err := os.MkdirTemp("", "wormhole-sdk-tour-env-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(home)) }()
	cmd := exec.CommandContext(ctx, binary, "env-child") // #nosec G204 G702 -- Operator-selected trusted build artifact; direct exec without shell expansion.
	cmd.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "SDK_TOUR_UPSTREAM=" + j.server.URL, "HTTP_PROXY=" + j.server.URL, "HTTPS_PROXY=" + j.server.URL, "NO_PROXY=127.0.0.1,localhost"}
	for _, p := range wormhole.KnownProviderProfiles() {
		base := j.server.URL + "/v1"
		if p.Kind == "native" && p.Name != "openai" {
			base = j.server.URL + "/" + p.Name
		}
		for _, key := range p.APIKeyEnv {
			cmd.Env = append(cmd.Env, key+"="+j.cfg.FakeKeys[0])
		}
		if p.BaseURLEnv != "" {
			cmd.Env = append(cmd.Env, p.BaseURLEnv+"="+base)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("environment child: %w: %s", err, output)
	}
	return require(strings.Contains(string(output), "PASS factories-env-child"), "environment child omitted result receipt")
}

func environmentChild(ctx context.Context, out io.Writer) error {
	var cfg fixtureConfig
	if err := json.Unmarshal(fixtureData, &cfg); err != nil {
		return err
	}
	base := os.Getenv("SDK_TOUR_UPSTREAM")
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		return errors.New("environment child requires owned loopback upstream")
	}
	j := &journey{cfg: cfg}
	factory := wormhole.NewSimpleFactory()
	clients := []*wormhole.Wormhole{factory.OpenAI(cfg.FakeKeys[0]), factory.Anthropic(cfg.FakeKeys[0]), factory.Gemini(cfg.FakeKeys[0]), factory.Groq(cfg.FakeKeys[0]), factory.Mistral(cfg.FakeKeys[0])}
	providers := []string{"openai", "anthropic", "gemini", "groq", "mistral"}
	defer func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}()
	for i, client := range clients {
		model := cfg.Models["text"]
		if providerModel := cfg.Models[providers[i]]; providerModel != "" {
			model = providerModel
		}
		resp, err := client.Text().Model(model).Prompt(cfg.Prompt).MaxTokens(cfg.Settings.MaxTokens).Generate(ctx)
		if err = j.expectText(resp, err); err != nil {
			return fmt.Errorf("factory %s: %w", providers[i], err)
		}
	}
	local, err := wormhole.QuickLocalOpenAI(base + "/v1")
	if err != nil {
		return err
	}
	clients = append(clients, local)
	envClient := wormhole.New(wormhole.WithAllProvidersFromEnv(), wormhole.WithProviderFromEnv("openai"), wormhole.WithDefaultProvider("openai"), wormhole.WithDiscovery(false), wormhole.WithModelValidation(false))
	clients = append(clients, envClient)
	if err := j.expectText(envClient.Text().Model(cfg.Models["text"]).Prompt(cfg.Prompt).Generate(ctx)); err != nil {
		return err
	}
	resp, err := local.Text().Model(cfg.Models["text"]).Prompt(cfg.Prompt).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	ollama, err := wormhole.QuickOllama(base + "/ollama")
	if err != nil {
		return err
	}
	clients = append(clients, ollama)
	resp, err = ollama.Text().Model(cfg.Models["ollama"]).Prompt(cfg.Prompt).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	// Close all explicitly owned clients before probing one-off helper lifetimes.
	for _, client := range clients {
		if err = client.Close(); err != nil {
			return err
		}
	}
	resp, err = wormhole.QuickTextWithContext(ctx, cfg.Models["text"], cfg.Prompt, cfg.FakeKeys[0])
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	resp, err = wormhole.QuickChatWithContext(ctx, cfg.Models["text"], cfg.System, cfg.Prompt, cfg.FakeKeys[0])
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	stream, err := wormhole.QuickStreamWithContext(ctx, cfg.Models["text"], cfg.Prompt, cfg.FakeKeys[0])
	if err != nil {
		return err
	}
	var got strings.Builder
	for chunk := range stream {
		if chunk.Error != nil {
			return chunk.Error
		}
		got.WriteString(chunk.Content())
	}
	if got.String() != cfg.Answer {
		return errors.New("quick stream content mismatch")
	}
	var stacks bytes.Buffer
	if err = pprof.Lookup("goroutine").WriteTo(&stacks, 1); err != nil {
		return err
	}
	if strings.Contains(stacks.String(), "DiscoveryService).StartBackgroundRefresh") {
		return fmt.Errorf("quick helpers retained discovery workers after completion: %s", stacks.String())
	}
	_, err = fmt.Fprintln(out, "PASS factories-env-child")
	return err
}
