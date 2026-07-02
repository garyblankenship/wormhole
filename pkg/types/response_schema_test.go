package types

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseHelpers(t *testing.T) {
	t.Parallel()
	stop := FinishReasonStop
	length := FinishReasonLength
	text := &TextResponse{Text: "hello", FinishReason: stop}
	assert.Equal(t, "hello", text.Content())
	assert.False(t, text.HasToolCalls())
	assert.True(t, text.IsComplete())
	assert.False(t, text.WasTruncated())

	toolResp := &TextResponse{FinishReason: length, ToolCalls: []ToolCall{{ID: "call-1"}}}
	assert.True(t, toolResp.HasToolCalls())
	assert.False(t, toolResp.IsComplete())
	assert.True(t, toolResp.WasTruncated())

	structured := &StructuredResponse{Data: map[string]any{"name": "Ada"}}
	assert.Equal(t, structured.Data, structured.Content())
	var out struct {
		Name string `json:"name"`
	}
	require.NoError(t, structured.ContentAs(&out))
	assert.Equal(t, "Ada", out.Name)
	require.NoError(t, (&StructuredResponse{}).ContentAs(&out))

	chunk := &TextChunk{Delta: &ChunkDelta{Content: "delta"}}
	assert.Equal(t, "delta", chunk.Content())
	chunk.Text = "text"
	assert.Equal(t, "text", chunk.Content())
	assert.False(t, chunk.IsDone())
	chunk.FinishReason = &stop
	assert.True(t, chunk.IsDone())
	chunk.Error = errors.New("bad")
	assert.True(t, chunk.HasError())
	chunk.ToolCalls = []ToolCall{{ID: "call-1"}}
	assert.True(t, chunk.HasToolCalls())

	embeddings := &EmbeddingsResponse{Embeddings: []Embedding{{Index: 0, Embedding: []float64{0.1, 0.2}}}}
	assert.Equal(t, []float64{0.1, 0.2}, embeddings.Content())
	assert.Equal(t, []float64{0.1, 0.2}, embeddings.Vector(0))
	assert.Nil(t, embeddings.Vector(1))
	assert.Equal(t, 1, embeddings.Count())
	assert.Nil(t, (&EmbeddingsResponse{}).Content())
}

func TestSchemaValidation(t *testing.T) {
	t.Parallel()
	name := &StringSchema{
		BaseSchema: BaseSchema{Type: "string", Description: "name"},
		MinLength:  ptr(2),
		MaxLength:  ptr(5),
		Pattern:    `^[A-Z]`,
	}
	age := &NumberSchema{
		BaseSchema: BaseSchema{Type: "number"},
		Minimum:    ptrFloat(0),
		Maximum:    ptrFloat(120),
	}
	active := &BooleanSchema{BaseSchema: BaseSchema{Type: "boolean"}}
	role := &EnumSchema{BaseSchema: BaseSchema{Type: "string"}, Enum: []any{"admin", "user"}}
	schema := &ObjectSchema{
		BaseSchema: BaseSchema{Type: "object"},
		Properties: map[string]SchemaInterface{
			"name":   name,
			"age":    age,
			"active": active,
			"role":   role,
		},
		Required: []string{"name", "role"},
	}

	require.NoError(t, schema.Validate(map[string]any{
		"name":   "Ada",
		"age":    int32(42),
		"active": true,
		"role":   "admin",
	}))
	require.NoError(t, schema.Validate(struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}{Name: "Ada", Role: "user"}))

	require.Error(t, schema.Validate(nil))
	require.Error(t, schema.Validate("not object"))
	require.Error(t, schema.Validate(map[string]any{"name": "Ada"}))
	require.Error(t, schema.Validate(map[string]any{"name": "ada", "role": "admin"}))
	require.Error(t, name.Validate("A"))
	require.Error(t, name.Validate("Alexander"))
	require.Error(t, name.Validate(123))
	require.Error(t, (&StringSchema{Pattern: `[`}).Validate("bad"))
	require.Error(t, age.Validate(-1))
	require.Error(t, age.Validate(121))
	require.Error(t, age.Validate("old"))
	require.Error(t, active.Validate("true"))
	require.Error(t, role.Validate("owner"))

	array := &ArraySchema{BaseSchema: BaseSchema{Type: "array"}, Items: name}
	require.NoError(t, array.Validate([]string{"Ada"}))
	require.NoError(t, array.Validate([1]string{"Ada"}))
	require.Error(t, array.Validate(nil))
	require.Error(t, array.Validate("Ada"))
	require.Error(t, array.Validate([]string{"ada"}))
}

func ptr(v int) *int { return &v }

func ptrFloat(v float64) *float64 { return &v }

