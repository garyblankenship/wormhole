package testing

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// MockProvider is a mock implementation of the Provider interface for testing
type MockProvider struct {
	name           string
	textResponses  []types.TextResponse
	textIndex      int
	streamChunks   []types.TextChunk
	structuredData any
	embeddings     []types.Embedding
	shouldError    bool
	errorMessage   string
}

// NewMockProvider creates a new mock provider
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		name: name,
	}
}

// WithTextResponse adds a text response to return
func (m *MockProvider) WithTextResponse(response types.TextResponse) *MockProvider {
	m.textResponses = append(m.textResponses, response)
	return m
}

// WithStreamChunks sets the stream chunks to return
func (m *MockProvider) WithStreamChunks(chunks []types.TextChunk) *MockProvider {
	m.streamChunks = chunks
	return m
}

// WithStructuredData sets the structured data to return
func (m *MockProvider) WithStructuredData(data any) *MockProvider {
	m.structuredData = data
	return m
}

// WithEmbeddings sets the embeddings to return
func (m *MockProvider) WithEmbeddings(embeddings []types.Embedding) *MockProvider {
	m.embeddings = embeddings
	return m
}

// WithError makes the provider return an error
func (m *MockProvider) WithError(message string) *MockProvider {
	m.shouldError = true
	m.errorMessage = message
	return m
}

// Name returns the provider name
func (m *MockProvider) Name() string {
	return m.name
}

// Text returns a mocked text response
func (m *MockProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}

	if len(m.textResponses) == 0 {
		return &types.TextResponse{
			ID:           "mock-" + fmt.Sprint(time.Now().Unix()),
			Model:        request.Model,
			Text:         "Mock response",
			FinishReason: types.FinishReasonStop,
			Created:      time.Now(),
		}, nil
	}

	response := m.textResponses[m.textIndex]
	m.textIndex = (m.textIndex + 1) % len(m.textResponses)
	return &response, nil
}

// Stream returns a mocked streaming response
func (m *MockProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}

	chunks := make(chan types.TextChunk, len(m.streamChunks))

	go func() {
		defer close(chunks)

		if len(m.streamChunks) == 0 {
			// Default stream chunks
			chunks <- types.TextChunk{
				ID:    "mock-stream",
				Model: request.Model,
				Text:  "Mock ",
			}
			chunks <- types.TextChunk{
				Text: "streaming ",
			}
			finishReason := types.FinishReasonStop
			chunks <- types.TextChunk{
				Text:         "response",
				FinishReason: &finishReason,
			}
			return
		}

		for _, chunk := range m.streamChunks {
			select {
			case <-ctx.Done():
				return
			case chunks <- chunk:
			}
		}
	}()

	return chunks, nil
}

// Structured returns a mocked structured response
func (m *MockProvider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}

	data := m.structuredData
	if data == nil {
		data = map[string]any{
			"mock": "structured response",
		}
	}

	return &types.StructuredResponse{
		ID:      "mock-structured",
		Model:   request.Model,
		Data:    data,
		Created: time.Now(),
	}, nil
}

// Embeddings returns mocked embeddings
func (m *MockProvider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}

	embeddings := m.embeddings
	if len(embeddings) == 0 {
		// Create mock embeddings for each input
		for i := range request.Input {
			embeddings = append(embeddings, types.Embedding{
				Index:     i,
				Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			})
		}
	}

	return &types.EmbeddingsResponse{
		ID:         "mock-embeddings",
		Model:      request.Model,
		Embeddings: embeddings,
		Created:    time.Now(),
	}, nil
}

// Audio returns a mocked audio response
func (m *MockProvider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}

	if request.Type == types.AudioRequestTypeTTS {
		return &types.AudioResponse{
			ID:     "mock-audio",
			Model:  request.Model,
			Audio:  []byte("mock audio data"),
			Format: request.ResponseFormat,
		}, nil
	}

	// STT response
	return &types.AudioResponse{
		ID:    "mock-stt",
		Model: request.Model,
		Text:  "Mock transcribed text",
	}, nil
}

// Images returns a mocked images response
func (m *MockProvider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMessage)
	}

	return &types.ImagesResponse{
		ID:    "mock-images",
		Model: request.Model,
		Images: []types.GeneratedImage{
			{URL: "https://example.com/mock-image.png"},
		},
		Created: time.Now(),
	}, nil
}

// StreamTextResponse creates a stream of text chunks from a text response
func StreamTextResponse(response *types.TextResponse) <-chan types.TextChunk {
	ch := make(chan types.TextChunk)

	go func() {
		defer close(ch)

		// Split text into word chunks
		words := []string{"Mock", "streaming", "response"}
		for _, word := range words {
			ch <- types.TextChunk{
				ID:    response.ID,
				Model: response.Model,
				Text:  word + " ",
			}
		}

		// Send finish reason
		ch <- types.TextChunk{
			FinishReason: &response.FinishReason,
		}
	}()

	return ch
}
