# 🧬 Embeddings Examples

This directory contains practical examples of using Wormhole's embeddings API for real-world applications.

## Examples

### [`basic_embeddings.go`](./basic_embeddings.go)
**What it does:** Demonstrates basic embedding generation across multiple providers (OpenAI, Ollama, Gemini) with proper error handling.

**Key features:**
- Single and batch embedding generation
- Provider comparison and error handling
- Usage statistics and token counting
- Dimension customization examples

**Run it:**
```bash
export OPENAI_API_KEY="your-key"
export GEMINI_API_KEY="your-key"  # Optional
go run basic_embeddings.go
```

### [`semantic_search.go`](./semantic_search.go)
**What it does:** Complete semantic search implementation that finds documents by meaning rather than keywords.

**Key features:**
- Document embedding and indexing
- Query embedding and similarity scoring
- Cosine similarity calculation
- Semantic understanding demonstration

**Real-world use cases:**
- Knowledge base search
- Document recommendation systems
- Content discovery platforms

**Run it:**
```bash
export OPENAI_API_KEY="your-key"
go run semantic_search.go
```

### [`batch_processing.go`](./batch_processing.go)
**What it does:** Optimized batch processing patterns for high-volume embedding generation with performance analysis.

**Key features:**
- Large batch processing (100+ texts)
- Concurrent processing with goroutines
- Batch size optimization testing
- Error handling and retry patterns
- Performance benchmarking

**Production insights:**
- Optimal batch sizes for different scenarios
- Rate limiting and concurrency patterns
- Cost optimization strategies

**Run it:**
```bash
export OPENAI_API_KEY="your-key"
go run batch_processing.go
```

### [`openai_compatible_providers.go`](./openai_compatible_providers.go)
**What it does:** Demonstrates how to use Groq, Mistral, and other OpenAI-compatible providers for embeddings without separate implementations.

**Key features:**
- BaseURL approach for any OpenAI-compatible API
- Examples with Groq and Mistral
- Provider fallback patterns
- Popular service endpoints reference

**Why this matters:**
- No need for separate provider packages
- Use ANY OpenAI-compatible service immediately
- Consistent API across all providers
- All Wormhole middleware features work

**Run it:**
```bash
export MISTRAL_API_KEY="your-key"  # Optional
export GROQ_API_KEY="your-key"    # Optional  
go run openai_compatible_providers.go
```

## 🚀 Getting Started

1. **Set up API keys** for the providers you want to test
2. **Start simple** with `basic_embeddings.go` to understand the API
3. **Build semantic search** with `semantic_search.go` for real applications
4. **Scale up** with `batch_processing.go` for production workloads

## 🎯 Production Tips

- **Cache embeddings** - Don't regenerate for identical text
- **Use appropriate dimensions** - Smaller for speed, larger for precision
- **Batch requests** - Much more efficient than individual calls
- **Choose the right provider** - OpenAI for quality, Ollama for local/free
- **Monitor costs** - Embedding generation can add up at scale

## 🧬 Vector Database Integration

For production applications, consider integrating with vector databases:
- **Pinecone** - Managed vector database
- **Weaviate** - Open-source vector database  
- **ChromaDB** - Simple embeddings database
- **Qdrant** - High-performance vector database
- **PostgreSQL + pgvector** - SQL database with vector support

## 📚 Learn More

- [Wormhole Documentation](../../docs/)
- [Embeddings API Reference](../../pkg/wormhole/embeddings_builder.go)
- [Integration Tests](../../pkg/wormhole/embeddings_integration_test.go)