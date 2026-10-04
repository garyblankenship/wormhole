package wormhole

import (
	"context"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type remediationPromptProvider struct {
	mockToolProvider
	t     *testing.T
	calls int
}

func (p *remediationPromptProvider) assertPrompt(prompt string, messages []types.Message) {
	p.t.Helper()
	if prompt != "" {
		p.t.Fatalf("dispatch SystemPrompt = %q", prompt)
	}
	count := 0
	for _, message := range messages {
		if message.GetRole() == types.RoleSystem {
			count++
			if message.GetContent() != "system" {
				p.t.Fatalf("system message = %#v", message)
			}
		}
	}
	if count != 1 {
		p.t.Fatalf("system message count = %d", count)
	}
	p.calls++
}
func (p *remediationPromptProvider) Text(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
	p.assertPrompt(request.SystemPrompt, request.Messages)
	return &types.TextResponse{Text: "done"}, nil
}
func (p *remediationPromptProvider) Stream(_ context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
	p.assertPrompt(request.SystemPrompt, request.Messages)
	ch := make(chan types.StreamChunk, 1)
	finish := types.FinishReasonStop
	ch <- types.StreamChunk{Text: "done", FinishReason: &finish}
	close(ch)
	return ch, nil
}
func (p *remediationPromptProvider) Structured(_ context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	p.assertPrompt(request.SystemPrompt, request.Messages)
	return &types.StructuredResponse{Data: map[string]any{}}, nil
}

// C4-01: every SDK dispatch contains one system prompt; builders remain reusable.
func TestRemediationC401SDKSystemPromptDispatch(t *testing.T) {
	t.Parallel()
	provider := &remediationPromptProvider{t: t}
	client := New(WithDefaultProvider("mock"), WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }), WithProviderConfig("mock", types.ProviderConfig{}), WithDiscovery(false))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	original := types.NewUserMessage("user")
	text := client.Text().Model("test").Messages(original).SystemPrompt("system")
	for range 2 {
		if _, err := text.Generate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	ch, err := text.Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if text.request.SystemPrompt != "system" || len(text.request.Messages) != 1 || original.Content != "user" {
		t.Fatal("text input mutated")
	}
	structured := client.Structured().Model("test").Messages(original).SystemPrompt("system").Schema(map[string]any{"type": "object"})
	if _, err := structured.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if structured.request.SystemPrompt != "system" || len(structured.request.Messages) != 1 {
		t.Fatal("structured input mutated")
	}
	if _, err := client.Agent().Model("test").System("system").AddTool("tool", "tool", nil, func(context.Context, map[string]any) (any, error) { return nil, nil }).Run(context.Background(), "user"); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 5 {
		t.Fatalf("calls=%d", provider.calls)
	}
}
