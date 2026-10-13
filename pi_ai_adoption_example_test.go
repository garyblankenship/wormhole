package wormhole_test

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/garyblankenship/wormhole/v3/wormholetest"
)

func ExampleTextRequestBuilder_StreamAndCollect() {
	stop := types.FinishReasonStop
	provider, err := wormholetest.NewScriptedProvider("fixture",
		wormholetest.Script{Operation: wormholetest.ScriptText, Response: &types.TextResponse{Text: "ready"}},
		wormholetest.Script{Operation: wormholetest.ScriptStream, Chunks: []types.TextChunk{
			{Text: "Hello "}, {Text: "world", FinishReason: &stop},
			{Usage: &types.Usage{PromptTokens: 4, CompletionTokens: 1}},
		}},
	)
	if err != nil {
		panic(err)
	}
	client := wormhole.New(
		wormhole.WithCustomProvider("fixture", wormholetest.ScriptedProviderFactory(provider)),
		wormhole.WithProviderConfig("fixture", types.ProviderConfig{}),
		wormhole.WithDefaultProvider("fixture"),
	)
	defer func() {
		if err := client.Close(); err != nil {
			panic(err)
		}
	}()
	response, err := client.Text().Model("offline").Prompt("ready?").Generate(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(response.Text)
	chunks, result, err := client.Text().Model("offline").Prompt("hello").StreamAndCollect(context.Background())
	if err != nil {
		panic(err)
	}
	for chunk := range chunks {
		fmt.Print(chunk.Content())
	}
	fmt.Println()
	full, err := result()
	if err != nil {
		panic(err)
	}
	fmt.Printf("tokens: %d/%d\n", full.Usage.PromptTokens, full.Usage.CompletionTokens)
	// Output:
	// ready
	// Hello world
	// tokens: 4/1
}

func Example_usageCostEstimate() {
	// Illustrative caller-supplied rates, per 1,000 tokens; no model-price lookup.
	read, write := 0.2, 2.0
	estimate, err := types.EstimateUsageCost(
		&types.ModelCost{InputTokens: 1, OutputTokens: 3, Currency: "USD"},
		types.Usage{PromptTokens: 1000, CompletionTokens: 100, CacheReadTokens: 200, CacheWriteTokens: 300},
		types.UsageCostOptions{PromptAccounting: types.PromptTokensInclusiveCache, CacheReadRate: &read, CacheWriteRate: &write},
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("known: %t\n", estimate.Known)
	fmt.Printf("estimate: %s %.2f\n", estimate.Currency, *estimate.Total)
	// Output:
	// known: true
	// estimate: USD 1.44
}
