# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-08-01

### Added
- Initial release of Prism Go SDK
- Support for 6 major LLM providers:
  - OpenAI (full feature support)
  - Anthropic (text, streaming, structured output, tools)
  - Gemini (text, streaming, structured output, embeddings, tools)
  - Groq (text, streaming, structured output, tools)
  - Mistral (text, streaming, structured output, embeddings, tools)
  - Ollama (text, streaming, structured output, embeddings)
- Complete feature set:
  - Text generation with streaming support
  - Structured output with JSON schema validation
  - Function/tool calling capabilities
  - Embeddings generation
  - Audio processing (TTS/STT for supported providers)
  - Image generation (OpenAI only)
- Production-ready features:
  - Automatic retry logic with exponential backoff
  - Multipart form data support for audio uploads
  - Comprehensive error handling
  - Type-safe interfaces throughout
  - Context support for timeouts and cancellation
- Builder pattern API for intuitive request construction
- Multimodal support (text, images, documents, audio)
- Mock provider for testing
- Comprehensive test suite with 100% interface coverage
- GitHub Actions CI/CD pipeline
- Complete documentation and examples

### Technical Features
- Unified interface across all providers
- Provider-specific optimizations and transformations
- Streaming support via Go channels
- Schema validation for structured outputs
- Request/response logging capabilities
- Configurable timeouts and retry policies
- Custom headers and provider options support
- Memory-efficient streaming implementations
- Concurrent-safe design

### Provider-Specific Features
- **OpenAI**: Full API compatibility including GPT-4, DALL-E, Whisper
- **Anthropic**: Claude models with tool calling and streaming
- **Gemini**: Multimodal capabilities with function calling
- **Groq**: High-speed inference with OpenAI-compatible API
- **Mistral**: European AI with OCR capabilities
- **Ollama**: Local deployment with model management

### Developer Experience
- Intuitive builder pattern API
- Comprehensive error messages
- Type-safe request/response handling
- Rich examples and documentation
- Testing utilities and mocks
- IDE-friendly with full type information

## [Unreleased]

### Planned
- Additional provider integrations
- Enhanced multimodal support
- Performance optimizations
- Extended testing utilities
- Metrics and observability features