package wormhole

import (
	"context"
	"fmt"
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

var (
	benchmarkEmbeddingsResponse *types.EmbeddingsResponse
	benchmarkEmbeddingsError    error
)

func BenchmarkEmbeddingsRequestBuilderGenerateBatchedPreparation(b *testing.B) {
	inputs := make([]string, 1000)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("embedding input %04d", i)
	}
	provider := newBenchmarkBatchedEmbeddingProvider(10, 100)
	client := New(
		WithCustomProvider("benchmark-batch", func(types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		WithDefaultProvider("benchmark-batch"),
	)
	options := map[string]any{
		"routing": map[string]any{
			"priority": []any{"latency", "throughput"},
		},
	}

	for _, batchSize := range []int{10, 100} {
		b.Run(fmt.Sprintf("batch_size_%d", batchSize), func(b *testing.B) {
			b.ReportAllocs()
			var response *types.EmbeddingsResponse
			var err error
			b.ResetTimer()
			for range b.N {
				response, err = client.Embeddings().
					Model("embed-benchmark").
					Input(inputs...).
					Dimensions(384).
					EncodingFormat(types.EmbeddingEncodingFloat).
					ProviderOptions(options).
					GenerateBatched(context.Background(), batchSize)
				if err != nil {
					b.Fatal(err)
				}
			}
			benchmarkEmbeddingsResponse = response
			benchmarkEmbeddingsError = err
		})
	}
}

type benchmarkBatchedEmbeddingProvider struct {
	*types.BaseProvider
	responses map[int]*types.EmbeddingsResponse
}

func newBenchmarkBatchedEmbeddingProvider(batchSizes ...int) *benchmarkBatchedEmbeddingProvider {
	provider := &benchmarkBatchedEmbeddingProvider{
		responses: make(map[int]*types.EmbeddingsResponse, len(batchSizes)),
	}
	for _, batchSize := range batchSizes {
		embeddings := make([]types.Embedding, batchSize)
		for i := range embeddings {
			embeddings[i] = types.Embedding{Index: i}
		}
		provider.responses[batchSize] = &types.EmbeddingsResponse{
			Model:      "embed-benchmark",
			Embeddings: embeddings,
		}
	}
	return provider
}

func (p *benchmarkBatchedEmbeddingProvider) Name() string {
	return "benchmark-batch"
}

func (p *benchmarkBatchedEmbeddingProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityEmbeddings}
}

func (p *benchmarkBatchedEmbeddingProvider) Embeddings(_ context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return p.responses[len(request.Input)], nil
}
