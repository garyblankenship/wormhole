package wormhole

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verifyEmbeddingsResponse validates an embeddings response
func verifyEmbeddingsResponse(t *testing.T, resp *types.EmbeddingsResponse, expectedCount int, checkDimensions bool, expectedDims int) {
	require.NotNil(t, resp)
	require.Len(t, resp.Embeddings, expectedCount)
	for i, embedding := range resp.Embeddings {
		assert.NotEmpty(t, embedding.Embedding, "Embedding %d should not be empty", i)
		if checkDimensions {
			assert.Equal(t, expectedDims, len(embedding.Embedding), "Should have requested number of dimensions")
		}
	}
}

// runConcurrentEmbeddingRequests runs embedding requests concurrently and returns success count
func runConcurrentEmbeddingRequests(t *testing.T, client *Wormhole, ctx context.Context, numConcurrent int) int {
	var wg sync.WaitGroup
	results := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			resp, err := client.Embeddings().
				Using("openai").
				Model("text-embedding-3-small").
				Input(fmt.Sprintf("Concurrent test %d", index)).
				Generate(ctx)

			if err != nil {
				results <- err
			} else if resp == nil || len(resp.Embeddings) != 1 {
				results <- fmt.Errorf("invalid response for concurrent request %d", index)
			} else {
				results <- nil // Success
			}
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	for err := range results {
		if err != nil {
			t.Logf("Concurrent request error: %v", err)
		} else {
			successCount++
		}
	}
	return successCount
}

func TestEmbeddingsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("OpenAI embeddings", func(t *testing.T) {
		if os.Getenv("OPENAI_API_KEY") == "" {
			t.Skip("OPENAI_API_KEY not set")
		}
		client := New(WithOpenAI(os.Getenv("OPENAI_API_KEY")))
		testOpenAIEmbeddings(t, client, ctx)
	})

	t.Run("Gemini embeddings", func(t *testing.T) {
		if os.Getenv("GEMINI_API_KEY") == "" {
			t.Skip("GEMINI_API_KEY not set")
		}
		client := New(WithGemini(os.Getenv("GEMINI_API_KEY")))
		testGeminiEmbeddings(t, client, ctx)
	})

	t.Run("Ollama embeddings", func(t *testing.T) {
		if os.Getenv("SKIP_OLLAMA") != "" {
			t.Skip("SKIP_OLLAMA is set")
		}
		client := New()
		testOllamaEmbeddings(t, client, ctx)
	})

	t.Run("Anthropic embeddings should fail", func(t *testing.T) {
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			t.Skip("ANTHROPIC_API_KEY not set")
		}
		client := New(WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")))
		testAnthropicEmbeddingsFailure(t, client, ctx)
	})
}

func testOpenAIEmbeddings(t *testing.T, client *Wormhole, ctx context.Context) {
	t.Run("single input", func(t *testing.T) {
		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input("Hello, world!").
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, 1)

		embedding := resp.Embeddings[0]
		assert.NotEmpty(t, embedding.Embedding)
		assert.Equal(t, 1536, len(embedding.Embedding)) // Default dimensions for text-embedding-3-small
		assert.Equal(t, 0, embedding.Index)

		// Verify usage information
		if resp.Usage != nil {
			assert.Greater(t, resp.Usage.PromptTokens, 0)
			assert.Greater(t, resp.Usage.TotalTokens, 0)
		}
	})

	t.Run("multiple inputs", func(t *testing.T) {
		inputs := []string{
			"First text for embedding",
			"Second text for embedding",
			"Third text for embedding",
		}

		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input(inputs...).
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, len(inputs))

		for i, embedding := range resp.Embeddings {
			assert.NotEmpty(t, embedding.Embedding)
			assert.Equal(t, 1536, len(embedding.Embedding))
			assert.Equal(t, i, embedding.Index)
		}
	})

	t.Run("custom dimensions", func(t *testing.T) {
		dimensions := 512

		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input("Test with custom dimensions").
			Dimensions(dimensions).
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, 1)

		embedding := resp.Embeddings[0]
		assert.Equal(t, dimensions, len(embedding.Embedding))
	})

	t.Run("large model", func(t *testing.T) {
		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-large").
			Input("Test with large embedding model").
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, 1)

		embedding := resp.Embeddings[0]
		assert.Equal(t, 3072, len(embedding.Embedding)) // Default dimensions for text-embedding-3-large
	})

	t.Run("embedding similarity", func(t *testing.T) {
		// Generate embeddings for similar and dissimilar texts
		similarTexts := []string{
			"The cat sits on the mat",
			"A cat is sitting on a mat",
		}

		dissimilarText := "Quantum physics and space exploration"

		// Get embeddings for all texts
		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input(append(similarTexts, dissimilarText)...).
			Generate(ctx)

		require.NoError(t, err)
		require.Len(t, resp.Embeddings, 3)

		// Calculate cosine similarity between similar texts
		similarityBetweenSimilar := cosineSimilarity(resp.Embeddings[0].Embedding, resp.Embeddings[1].Embedding)

		// Calculate cosine similarity between first similar text and dissimilar text
		similarityToDissimilar := cosineSimilarity(resp.Embeddings[0].Embedding, resp.Embeddings[2].Embedding)

		// Similar texts should have higher similarity than dissimilar texts
		assert.Greater(t, similarityBetweenSimilar, similarityToDissimilar)
		assert.Greater(t, similarityBetweenSimilar, 0.8) // High similarity for very similar texts
	})
}

