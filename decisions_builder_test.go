package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

const decisionsFixture = `{"id":"decision-1","model":"typesafe/jev-1.13","answers":{"accept":{"type":"noul","noul":0},"route":{"type":"choice","choice":"review","probabilities":{"review":0.8,"allow":0.2},"confidence":0.8},"quality":{"type":"score","score":0.25,"probabilities":{"0":0.75,"1":0.25},"confidence":0.75,"legend":{"0":"low","1":"high"}}},"usage":{"input_tokens":7,"output_tokens":3,"cost":0},"future_field":true}`

func decisionsQuestions() map[string]types.DecisionQuestion {
	return map[string]types.DecisionQuestion{
		"accept": {Type: types.DecisionNoul, Instructions: "accept?", Criteria: map[string]any{"true": []string{"yes"}, "false": map[string]string{"description": "no"}}},
		"route": {Type: types.DecisionChoice, Instructions: struct {
			Task string `json:"task"`
		}{"route"}, Criteria: map[string]any{"review": nil, "allow": "safe"}},
		"quality": {Type: types.DecisionScore, Instructions: []string{"quality"}, Criteria: []any{"low", map[string]string{"description": "high"}}},
	}
}
func decisionsFixtureClient(t *testing.T, handler http.HandlerFunc, options ...Option) *Wormhole {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	zero := 0
	opts := []Option{WithDiscovery(false), WithDefaultProvider("openrouter"), WithOpenAICompatible("openrouter", server.URL+"/api/v1", types.ProviderConfig{APIKey: "sk-or-v1-fixture", MaxRetries: &zero, Headers: map[string]string{"X-Title": "fixture"}})}
	opts = append(opts, options...)
	client := New(opts...)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func writeDecisionsFixture(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := fmt.Fprint(w, body); err != nil {
		t.Error(err)
	}
}

func TestDecisionsBuilderWireAndAnswers(t *testing.T) {
	t.Parallel()
	client := decisionsFixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !assertDecisionsWireRequest(t, w, r) {
			return
		}
		writeDecisionsFixture(t, w, decisionsFixture)
	})
	response, err := client.Decisions().Model("typesafe/jev-1.13").State(json.RawMessage(`{"id":9007199254740993}`)).Questions(decisionsQuestions()).Evaluate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertDecisionsWireAnswers(t, response)
}

func assertDecisionsWireRequest(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	t.Helper()
	if r.Method != "POST" || r.URL.Path != "/api/alpha/decisions" || r.Header.Get("Authorization") != "Bearer sk-or-v1-fixture" || r.Header.Get("X-Title") != "fixture" {
		t.Errorf("unexpected request: %s %s %v", r.Method, r.URL.Path, r.Header)
	}
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		t.Error(err)
		w.WriteHeader(400)
		return false
	}
	if len(payload) != 3 || string(payload["model"]) != `"typesafe/jev-1.13"` || !strings.Contains(string(payload["state"]), "9007199254740993") {
		t.Errorf("wrong payload: %s", payload)
	}
	var questions map[string]struct {
		Type         string          `json:"type"`
		Instructions json.RawMessage `json:"instructions"`
		Criteria     json.RawMessage `json:"criteria"`
	}
	if err := json.Unmarshal(payload["questions"], &questions); err != nil {
		t.Error(err)
	}
	if len(questions) != 3 || questions["accept"].Type != "noul" || string(questions["route"].Instructions) != `{"task":"route"}` || string(questions["route"].Criteria) != `{"allow":"safe","review":null}` || string(questions["quality"].Criteria) != `["low",{"description":"high"}]` {
		t.Errorf("wrong questions: %s", payload["questions"])
	}
	return true
}

