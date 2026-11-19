package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C6-02: transformTextResponse must not corrupt ordinary prose containing
// markdown JSON code snippets or multi-block responses.
func TestRemediationC602_PreserveProseWithJSONSnippets(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	prose := "Use this config:\n```json\n{\"port\":8080}\n```\nThen restart."
	resp := p.transformTextResponse(&chatCompletionResponse{
		ID:      "id-1",
		Model:   "gpt-4o",
		Created: time.Now().Unix(),
		Choices: []chatCompletionChoice{{
			Message:      message{Content: prose},
			FinishReason: "stop",
		}},
	})
	assert.Equal(t, prose, resp.Text)
}

// C6-08: Responses API separates refusal from text, streams response.refusal.delta,
// and returns an error on response.failed even when error payload is nil.
func TestRemediationC608_ResponsesRefusalAndFailedEvent(t *testing.T) {
	t.Parallel()

	t.Run("refusal populated separately and refusal-only response succeeds", func(t *testing.T) {
		t.Parallel()
		provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(responsesResponse{
				ID:        "resp-refusal",
				CreatedAt: 100,
				Model:     "gpt-5",
				Status:    "completed",
				Output: []responsesOutputItem{{
					Type: responsesItemMessage,
					Role: "assistant",
					Content: []responsesContentPart{{
						Type:    responsesContentRefusal,
						Refusal: "I cannot assist with that.",
					}},
				}},
			}))
		})
		provider.Config.UseResponsesAPI = true

		resp, err := provider.Text(context.Background(), types.TextRequest{
			BaseRequest: types.BaseRequest{Model: "gpt-5"},
			Messages:    []types.Message{types.NewUserMessage("bad request")},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Text)
		assert.Equal(t, "I cannot assist with that.", resp.Refusal)
	})

	t.Run("response.refusal.delta populates Refusal and Delta.Refusal", func(t *testing.T) {
		t.Parallel()
		p := New(types.ProviderConfig{APIKey: "test-key", UseResponsesAPI: true})
		chunk, err := p.parseResponsesStreamChunk([]byte(`{"type":"response.refusal.delta","delta":"Sorry"}`))
		require.NoError(t, err)
		require.NotNil(t, chunk)
		assert.Equal(t, "Sorry", chunk.Refusal)
		require.NotNil(t, chunk.Delta)
		assert.Equal(t, "Sorry", chunk.Delta.Refusal)
	})

	t.Run("response.failed returns non-nil error even without response.error", func(t *testing.T) {
		t.Parallel()
		p := New(types.ProviderConfig{APIKey: "test-key", UseResponsesAPI: true})

		for _, raw := range []string{
			`{"type":"response.failed"}`,
			`{"type":"response.failed","response":{"id":"r1","status":"failed"}}`,
		} {
			chunk, err := p.parseResponsesStreamChunk([]byte(raw))
			require.NoError(t, err)
			require.NotNil(t, chunk)
			require.Error(t, chunk.Error)
			var wErr *types.WormholeError
			require.True(t, errors.As(chunk.Error, &wErr))
		}
	})
}

// C6-11: Stream respects ProviderOptions["stream_options"] override and nil removal.
func TestRemediationC611_StreamOptionsOverrideAndNil(t *testing.T) {
	t.Parallel()

	capturePayload := func(t *testing.T, opts map[string]any) map[string]any {
		t.Helper()
		var got map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		}))
		t.Cleanup(server.Close)

		p := New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
		ch, err := p.Stream(context.Background(), types.TextRequest{
			BaseRequest: types.BaseRequest{
				Model:           "gpt-4o",
				ProviderOptions: opts,
			},
			Messages: []types.Message{types.NewUserMessage("hi")},
		})
		require.NoError(t, err)
		for range ch {
		}
		return got
	}

	t.Run("default sets include_usage true", func(t *testing.T) {
		t.Parallel()
		got := capturePayload(t, nil)
		assert.Equal(t, map[string]any{"include_usage": true}, got["stream_options"])
	})

	t.Run("explicit override preserved", func(t *testing.T) {
		t.Parallel()
		got := capturePayload(t, map[string]any{"stream_options": map[string]any{"include_usage": false}})
		assert.Equal(t, map[string]any{"include_usage": false}, got["stream_options"])
	})

	t.Run("explicit nil deletes stream_options", func(t *testing.T) {
		t.Parallel()
		got := capturePayload(t, map[string]any{"stream_options": nil})
		_, exists := got["stream_options"]
		assert.False(t, exists, "stream_options must be omitted when explicitly set to nil")
	})
}

// C6-12: responsesMessageContent must return a validation error instead of
// panicking when a file part has wrong Data type or nil *FileReferenceMedia.
func TestRemediationC612_ResponsesFilePartValidation(t *testing.T) {
	t.Parallel()

	t.Run("wrong data type returns validation error", func(t *testing.T) {
		t.Parallel()
		_, err := responsesMessageContent([]types.MessagePart{{Type: "file", Data: "not-a-file-ref"}})
		require.Error(t, err)
	})

	t.Run("nil FileReferenceMedia returns validation error", func(t *testing.T) {
		t.Parallel()
		var nilRef *types.FileReferenceMedia
		_, err := responsesMessageContent([]types.MessagePart{{Type: "file", Data: nilRef}})
		require.Error(t, err)
	})
}

// C2-08 (OpenAI side): handleSpeechToText uses request.AudioFormat for multipart
// filename (defaulting to wav and trimming leading dot).
func TestRemediationC208_SpeechToTextAudioFormat(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name            string
		audioFormat     string
		wantFilename    string
		wantContentType string
	}{
		{"default wav", "", "audio.wav", "audio/wav"},
		{"mp3", "mp3", "audio.mp3", "audio/mpeg"},
		{"leading dot ogg", ".ogg", "audio.ogg", "audio/ogg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotFilename, gotContentType string
			provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
				require.NoError(t, err)
				mr := multipart.NewReader(r.Body, params["boundary"])
				for {
					part, err := mr.NextPart()
					if errors.Is(err, io.EOF) {
						break
					}
					require.NoError(t, err)
					if part.FormName() == "file" {
						gotFilename = part.FileName()
						gotContentType = part.Header.Get("Content-Type")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"text":"hello"}`))
			})

			resp, err := provider.Audio(context.Background(), types.AudioRequest{
				Type:        types.AudioRequestTypeSTT,
				Model:       "whisper-1",
				Input:       []byte("fake-audio"),
				AudioFormat: tc.audioFormat,
			})
			require.NoError(t, err)
			assert.Equal(t, "hello", resp.Text)
			assert.Equal(t, tc.wantFilename, gotFilename)
			assert.Equal(t, tc.wantContentType, gotContentType)
		})
	}
}
