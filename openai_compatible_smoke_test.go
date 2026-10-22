package wormhole

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOpenAICompatibleSmokeDiagnostics(t *testing.T) {
	t.Parallel()

	t.Run("reports upstream model identity without failing the chat check", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Upstream serves an alias, not the requested model: diagnostic
			// evidence, never an automatic failure.
			smokeWriteChat(t, w, smokeChatFixture{
				ID: "chatcmpl-alias", Model: "served-alias-v9", Content: "pong",
				Reasoning: "step by step",
				Usage:     map[string]int{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
			})
		}))
		t.Cleanup(server.Close)

		result, err := RunOpenAICompatibleSmoke(context.Background(), OpenAICompatibleSmokeConfig{
			BaseURL: server.URL + "/v1",
			Model:   "local-model",
			Prompt:  "ping",
		})
		require.NoError(t, err)
		assert.Equal(t, "local-model", result.Model, "compatibility field stays the requested model")
		assert.Equal(t, "local-model", result.RequestedModel)
		assert.Equal(t, "served-alias-v9", result.ReportedModel, "reported identity is the parsed upstream value")
		assert.Equal(t, types.FinishReasonStop, result.FinishReason)
		require.NotNil(t, result.Usage)
		assert.Equal(t, 3, result.Usage.PromptTokens)
		assert.Equal(t, 5, result.Usage.CompletionTokens)
		assert.Equal(t, 8, result.Usage.TotalTokens)
		require.NotNil(t, result.Thinking)
		assert.Equal(t, "step by step", result.Thinking.Content)
		require.Len(t, result.Checks, 1)
		assert.Equal(t, OpenAICompatibleSmokeCheck{Name: "chat", Passed: true}, result.Checks[0])
	})

	t.Run("reports requested model when upstream omits model identity", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			smokeWriteChat(t, w, smokeChatFixture{ID: "chatcmpl-nomodel", Content: "pong"})
		}))
		t.Cleanup(server.Close)

		result, err := RunOpenAICompatibleSmoke(context.Background(), OpenAICompatibleSmokeConfig{
			BaseURL: server.URL + "/v1",
			Model:   "local-model",
			Prompt:  "ping",
		})
		require.NoError(t, err)
		// Known limitation, not upstream evidence: the OpenAI-compatible
		// provider backfills an empty upstream model with the requested
		// model, so this smoke cannot distinguish an omitted model field
		// from an exact echo. The smoke claims nothing stronger.
		assert.Equal(t, "local-model", result.ReportedModel)
		assert.Equal(t, "local-model", result.RequestedModel)
		// The adapter also normalizes absent usage; this is not evidence
		// that the upstream reported a real zero-token response.
		require.NotNil(t, result.Usage)
		assert.Equal(t, &types.Usage{}, result.Usage)
		assert.Nil(t, result.Thinking)
	})
}

func TestRunOpenAICompatibleSmokeValidateResponse(t *testing.T) {
	t.Parallel()

	domainConfig := func(server *httptest.Server) OpenAICompatibleSmokeConfig {
		return OpenAICompatibleSmokeConfig{
			BaseURL:         server.URL + "/v1",
			Model:           "local-model",
			Prompt:          "ping",
			CheckStreaming:  true,
			CheckEmbeddings: true,
		}
	}

	t.Run("successful validator passes and optional probes continue", func(t *testing.T) {
		t.Parallel()
		server, seen := smokeDomainServer(t)
		validator := func(resp *types.TextResponse) error {
			var payload struct {
				Answer string `json:"answer"`
			}
			if err := json.Unmarshal([]byte(resp.Text), &payload); err != nil {
				return fmt.Errorf("response is not the expected JSON object: %w", err)
			}
			if payload.Answer == "" {
				return errors.New("response JSON is missing the answer field")
			}
			return nil
		}

		cfg := domainConfig(server)
		cfg.ValidateResponse = validator
		result, err := RunOpenAICompatibleSmoke(context.Background(), cfg)
		require.NoError(t, err)

		assert.Equal(t, []OpenAICompatibleSmokeCheck{
			{Name: "chat", Passed: true},
			{Name: "response_validation", Passed: true},
			{Name: "stream", Passed: true},
			{Name: "embeddings", Passed: true},
		}, result.Checks, "semantic validation is a separate check, not an extra request")
		assert.Equal(t, []string{"/v1/chat/completions", "/v1/chat/completions", "/v1/embeddings"}, *seen)
	})

	t.Run("failed validator wraps its error and stops further probes", func(t *testing.T) {
		t.Parallel()
		server, seen := smokeDomainServer(t)
		errWrongShape := errors.New("response JSON is missing the citations field")
		validator := func(resp *types.TextResponse) error {
			var payload struct {
				Citations []string `json:"citations"`
			}
			if err := json.Unmarshal([]byte(resp.Text), &payload); err != nil {
				return err
			}
			if len(payload.Citations) == 0 {
				return errWrongShape
			}
			return nil
		}

		cfg := domainConfig(server)
		cfg.ValidateResponse = validator
		result, err := RunOpenAICompatibleSmoke(context.Background(), cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, errWrongShape, "callback error identity survives wrapping")
		assert.Contains(t, err.Error(), "response validation failed")

		// Useful chat diagnostics survive the semantic failure.
		assert.Equal(t, "pong", mustDecodeAnswer(t, result.Text))
		assert.Equal(t, "local-model", result.ReportedModel)
		require.NotNil(t, result.Usage)

		require.Len(t, result.Checks, 2)
		assert.True(t, result.Checks[0].Passed)
		assert.Equal(t, "chat", result.Checks[0].Name)
		assert.False(t, result.Checks[1].Passed)
		assert.Equal(t, "response_validation", result.Checks[1].Name)
		assert.Contains(t, result.Checks[1].Error, "missing the citations field")

		// Zero extra calls after failure: no stream or embeddings requests.
		assert.Equal(t, []string{"/v1/chat/completions"}, *seen)
	})

	t.Run("keeps default checks without a validator", func(t *testing.T) {
		t.Parallel()
		server, seen := smokeDomainServer(t)

		result, err := RunOpenAICompatibleSmoke(context.Background(), domainConfig(server))
		require.NoError(t, err)
		require.Len(t, result.Checks, 3)
		assert.Equal(t, "chat", result.Checks[0].Name)
		assert.Equal(t, "stream", result.Checks[1].Name)
		assert.Equal(t, "embeddings", result.Checks[2].Name)
		for _, check := range result.Checks {
			assert.True(t, check.Passed)
		}
		assert.Equal(t, []string{"/v1/chat/completions", "/v1/chat/completions", "/v1/embeddings"}, *seen)
	})
}

// mustDecodeAnswer extracts the answer field from a smoke JSON response.
func mustDecodeAnswer(t *testing.T, text string) string {
	t.Helper()
	var payload struct {
		Answer string `json:"answer"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &payload))
	return payload.Answer
}
