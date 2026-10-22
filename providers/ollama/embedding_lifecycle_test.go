package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

type batchLifecycleHTTPClient struct {
	mu           sync.Mutex
	inputs       []string
	gate         chan struct{}
	fail         bool
	started      int
	exited       int
	failingInput string
}

func (f *batchLifecycleHTTPClient) Do(r *http.Request) (*http.Response, error) {
	defer r.Body.Close()
	var input embeddingsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		return nil, err
	}
	index := slices.Index(f.inputs, input.Prompt)
	if index < 0 {
		return nil, fmt.Errorf("unexpected input %q", input.Prompt)
	}
	f.mu.Lock()
	f.started++
	fail, first := f.fail, f.started == 1
	if fail && first {
		f.failingInput = input.Prompt
	}
	if fail && f.started == 3 {
		close(f.gate)
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.exited++
		f.mu.Unlock()
	}()
	if fail {
		select {
		case <-f.gate:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		if first {
			return batchLifecycleResponse(http.StatusBadRequest, "failed", map[string]any{"error": "fixture rejected: " + input.Prompt})
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}
	return batchLifecycleResponse(http.StatusOK, fmt.Sprintf("fresh-%d", index), map[string]any{"embedding": []float64{float64(index) + 0.25, float64(index) + 0.5}})
}

func batchLifecycleResponse(status int, requestID string, payload any) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Request-Id": []string{requestID}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func TestConcurrentEmbeddingsFailureOwnsWorkers(t *testing.T) {
	t.Parallel()
	inputs := []string{"ledger 0", "ledger 1", "ledger 2", "ledger 3", "ledger 4", "ledger 5"}
	fake := &batchLifecycleHTTPClient{inputs: inputs, gate: make(chan struct{}), fail: true}
	zero := 0
	provider := &Provider{BaseProvider: providers.NewBaseProviderWithAuth("ollama", types.ProviderConfig{
		BaseURL: "http://fixture.invalid", MaxRetries: &zero,
	}, nil, &providers.NoAuthStrategy{}, fake)}
	t.Cleanup(func() { require.NoError(t, provider.Close()) })
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	request := types.EmbeddingsRequest{Model: "fixture-embedding", Input: inputs}
	response, err := provider.Embeddings(ctx, request)
	require.Nil(t, response)
	require.Error(t, err)
	require.NoError(t, ctx.Err())
	fake.mu.Lock()
	started, exited, failedInput := fake.started, fake.exited, fake.failingInput
	fake.mu.Unlock()
	require.Equal(t, 3, started)
	require.Equal(t, started, exited)
	require.NotEmpty(t, failedInput)
	require.Contains(t, err.Error(), "fixture rejected: "+failedInput)
	require.Contains(t, err.Error(), fmt.Sprintf("input %d", slices.Index(inputs, failedInput)))
	require.False(t, strings.Contains(err.Error(), "context canceled"))
	fake.mu.Lock()
	fake.fail, fake.started, fake.exited = false, 0, 0
	fake.mu.Unlock()
	response, err = provider.Embeddings(ctx, request)
	require.NoError(t, err)
	require.Len(t, response.Embeddings, len(inputs))
	for i, embedding := range response.Embeddings {
		require.Equal(t, i, embedding.Index)
		require.Equal(t, []float64{float64(i) + 0.25, float64(i) + 0.5}, embedding.Embedding)
	}
	require.Equal(t, "fresh-5", response.ResponseMetadata.RequestID)
	fake.mu.Lock()
	started, exited = fake.started, fake.exited
	fake.mu.Unlock()
	require.Equal(t, len(inputs), started)
	require.Equal(t, started, exited)
}
