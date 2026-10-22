package types

import (
	"context"
	"encoding/json"
	"fmt"
)

// DecisionType identifies a Jev decision primitive.
type DecisionType string

const (
	DecisionNoul   DecisionType = "noul"
	DecisionChoice DecisionType = "choice"
	DecisionScore  DecisionType = "score"
)

// DecisionQuestion describes a decision. Instructions accept a JSON string,
// object or array. Criteria depend on Type; see DecisionsRequest.Validate.
type DecisionQuestion struct {
	Type         DecisionType `json:"type"`
	Instructions any          `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

// DecisionsRequest is the direct Decisions API payload, separate from chat.
type DecisionsRequest struct {
	Model     string                      `json:"model"`
	State     any                         `json:"state"`
	Questions map[string]DecisionQuestion `json:"questions"`
}

// DecisionAnswer preserves missing/null fields separately from valid zero values.
type DecisionAnswer struct {
	Type          DecisionType       `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        *string            `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

// DecisionUsage contains token counts and optional provider-reported cost.
type DecisionUsage struct {
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	Cost         *float64 `json:"cost,omitempty"`
}

// DecisionsResponse contains typed answers keyed by the requested question names.
type DecisionsResponse struct {
	ResponseMetadata *ResponseMetadata         `json:"response_metadata,omitempty"`
	ID               string                    `json:"id"`
	Model            string                    `json:"model"`
	Provider         string                    `json:"provider,omitempty"`
	Answers          map[string]DecisionAnswer `json:"answers"`
	Usage            DecisionUsage             `json:"usage"`
}

// DecisionsProvider is an optional capability. It does not change Provider or
// ProviderMiddleware; ordinary text middleware is not applied to Decisions.
type DecisionsProvider interface {
	Decisions(context.Context, DecisionsRequest) (*DecisionsResponse, error)
}

// UnmarshalJSON rejects null distribution/legend values instead of silently
// converting them to a valid zero probability or empty description.
func (a *DecisionAnswer) UnmarshalJSON(data []byte) error {
	type plain DecisionAnswer
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range []string{"probabilities", "legend"} {
		var entries map[string]json.RawMessage
		if field, ok := fields[name]; ok {
			if err := json.Unmarshal(field, &entries); err != nil {
				return err
			}
			for key, value := range entries {
				if string(value) == "null" {
					return fmt.Errorf("%s.%s must not be null", name, key)
				}
			}
		}
	}
	*a = DecisionAnswer(decoded)
	return nil
}