func TestMessagesConversationAndToolChoice(t *testing.T) {
	t.Parallel()
	messages := []Message{
		NewSystemMessage("system"),
		NewUserMessage("user"),
		NewAssistantMessage("assistant"),
		NewToolResultMessage("call-1", "tool"),
		BaseMessage{Role: RoleUser, Content: "base"},
	}
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		assert.Contains(t, string(data), string(msg.GetRole()))
	}

	conv := NewConversation().
		System("system").
		User("user").
		Assistant("assistant").
		Add(NewToolResultMessage("call-1", "tool")).
		AddAll(NewUserMessage("next"))
	assert.Equal(t, 5, conv.Len())
	assert.False(t, conv.IsEmpty())
	assert.Equal(t, RoleUser, conv.FirstUserMessage().GetRole())
	assert.Equal(t, RoleSystem, conv.SystemMessage().GetRole())
	assert.Equal(t, RoleUser, conv.Last().GetRole())
	assert.Equal(t, 4, conv.WithoutSystem().Len())

	clone := conv.Clone()
	clone.Clear()
	assert.True(t, clone.IsEmpty())
	assert.Equal(t, 5, conv.Len())
	assert.Nil(t, NewConversation().Last())
	assert.Nil(t, NewConversation().FirstUserMessage())
	assert.Nil(t, NewConversation().SystemMessage())

	from := FromMessages([]Message{NewUserMessage("from")})
	assert.Equal(t, 1, from.Len())

	fewShot := FewShot("system", []ExamplePair{{User: "hello", Assistant: "hola"}})
	assert.Equal(t, 3, fewShot.Len())

	data, err := json.Marshal(&ToolChoice{Type: ToolChoiceTypeAuto})
	require.NoError(t, err)
	assert.JSONEq(t, `"auto"`, string(data))
	data, err = json.Marshal(&ToolChoice{Type: ToolChoiceTypeSpecific, ToolName: "lookup"})
	require.NoError(t, err)
	assert.Contains(t, string(data), "lookup")
}

type fakeProvider struct {
	*BaseProvider
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{BaseProvider: NewBaseProvider("fake")}
}

func (p *fakeProvider) Text(ctx context.Context, request TextRequest) (*TextResponse, error) {
	return &TextResponse{Model: request.Model, Text: "text"}, nil
}

func (p *fakeProvider) Stream(ctx context.Context, request TextRequest) (<-chan TextChunk, error) {
	ch := make(chan TextChunk, 1)
	ch <- TextChunk{Text: "stream"}
	close(ch)
	return ch, nil
}

func (p *fakeProvider) Structured(ctx context.Context, request StructuredRequest) (*StructuredResponse, error) {
	return &StructuredResponse{Model: request.Model, Data: map[string]any{"ok": true}}, nil
}

func (p *fakeProvider) Embeddings(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error) {
	return &EmbeddingsResponse{Model: request.Model, Embeddings: []Embedding{{Index: 0}}}, nil
}

func (p *fakeProvider) Audio(ctx context.Context, request AudioRequest) (*AudioResponse, error) {
	return &AudioResponse{Model: request.Model, Text: "audio"}, nil
}

func (p *fakeProvider) Images(ctx context.Context, request ImagesRequest) (*ImagesResponse, error) {
	return &ImagesResponse{Model: request.Model, Images: []GeneratedImage{{URL: "url"}}}, nil
}

func (p *fakeProvider) GenerateImage(ctx context.Context, request ImageRequest) (*ImageResponse, error) {
	return &ImageResponse{Model: request.Model, Images: []GeneratedImage{{URL: "url"}}}, nil
}

func (p *fakeProvider) SpeechToText(ctx context.Context, request SpeechToTextRequest) (*SpeechToTextResponse, error) {
	return &SpeechToTextResponse{Text: "speech"}, nil
}

func (p *fakeProvider) TextToSpeech(ctx context.Context, request TextToSpeechRequest) (*TextToSpeechResponse, error) {
	return &TextToSpeechResponse{Audio: []byte("speech")}, nil
}

type countingMiddleware struct {
	count int
}

func (m *countingMiddleware) ApplyText(next TextHandler) TextHandler {
	return func(ctx context.Context, request TextRequest) (*TextResponse, error) {
		m.count++
		return next(ctx, request)
	}
}
func (m *countingMiddleware) ApplyStream(next StreamHandler) StreamHandler {
	return func(ctx context.Context, request TextRequest) (<-chan StreamChunk, error) {
		m.count++
		return next(ctx, request)
	}
}
func (m *countingMiddleware) ApplyStructured(next StructuredHandler) StructuredHandler {
	return func(ctx context.Context, request StructuredRequest) (*StructuredResponse, error) {
		m.count++
		return next(ctx, request)
	}
}
func (m *countingMiddleware) ApplyEmbeddings(next EmbeddingsHandler) EmbeddingsHandler {
	return func(ctx context.Context, request EmbeddingsRequest) (*EmbeddingsResponse, error) {
		m.count++
		return next(ctx, request)
	}
}

func (m *countingMiddleware) ApplyRerank(next RerankHandler) RerankHandler {
	return func(ctx context.Context, request RerankRequest) (*RerankResponse, error) {
		m.count++
		return next(ctx, request)
	}
}

