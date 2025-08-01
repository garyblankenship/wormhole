// Package types defines the core types and interfaces used throughout the Prism library.
//
// This package contains:
//   - Provider interface definitions
//   - Request and response types for all modalities
//   - Message types for conversations
//   - Tool/function calling types
//   - Common error types
//
// All providers must implement the Provider interface, with optional support for
// specific capabilities like streaming, structured output, or embeddings.
package types
