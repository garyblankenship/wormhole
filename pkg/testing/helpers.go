package testing

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ==================== Test Client Factory ====================

// TestClientConfig configures a test client with mock providers.
type TestClientConfig struct {
	Mocks map[string]*MockProvider
}

// NewTestClientConfig creates a new test client configuration.
func NewTestClientConfig() *TestClientConfig {
	return &TestClientConfig{
		Mocks: make(map[string]*MockProvider),
	}
}

// WithMock adds a mock provider to the configuration.
func (c *TestClientConfig) WithMock(name string, mock *MockProvider) *TestClientConfig {
	c.Mocks[name] = mock
	return c
}

// MockProviderFactory returns a ProviderFactory for use with wormhole.WithCustomProvider.
// This is the primary way to inject mock providers into wormhole clients for testing.
//
// Example:
//
//	mock := testing.NewMockProvider("openai").
//	    WithTextResponse(testing.TextResponseWith("Hello!"))
//
//	client := wormhole.New(
//	    wormhole.WithCustomProvider("openai", testing.MockProviderFactory(mock)),
//	    wormhole.WithProviderConfig("openai", types.ProviderConfig{}),
//	    wormhole.WithDefaultProvider("openai"),
//	)
//
//	// Now client.Text().Generate() will return the mock response
func MockProviderFactory(mock *MockProvider) func(types.ProviderConfig) (types.Provider, error) {
	return func(cfg types.ProviderConfig) (types.Provider, error) {
		return mock, nil
	}
}

// CollectStream consumes a streaming text response and returns all text deltas.
// This is useful for testing streaming responses without manual channel iteration.
//
// Example:
//
//	stream, _ := client.Text().Model("gpt-4o").Prompt("Hello").Stream(ctx)
//	texts, err := testing.CollectStream(ctx, stream)
//	require.NoError(t, err)
//	assert.Equal(t, []string{"Hello", " ", "World"}, texts)
func CollectStream(ctx context.Context, stream <-chan types.TextChunk) ([]string, error) {
	var texts []string
	for {
		select {
		case <-ctx.Done():
			return texts, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				return texts, nil
			}
			if chunk.Error != nil {
				return texts, chunk.Error
			}
			if chunk.Text != "" {
				texts = append(texts, chunk.Text)
			}
		}
	}
}

// CollectStreamText consumes a streaming response and returns concatenated text.
// This is simpler than CollectStream when you just need the final text.
//
// Example:
//
//	stream, _ := client.Text().Model("gpt-4o").Prompt("Hello").Stream(ctx)
//	fullText, err := testing.CollectStreamText(ctx, stream)
//	require.NoError(t, err)
//	assert.Equal(t, "Hello World", fullText)
func CollectStreamText(ctx context.Context, stream <-chan types.TextChunk) (string, error) {
	var sb strings.Builder
	for {
		select {
		case <-ctx.Done():
			return sb.String(), ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				return sb.String(), nil
			}
			if chunk.Error != nil {
				return sb.String(), chunk.Error
			}
			sb.WriteString(chunk.Text)
		}
	}
}

// SkipIfNoAPIKey skips a test if the specified provider's API key is not set.
// This is useful for integration tests that require real API credentials.
//
// Example:
//
//	func TestOpenAIIntegration(t *testing.T) {
//	    testing.SkipIfNoAPIKey(t, "openai")
//	    // ... test code that requires OPENAI_API_KEY
//	}
func SkipIfNoAPIKey(t testing.TB, provider string) {
	t.Helper()
	envKey := strings.ToUpper(provider) + "_API_KEY"
	if os.Getenv(envKey) == "" {
		t.Skipf("Skipping integration test: %s not set", envKey)
	}
}

// RequireAPIKey fails a test if the specified provider's API key is not set.
// Use this instead of SkipIfNoAPIKey when the test MUST run with real credentials.
func RequireAPIKey(t testing.TB, provider string) {
	t.Helper()
	envKey := strings.ToUpper(provider) + "_API_KEY"
	if os.Getenv(envKey) == "" {
		t.Fatalf("Test requires %s to be set", envKey)
	}
}

// GetAPIKey returns the API key for a provider from environment variables.
// Returns empty string if not set.
func GetAPIKey(provider string) string {
	return os.Getenv(strings.ToUpper(provider) + "_API_KEY")
}

// TextResponseWith creates a TextResponse with the given text and default values.
// This is a convenience function for setting up mock responses.
//
// Example:
//
//	mock := testing.NewMockProvider("test").
//	    WithTextResponse(testing.TextResponseWith("Hello, World!"))
func TextResponseWith(text string) types.TextResponse {
	return types.TextResponse{
		ID:           "test-response",
		Model:        "test-model",
		Text:         text,
		FinishReason: types.FinishReasonStop,
	}
}

// StreamChunksFrom creates stream chunks from a slice of text strings.
// The final chunk will have FinishReason set to Stop.
//
// Example:
//
//	chunks := testing.StreamChunksFrom("Hello", " ", "World", "!")
//	mock := testing.NewMockProvider("test").WithStreamChunks(chunks)
func StreamChunksFrom(texts ...string) []types.TextChunk {
	chunks := make([]types.TextChunk, len(texts))
	for i, text := range texts {
		chunks[i] = types.TextChunk{
			ID:    "test-chunk",
			Model: "test-model",
			Text:  text,
		}
		if i == len(texts)-1 {
			finishReason := types.FinishReasonStop
			chunks[i].FinishReason = &finishReason
		}
	}
	return chunks
}

// ErrorResponseWith creates a WormholeError with the given details.
// This is useful for testing error handling paths.
//
// Example:
//
//	mock := testing.NewMockProvider("test").
//	    WithError("rate limit exceeded")
//
//	// For typed errors, use the types package directly:
//	err := types.ErrRateLimited.WithDetails("too many requests")
func ErrorResponseWith(message string) error {
	return types.NewWormholeError(types.ErrorCodeUnknown, message, false)
}
