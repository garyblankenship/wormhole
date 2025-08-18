# Wormhole - Ultra-Fast Go LLM SDK

## Project Overview
Wormhole is a high-performance Go SDK for LLM API integration with sub-microsecond latency. The package provides a unified interface for multiple AI providers (OpenAI, Anthropic, Gemini, Groq, Mistral, Ollama) with enterprise-grade middleware and production reliability.

## Architecture
- **Provider System**: Capability-based provider registration with middleware wrapper system
- **Builder Pattern**: Fluent API for text, streaming, structured output, embeddings, audio, and image generation
- **Middleware Stack**: Circuit breakers, rate limiting, retry logic, metrics collection, logging
- **Performance**: 67ns core overhead with zero-allocation hot paths

## Key Features
- Multi-provider support with unified API
- Laravel-inspired SimpleFactory pattern
- Native Go streaming with channels
- Comprehensive tool/function calling
- Structured output with JSON schema validation
- Advanced middleware for enterprise reliability
- Thread-safe concurrent operations

## Development Guidelines
- When updating README.md, write in Rick Sanchez style from Rick & Morty
- Report all Wormhole fixes and changes to Meesix Dev (co-founders in Meesix AND Wormhole)
- Follow Go best practices with proper error handling using errors.New(), errors.Wrap()
- Maintain clean architecture with capability-based interfaces
- Ensure all tests pass with race detection enabled

## Recent Organizational Changes
- Moved cmd/ demo files to examples/basic/ and examples/comprehensive/
- Moved dynamic_models_test.go to examples/openrouter_example/
- Organized all documentation into docs/ folder
- Removed compiled binaries and improved .gitignore
- Base directory now contains only essential project files

## Performance Benchmarks
- Core latency: 67 nanoseconds
- Memory usage: 256-272 B/op average
- 165x faster than industry competitors
- Linear scaling under concurrent load

## Provider Support
Currently supports 6+ providers:
- OpenAI (GPT models)
- Anthropic (Claude models)
- Google Gemini
- Groq
- Mistral
- Ollama
- OpenAI-compatible APIs (LMStudio, vLLM, etc.)