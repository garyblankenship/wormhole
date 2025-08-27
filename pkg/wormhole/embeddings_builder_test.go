package wormhole

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingsRequestBuilder(t *testing.T) {
	client := New()

	t.Run("Builder Method Chaining", func(t *testing.T) {
		builder := client.Embeddings()
		assert.NotNil(t, builder)

		// Test method chaining returns the same builder instance
		result := builder.Model("test-model")
		assert.Equal(t, builder, result, "Model() should return the same builder instance")

		result = builder.Input("test input")
		assert.Equal(t, builder, result, "Input() should return the same builder instance")

		result = builder.AddInput("another input")
		assert.Equal(t, builder, result, "AddInput() should return the same builder instance")

		result = builder.Dimensions(512)
		assert.Equal(t, builder, result, "Dimensions() should return the same builder instance")

		result = builder.Using("openai")
		assert.Equal(t, builder, result, "Using() should return the same builder instance")

		result = builder.Provider("openai")
		assert.Equal(t, builder, result, "Provider() should return the same builder instance")

		result = builder.BaseURL("https://api.openai.com/v1")
		assert.Equal(t, builder, result, "BaseURL() should return the same builder instance")

		result = builder.ProviderOptions(map[string]any{"key": "value"})
		assert.Equal(t, builder, result, "ProviderOptions() should return the same builder instance")
	})

	t.Run("Model Method", func(t *testing.T) {
		t.Run("valid model", func(t *testing.T) {
			builder := client.Embeddings()
			result := builder.Model("text-embedding-3-small")
			assert.Equal(t, "text-embedding-3-small", result.request.Model)
		})

		t.Run("empty model panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.Model("")
			}, "Empty model should panic")
		})
	})

	t.Run("Input Method", func(t *testing.T) {
		t.Run("single input", func(t *testing.T) {
			builder := client.Embeddings()
			builder.Input("test input")
			assert.Equal(t, []string{"test input"}, builder.request.Input)
		})

		t.Run("multiple inputs", func(t *testing.T) {
			builder := client.Embeddings()
			builder.Input("input1", "input2", "input3")
			assert.Equal(t, []string{"input1", "input2", "input3"}, builder.request.Input)
		})

		t.Run("empty input slice panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.Input()
			}, "Empty input slice should panic")
		})

		t.Run("empty string in input panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.Input("valid", "", "also valid")
			}, "Empty string in input should panic")
		})
	})

	t.Run("AddInput Method", func(t *testing.T) {
		t.Run("add single input", func(t *testing.T) {
			builder := client.Embeddings()
			builder.Input("first")
			builder.AddInput("second")
			assert.Equal(t, []string{"first", "second"}, builder.request.Input)
		})

		t.Run("add multiple inputs", func(t *testing.T) {
			builder := client.Embeddings()
			builder.Input("first")
			builder.AddInput("second")
			builder.AddInput("third")
			assert.Equal(t, []string{"first", "second", "third"}, builder.request.Input)
		})

		t.Run("empty input panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.AddInput("")
			}, "Empty string to AddInput should panic")
		})
	})

	t.Run("Dimensions Method", func(t *testing.T) {
		t.Run("valid dimensions", func(t *testing.T) {
			builder := client.Embeddings()
			builder.Dimensions(512)
			require.NotNil(t, builder.request.Dimensions)
			assert.Equal(t, 512, *builder.request.Dimensions)
		})

		t.Run("zero dimensions panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.Dimensions(0)
			}, "Zero dimensions should panic")
		})

		t.Run("negative dimensions panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.Dimensions(-1)
			}, "Negative dimensions should panic")
		})

		t.Run("too large dimensions panics", func(t *testing.T) {
			builder := client.Embeddings()
			assert.Panics(t, func() {
				builder.Dimensions(20000)
			}, "Too large dimensions should panic")
		})
	})

	t.Run("ProviderOptions Method", func(t *testing.T) {
		t.Run("set options", func(t *testing.T) {
			builder := client.Embeddings()
			options := map[string]any{
				"taskType": "SEMANTIC_SIMILARITY",
				"title":    "Test embeddings",
			}
			builder.ProviderOptions(options)
			assert.Equal(t, options, builder.request.ProviderOptions)
		})

		t.Run("nil options", func(t *testing.T) {
			builder := client.Embeddings()
			builder.ProviderOptions(nil)
			assert.Nil(t, builder.request.ProviderOptions)
		})

		t.Run("empty options", func(t *testing.T) {
			builder := client.Embeddings()
			options := map[string]any{}
			builder.ProviderOptions(options)
			assert.Equal(t, options, builder.request.ProviderOptions)
		})
	})

	t.Run("Memory Pool Usage", func(t *testing.T) {
		t.Run("new builder gets clean request", func(t *testing.T) {
			builder1 := client.Embeddings()
			builder2 := client.Embeddings()

			// Both should start with clean state regardless of whether instances are reused
			assert.Empty(t, builder1.request.Input, "Builder1 should start with empty input")
			assert.Empty(t, builder2.request.Input, "Builder2 should start with empty input")
			assert.Empty(t, builder1.request.Model, "Builder1 should start with empty model")
			assert.Empty(t, builder2.request.Model, "Builder2 should start with empty model")
			assert.Nil(t, builder1.request.Dimensions, "Builder1 should start with nil dimensions")
			assert.Nil(t, builder2.request.Dimensions, "Builder2 should start with nil dimensions")
			assert.Nil(t, builder1.request.ProviderOptions, "Builder1 should start with nil options")
			assert.Nil(t, builder2.request.ProviderOptions, "Builder2 should start with nil options")
		})

		t.Run("request reset after pool retrieval", func(t *testing.T) {
			// First builder sets some values
			builder1 := client.Embeddings().Model("test-model").Input("test")
			assert.Equal(t, "test-model", builder1.request.Model)
			assert.Equal(t, []string{"test"}, builder1.request.Input)

			// Force pool return by triggering GC behavior simulation
			// (In real usage, this happens in Generate() method)
			putEmbeddingsRequest(builder1.request)

			// New builder should get a clean request (possibly recycled)
			builder2 := client.Embeddings()
			assert.Empty(t, builder2.request.Model, "New builder should have empty model")
			assert.Empty(t, builder2.request.Input, "New builder should have empty input")
		})
	})

	t.Run("Input Slice Capacity Preservation", func(t *testing.T) {
		// This tests that the memory pool properly reuses slice capacity
		builder := client.Embeddings()

		// Set inputs to fill the pre-allocated capacity
		builder.Input("input1", "input2")
		originalCap := cap(builder.request.Input)

		// The capacity should be at least as large as the pre-allocated amount
		assert.GreaterOrEqual(t, originalCap, 2, "Input slice should have pre-allocated capacity")

		// Adding more inputs should potentially grow the slice
		builder.AddInput("input3")
		builder.AddInput("input4")

		// Length should be correct
		assert.Len(t, builder.request.Input, 4)
		assert.Equal(t, []string{"input1", "input2", "input3", "input4"}, builder.request.Input)
	})
}

