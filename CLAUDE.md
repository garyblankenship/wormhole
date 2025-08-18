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

## File Organization Standards (CRITICAL - NO FUCKING CLUTTER)
### Base Directory Rules
- **ONLY**: README.md, CLAUDE.md, go.mod, go.sum, LICENSE, Makefile, .gitignore
- **NO** test files, compiled binaries, coverage outputs, random .md files
- **NO** examples, demos, or documentation files in base

### Documentation Standards
- **ALL** documentation goes in docs/ folder
- **Consistent naming**: lowercase-with-hyphens.md (never UPPERCASE.md)
- **Descriptive names**: "quick-start.md" not "QUICK_START.md"
- **Professional appearance**: Must look clean when ls docs/ is run
- **Comprehensive docs/README.md**: MUST explain every file's purpose

### Examples Standards  
- **ALL** examples go in examples/ folder
- **NO** quirky Rick & Morty themed names (quantum_chat, portal_stream, etc.)
- **Descriptive names**: "interactive-chat", "streaming-demo", "concurrent-analysis"
- **NO** compiled binaries committed to git
- **NO** random test reports or artifacts

### Git Standards
- **Comprehensive .gitignore**: Must prevent compiled binaries, coverage files, test artifacts
- **NO** committing temporary files, build outputs, or IDE artifacts
- **Clean repository**: Every file must have a justified purpose

### Quality Control
- **Before ANY commit**: Verify base directory contains ONLY essential files
- **Documentation consistency**: All internal links must use new lowercase filenames
- **Example relevance**: Remove outdated, duplicate, or confusing examples
- **Professional presentation**: Repository must look enterprise-ready at first glance

**VIOLATION = IMMEDIATE CLEANUP REQUIRED**

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