func assertDecisionsWireAnswers(t *testing.T, response *types.DecisionsResponse) {
	t.Helper()
	if response.ID != "decision-1" || response.Provider != "openrouter" || response.Model != "typesafe/jev-1.13" || response.Usage.InputTokens != 7 || response.Usage.OutputTokens != 3 || response.Usage.Cost == nil || *response.Usage.Cost != 0 {
		t.Fatalf("metadata lost: %+v", response)
	}
	if a := response.Answers["accept"]; a.Noul == nil || *a.Noul != 0 {
		t.Fatal("noul zero lost")
	}
	if a := response.Answers["route"]; a.Choice == nil || *a.Choice != "review" || a.Probabilities["review"] != 0.8 || a.Confidence == nil || *a.Confidence != 0.8 {
		t.Fatalf("choice lost: %+v", a)
	}
	if a := response.Answers["quality"]; a.Score == nil || *a.Score != 0.25 || a.Legend["1"] != "high" {
		t.Fatalf("score lost: %+v", a)
	}
}

func TestDecisionsBuilderValidationBeforeHTTP(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	client := decisionsFixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		writeDecisionsFixture(t, w, decisionsFixture)
	})
	for _, state := range []any{nil, true, 42, json.RawMessage(`null`), make(chan int)} {
		if _, err := client.Decisions().Model("typesafe/jev-1.13").State(state).Questions(decisionsQuestions()).Evaluate(t.Context()); err == nil {
			t.Fatalf("accepted invalid state %T", state)
		}
	}
	if _, err := client.Decisions().State("x").Questions(decisionsQuestions()).Evaluate(t.Context()); err == nil {
		t.Fatal("accepted missing model")
	}
	if _, err := client.Decisions().Model("x").State("x").Evaluate(t.Context()); err == nil {
		t.Fatal("accepted missing questions")
	}
	bad := decisionsQuestions()
	bad["route"] = types.DecisionQuestion{Type: types.DecisionChoice, Instructions: "x", Criteria: map[string]any{}}
	if _, err := client.Decisions().Model("x").State("x").Questions(bad).Evaluate(t.Context()); err == nil {
		t.Fatal("accepted empty choice criteria")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests made %d calls", calls.Load())
	}
}

