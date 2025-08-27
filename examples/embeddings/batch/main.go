package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// EmbeddingJob represents a batch embedding job
type EmbeddingJob struct {
	ID    int
	Texts []string
}

// EmbeddingResult represents the result of a batch embedding job
type EmbeddingResult struct {
	JobID      int
	Embeddings [][]float64
	Duration   time.Duration
	Error      error
}

func main() {
	// Create a new Wormhole client
	client := wormhole.New()

	// Ensure we have an OpenAI API key for this example
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	fmt.Println("=== Batch Processing Example ===")

	// Example 1: Single large batch
	fmt.Println("\n1. Processing Large Batch (100 texts)")

	// Generate 100 sample texts
	var largeBatch []string
	for i := 0; i < 100; i++ {
		largeBatch = append(largeBatch, fmt.Sprintf("Sample text number %d for embedding generation", i))
	}

	start := time.Now()
	response, err := client.Embeddings().
		Provider("openai").
		Model("text-embedding-3-small").
		Input(largeBatch...).
		Dimensions(256). // Smaller dimensions for faster processing
		Generate(context.Background())

	duration := time.Since(start)

	if err != nil {
		log.Printf("Large batch failed: %v", err)
	} else {
		fmt.Printf("✓ Generated %d embeddings in %v (%.2f embeddings/sec)\n",
			len(response.Embeddings),
			duration,
			float64(len(response.Embeddings))/duration.Seconds())

		if response.Usage != nil {
			fmt.Printf("  Tokens used: %d\n", response.Usage.TotalTokens)
		}
	}

	// Example 2: Concurrent batch processing
	fmt.Println("\n2. Concurrent Batch Processing (5 batches of 20 texts each)")

	// Create 5 batches of 20 texts each
	jobs := make([]EmbeddingJob, 5)
	for i := 0; i < 5; i++ {
		job := EmbeddingJob{ID: i + 1, Texts: make([]string, 20)}
		for j := 0; j < 20; j++ {
			job.Texts[j] = fmt.Sprintf("Batch %d, text %d: concurrent processing example", i+1, j+1)
		}
		jobs[i] = job
	}

	// Process batches concurrently
	start = time.Now()
	results := processBatchesConcurrently(client, jobs)
	totalDuration := time.Since(start)

	fmt.Printf("Concurrent processing completed in %v\n", totalDuration)

	var totalEmbeddings int
	successCount := 0

	for _, result := range results {
		if result.Error != nil {
			fmt.Printf("✗ Job %d failed: %v\n", result.JobID, result.Error)
		} else {
			fmt.Printf("✓ Job %d: %d embeddings in %v\n",
				result.JobID,
				len(result.Embeddings),
				result.Duration)
			totalEmbeddings += len(result.Embeddings)
			successCount++
		}
	}

	if successCount > 0 {
		fmt.Printf("Total: %d embeddings from %d successful batches\n",
			totalEmbeddings,
			successCount)
		fmt.Printf("Overall rate: %.2f embeddings/sec\n",
			float64(totalEmbeddings)/totalDuration.Seconds())
	}

	// Example 3: Comparing different batch sizes
	fmt.Println("\n3. Batch Size Optimization")

	batchSizes := []int{5, 10, 25, 50}
	sampleTexts := make([]string, 50)
	for i := range sampleTexts {
		sampleTexts[i] = fmt.Sprintf("Optimization test text number %d", i)
	}

	for _, batchSize := range batchSizes {
		start := time.Now()

		// Process in batches of the specified size
		var allEmbeddings [][]float64
		for i := 0; i < len(sampleTexts); i += batchSize {
			end := i + batchSize
			if end > len(sampleTexts) {
				end = len(sampleTexts)
			}

			batch := sampleTexts[i:end]
			response, err := client.Embeddings().
				Provider("openai").
				Model("text-embedding-3-small").
				Input(batch...).
				Dimensions(256).
				Generate(context.Background())

			if err != nil {
				log.Printf("Batch size %d failed: %v", batchSize, err)
				break
			}

			for _, embedding := range response.Embeddings {
				allEmbeddings = append(allEmbeddings, embedding.Embedding)
			}
		}

		duration := time.Since(start)
		if len(allEmbeddings) > 0 {
			fmt.Printf("Batch size %2d: %d embeddings in %v (%.2f/sec)\n",
				batchSize,
				len(allEmbeddings),
				duration,
				float64(len(allEmbeddings))/duration.Seconds())
		}

		// Add delay between tests to respect rate limits
		time.Sleep(1 * time.Second)
	}

	// Example 4: Error handling and retry logic
	fmt.Println("\n4. Error Handling and Retry Example")

	// Simulate different error conditions
	testCases := []struct {
		name     string
		provider string
		model    string
		input    []string
	}{
		{
			name:     "Valid request",
			provider: "openai",
			model:    "text-embedding-3-small",
			input:    []string{"Valid text for embedding"},
		},
		{
			name:     "Invalid model",
			provider: "openai",
			model:    "nonexistent-model",
			input:    []string{"Test text"},
		},
		{
			name:     "Empty input",
			provider: "openai",
			model:    "text-embedding-3-small",
			input:    []string{},
		},
		{
			name:     "Unsupported provider",
			provider: "anthropic",
			model:    "any-model",
			input:    []string{"Test text"},
		},
	}

	for _, tc := range testCases {
		fmt.Printf("Testing: %s... ", tc.name)

		_, err := client.Embeddings().
			Provider(tc.provider).
			Model(tc.model).
			Input(tc.input...).
			Generate(context.Background())

		if err != nil {
			fmt.Printf("✗ Error (expected): %v\n", err)
		} else {
			fmt.Printf("✓ Success\n")
		}
	}

	fmt.Println("\n=== Batch Processing Best Practices ===")
	fmt.Println("1. Optimal batch sizes are typically 10-50 texts per request")
	fmt.Println("2. Use concurrent processing for multiple independent batches")
	fmt.Println("3. Implement exponential backoff for rate limit errors")
	fmt.Println("4. Monitor token usage to optimize costs")
	fmt.Println("5. Cache embeddings to avoid reprocessing identical texts")
	fmt.Println("6. Use smaller dimensions when high precision isn't critical")
}

// processBatchesConcurrently processes multiple embedding jobs concurrently
func processBatchesConcurrently(client *wormhole.Wormhole, jobs []EmbeddingJob) []EmbeddingResult {
	var wg sync.WaitGroup
	results := make([]EmbeddingResult, len(jobs))

	// Process each job in a separate goroutine
	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j EmbeddingJob) {
			defer wg.Done()

			start := time.Now()
			response, err := client.Embeddings().
				Provider("openai").
				Model("text-embedding-3-small").
				Input(j.Texts...).
				Dimensions(256).
				Generate(context.Background())

			duration := time.Since(start)

			result := EmbeddingResult{
				JobID:    j.ID,
				Duration: duration,
				Error:    err,
			}

			if err == nil {
				for _, embedding := range response.Embeddings {
					result.Embeddings = append(result.Embeddings, embedding.Embedding)
				}
			}

			results[idx] = result
		}(i, job)
	}

	wg.Wait()
	return results
}
