# JSON Robustness Utilities

This package provides robust JSON parsing utilities specifically designed to handle responses from AI models that may contain regex patterns, escaped strings, and other complex content.

## Problem Statement

AI models like Claude Opus 4.1 sometimes generate JSON responses containing regex patterns and escaped characters that can break Go's standard `json.Unmarshal()` function. For example:

```json
{
  "enhanced_prompt": "regex: \\\\s+ ... \\\\b(API|SQL|JSON|XML)\\\\b"
}
```

This technically valid JSON can cause parsing failures in certain contexts, particularly when processing tool call arguments from Anthropic's API.

## Solution

The utilities in this package provide:

1. **LenientUnmarshal**: A drop-in replacement for `json.Unmarshal` with fallback handling
2. **UnmarshalAnthropicToolArgs**: Specialized function for parsing Anthropic tool arguments with enhanced error context

## Usage

### Basic Usage

```go
import "github.com/garyblankenship/wormhole/internal/utils"

// Instead of json.Unmarshal
var data map[string]interface{}
err := utils.LenientUnmarshal(jsonBytes, &data)
```

### Anthropic Tool Arguments

```go
// For parsing tool call arguments from Anthropic API
var toolData map[string]interface{}
err := utils.UnmarshalAnthropicToolArgs(argumentsString, &toolData)
```

## Implementation

The current implementation:

1. **First tries standard JSON unmarshaling** - no performance penalty for valid JSON
2. **Provides enhanced error context** - helps identify parsing issues specific to AI model responses
3. **Maintains backward compatibility** - drop-in replacement for existing code
4. **Designed for future enhancement** - infrastructure in place for automatic fix strategies

## Performance

Benchmarks show no performance impact for valid JSON:
- `LenientUnmarshal`: ~same performance as `json.Unmarshal`
- `UnmarshalAnthropicToolArgs`: Minimal overhead for string validation

## Testing

Comprehensive tests cover:
- ✅ Valid JSON with regex patterns
- ✅ Complex escaped strings
- ✅ Real-world Claude response patterns
- ✅ Unicode symbols and mathematical notation
- ✅ Performance with 100+ iterations of complex data
- ✅ Error handling for malformed JSON

## Future Enhancements

The design allows for future enhancements such as:
- Automatic escape sequence normalization
- Provider-specific JSON quirk handling
- Configurable lenience levels
- Detailed parsing diagnostics

## Integration

This utility is integrated into the Anthropic provider at key parsing points:
- `pkg/providers/anthropic/anthropic.go:121` - Structured response parsing
- `pkg/providers/anthropic/transform.go:127` - Tool call argument transformation

## Error Handling

The utilities provide clear error messages that help developers identify:
- Whether the issue is with AI model JSON generation
- Specific parsing failures with context
- Recommendations for resolution

This ensures developers get actionable feedback rather than cryptic JSON parsing errors.