package wormhole

import (
	"context"
	"encoding/json"

	"github.com/garyblankenship/wormhole/v3/types"
)

// DecisionsRequestBuilder builds direct Jev decisions. Use a separate builder
// per goroutine. Decisions does not invoke text/structured middleware.
type DecisionsRequestBuilder struct {
	CommonBuilder
	request *types.DecisionsRequest
}

// Using selects the provider implementing the optional Decisions capability.
func (b *DecisionsRequestBuilder) Using(provider string) *DecisionsRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL overrides the provider API root for this request.
func (b *DecisionsRequestBuilder) BaseURL(url string) *DecisionsRequestBuilder {
	b.setBaseURL(url)
	return b
}

// Model sets the pass-through direct Decisions model identifier.
func (b *DecisionsRequestBuilder) Model(model string) *DecisionsRequestBuilder {
	b.request.Model = model
	return b
}

// State sets the shared JSON string, object or array evaluated by all questions.
func (b *DecisionsRequestBuilder) State(state any) *DecisionsRequestBuilder {
	b.request.State = state
	return b
}

// Questions sets typed decision questions keyed by application-defined names.
func (b *DecisionsRequestBuilder) Questions(questions map[string]types.DecisionQuestion) *DecisionsRequestBuilder {
	b.request.Questions = questions
	return b
}

// Validate checks required fields and serialized JSON shapes without HTTP.
func (b *DecisionsRequestBuilder) Validate() error { return b.request.Validate() }

// Evaluate validates and snapshots the JSON payload, then uses normal client
// request tracking, provider leases, context and opt-in idempotency.
func (b *DecisionsRequestBuilder) Evaluate(ctx context.Context) (*types.DecisionsResponse, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(b.request)
	if err != nil {
		return nil, types.NewValidationError("decisions", "json", nil, "request must be valid JSON")
	}
	// RawMessage preserves exact serialized state/criteria (including large JSON
	// numbers) while detaching the executing request from mutable caller inputs.
	var snapshot struct {
		Model     string          `json:"model"`
		State     json.RawMessage `json:"state"`
		Questions map[string]struct {
			Type         types.DecisionType `json:"type"`
			Instructions json.RawMessage    `json:"instructions"`
			Criteria     json.RawMessage    `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	request := types.DecisionsRequest{Model: snapshot.Model, State: snapshot.State, Questions: make(map[string]types.DecisionQuestion, len(snapshot.Questions))}
	for name, q := range snapshot.Questions {
		var criteria any
		if len(q.Criteria) > 0 {
			criteria = q.Criteria
		}
		request.Questions[name] = types.DecisionQuestion{Type: q.Type, Instructions: q.Instructions, Criteria: criteria}
	}
	return executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("decisions.evaluate"), request, func(ctx context.Context) (*types.DecisionsResponse, error) {
		return executeProviderOperation(ctx, &b.CommonBuilder, "decisions", func(ctx context.Context, provider types.Provider) (*types.DecisionsResponse, error) {
			decisions, ok := provider.(types.DecisionsProvider)
			if !ok {
				return nil, types.NotImplementedError(provider.Name(), "Decisions")
			}
			return decisions.Decisions(ctx, request)
		})
	})
}