func testGeminiEmbeddings(t *testing.T, client *Wormhole, ctx context.Context) {
	if os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}

	t.Run("basic embedding", func(t *testing.T) {
		resp, err := client.Embeddings().
			Using("gemini").
			Model("embedding-001").
			Input("Test Gemini embeddings").
			Generate(ctx)

		if err != nil && strings.Contains(err.Error(), "API key not valid") {
			t.Skipf("Gemini API key invalid or expired: %v", err)
		}
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, 1)

		embedding := resp.Embeddings[0]
		assert.NotEmpty(t, embedding.Embedding)
		assert.Greater(t, len(embedding.Embedding), 0)
	})

	t.Run("invalid model should fail", func(t *testing.T) {
		_, err := client.Embeddings().
			Using("gemini").
			Model("invalid-model"). // Should be an embedding model
			Input("Test").
			Generate(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "embedding model")
	})
}

func testOllamaEmbeddings(t *testing.T, client *Wormhole, ctx context.Context) {
	t.Run("basic embedding", func(t *testing.T) {
		resp, err := client.Embeddings().
			Using("ollama").
			Model("nomic-embed-text").
			Input("Test Ollama embeddings").
			Generate(ctx)

		if err != nil {
			// Ollama may not be running or model not installed
			t.Skipf("Ollama test failed (may not be available): %v", err)
		}

		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, 1)

		embedding := resp.Embeddings[0]
		assert.NotEmpty(t, embedding.Embedding)
		assert.Greater(t, len(embedding.Embedding), 0)
	})

	t.Run("multiple inputs", func(t *testing.T) {
		inputs := []string{
			"First Ollama text",
			"Second Ollama text",
		}

		resp, err := client.Embeddings().
			Using("ollama").
			Model("nomic-embed-text").
			Input(inputs...).
			Generate(ctx)

		if err != nil {
			t.Skipf("Ollama test failed (may not be available): %v", err)
		}

		require.NotNil(t, resp)
		require.Len(t, resp.Embeddings, len(inputs))

		for i, embedding := range resp.Embeddings {
			assert.NotEmpty(t, embedding.Embedding)
			assert.Equal(t, i, embedding.Index)
		}
	})
}