func TestDecisionsBuilderIdempotencyAndChatRoute(t *testing.T) {
	t.Parallel()
	var decisions, chat atomic.Int32
	client := decisionsFixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/alpha/decisions":
			decisions.Add(1)
			writeDecisionsFixture(t, w, decisionsFixture)
		case "/api/v1/chat/completions":
			chat.Add(1)
			writeDecisionsFixture(t, w, `{"id":"chat-1","choices":[{"message":{"role":"assistant","content":"routed"},"finish_reason":"stop"}],"usage":{}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}, WithIdempotencyKey("fixture"), WithModelValidation(false))
	builder := client.Decisions().Model("typesafe/jev-1.13").State("x").Questions(decisionsQuestions())
	first, err := builder.Evaluate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	a := first.Answers["accept"]
	*a.Noul = 1
	second, err := builder.Evaluate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if *second.Answers["accept"].Noul != 0 || decisions.Load() != 1 {
		t.Fatal("cached response not isolated or duplicate HTTP")
	}
	if _, err := client.Text().Model("typesafe/jev-router").Prompt("route chat").Generate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if chat.Load() != 1 {
		t.Fatal("router did not use standard chat")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Evaluate(t.Context()); err == nil {
		t.Fatal("admitted after shutdown")
	}
}

type decisionsOptInProvider struct {
	*types.BaseProvider
	invoke func(context.Context, types.DecisionsRequest) (*types.DecisionsResponse, error)
	closed atomic.Bool
}

func (p *decisionsOptInProvider) Decisions(ctx context.Context, r types.DecisionsRequest) (*types.DecisionsResponse, error) {
	return p.invoke(ctx, r)
}
func (p *decisionsOptInProvider) Close() error { p.closed.Store(true); return nil }

// The historical wrapper exposes middleware operations, not the full Provider
// lifecycle interface. Preserve that surface while exercising its forwarding.
type decisionsWrappedTestProvider struct {
	types.Provider
	wrapper *types.ProviderWrapper
}

func (p *decisionsWrappedTestProvider) Decisions(ctx context.Context, request types.DecisionsRequest) (*types.DecisionsResponse, error) {
	return p.wrapper.Decisions(ctx, request)
}

func TestDecisionsBuilderOptionalProviderAndShutdownLease(t *testing.T) {
	t.Parallel()
	entered, release := make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	p := &decisionsOptInProvider{BaseProvider: types.NewBaseProvider("custom")}
	p.invoke = func(ctx context.Context, r types.DecisionsRequest) (*types.DecisionsResponse, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if p.closed.Load() {
			t.Error("provider closed during operation")
		}
		return &types.DecisionsResponse{ID: "custom"}, nil
	}
	client := New(WithDiscovery(false), WithDefaultProvider("custom"), WithProviderConfig("custom", types.ProviderConfig{}), WithCustomProvider("custom", func(types.ProviderConfig) (types.Provider, error) {
		return &decisionsWrappedTestProvider{Provider: p, wrapper: types.NewProviderWrapper(p)}, nil
	}))
	t.Cleanup(func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
		_ = client.Close()
	})
	done := make(chan error, 1)
	go func() {
		_, err := client.Decisions().Model("arbitrary-direct-model").State("x").Questions(decisionsQuestions()).Evaluate(t.Context())
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not entered")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.Shutdown(ctx); err == nil {
		t.Fatal("shutdown did not wait for operation")
	}
	if p.closed.Load() {
		t.Fatal("closed leased provider")
	}
	released.Store(true)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if !p.closed.Load() {
		t.Fatal("provider not closed")
	}
}

func TestDecisionsBuilderOldProviderUnsupported(t *testing.T) {
	t.Parallel()
	p := types.NewBaseProvider("old")
	client := New(WithDiscovery(false), WithDefaultProvider("old"), WithProviderConfig("old", types.ProviderConfig{}), WithCustomProvider("old", func(types.ProviderConfig) (types.Provider, error) {
		return &decisionsWrappedTestProvider{Provider: p, wrapper: types.NewProviderWrapper(p)}, nil
	}))
	t.Cleanup(func() { _ = client.Close() })
	if _, err := client.Decisions().Model("x").State("x").Questions(decisionsQuestions()).Evaluate(t.Context()); err == nil || !strings.Contains(err.Error(), "does not support Decisions") {
		t.Fatalf("legacy provider error=%v", err)
	}
}

func TestDecisionsBuilderBaseURLOverride(t *testing.T) {
	t.Parallel()
	var original, override atomic.Int32
	client := decisionsFixtureClient(t, func(http.ResponseWriter, *http.Request) { original.Add(1) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		override.Add(1)
		if r.URL.Path != "/isolated/alpha/decisions" {
			t.Errorf("override path=%s", r.URL.Path)
		}
		writeDecisionsFixture(t, w, decisionsFixture)
	}))
	t.Cleanup(server.Close)
	if _, err := client.Decisions().BaseURL(server.URL + "/isolated/v1").Model("typesafe/jev-1.13").State("x").Questions(decisionsQuestions()).Evaluate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if original.Load() != 0 || override.Load() != 1 {
		t.Fatal("base override did not isolate endpoint")
	}
}

// This example is compiled but intentionally does not execute a live provider.
func ExampleWormhole_Decisions() {
	client := New(WithDefaultProvider("openrouter"), WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{APIKey: "sk-or-v1-placeholder"}))
	defer func() { _ = client.Close() }()
	result, err := client.Decisions().Model("typesafe/jev-1.13").State(map[string]any{"message": "Review this change"}).Questions(map[string]types.DecisionQuestion{
		"approve": {Type: types.DecisionNoul, Instructions: "Is this ready?"},
		"route":   {Type: types.DecisionChoice, Instructions: "Choose the next step", Criteria: map[string]any{"review": "Needs review", "merge": nil}},
		"quality": {Type: types.DecisionScore, Instructions: "Assess quality", Criteria: []string{"Needs work", "Ready"}},
	}).Evaluate(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(*result.Answers["approve"].Noul, *result.Answers["route"].Choice, *result.Answers["quality"].Score)
}
