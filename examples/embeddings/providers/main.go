package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func main() {
	response, err := generateMistralEmbedding(context.Background(), os.Getenv("MISTRAL_API_KEY"), "")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Generated Mistral embedding: %d dimensions\n", len(response.Embeddings[0].Embedding))
}

func generateMistralEmbedding(ctx context.Context, apiKey, baseURL string) (response *types.EmbeddingsResponse, err error) {
	if apiKey == "" {
		return nil, errors.New("MISTRAL_API_KEY environment variable is required")
	}

	config := types.ProviderConfig{APIKey: apiKey}
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	client := wormhole.New(
		wormhole.WithMistral(config),
		wormhole.WithDefaultProvider("mistral"),
		wormhole.WithDiscovery(false),
	)
	defer func() {
		err = errors.Join(err, client.Close())
	}()

	return client.Embeddings().
		Model("mistral-embed").
		Input("Mistral embeddings through its OpenAI-compatible API").
		Generate(ctx)
}