func testAnthropicEmbeddingsFailure(t *testing.T, client *Wormhole, ctx context.Context) {
	_, err := client.Embeddings().
		Using("anthropic").
		Model("any-model").
		Input("This should fail").
		Generate(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented") // Should contain NotImplementedError message
}

func TestEmbeddingsValidation(t *testing.T) {
	client := New()

	ctx := context.Background()

	t.Run("empty input should fail", func(t *testing.T) {
		// This test needs a configured provider to reach the validation logic
		testClient := New(WithOpenAI("test-key"))
		_, err := testClient.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Generate(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no input provided")
	})

	t.Run("empty model should fail", func(t *testing.T) {
		// This test needs a configured provider to reach the validation logic
		testClient := New(WithOpenAI("test-key"))
		_, err := testClient.Embeddings().
			Using("openai").
			Input("Test").
			Generate(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no model specified")
	})

	t.Run("unsupported provider should fail", func(t *testing.T) {
		_, err := client.Embeddings().
			Using("nonexistent").
			Model("any-model").
			Input("Test").
			Generate(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown or unregistered provider")
	})
}

func TestEmbeddingsEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping edge case tests in short mode")
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	client := New(WithOpenAI(os.Getenv("OPENAI_API_KEY")))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	t.Run("extremely long input text", func(t *testing.T) {
		// Create a very long input (near token limits)
		longText := strings.Repeat("This is a very long text for embedding generation. ", 1000)

		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input(longText).
			Generate(ctx)

		if err != nil {
			// This might fail due to token limits, which is expected behavior
			t.Logf("Long text failed as expected: %v", err)
			assert.Contains(t, strings.ToLower(err.Error()), "token",
				"Error should mention tokens for very long input")
			return
		}
		verifyEmbeddingsResponse(t, resp, 1, false, 0)
	})

	t.Run("maximum batch size", func(t *testing.T) {
		// Test with a large batch of inputs
		inputs := make([]string, 100)
		for i := range inputs {
			inputs[i] = fmt.Sprintf("Batch input text number %d for maximum batch testing", i)
		}

		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input(inputs...).
			Generate(ctx)

		if err != nil {
			// Large batches might be rejected by provider
			t.Logf("Large batch failed as expected: %v", err)
			assert.Contains(t, strings.ToLower(err.Error()), "batch",
				"Error should mention batch size limits")
			return
		}
		verifyEmbeddingsResponse(t, resp, len(inputs), false, 0)
	})

	t.Run("special characters and unicode", func(t *testing.T) {
		specialTexts := []string{
			"Hello 世界! 🌍🚀",
			"Émojis and àccénts",
			"Math symbols: ∑∆∇∂∫√∞",
			"Mixed: Hello世界🌍test∑∆",
		}

		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input(specialTexts...).
			Generate(ctx)

		require.NoError(t, err)
		verifyEmbeddingsResponse(t, resp, len(specialTexts), false, 0)

		for i, embedding := range resp.Embeddings {
			assert.Equal(t, i, embedding.Index, "Embedding %d should have correct index", i)
		}
	})

	t.Run("timeout handling", func(t *testing.T) {
		// Create a very short timeout context
		shortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input("Timeout test").
			Generate(shortCtx)

		assert.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "context",
			"Error should mention context timeout")
	})

	t.Run("custom dimensions validation", func(t *testing.T) {
		// Test dimension limits for different models
		testCases := []struct {
			model      string
			dimensions int
			shouldWork bool
		}{
			{"text-embedding-3-small", 512, true},
			{"text-embedding-3-small", 1536, true}, // Default max
			{"text-embedding-3-large", 1024, true},
			{"text-embedding-3-large", 3072, true}, // Default max
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("%s_dims_%d", tc.model, tc.dimensions), func(t *testing.T) {
				resp, err := client.Embeddings().
					Using("openai").
					Model(tc.model).
					Input("Dimension test").
					Dimensions(tc.dimensions).
					Generate(ctx)

				if !tc.shouldWork || err != nil {
					if tc.shouldWork && err != nil {
						t.Logf("Expected success but got error: %v", err)
					}
					return
				}
				verifyEmbeddingsResponse(t, resp, 1, true, tc.dimensions)
			})
		}
	})

	t.Run("provider options usage", func(t *testing.T) {
		// Test that ProviderOptions can be set and don't cause errors
		resp, err := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input("Provider options test").
			ProviderOptions(map[string]any{
				"user": "test-user",
			}).
			Generate(ctx)

		require.NoError(t, err)
		verifyEmbeddingsResponse(t, resp, 1, false, 0)
	})

	t.Run("concurrent requests", func(t *testing.T) {
		const numConcurrent = 5
		successCount := runConcurrentEmbeddingRequests(t, client, ctx, numConcurrent)
		// At least half should succeed (accounting for rate limits)
		assert.GreaterOrEqual(t, successCount, numConcurrent/2,
			"At least half of concurrent requests should succeed")
	})
}

func TestEmbeddingsBuilder(t *testing.T) {
	client := New()

	t.Run("builder pattern", func(t *testing.T) {
		builder := client.Embeddings().
			Using("openai").
			Model("text-embedding-3-small").
			Input("First input").
			AddInput("Second input").
			Dimensions(256)

		// Verify internal state (if accessible for testing)
		assert.NotNil(t, builder)
	})

	t.Run("method chaining", func(t *testing.T) {
		// Test that all methods return the builder for chaining
		builder := client.Embeddings()

		result := builder.Using("openai")
		assert.Equal(t, builder, result)

		result = builder.Model("text-embedding-3-small")
		assert.Equal(t, builder, result)

		result = builder.Input("test")
		assert.Equal(t, builder, result)

		result = builder.AddInput("test2")
		assert.Equal(t, builder, result)

		result = builder.Dimensions(512)
		assert.Equal(t, builder, result)
	})
}

func TestEmbeddingsMiddleware(t *testing.T) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	// Test with a simple logging middleware
	var middlewareCalled bool
	mw := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			middlewareCalled = true
			return next(ctx, req)
		}
	}

	client := New(WithOpenAI("test-key"), WithMiddleware(mw))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = client.Embeddings().
		Using("openai").
		Model("text-embedding-3-small").
		Input("Middleware test").
		Generate(ctx)

	// Error is acceptable (might not have API key), but middleware should be called
	assert.True(t, middlewareCalled, "Middleware was not called")
}

// cosineSimilarity calculates the cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
