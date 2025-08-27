package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Document represents a searchable document with its embedding
type Document struct {
	ID        int
	Title     string
	Content   string
	Embedding []float64
}

// SearchResult represents a search result with similarity score
type SearchResult struct {
	Document   Document
	Similarity float64
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

func main() {
	// Create a new Wormhole client
	client := wormhole.New()

	// Ensure we have an OpenAI API key for this example
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	fmt.Println("=== Semantic Search Example ===")

	// Sample documents to search through
	documents := []Document{
		{ID: 1, Title: "Go Programming", Content: "Go is a programming language developed by Google. It's known for its simplicity and performance."},
		{ID: 2, Title: "Python Basics", Content: "Python is a high-level programming language known for its readability and versatility."},
		{ID: 3, Title: "Machine Learning", Content: "Machine learning is a subset of artificial intelligence that enables computers to learn without explicit programming."},
		{ID: 4, Title: "Web Development", Content: "Web development involves creating websites and web applications using various technologies."},
		{ID: 5, Title: "Database Design", Content: "Database design is the process of creating a detailed data model for a database system."},
		{ID: 6, Title: "Cloud Computing", Content: "Cloud computing provides on-demand access to computing resources over the internet."},
		{ID: 7, Title: "API Development", Content: "API development involves creating interfaces that allow different software applications to communicate."},
		{ID: 8, Title: "DevOps Practices", Content: "DevOps combines development and operations to improve collaboration and productivity."},
	}

	// Step 1: Generate embeddings for all documents
	fmt.Println("Generating embeddings for documents...")

	// Collect all content for batch processing
	var contents []string
	for _, doc := range documents {
		contents = append(contents, doc.Content)
	}

	// Generate embeddings in batch for efficiency
	response, err := client.Embeddings().
		Provider("openai").
		Model("text-embedding-3-small").
		Input(contents...).
		Dimensions(384). // Smaller dimensions for faster processing
		Generate(context.Background())

	if err != nil {
		log.Fatalf("Failed to generate document embeddings: %v", err)
	}

	// Assign embeddings to documents
	for i, embedding := range response.Embeddings {
		if i < len(documents) {
			documents[i].Embedding = embedding.Embedding
		}
	}

	fmt.Printf("Generated embeddings for %d documents\n", len(documents))

	// Step 2: Process search queries
	searchQueries := []string{
		"programming languages",
		"artificial intelligence",
		"building websites",
		"storing data",
		"software deployment",
	}

	for _, query := range searchQueries {
		fmt.Printf("\n--- Searching for: '%s' ---\n", query)

		// Generate embedding for the search query
		queryResponse, err := client.Embeddings().
			Provider("openai").
			Model("text-embedding-3-small").
			Input(query).
			Dimensions(384). // Match document embedding dimensions
			Generate(context.Background())

		if err != nil {
			log.Printf("Failed to generate query embedding: %v", err)
			continue
		}

		queryEmbedding := queryResponse.Embeddings[0].Embedding

		// Calculate similarity scores for all documents
		// Pre-allocate slice to avoid repeated allocations
		results := make([]SearchResult, 0, len(documents))
		for _, doc := range documents {
			similarity := cosineSimilarity(queryEmbedding, doc.Embedding)
			results = append(results, SearchResult{
				Document:   doc,
				Similarity: similarity,
			})
		}

		// Sort by similarity score (descending)
		sort.Slice(results, func(i, j int) bool {
			return results[i].Similarity > results[j].Similarity
		})

		// Display top 3 results
		fmt.Println("Top 3 most relevant documents:")
		for i, result := range results[:3] {
			fmt.Printf("%d. %s (Score: %.3f)\n",
				i+1,
				result.Document.Title,
				result.Similarity)
			fmt.Printf("   %s\n", result.Document.Content)
		}
	}

	// Step 3: Demonstrate semantic understanding
	fmt.Println("\n=== Semantic Understanding Demo ===")

	// These queries should find relevant results even without exact keyword matches
	semanticQueries := []string{
		"coding in Go", // Should match Go Programming
		"AI and ML",    // Should match Machine Learning
		"REST APIs",    // Should match API Development
	}

	for _, query := range semanticQueries {
		fmt.Printf("\nSemantic search: '%s'\n", query)

		queryResponse, err := client.Embeddings().
			Provider("openai").
			Model("text-embedding-3-small").
			Input(query).
			Dimensions(384).
			Generate(context.Background())

		if err != nil {
			continue
		}

		queryEmbedding := queryResponse.Embeddings[0].Embedding

		// Find the most similar document
		var bestMatch SearchResult
		for _, doc := range documents {
			similarity := cosineSimilarity(queryEmbedding, doc.Embedding)
			if similarity > bestMatch.Similarity {
				bestMatch = SearchResult{Document: doc, Similarity: similarity}
			}
		}

		fmt.Printf("Best match: %s (Score: %.3f)\n",
			bestMatch.Document.Title,
			bestMatch.Similarity)
	}

	// Display usage statistics
	if response.Usage != nil {
		fmt.Printf("\nEmbedding Usage Statistics:\n")
		fmt.Printf("- Total requests: %d (documents) + %d (queries)\n",
			len(documents), len(searchQueries)+len(semanticQueries))
		fmt.Printf("- Approximate tokens: %d\n", response.Usage.TotalTokens)
	}

	fmt.Println("\n=== Tips for Production Use ===")
	fmt.Println("1. Cache document embeddings to avoid regenerating them")
	fmt.Println("2. Use vector databases (Pinecone, Weaviate, ChromaDB) for large-scale search")
	fmt.Println("3. Consider different embedding models for different use cases")
	fmt.Println("4. Implement embedding refresh strategies for dynamic content")
	fmt.Println("5. Use batch processing for generating multiple embeddings efficiently")
}