// TestProviderMiddlewareChainApplyRerank proves ApplyRerank routes through
// the middleware chain like every other handler type (regression for AFK
// task 90b8580d: Rerank used to bypass the chain entirely).
func TestProviderMiddlewareChainApplyRerank(t *testing.T) {
	t.Parallel()

	mw := &countingMiddleware{}
	chain := NewProviderChain(mw)

	handler := chain.ApplyRerank(func(ctx context.Context, request RerankRequest) (*RerankResponse, error) {
		return &RerankResponse{Model: request.Model}, nil
	})

	resp, err := handler(context.Background(), RerankRequest{Model: "rerank-1", Query: "q", Documents: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("ApplyRerank handler returned error: %v", err)
	}
	if resp.Model != "rerank-1" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "rerank-1")
	}
	if mw.count != 1 {
		t.Fatalf("mw.count = %d, want 1: ApplyRerank must route through the middleware chain", mw.count)
	}
}
func (m *countingMiddleware) ApplyAudio(next AudioHandler) AudioHandler {
	return func(ctx context.Context, request AudioRequest) (*AudioResponse, error) {
		m.count++
		return next(ctx, request)
	}
}
func (m *countingMiddleware) ApplyImage(next ImageHandler) ImageHandler {
	return func(ctx context.Context, request ImageRequest) (*ImageResponse, error) {
		m.count++
		return next(ctx, request)
	}
}

func TestProviderConfigBaseProviderAndWrapper(t *testing.T) {
	t.Parallel()
	cfg := NewProviderConfig("key").
		WithBaseURL("https://example.test").
		WithHeader("A", "B").
		WithHeaders(map[string]string{"C": "D"}).
		WithTimeout(10).
		WithTimeoutDuration(2*time.Second).
		WithRetries(3, time.Millisecond).
		WithMaxRetryDelay(time.Second).
		WithDynamicModels().
		WithParam("one", 1).
		WithParams(map[string]any{"two": 2}).
		WithTLSConfigParam("server_name", "example.test").
		WithInsecureTLS(true)
	assert.Equal(t, "key", cfg.APIKey)
	assert.Equal(t, "https://example.test", cfg.BaseURL)
	assert.Equal(t, "B", cfg.Headers["A"])
	assert.Equal(t, 2, cfg.Timeout)
	assert.True(t, cfg.DynamicModels)
	assert.Equal(t, 3, *cfg.MaxRetries)
	assert.True(t, cfg.HasTLSConfig())

	base := NewBaseProvider("base")
	assert.Equal(t, "base", base.Name())
	assert.Empty(t, base.SupportedCapabilities())
	require.NoError(t, base.Close())
	_, err := base.Text(context.Background(), TextRequest{})
	require.Error(t, err)
	_, err = base.Stream(context.Background(), TextRequest{})
	require.Error(t, err)
	_, err = base.Structured(context.Background(), StructuredRequest{})
	require.Error(t, err)
	_, err = base.Embeddings(context.Background(), EmbeddingsRequest{})
	require.Error(t, err)
	_, err = base.Audio(context.Background(), AudioRequest{})
	require.Error(t, err)
	_, err = base.SpeechToText(context.Background(), SpeechToTextRequest{})
	require.Error(t, err)
	_, err = base.TextToSpeech(context.Background(), TextToSpeechRequest{})
	require.Error(t, err)
	_, err = base.Images(context.Background(), ImagesRequest{})
	require.Error(t, err)
	_, err = base.GenerateImage(context.Background(), ImageRequest{})
	require.Error(t, err)
	assert.True(t, IsNotSupportedError(err))
	assert.False(t, IsNotSupportedError(nil))
	assert.True(t, IsMethodSupported(base, "Text"))

	mw := &countingMiddleware{}
	wrapper := NewProviderWrapper(newFakeProvider(), mw)
	assert.Equal(t, "fake", wrapper.Name())
	assert.Same(t, wrapper.Unwrap(), wrapper.provider)
	_, err = wrapper.Text(context.Background(), TextRequest{BaseRequest: BaseRequest{Model: "m"}})
	require.NoError(t, err)
	_, err = wrapper.Stream(context.Background(), TextRequest{BaseRequest: BaseRequest{Model: "m"}})
	require.NoError(t, err)
	_, err = wrapper.Structured(context.Background(), StructuredRequest{BaseRequest: BaseRequest{Model: "m"}})
	require.NoError(t, err)
	_, err = wrapper.Embeddings(context.Background(), EmbeddingsRequest{Model: "m"})
	require.NoError(t, err)
	_, err = wrapper.Audio(context.Background(), AudioRequest{Model: "m"})
	require.NoError(t, err)
	_, err = wrapper.GenerateImage(context.Background(), ImageRequest{Model: "m"})
	require.NoError(t, err)
	_, err = wrapper.Images(context.Background(), ImagesRequest{Model: "m"})
	require.NoError(t, err)
	_, err = wrapper.SpeechToText(context.Background(), SpeechToTextRequest{Model: "m"})
	require.NoError(t, err)
	_, err = wrapper.TextToSpeech(context.Background(), TextToSpeechRequest{Model: "m"})
	require.NoError(t, err)
	assert.Equal(t, 6, mw.count)
}
