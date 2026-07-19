package wormhole

import (
	"encoding/json"

	"github.com/garyblankenship/wormhole/v2/types"
)

// ToJSON returns the request as JSON
func (b *TextRequestBuilder) ToJSON() (string, error) {
	jsonBytes, err := json.MarshalIndent(b.request, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// Validate checks the request configuration for errors before calling Generate().
// This enables fail-fast behavior to catch configuration issues early.
//
// Validates:
//   - Model is specified
//   - Messages are provided (either via Prompt, Messages, or Conversation)
//   - Temperature is in valid range (0.0-2.0)
//   - TopP is in valid range (0.0-1.0)
//   - MaxTokens is positive if specified
//
// Example:
//
//	builder := client.Text().Model("gpt-4o").Temperature(0.7)
//	if err := builder.Validate(); err != nil {
//	    log.Fatal("Invalid configuration:", err)
//	}
//	// Safe to call Generate()
//	resp, _ := builder.Prompt("Hello").Generate(ctx)
func (b *TextRequestBuilder) Validate() error {
	var errs types.ValidationErrors

	// Required fields
	if b.request.Model == "" {
		errs.Add("model", "required", nil, "model must be specified")
	}

	// Messages are checked but allowed to be empty at validation time
	// (they might be set later via Prompt() before Generate())

	// Temperature range
	if b.request.Temperature != nil {
		temp := *b.request.Temperature
		if temp < 0 || temp > 2 {
			errs.Add("temperature", "range", temp, "must be between 0.0 and 2.0")
		}
	}

	// TopP range
	if b.request.TopP != nil {
		topP := *b.request.TopP
		if topP < 0 || topP > 1 {
			errs.Add("top_p", "range", topP, "must be between 0.0 and 1.0")
		}
	}

	// MaxTokens positive
	if b.request.MaxTokens != nil && *b.request.MaxTokens <= 0 {
		errs.Add("max_tokens", "positive", *b.request.MaxTokens, "must be a positive integer")
	}

	// Frequency/Presence penalty ranges
	if b.request.FrequencyPenalty != nil {
		fp := *b.request.FrequencyPenalty
		if fp < -2 || fp > 2 {
			errs.Add("frequency_penalty", "range", fp, "must be between -2.0 and 2.0")
		}
	}
	if b.request.PresencePenalty != nil {
		pp := *b.request.PresencePenalty
		if pp < -2 || pp > 2 {
			errs.Add("presence_penalty", "range", pp, "must be between -2.0 and 2.0")
		}
	}

	return errs.Error()
}

// MustValidate calls Validate() and panics if validation fails.
// Use this for development/testing when invalid configuration should not occur.
//
// Example:
//
//	builder := client.Text().Model("gpt-4o").Temperature(0.7).MustValidate()
func (b *TextRequestBuilder) MustValidate() *TextRequestBuilder {
	if err := b.Validate(); err != nil {
		panic(err)
	}
	return b
}
