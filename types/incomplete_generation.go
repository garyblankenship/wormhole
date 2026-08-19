package types

import "errors"

// IncompleteGenerationReason identifies why generation produced no app-visible
// text, refusal, or tool calls.
type IncompleteGenerationReason string

const (
	IncompleteGenerationTruncated     IncompleteGenerationReason = "truncated"
	IncompleteGenerationReasoningOnly IncompleteGenerationReason = "reasoning_only"
	IncompleteGenerationEmpty         IncompleteGenerationReason = "empty"
)

// IncompleteGenerationError preserves safe response metadata when generation
// ends without app-visible output. It never retains reasoning content.
type IncompleteGenerationError struct {
	*WormholeError
	Reason           IncompleteGenerationReason `json:"reason"`
	RequestID        string                     `json:"request_id,omitempty"`
	FinishReason     FinishReason               `json:"finish_reason,omitempty"`
	Usage            *Usage                     `json:"usage,omitempty"`
	ReasoningPresent bool                       `json:"reasoning_present,omitempty"`
}

// NewIncompleteGenerationError classifies an empty app-visible response and
// copies the safe metadata callers need for recovery decisions.
func NewIncompleteGenerationError(response *TextResponse) *IncompleteGenerationError {
	if response == nil {
		response = &TextResponse{}
	}

	reason := IncompleteGenerationEmpty
	message := "generation returned empty response"
	if response.FinishReason == FinishReasonLength {
		reason = IncompleteGenerationTruncated
		message = "generation truncated before visible content"
	} else if response.Thinking != nil && response.Thinking.Content != "" {
		reason = IncompleteGenerationReasoningOnly
		message = "generation returned reasoning without visible content"
	}

	var usage *Usage
	if response.Usage != nil {
		usageCopy := *response.Usage
		usage = &usageCopy
	}

	return &IncompleteGenerationError{
		WormholeError: NewWormholeError(ErrorCodeProvider, message, false).
			WithProvider(response.Provider).
			WithModel(response.Model),
		Reason:           reason,
		RequestID:        response.ID,
		FinishReason:     response.FinishReason,
		Usage:            usage,
		ReasoningPresent: response.Thinking != nil && response.Thinking.Content != "",
	}
}

// Error implements error while keeping the zero value safe.
func (e *IncompleteGenerationError) Error() string {
	if e == nil || e.WormholeError == nil {
		return "incomplete generation"
	}
	return e.WormholeError.Error()
}

// Unwrap exposes the standard Wormhole error classification fields.
func (e *IncompleteGenerationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.WormholeError
}

// AsIncompleteGenerationError extracts typed incomplete-generation metadata.
func AsIncompleteGenerationError(err error) (*IncompleteGenerationError, bool) {
	var incompleteErr *IncompleteGenerationError
	if errors.As(err, &incompleteErr) {
		return incompleteErr, true
	}
	return nil, false
}