func TestEmbeddingsRequestBuilderConcurrency(t *testing.T) {
	t.Run("concurrent builder creation is safe", func(t *testing.T) {
		client := New()
		const numGoroutines = 10

		builders := make([]*EmbeddingsRequestBuilder, numGoroutines)
		done := make(chan bool, numGoroutines)

		// Create builders concurrently with different values
		for i := 0; i < numGoroutines; i++ {
			go func(index int) {
				defer func() { done <- true }()
				builders[index] = client.Embeddings().
					Model(fmt.Sprintf("test-model-%d", index)).
					Input(fmt.Sprintf("test input %d", index))
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// Verify each builder has the correct state
		for i := 0; i < numGoroutines; i++ {
			assert.NotNil(t, builders[i], "Builder %d should not be nil", i)
			assert.Equal(t, fmt.Sprintf("test-model-%d", i), builders[i].request.Model,
				"Builder %d should have correct model", i)
			assert.Equal(t, []string{fmt.Sprintf("test input %d", i)}, builders[i].request.Input,
				"Builder %d should have correct input", i)
		}

		// Verify no state corruption between builders
		for i := 0; i < numGoroutines; i++ {
			for j := i + 1; j < numGoroutines; j++ {
				// Models should be different
				assert.NotEqual(t, builders[i].request.Model, builders[j].request.Model,
					"Builders %d and %d should have different models", i, j)
				// Inputs should be different
				assert.NotEqual(t, builders[i].request.Input, builders[j].request.Input,
					"Builders %d and %d should have different inputs", i, j)
			}
		}
	})
}

// Benchmarks for performance validation
func BenchmarkEmbeddingsBuilder(b *testing.B) {
	client := New()

	b.Run("builder_creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = client.Embeddings()
		}
	})

	b.Run("method_chaining", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = client.Embeddings().
				Model("text-embedding-3-small").
				Input("benchmark test").
				Dimensions(512).
				ProviderOptions(map[string]any{"test": "value"})
		}
	})

	b.Run("memory_pool_usage", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			builder := client.Embeddings()
			// Simulate the cleanup that happens in Generate()
			putEmbeddingsRequest(builder.request)
		}
	})
}
